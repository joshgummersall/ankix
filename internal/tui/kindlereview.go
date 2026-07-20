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
	kVisual
	kEditSentence
	kConfirm
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

// KindleModel is the root Bubble Tea model for reviewing Kindle vocab
// lookups. Entries are grouped by their usage sentence up front, so every
// word looked up within the same sentence is reviewed together: the
// sentence is shown once with every candidate word in it highlighted, and
// the user steps word by word through that group, extending or moving the
// current word's highlight (e.g. to capture a reflexive verb form or
// multi-word phrase) before it's defined and synced to Anki.
type KindleModel struct {
	cfg    KindleConfig
	groups []kindle.SentenceGroup

	groupIdx int
	wordIdx  int // index into groups[groupIdx].Entries

	width, height int

	state kState

	sentence   string
	tokens     []token
	wordTokens []int // indices into tokens that are words
	wordCursor int

	// visualLo, visualHi are independent word-token boundaries (indices into
	// wordTokens) of the in-progress visual selection, so the phrase can be
	// grown to the left and right independently instead of pivoting around
	// a single fixed anchor.
	visualLo, visualHi int

	otherRanges [][2]int // byte ranges of this sentence's other candidate words

	// sentenceInput edits the current group's sentence once, for every word
	// in that sentence — see enterEditSentence.
	sentenceInput textarea.Model

	selStart, selEnd int // byte offsets of the confirmed phrase in sentence, or -1 if none

	definition string
	defPending bool
	defErr     error

	added, duplicates, skipped int

	status    string
	statusErr bool
}

// NewKindleReview returns a KindleModel positioned at the first word of the
// first sentence group.
func NewKindleReview(cfg KindleConfig) KindleModel {
	sei := textarea.New()
	sei.Prompt = "edit: "
	sei.ShowLineNumbers = false
	sei.SetWidth(120)
	sei.SetHeight(3)

	m := KindleModel{cfg: cfg, groups: kindle.GroupBySentence(cfg.Entries), sentenceInput: sei}
	m.loadWord(0, 0)
	return m
}

func (m *KindleModel) currentEntry() kindle.Entry {
	return m.groups[m.groupIdx].Entries[m.wordIdx]
}

// flatPosition returns this word's 1-based position across every word in
// every group, for display (e.g. "word 4/12").
func (m *KindleModel) flatPosition() int {
	n := m.wordIdx
	for _, g := range m.groups[:m.groupIdx] {
		n += len(g.Entries)
	}
	return n + 1
}

func (m *KindleModel) totalWords() int {
	return len(m.cfg.Entries)
}

func (m *KindleModel) advance() {
	m.loadWord(m.groupIdx, m.wordIdx+1)
}

// loadWord positions the model at groups[groupIdx].Entries[wordIdx],
// advancing to the next group once wordIdx runs past the current one, and
// finishing review once groupIdx runs past the last group.
func (m *KindleModel) loadWord(groupIdx, wordIdx int) {
	for groupIdx < len(m.groups) && wordIdx >= len(m.groups[groupIdx].Entries) {
		groupIdx++
		wordIdx = 0
	}
	if groupIdx >= len(m.groups) {
		m.state = kDone
		return
	}
	m.groupIdx, m.wordIdx = groupIdx, wordIdx
	group := m.groups[groupIdx]
	e := group.Entries[wordIdx]

	m.sentence = group.Usage
	m.tokens = tokenize(m.sentence)
	m.wordTokens = m.wordTokens[:0]
	for ti, t := range m.tokens {
		if t.isWord {
			m.wordTokens = append(m.wordTokens, ti)
		}
	}

	m.otherRanges = m.otherRanges[:0]
	for oi, other := range group.Entries {
		if oi == wordIdx {
			continue
		}
		if start, end := kindle.FindPhrase(m.sentence, other.Word); start >= 0 {
			m.otherRanges = append(m.otherRanges, [2]int{start, end})
		}
	}

	m.selStart, m.selEnd = kindle.FindPhrase(m.sentence, e.Word)
	m.wordCursor = m.tokenCursorFor(m.selStart)
	m.definition = ""
	m.defPending = false
	m.defErr = nil
	m.state = kPicking

	status := fmt.Sprintf("word %d/%d: %q — tab/shift+tab move, v extend to a phrase, enter accept", m.flatPosition(), m.totalWords(), e.Word)
	if len(group.Entries) > 1 {
		status = fmt.Sprintf("word %d/%d: %q (%d words from this sentence) — tab/shift+tab move, v extend to a phrase, enter accept", m.flatPosition(), m.totalWords(), e.Word, len(group.Entries))
	}
	m.setStatus(status, false)
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
	return nil
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
		m.defPending = false
		if msg.err != nil {
			m.defErr = msg.err
			m.setStatus(fmt.Sprintf("definition lookup failed: %v", msg.err), true)
			return m, nil
		}
		m.definition = msg.definition
		if m.definition == "" {
			m.setStatus("no definition found — enter to add without one, esc to adjust the selection", true)
		} else {
			m.setStatus("enter to add, esc to adjust the selection, s to skip this word", false)
		}
		return m, nil

	case kindleSubmitResultMsg:
		switch {
		case msg.err != nil:
			m.setStatus(fmt.Sprintf("failed to add %q: %v", m.currentEntry().Word, msg.err), true)
			m.state = kConfirm
			return m, nil
		case msg.duplicate:
			m.duplicates++
		default:
			m.added++
		}
		m.advance()
		return m, nil

	case kindleSkipResultMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("failed to record skip: %v", msg.err), true)
		}
		m.skipped++
		m.advance()
		return m, nil
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
	case kVisual:
		return m.handleVisualKey(msg)
	case kEditSentence:
		return m.handleEditSentenceKey(msg)
	case kConfirm:
		return m.handleConfirmKey(msg)
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
		m.visualLo = m.wordCursor
		m.visualHi = m.wordCursor
		m.state = kVisual
		m.setStatus("tab grow right, shift+tab grow left, enter accept, esc cancel", false)
		return m, nil
	case "e":
		return m, m.enterEditSentence()
	case "s":
		return m, kindleSkipCmd(m.cfg, m.currentEntry())
	case "enter":
		if len(m.wordTokens) == 0 {
			m.setStatus("no word to select in this sentence — s to skip", true)
			return m, nil
		}
		return m.acceptSelection(m.wordCursor, m.wordCursor)
	}
	return m, nil
}

