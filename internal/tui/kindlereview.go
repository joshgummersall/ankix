package tui

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict"
	"github.com/joshgummersall/ankix/internal/kindle"
)

type kState int

const (
	kPicking kState = iota
	kExpanding
	kEditSentence
	kSubmitting
	kDone
)

// KindleConfig holds everything the Kindle vocab review TUI needs to run.
type KindleConfig struct {
	Entries []kindle.Entry // one per word, most-recently-looked-up first

	// MaxLookupTimestamp is the newest LOOKUPS.timestamp seen across every
	// lookup of a word (keyed by strings.ToLower(word)), even though only
	// one Entry per word is reviewed. The sync watermark advances to this
	// value once a word has been reviewed, whether or not a card was added.
	MaxLookupTimestamp map[string]int64

	Deck       string
	Tags       []string
	AnkiClient *anki.Client
	Dict       dict.Provider // nil disables definition lookups

	// DB, if non-nil, is a read-write vocab.db handle used to persist the
	// sync watermark after each reviewed word and, with Mastered, to mark
	// synced words as Mastered.
	DB       *sql.DB
	Mastered bool
}

// kindlePhrase is the live, editable word/phrase for one entry in the
// current sentence group: lo/hi are word-token indices (into m.wordTokens),
// inclusive, defaulting to that entry's own single-word occurrence.
// Included toggles whether it's added as a card when the sentence submits.
// mergedInto is -1 for a standalone phrase; otherwise it's the index of
// another phrase this one was absorbed into after an expansion made the two
// overlap, and lo/hi/included are stale and ignored.
type kindlePhrase struct {
	lo, hi     int
	included   bool
	entry      kindle.Entry
	mergedInto int

	// Definition lookup for the phrase's current text, so a preview can be
	// shown before submitting. defText is the phrase text the lookup
	// below applies to — once lo/hi/text changes, it no longer matches and
	// a fresh lookup is due (see refreshDefinitions).
	defText    string
	definition string
	defErr     error
	defPending bool
}

// kindleSelection is one accepted word/phrase, resolved to byte offsets in
// the sentence, ready to become a card. entries holds every entry the
// phrase covers — more than one if expanding it merged it with a
// neighboring word's phrase. definition is whatever was already fetched and
// previewed for this phrase, reused as-is rather than looked up again.
type kindleSelection struct {
	entries    []kindle.Entry
	start, end int
	definition string
}

// KindleModel is the root Bubble Tea model for reviewing Kindle vocab
// lookups. Entries are grouped by their usage sentence up front, so every
// word looked up within the same sentence is reviewed together: the
// sentence is shown once with a live, editable phrase for every candidate
// word in it. Pressing v starts expanding whichever word is nearest the
// cursor; tab/shift+tab then grow that word's phrase right/left, and enter
// confirms it (esc cancels back to before expanding). If expanding makes
// two words' phrases overlap, they're merged into a single card covering
// both. s toggles a word out of the batch. Since every word already has a
// sensible default (its own single-word phrase, included), enter from the
// normal (non-expanding) state submits every included word in the sentence
// as its own card in one action — no per-word accept step required.
type KindleModel struct {
	cfg    KindleConfig
	groups []kindle.SentenceGroup

	groupIdx int

	width, height int

	state kState

	sentence   string
	tokens     []token
	wordTokens []int // indices into tokens that are words
	wordCursor int   // index into wordTokens

	phrases []kindlePhrase // live phrase state for groups[groupIdx].Entries

	// expandIdx is the phrase currently being grown (valid only in
	// kExpanding); expandOrigLo/Hi is its pre-expansion range, restored on
	// esc.
	expandIdx                  int
	expandOrigLo, expandOrigHi int

	// sentenceInput edits the current group's sentence once, for every word
	// in that sentence — see enterEditSentence.
	sentenceInput textarea.Model

	added, duplicates, skipped int

	status    string
	statusErr bool

	initCmd tea.Cmd // definition lookups queued by the first loadGroup, returned from Init
}

