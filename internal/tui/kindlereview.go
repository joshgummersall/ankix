package tui

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
// lookups: each word is shown highlighted in its usage sentence, and the
// user can extend or move that highlight (e.g. to capture a reflexive verb
// form or multi-word phrase) before the word is defined and synced to Anki.
type KindleModel struct {
	cfg KindleConfig
	idx int

	width, height int

	state kState

	sentence   string
	tokens     []token
	wordTokens []int // indices into tokens that are words
	wordCursor int
	visualFrom int // index into wordTokens: anchor of the in-progress visual selection

	selStart, selEnd int // byte offsets of the confirmed phrase in sentence, or -1 if none

	definition string
	defPending bool
	defErr     error

	added, duplicates, skipped int

	status    string
	statusErr bool
}

// NewKindleReview returns a KindleModel positioned at the first entry.
func NewKindleReview(cfg KindleConfig) KindleModel {
	m := KindleModel{cfg: cfg}
	m.loadEntry(0)
	return m
}

func (m *KindleModel) currentEntry() kindle.Entry {
	return m.cfg.Entries[m.idx]
}

func (m *KindleModel) loadEntry(i int) {
	if i >= len(m.cfg.Entries) {
		m.state = kDone
		return
	}
	m.idx = i
	e := m.cfg.Entries[i]

	m.sentence = e.Usage
	m.tokens = tokenize(m.sentence)
	m.wordTokens = m.wordTokens[:0]
	for ti, t := range m.tokens {
		if t.isWord {
			m.wordTokens = append(m.wordTokens, ti)
		}
	}

	m.selStart, m.selEnd = kindle.FindPhrase(m.sentence, e.Word)
	m.wordCursor = m.tokenCursorFor(m.selStart)
	m.definition = ""
	m.defPending = false
	m.defErr = nil
	m.state = kPicking
	m.setStatus(fmt.Sprintf("word %d/%d: %q — tab/shift+tab move, v extend to a phrase, enter accept", i+1, len(m.cfg.Entries), e.Word), false)
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
		m.loadEntry(m.idx + 1)
		return m, nil

	case kindleSkipResultMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("failed to record skip: %v", msg.err), true)
		}
		m.skipped++
		m.loadEntry(m.idx + 1)
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

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	switch m.state {
	case kPicking:
		return m.handlePickingKey(msg)
	case kVisual:
		return m.handleVisualKey(msg)
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
		m.visualFrom = m.wordCursor
		m.state = kVisual
		m.setStatus("tab/shift+tab extend the phrase, enter accept, esc cancel", false)
		return m, nil
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
		if m.wordCursor < len(m.wordTokens)-1 {
			m.wordCursor++
		}
		return m, nil
	case "h", "left", "shift+tab":
		if m.wordCursor > 0 {
			m.wordCursor--
		}
		return m, nil
	case "esc":
		m.state = kPicking
		m.setStatus("selection cancelled", false)
		return m, nil
	case "enter":
		lo, hi := m.visualFrom, m.wordCursor
		if lo > hi {
			lo, hi = hi, lo
		}
		return m.acceptSelection(lo, hi)
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
	header := titleStyle.Render(fmt.Sprintf("ankindle review — %d/%d", m.idx+1, len(m.cfg.Entries)))
	if e.BookTitle != "" {
		header += "  " + helpStyle.Render(e.BookTitle)
	}

	var body string
	switch m.state {
	case kConfirm, kSubmitting:
		body = m.renderKindleConfirm()
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

	selLo, selHi := -1, -1
	if m.state == kVisual {
		selLo, selHi = m.visualFrom, m.wordCursor
		if selLo > selHi {
			selLo, selHi = selHi, selLo
		}
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
		}
		b.WriteString(text)
	}
	return "\n" + b.String() + "\n"
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
		return "tab/shift+tab extend  enter accept  esc cancel  q quit"
	case kConfirm, kSubmitting:
		return "enter add  esc adjust selection  s skip  q quit"
	default:
		return "tab/shift+tab move  v extend to a phrase  enter accept  s skip  q quit"
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