func (m KindleModel) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l", "right", "tab":
		if m.visualHi < len(m.wordTokens)-1 {
			m.visualHi++
		}
		return m, nil
	case "h", "left", "shift+tab":
		if m.visualLo > 0 {
			m.visualLo--
		}
		return m, nil
	case "esc":
		m.state = kPicking
		m.setStatus("selection cancelled", false)
		return m, nil
	case "enter":
		return m.acceptSelection(m.visualLo, m.visualHi)
	}
	return m, nil
}

// acceptSelection confirms the phrase spanning word-token indices
// [lo,hi] (inclusive, indices into m.wordTokens) as the reviewed word/phrase
// and kicks off its definition lookup.
func (m KindleModel) acceptSelection(lo, hi int) (tea.Model, tea.Cmd) {
	m.selStart = m.tokens[m.wordTokens[lo]].start
	m.selEnd = m.tokens[m.wordTokens[hi]].end
	m.state = kConfirm
	m.definition = ""
	m.defErr = nil

	if m.cfg.Dict == nil {
		m.setStatus("enter to add, esc to adjust the selection, s to skip this word", false)
		return m, nil
	}
	m.defPending = true
	m.setStatus("looking up definition...", false)
	return m, kindleDefCmd(m.cfg.Dict, m.sentence[m.selStart:m.selEnd], m.sentence)
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
		m.setStatus("", false)
		return m, nil
	case "enter":
		m.applyEditedSentence(m.sentenceInput.Value())
		m.sentenceInput.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.sentenceInput, cmd = m.sentenceInput.Update(msg)
	return m, cmd
}

// applyEditedSentence saves the edit back onto the current sentence group
// (so every word from this sentence sees it) and reloads the current word
// against the new text. Word/phrase byte offsets are recomputed fresh via
// kindle.FindPhrase rather than carried over, so — unlike the YouTube
// sentence editor — no marks need to be dropped.
func (m *KindleModel) applyEditedSentence(edited string) {
	if edited != m.sentence {
		m.groups[m.groupIdx].Usage = edited
	}
	m.loadWord(m.groupIdx, m.wordIdx)
}

func (m KindleModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = kPicking
		m.setStatus("tab/shift+tab move, v extend to a phrase, enter accept", false)
		return m, nil
	case "s":
		return m, kindleSkipCmd(m.cfg, m.currentEntry())
	case "enter":
		if m.defPending {
			return m, nil
		}
		m.state = kSubmitting
		m.setStatus("adding...", false)
		phrase := m.sentence[m.selStart:m.selEnd]
		back := m.definition
		if back != "" {
			back = kindle.FormatDefinition(phrase, back)
		}
		note := kindle.BuildNote(m.cfg.Deck, m.cfg.Tags, m.currentEntry(), m.sentence, m.selStart, m.selEnd, back)
		return m, kindleSubmitCmd(m.cfg, m.currentEntry(), note)
	}
	return m, nil
}