// NewKindleReview returns a KindleModel positioned at the first sentence
// group.
func NewKindleReview(cfg KindleConfig) KindleModel {
	sei := textarea.New()
	sei.Prompt = "edit: "
	sei.ShowLineNumbers = false
	sei.SetWidth(120)
	sei.SetHeight(3)

	m := KindleModel{cfg: cfg, groups: kindle.GroupBySentence(cfg.Entries), sentenceInput: sei}
	m.initCmd = m.loadGroup(0)
	return m
}

// resetPhrasesForSentence (re)tokenizes m.sentence and rebuilds m.phrases
// with each entry defaulted to its own single-word occurrence, included.
// Called whenever the group changes or its sentence text is edited, since
// either invalidates every previously computed byte offset.
func (m *KindleModel) resetPhrasesForSentence() {
	group := m.groups[m.groupIdx]

	m.tokens = tokenize(m.sentence)
	m.wordTokens = m.wordTokens[:0]
	for ti, t := range m.tokens {
		if t.isWord {
			m.wordTokens = append(m.wordTokens, ti)
		}
	}

	m.phrases = make([]kindlePhrase, len(group.Entries))
	for i, e := range group.Entries {
		start, _ := kindle.FindPhrase(m.sentence, e.Word)
		idx := m.tokenCursorFor(start)
		m.phrases[i] = kindlePhrase{lo: idx, hi: idx, included: true, entry: e, mergedInto: -1}
	}

	m.wordCursor = 0
	if len(m.phrases) > 0 {
		m.wordCursor = m.phrases[0].lo
	}
}

// refreshDefinitions kicks off a definition lookup for every included,
// standalone phrase whose current text hasn't been looked up yet (or has
// changed since it last was, e.g. after an expansion or merge), so a
// preview of what will be saved is visible before submitting.
func (m *KindleModel) refreshDefinitions() tea.Cmd {
	if m.cfg.Dict == nil {
		return nil
	}
	var cmds []tea.Cmd
	for i := range m.phrases {
		p := &m.phrases[i]
		if p.mergedInto != -1 || !p.included {
			continue
		}
		text := m.sentence[m.tokens[m.wordTokens[p.lo]].start:m.tokens[m.wordTokens[p.hi]].end]
		if p.defText == text {
			continue
		}
		p.defText = text
		p.definition = ""
		p.defErr = nil
		p.defPending = true
		cmds = append(cmds, kindleDefCmd(m.cfg.Dict, text, m.sentence, i, text))
	}
	return tea.Batch(cmds...)
}

// loadGroup positions the model at groups[groupIdx], or finishes review
// once groupIdx runs past the last group. It returns a command to fetch
// definition previews for the new sentence's words.
func (m *KindleModel) loadGroup(groupIdx int) tea.Cmd {
	if groupIdx >= len(m.groups) {
		m.state = kDone
		return nil
	}
	m.groupIdx = groupIdx
	m.sentence = m.groups[groupIdx].Usage
	m.resetPhrasesForSentence()
	m.state = kPicking
	m.setStatus(m.pickingStatus(), false)
	return m.refreshDefinitions()
}

func (m *KindleModel) pickingStatus() string {
	cards, included := 0, 0
	for _, p := range m.phrases {
		if p.mergedInto != -1 {
			continue
		}
		cards++
		if p.included {
			included++
		}
	}
	word := "card"
	if cards != 1 {
		word = "cards"
	}
	return fmt.Sprintf("sentence %d/%d — %d %s, %d will be added — tab/shift+tab move, v expand nearest word, s toggle skip, e edit sentence, enter add",
		m.groupIdx+1, len(m.groups), cards, word, included)
}

