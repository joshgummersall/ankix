package tui

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
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

	Deck       string
	Tags       []string
	AnkiClient *anki.Client
	Dict       dict.Provider // nil disables definition lookups

	// DB, if non-nil, is a read-write vocab.db handle used to mark synced
	// words as Mastered.
	DB *sql.DB
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
// word in it. Pressing v on a word that already has a (non-deleted) phrase
// starts expanding it; on a deleted word it restores that phrase to its
// original single-word state; on any other word in the sentence, it first
// adds that word as a new phrase (not from a Kindle lookup) and starts
// expanding that. Either way, h/l then move the cursor to extend or shrink
// the phrase (vim-visual-mode style, anchored where expanding began), and
// enter confirms it (esc cancels back to before expanding,
// discarding a newly added word entirely). If expanding makes two words'
// phrases overlap, they're merged into a single card covering both. d
// deletes a word, clearing its selection state entirely (v re-adds it).
// Since every looked-up word already has a sensible default (its own
// single-word phrase), enter from the normal (non-expanding) state submits
// every non-deleted word in the sentence as its own card in one action —
// no per-word accept step required.
type KindleModel struct {
	cfg    KindleConfig
	groups []kindle.SentenceGroup

	groupIdx int

	width, height int

	state kState

	sentence string
	ps       phraseSet[kindle.Entry] // live phrase state for groups[groupIdx].Entries; each phrase's payload is the kindle.Entry it originated from

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
// with each entry defaulted to its own single-word occurrence. Called
// whenever the group changes or its sentence text is edited, since either
// invalidates every previously computed byte offset.
func (m *KindleModel) resetPhrasesForSentence() {
	group := m.groups[m.groupIdx]

	m.ps.tokens = tokenize(m.sentence)
	m.ps.setWordTokens()

	m.ps.phrases = make([]phrase[kindle.Entry], len(group.Entries))
	for i, e := range group.Entries {
		start, _ := kindle.FindPhrase(m.sentence, e.Word)
		idx := m.tokenCursorFor(start)
		m.ps.phrases[i] = phrase[kindle.Entry]{lo: idx, hi: idx, defaultLo: idx, defaultHi: idx, payload: e, mergedInto: -1}
	}

	m.ps.wordCursor = 0
	if len(m.ps.phrases) > 0 {
		m.ps.wordCursor = m.ps.phrases[0].lo
	}
}

// refreshDefinitions kicks off a definition lookup for every non-deleted,
// standalone phrase whose current text hasn't been looked up yet (or has
// changed since it last was, e.g. after an expansion or merge), so a
// preview of what will be saved is visible before submitting.
func (m *KindleModel) refreshDefinitions() tea.Cmd {
	if m.cfg.Dict == nil {
		return nil
	}
	text := func(p *phrase[kindle.Entry]) string {
		return m.sentence[m.ps.tokens[m.ps.wordTokens[p.lo]].start:m.ps.tokens[m.ps.wordTokens[p.hi]].end]
	}
	lookup := func(i int, text string) tea.Cmd {
		return kindleDefCmd(m.cfg.Dict, text, m.sentence, i, text)
	}
	return m.ps.refreshPreviews(text, lookup)
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
	cards := 0
	for _, p := range m.ps.phrases {
		if p.mergedInto == -1 && !p.deleted {
			cards++
		}
	}
	word := "card"
	if cards != 1 {
		word = "cards"
	}
	return fmt.Sprintf("sentence %d/%d — %d %s will be added — h/l move, v expand/add word, d delete word, e edit sentence, enter add",
		m.groupIdx+1, len(m.groups), cards, word)
}

// addPhraseAtCursor adds a new single-word phrase for the word under the
// cursor — a word with no Kindle lookup of its own, being added manually.
func (m *KindleModel) newEntryAtCursor() kindle.Entry {
	tok := m.ps.tokens[m.ps.wordTokens[m.ps.wordCursor]]
	e := kindle.Entry{Word: m.sentence[tok.start:tok.end], Usage: m.sentence}
	if group := m.groups[m.groupIdx]; len(group.Entries) > 0 {
		e.BookTitle = group.Entries[0].BookTitle
		e.Authors = group.Entries[0].Authors
		e.Lang = group.Entries[0].Lang
	}
	return e
}

// tokenCursorFor returns the index into m.ps.wordTokens of the first word
// token starting at byte offset start, or 0 if start is negative or not
// found.
func (m *KindleModel) tokenCursorFor(start int) int {
	if start < 0 {
		return 0
	}
	for wi, ti := range m.ps.wordTokens {
		if m.ps.tokens[ti].start == start {
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

	case debounceExpandMsg:
		if msg.gen != m.ps.debounceGen {
			return m, nil
		}
		return m, m.refreshDefinitions()

	case kindleDefResultMsg:
		if msg.idx < len(m.ps.phrases) && m.ps.phrases[msg.idx].previewText == msg.text {
			m.ps.phrases[msg.idx].previewPending = false
			m.ps.phrases[msg.idx].preview = msg.definition
			m.ps.phrases[msg.idx].previewErr = msg.err
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
	case "l", "right":
		m.ps.moveCursorRight()
		return m, nil
	case "h", "left":
		m.ps.moveCursorLeft()
		return m, nil
	case "v":
		if len(m.ps.wordTokens) == 0 {
			return m, nil
		}
		m.ps.beginExpand(m.newEntryAtCursor())
		m.state = kExpanding
		m.setStatus("h/l extend selection, enter confirm, esc cancel", false)
		return m, m.ps.debounceRefresh()
	case "d":
		m.ps.deleteNearestPhrase()
		m.setStatus(m.pickingStatus(), false)
		return m, nil
	case "e":
		return m, m.enterEditSentence()
	case "enter":
		return m.submitGroup()
	}
	return m, nil
}

// handleExpandingKey moves the cursor within phrases[m.ps.expandIdx] and
// merges it with any other phrase it comes to overlap.
func (m KindleModel) handleExpandingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l", "right":
		m.ps.moveExpandCursor(1)
		return m, m.ps.debounceRefresh()
	case "h", "left":
		m.ps.moveExpandCursor(-1)
		return m, m.ps.debounceRefresh()
	case "esc":
		m.ps.cancelExpand()
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
// sentence and submits them all in one action. Merged phrases contribute
// every entry they absorbed to whichever single card their merged range
// produces.
func (m KindleModel) submitGroup() (tea.Model, tea.Cmd) {
	for _, p := range m.ps.phrases {
		if p.mergedInto == -1 && !p.deleted && p.previewPending {
			m.setStatus("still looking up definitions...", false)
			return m, nil
		}
	}

	var sels []kindleSelection
	var skipped []kindle.Entry
	for i, p := range m.ps.phrases {
		if p.mergedInto != -1 {
			continue
		}
		entries := []kindle.Entry{p.payload}
		for j, q := range m.ps.phrases {
			if j != i && q.mergedInto == i {
				entries = append(entries, q.payload)
			}
		}
		if p.deleted {
			skipped = append(skipped, entries...)
			continue
		}
		start := m.ps.tokens[m.ps.wordTokens[p.lo]].start
		end := m.ps.tokens[m.ps.wordTokens[p.hi]].end
		sels = append(sels, kindleSelection{entries: entries, start: start, end: end, definition: p.preview})
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
	header := titleStyle.Render(fmt.Sprintf("ankix review — sentence %d/%d", m.groupIdx+1, len(m.groups)))
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

func (m KindleModel) renderKindlePicker() string {
	if m.sentence == "" {
		return "\n" + helpStyle.Render("no usage sentence recorded for this lookup") + "\n"
	}

	var b strings.Builder
	b.WriteString(m.ps.render(m.sentence))

	if m.cfg.Dict != nil {
		b.WriteString("\n\n")
		ordered := make([]phrase[kindle.Entry], len(m.ps.phrases))
		copy(ordered, m.ps.phrases)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].lo < ordered[j].lo })
		for _, p := range ordered {
			if p.mergedInto != -1 || p.deleted {
				continue
			}
			text := m.sentence[m.ps.tokens[m.ps.wordTokens[p.lo]].start:m.ps.tokens[m.ps.wordTokens[p.hi]].end]
			switch {
			case p.previewPending:
				fmt.Fprintf(&b, "%s: looking up...\n", text)
			case p.previewErr != nil:
				fmt.Fprintf(&b, "%s: lookup failed (%v)\n", text, p.previewErr)
			case p.preview == "":
				fmt.Fprintf(&b, "%s: (none)\n", text)
			default:
				fmt.Fprintf(&b, "%s: %s\n", text, p.preview)
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
		return "h/l extend selection  enter confirm  esc cancel"
	case kEditSentence:
		return "enter confirm edit  esc cancel"
	case kSubmitting:
		return "submitting..."
	default:
		return "h/l move  v expand/add word under cursor  d delete word  e edit sentence  enter add all  q quit"
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
// whose phrase already has a note in the deck), marking each word Mastered
// in vocab.db as it's added, found to already exist, or explicitly deleted
// from review (skipped) — every word the user has reviewed is done with,
// regardless of whether it became a card.
func kindleBatchSubmitCmd(cfg KindleConfig, sentence string, sels []kindleSelection, skipped []kindle.Entry) tea.Cmd {
	return func() tea.Msg {
		if len(sels) > 0 {
			if err := cfg.AnkiClient.CreateDeck(cfg.Deck); err != nil {
				return kindleBatchSubmitResultMsg{err: err}
			}
		}

		for _, e := range skipped {
			if err := markMastered(cfg, e); err != nil {
				return kindleBatchSubmitResultMsg{skipped: len(skipped), err: err}
			}
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
				if err := markMastered(cfg, e); err != nil {
					return kindleBatchSubmitResultMsg{added: added, duplicates: duplicates, err: err}
				}
			}
		}

		return kindleBatchSubmitResultMsg{added: added, duplicates: duplicates, skipped: len(skipped)}
	}
}

// markMastered marks e Mastered in vocab.db. Manually-added words (see
// addPhraseAtCursor) have no vocab.db row, so there's nothing to mark.
func markMastered(cfg KindleConfig, e kindle.Entry) error {
	if cfg.DB == nil || e.ID == "" {
		return nil
	}
	return kindle.MarkMastered(cfg.DB, e.ID)
}