func (m KindleModel) View() string {
	if m.state == kDone {
		return fmt.Sprintf("\ndone: %d added, %d already in Anki, %d skipped\n\npress enter to exit\n", m.added, m.duplicates, m.skipped)
	}

	e := m.currentEntry()
	header := titleStyle.Render(fmt.Sprintf("ankindle review — %d/%d", m.flatPosition(), m.totalWords()))
	if e.BookTitle != "" {
		header += "  " + helpStyle.Render(e.BookTitle)
	}

	var body string
	switch m.state {
	case kConfirm, kSubmitting:
		body = m.renderKindleConfirm()
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

// inOtherRange reports whether [start,end) falls within one of this
// sentence's other candidate words, so they can be dimly underlined while
// the current word is highlighted.
func (m KindleModel) inOtherRange(start, end int) bool {
	for _, r := range m.otherRanges {
		if start >= r[0] && end <= r[1] {
			return true
		}
	}
	return false
}

func (m KindleModel) renderKindlePicker() string {
	if m.sentence == "" {
		return "\n" + helpStyle.Render("no usage sentence recorded for this lookup") + "\n"
	}

	selLo, selHi := -1, -1
	if m.state == kVisual {
		selLo, selHi = m.visualLo, m.visualHi
	}

	var b strings.Builder
	wordPos := -1
	for _, t := range m.tokens {
		text := m.sentence[t.start:t.end]
		if t.isWord {
			wordPos++
		}
		switch {
		case t.isWord && wordPos == m.wordCursor:
			text = wordCursorStyle.Render(text)
		case t.isWord && selLo != -1 && wordPos >= selLo && wordPos <= selHi:
			text = selectedLineStyle.Render(text)
		case m.selStart >= 0 && t.start >= m.selStart && t.end <= m.selEnd:
			text = markedWordStyle.Render(text)
		case t.isWord && m.inOtherRange(t.start, t.end):
			text = pendingWordStyle.Render(text)
		}
		b.WriteString(text)
	}
	return "\n" + b.String() + "\n"
}

func (m KindleModel) renderKindleEditSentence() string {
	return "\n" + helpStyle.Render("fix typos in the sentence, then confirm — applies to every word from this sentence") + "\n\n" + m.sentenceInput.View() + "\n"
}

func (m KindleModel) renderKindleConfirm() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.selStart >= 0 {
		b.WriteString(m.sentence[:m.selStart])
		b.WriteString(markedWordStyle.Render(m.sentence[m.selStart:m.selEnd]))
		b.WriteString(m.sentence[m.selEnd:])
	} else {
		b.WriteString(m.sentence)
	}
	b.WriteString("\n\n")

	phrase := m.currentEntry().Word
	if m.selStart >= 0 {
		phrase = m.sentence[m.selStart:m.selEnd]
	}
	b.WriteString(fmt.Sprintf("headword: %s\n", phrase))

	switch {
	case m.defPending:
		b.WriteString("definition: looking up...\n")
	case m.defErr != nil:
		b.WriteString(fmt.Sprintf("definition: lookup failed (%v)\n", m.defErr))
	case m.definition == "":
		b.WriteString("definition: (none)\n")
	default:
		b.WriteString(fmt.Sprintf("definition: %s\n", m.definition))
	}

	if m.state == kSubmitting {
		b.WriteString("\nsubmitting...\n")
	}
	return b.String()
}

func (m KindleModel) helpText() string {
	switch m.state {
	case kVisual:
		return "tab grow right  shift+tab grow left  enter accept  esc cancel  q quit"
	case kEditSentence:
		return "enter confirm edit  esc cancel"
	case kConfirm, kSubmitting:
		return "enter add  esc adjust selection  s skip  q quit"
	default:
		return "tab/shift+tab move  v extend to a phrase  e edit sentence  enter accept  s skip  q quit"
	}
}

type kindleDefResultMsg struct {
	definition string
	err        error
}

func kindleDefCmd(p dict.Provider, phrase, sentence string) tea.Cmd {
	return func() tea.Msg {
		def, err := p.Define(phrase, sentence)
		return kindleDefResultMsg{definition: def, err: err}
	}
}

type kindleSubmitResultMsg struct {
	duplicate bool
	err       error
}

// kindleSubmitCmd adds note to Anki (skipping if a note for this phrase
// already exists in the deck), then advances the sync watermark and, if
// configured, marks the word Mastered in vocab.db.
func kindleSubmitCmd(cfg KindleConfig, e kindle.Entry, note anki.Note) tea.Cmd {
	return func() tea.Msg {
		if err := cfg.AnkiClient.CreateDeck(cfg.Deck); err != nil {
			return kindleSubmitResultMsg{err: err}
		}

		_, err := cfg.AnkiClient.AddNote(note)
		duplicate := errors.Is(err, anki.ErrDuplicate)
		if err != nil && !duplicate {
			return kindleSubmitResultMsg{err: err}
		}

		if err := finishEntry(cfg, e, duplicate || err == nil); err != nil {
			return kindleSubmitResultMsg{duplicate: duplicate, err: err}
		}
		return kindleSubmitResultMsg{duplicate: duplicate}
	}
}

type kindleSkipResultMsg struct {
	err error
}

func kindleSkipCmd(cfg KindleConfig, e kindle.Entry) tea.Cmd {
	return func() tea.Msg {
		return kindleSkipResultMsg{err: finishEntry(cfg, e, false)}
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