// nearestPhrase returns the index into m.phrases whose range is closest to
// m.wordCursor (0 if the cursor is already within it), ties broken toward
// the lowest index. Phrases already merged into another are ignored.
func (m *KindleModel) nearestPhrase() int {
	best, bestDist := -1, -1
	for i, p := range m.phrases {
		if p.mergedInto != -1 {
			continue
		}
		dist := 0
		switch {
		case m.wordCursor < p.lo:
			dist = p.lo - m.wordCursor
		case m.wordCursor > p.hi:
			dist = m.wordCursor - p.hi
		}
		if bestDist == -1 || dist < bestDist {
			best, bestDist = i, dist
		}
	}
	if best == -1 {
		return 0
	}
	return best
}

// mergeOverlaps folds any other standalone phrase whose range now overlaps
// phrases[i] into phrases[i], growing i's range to cover the union. Called
// after every expansion so merges happen live as soon as two words' phrases
// touch.
func (m *KindleModel) mergeOverlaps(i int) {
	for j := range m.phrases {
		if j == i || m.phrases[j].mergedInto != -1 {
			continue
		}
		if m.phrases[j].hi < m.phrases[i].lo || m.phrases[j].lo > m.phrases[i].hi {
			continue
		}
		if m.phrases[j].lo < m.phrases[i].lo {
			m.phrases[i].lo = m.phrases[j].lo
		}
		if m.phrases[j].hi > m.phrases[i].hi {
			m.phrases[i].hi = m.phrases[j].hi
		}
		for k := range m.phrases {
			if m.phrases[k].mergedInto == j {
				m.phrases[k].mergedInto = i
			}
		}
		m.phrases[j].mergedInto = i
	}
}

// tokenCursorFor returns the index into m.wordTokens of the first word token
// starting at byte offset start, or 0 if start is negative or not found.
func (m *KindleModel) tokenCursorFor(start int) int {
	if start < 0 {
		return 0
	}
	for wi, ti := range m.wordTokens {
		if m.tokens[ti].start == start {
			return wi
		}
	}
	return 0
}

func (m *KindleModel) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
}

func (m KindleModel) Init() tea.Cmd {
	return m.initCmd
}

func (m KindleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sentenceInput.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		return m.handleKindleKey(msg)

	case kindleDefResultMsg:
		if msg.idx < len(m.phrases) && m.phrases[msg.idx].defText == msg.text {
			m.phrases[msg.idx].defPending = false
			m.phrases[msg.idx].definition = msg.definition
			m.phrases[msg.idx].defErr = msg.err
		}
		return m, nil

	case kindleBatchSubmitResultMsg:
		m.added += msg.added
		m.duplicates += msg.duplicates
		m.skipped += msg.skipped
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("failed partway through this sentence (%d added, %d already in Anki before the error): %v — enter to retry", msg.added, msg.duplicates, msg.err), true)
			m.state = kPicking
			return m, nil
		}
		return m, m.loadGroup(m.groupIdx + 1)
	}
	return m, nil
}

func (m KindleModel) handleKindleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state == kDone {
		switch msg.String() {
		case "q", "ctrl+c", "enter", "esc":
			return m, tea.Quit
		}
		return m, nil
	}

	if m.state != kEditSentence {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	switch m.state {
	case kPicking:
		return m.handlePickingKey(msg)
	case kExpanding:
		return m.handleExpandingKey(msg)
	case kEditSentence:
		return m.handleEditSentenceKey(msg)
	}
	return m, nil
}

func (m KindleModel) handlePickingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l", "right", "tab":
		if m.wordCursor < len(m.wordTokens)-1 {
			m.wordCursor++
		}
		return m, nil
	case "h", "left", "shift+tab":
		if m.wordCursor > 0 {
			m.wordCursor--
		}
		return m, nil
	case "v":
		if len(m.phrases) == 0 {
			return m, nil
		}
		i := m.nearestPhrase()
		m.expandIdx = i
		m.expandOrigLo, m.expandOrigHi = m.phrases[i].lo, m.phrases[i].hi
		m.state = kExpanding
		m.setStatus("tab grow right, shift+tab grow left, enter confirm, esc cancel", false)
		return m, nil
	case "s":
		if len(m.phrases) == 0 {
			return m, nil
		}
		i := m.nearestPhrase()
		m.phrases[i].included = !m.phrases[i].included
		m.setStatus(m.pickingStatus(), false)
		return m, m.refreshDefinitions()
	case "e":
		return m, m.enterEditSentence()
	case "enter":
		return m.submitGroup()
	}
	return m, nil
}

// handleExpandingKey grows or shrinks phrases[m.expandIdx] and merges it
// with any other phrase it comes to overlap.
func (m KindleModel) handleExpandingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l", "right", "tab":
		if m.phrases[m.expandIdx].hi < len(m.wordTokens)-1 {
			m.phrases[m.expandIdx].hi++
		}
		m.mergeOverlaps(m.expandIdx)
		m.wordCursor = m.phrases[m.expandIdx].hi
		return m, nil
	case "h", "left", "shift+tab":
		if m.phrases[m.expandIdx].lo > 0 {
			m.phrases[m.expandIdx].lo--
		}
		m.mergeOverlaps(m.expandIdx)
		m.wordCursor = m.phrases[m.expandIdx].lo
		return m, nil
	case "esc":
		for j := range m.phrases {
			if m.phrases[j].mergedInto == m.expandIdx {
				m.phrases[j].mergedInto = -1
			}
		}
		m.phrases[m.expandIdx].lo = m.expandOrigLo
		m.phrases[m.expandIdx].hi = m.expandOrigHi
		m.state = kPicking
		m.setStatus(m.pickingStatus(), false)
		return m, m.refreshDefinitions()
	case "enter":
		m.state = kPicking
		m.setStatus(m.pickingStatus(), false)
		return m, m.refreshDefinitions()
	}
	return m, nil
}

// submitGroup builds a card for every included phrase in the current
// sentence and submits them all in one action, and records a skip for
// every excluded entry so the sync watermark still advances past it. Merged
// phrases contribute every entry they absorbed to whichever single card
// their merged range produces.
func (m KindleModel) submitGroup() (tea.Model, tea.Cmd) {
	for _, p := range m.phrases {
		if p.mergedInto == -1 && p.included && p.defPending {
			m.setStatus("still looking up definitions...", false)
			return m, nil
		}
	}

	var sels []kindleSelection
	var skipped []kindle.Entry
	for i, p := range m.phrases {
		if p.mergedInto != -1 {
			continue
		}
		entries := []kindle.Entry{p.entry}
		for j, q := range m.phrases {
			if j != i && q.mergedInto == i {
				entries = append(entries, q.entry)
			}
		}
		if !p.included {
			skipped = append(skipped, entries...)
			continue
		}
		start := m.tokens[m.wordTokens[p.lo]].start
		end := m.tokens[m.wordTokens[p.hi]].end
		sels = append(sels, kindleSelection{entries: entries, start: start, end: end, definition: p.definition})
	}

	if len(sels) == 0 {
		for _, e := range skipped {
			if err := finishEntry(m.cfg, e, false); err != nil {
				m.setStatus(fmt.Sprintf("failed to record skip: %v", err), true)
				return m, nil
			}
		}
		m.skipped += len(skipped)
		return m, m.loadGroup(m.groupIdx + 1)
	}

	m.state = kSubmitting
	m.setStatus("adding...", false)
	return m, kindleBatchSubmitCmd(m.cfg, m.sentence, sels, skipped)
}

// enterEditSentence opens a text input pre-filled with the current
// sentence, so typos can be fixed once for every word this sentence
// produces a card for, rather than per word.
func (m *KindleModel) enterEditSentence() tea.Cmd {
	m.sentenceInput.SetValue(m.sentence)
	m.sentenceInput.CursorEnd()
	cmd := m.sentenceInput.Focus()
	m.state = kEditSentence
	m.setStatus("", false)
	return cmd
}

func (m KindleModel) handleEditSentenceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sentenceInput.Blur()
		m.state = kPicking
		m.setStatus(m.pickingStatus(), false)
		return m, nil
	case "enter":
		cmd := m.applyEditedSentence(m.sentenceInput.Value())
		m.sentenceInput.Blur()
		return m, cmd
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.sentenceInput, cmd = m.sentenceInput.Update(msg)
	return m, cmd
}

// applyEditedSentence saves the edit back onto the current sentence group
// (so every word from this sentence sees it) and rebuilds every phrase
// against the new text — any phrase extensions made before the edit are
// lost, since their byte offsets no longer apply.
func (m *KindleModel) applyEditedSentence(edited string) tea.Cmd {
	if edited != m.sentence {
		m.groups[m.groupIdx].Usage = edited
		m.sentence = edited
		m.resetPhrasesForSentence()
	}
	m.state = kPicking
	m.setStatus(m.pickingStatus(), false)
	return m.refreshDefinitions()
}

func (m KindleModel) View() string {
	if m.state == kDone {
		return fmt.Sprintf("\ndone: %d added, %d already in Anki, %d skipped\n\npress enter to exit\n", m.added, m.duplicates, m.skipped)
	}

	group := m.groups[m.groupIdx]
	header := titleStyle.Render(fmt.Sprintf("ankindle review — sentence %d/%d", m.groupIdx+1, len(m.groups)))
	if len(group.Entries) > 0 && group.Entries[0].BookTitle != "" {
		header += "  " + helpStyle.Render(group.Entries[0].BookTitle)
	}

	var body string
	switch m.state {
	case kEditSentence:
		body = m.renderKindleEditSentence()
	default:
		body = m.renderKindlePicker()
	}
	if m.width > 0 {
		body = lipgloss.NewStyle().Width(m.width).Render(body)
	}

	statusLine := statusStyle
	if m.statusErr {
		statusLine = errStatusStyle
	}
	footer := statusLine.Render(m.status) + "\n" + helpStyle.Render(m.helpText())

	return header + "\n" + body + "\n" + footer + "\n"
}

// styleForWord returns the style (if any) for the word token at wordPos:
// the cursor takes priority, then whichever phrase (if any) contains it,
// styled as included or skipped.
func (m KindleModel) styleForWord(wordPos int) (lipgloss.Style, bool) {
	if wordPos == m.wordCursor {
		return wordCursorStyle, true
	}
	for _, p := range m.phrases {
		if p.mergedInto != -1 {
			continue
		}
		if wordPos >= p.lo && wordPos <= p.hi {
			if p.included {
				return markedWordStyle, true
			}
			return pendingWordStyle, true
		}
	}
	return lipgloss.Style{}, false
}

func (m KindleModel) renderKindlePicker() string {
	if m.sentence == "" {
		return "\n" + helpStyle.Render("no usage sentence recorded for this lookup") + "\n"
	}

	var b strings.Builder
	wordPos := -1
	for _, t := range m.tokens {
		text := m.sentence[t.start:t.end]
		if t.isWord {
			wordPos++
			if style, ok := m.styleForWord(wordPos); ok {
				text = style.Render(text)
			}
		}
		b.WriteString(text)
	}

	if m.cfg.Dict != nil {
		b.WriteString("\n\n")
		for _, p := range m.phrases {
			if p.mergedInto != -1 || !p.included {
				continue
			}
			phrase := m.sentence[m.tokens[m.wordTokens[p.lo]].start:m.tokens[m.wordTokens[p.hi]].end]
			switch {
			case p.defPending:
				fmt.Fprintf(&b, "%s: looking up...\n", phrase)
			case p.defErr != nil:
				fmt.Fprintf(&b, "%s: lookup failed (%v)\n", phrase, p.defErr)
			case p.definition == "":
				fmt.Fprintf(&b, "%s: (none)\n", phrase)
			default:
				fmt.Fprintf(&b, "%s: %s\n", phrase, p.definition)
			}
		}
	}

	return "\n" + b.String() + "\n"
}

func (m KindleModel) renderKindleEditSentence() string {
	return "\n" + helpStyle.Render("fix typos in the sentence, then confirm — applies to every word from this sentence") + "\n\n" + m.sentenceInput.View() + "\n"
}

func (m KindleModel) helpText() string {
	switch m.state {
	case kExpanding:
		return "tab grow right  shift+tab grow left  enter confirm  esc cancel"
	case kEditSentence:
		return "enter confirm edit  esc cancel"
	case kSubmitting:
		return "submitting..."
	default:
		return "tab/shift+tab move  v expand nearest word  s toggle skip  e edit sentence  enter add all  q quit"
	}
}

// kindleDefResultMsg carries a definition lookup result back for the phrase
// at idx, tagged with the text it was fetched for (text) so a stale result
// for a phrase that's since changed can be ignored.
type kindleDefResultMsg struct {
	idx        int
	text       string
	definition string
	err        error
}

func kindleDefCmd(p dict.Provider, phrase, sentence string, idx int, text string) tea.Cmd {
	return func() tea.Msg {
		def, err := p.Define(phrase, sentence)
		return kindleDefResultMsg{idx: idx, text: text, definition: def, err: err}
	}
}

type kindleBatchSubmitResultMsg struct {
	added, duplicates, skipped int
	err                        error
}

// kindleBatchSubmitCmd adds one note per selection to Anki (skipping any
// whose phrase already has a note in the deck), advancing the sync
// watermark and, if configured, marking each word Mastered in vocab.db as
// it's added — then does the same (without adding a note) for every
// explicitly skipped entry, so the watermark advances past those too.
func kindleBatchSubmitCmd(cfg KindleConfig, sentence string, sels []kindleSelection, skipped []kindle.Entry) tea.Cmd {
	return func() tea.Msg {
		if err := cfg.AnkiClient.CreateDeck(cfg.Deck); err != nil {
			return kindleBatchSubmitResultMsg{err: err}
		}

		var added, duplicates int
		for _, sel := range sels {
			phrase := sentence[sel.start:sel.end]
			var back string
			if sel.definition != "" {
				back = kindle.FormatDefinition(phrase, sel.definition)
			}
			note := kindle.BuildNote(cfg.Deck, cfg.Tags, sel.entries[0], sentence, sel.start, sel.end, back)

			_, err := cfg.AnkiClient.AddNote(note)
			duplicate := errors.Is(err, anki.ErrDuplicate)
			if err != nil && !duplicate {
				return kindleBatchSubmitResultMsg{added: added, duplicates: duplicates, err: err}
			}
			if duplicate {
				duplicates++
			} else {
				added++
			}

			for _, e := range sel.entries {
				if err := finishEntry(cfg, e, true); err != nil {
					return kindleBatchSubmitResultMsg{added: added, duplicates: duplicates, err: err}
				}
			}
		}

		for _, e := range skipped {
			if err := finishEntry(cfg, e, false); err != nil {
				return kindleBatchSubmitResultMsg{added: added, duplicates: duplicates, skipped: len(skipped), err: err}
			}
		}

		return kindleBatchSubmitResultMsg{added: added, duplicates: duplicates, skipped: len(skipped)}
	}
}

// finishEntry persists the sync watermark for e and, if synced (added or
// already a duplicate) and Mastered is set, marks e Mastered in vocab.db.
// Entries aren't necessarily reviewed in increasing timestamp order (they're
// listed most-recently-looked-up first), so the watermark only ever moves
// forward: it's set to the max of its current value and e's timestamp.
func finishEntry(cfg KindleConfig, e kindle.Entry, synced bool) error {
	if cfg.DB == nil {
		return nil
	}
	if synced && cfg.Mastered {
		if err := kindle.MarkMastered(cfg.DB, e.ID); err != nil {
			return err
		}
	}
	current, err := kindle.LastSynced(cfg.DB)
	if err != nil {
		return err
	}
	ts := cfg.MaxLookupTimestamp[strings.ToLower(e.Word)]
	if ts > current {
		if err := kindle.SetLastSynced(cfg.DB, ts); err != nil {
			return err
		}
	}
	return nil
}
