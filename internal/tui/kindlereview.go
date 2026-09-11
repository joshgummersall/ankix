package tui

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
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
	kRefine
	kSubmitting
	kDone
)

// KindleConfig holds everything the Kindle vocab review TUI needs to run.
type KindleConfig struct {
	Entries []kindle.Entry // one per word, most-recently-looked-up first

	Deck       string
	Tags       []string
	AnkiClient *anki.Client
	Dict       dict.Provider   // nil disables definition lookups
	Templates  *anki.Templates // nil uses the built-in default card templates

	// DB, if non-nil, is a read-write vocab.db handle used to mark synced
	// words as Mastered.
	DB *sql.DB
}

// kindleSelection is one accepted word/phrase, resolved to byte offsets in
// the sentence, ready to become a card. entries holds every entry the
// phrase covers — more than one if expanding it merged it with a
// neighboring word's phrase. definition/lemma are whatever was already
// fetched and previewed for this phrase, reused as-is rather than looked up
// again.
type kindleSelection struct {
	entries    []kindle.Entry
	start, end int
	definition string
	lemma      string
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

	// refineInput collects a free-form correction for the definition of
	// phrases[refineIdx] — see enterRefine.
	refineInput textinput.Model
	refineIdx   int

	added, duplicates, skipped int

	status    string
	statusErr bool

	showHelp   bool
	helpScroll int

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

	m := KindleModel{
		cfg:           cfg,
		groups:        kindle.GroupBySentence(cfg.Entries),
		sentenceInput: sei,
		refineInput:   newRefineInput(),
	}
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

// refreshDefinitions kicks off a definition lookup for every phrase whose
// text is not previewed yet, so what will be saved is visible before
// submitting.
func (m *KindleModel) refreshDefinitions() tea.Cmd {
	if m.cfg.Dict == nil {
		return nil
	}
	return m.ps.refreshPreviews(m.sentence, func(i, gen int, text string) tea.Cmd {
		return kindleDefCmd(m.cfg.Dict, text, m.sentence, i, gen)
	})
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
	m.setStatus("", false)
	return m.refreshDefinitions()
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

// progressText is the dim header readout of how far this review has got,
// mirroring Model.progressText for documents. It is the only place the
// sentence count appears — the status line below is for transient messages.
func (m KindleModel) progressText() string {
	return fmt.Sprintf("sentence %d/%d", m.groupIdx+1, len(m.groups))
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
		m.refineInput.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		return m.handleKindleKey(msg)

	case debounceExpandMsg:
		if msg.gen != m.ps.debounceGen {
			return m, nil
		}
		return m, m.refreshDefinitions()

	case kindleDefResultMsg:
		m.ps.applyPreview(msg.idx, msg.gen, msg.definition, msg.lemma, msg.err, msg.refined)
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
	if m.showHelp {
		sections := m.helpSections()
		rows, _ := helpViewport(m.width, m.height)
		m.helpScroll, m.showHelp = handleHelpKey(msg.String(), m.helpScroll, helpMaxScroll(m.width, m.height, sections), rows)
		return m, nil
	}

	if m.state == kDone {
		switch msg.String() {
		case "q", "ctrl+c", "enter", "esc":
			return m, tea.Quit
		}
		return m, nil
	}

	// Both text prompts handle ctrl+c themselves and must keep every other
	// rune, including a bare q — "quitar" typed into a correction would
	// otherwise quit mid-review and lose the whole sentence group. `?` is
	// likewise a literal there, so help is reachable from every other state.
	if m.state != kEditSentence && m.state != kRefine {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			m.helpScroll = 0
			return m, nil
		}
	}

	switch m.state {
	case kPicking:
		return m.handlePickingKey(msg)
	case kExpanding:
		return m.handleExpandingKey(msg)
	case kEditSentence:
		return m.handleEditSentenceKey(msg)
	case kRefine:
		return m.handleRefineKey(msg)
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
		m.setStatus("", false)
		return m, m.ps.debounceRefresh()
	case "d":
		m.ps.deleteNearestPhrase()
		m.setStatus("", false)
		return m, nil
	case "e":
		return m, m.enterEditSentence()
	case "r":
		cmd := m.enterRefine()
		return m, cmd
	case "enter":
		return m.submitGroup()
	}
	return m, nil
}

// handleExpandingKey moves the cursor within phrases[m.ps.expandIdx] and
// merges it with any other phrase it comes to overlap.
func (m KindleModel) handleExpandingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch action, cmd := m.ps.handleExpandKey(msg); action {
	case expandMoved:
		return m, cmd
	case expandConfirmed, expandCanceled:
		m.state = kPicking
		m.setStatus("", false)
		return m, m.refreshDefinitions()
	}
	return m, nil
}

// submitGroup builds a card for every included phrase in the current
// sentence and submits them all in one action. Merged phrases contribute
// every entry they absorbed to whichever single card their merged range
// produces.
func (m KindleModel) submitGroup() (tea.Model, tea.Cmd) {
	if m.ps.previewsPending() {
		m.setStatus("still looking up definitions...", false)
		return m, nil
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
		start, end := m.ps.phraseBounds(p)
		sels = append(sels, kindleSelection{entries: entries, start: start, end: end, definition: p.preview, lemma: p.previewLemma})
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
		m.setStatus("", false)
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
	m.setStatus("", false)
	return m.refreshDefinitions()
}

func (m KindleModel) View() string {
	if m.state == kDone {
		return fmt.Sprintf("\ndone: %d added, %d already in Anki, %d skipped\n\npress enter to exit\n", m.added, m.duplicates, m.skipped)
	}

	group := m.groups[m.groupIdx]
	title := "ankix review"
	if len(group.Entries) > 0 && group.Entries[0].BookTitle != "" {
		title = group.Entries[0].BookTitle
	}
	header := titleStyle.Render(title) +
		"  " + helpStyle.Render(m.progressText())

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

	view := header + "\n" + body + "\n" + footer + "\n"

	if m.showHelp {
		view = overlayHelp(view, m.width, m.height, m.helpSections(), m.helpScroll)
	}

	return view
}

func (m KindleModel) renderKindlePicker() string {
	if m.sentence == "" {
		return "\n" + helpStyle.Render("no usage sentence recorded for this lookup") + "\n"
	}

	var b strings.Builder
	b.WriteString(m.ps.render(m.sentence))

	cards := m.ps.countIncluded()
	word := "card"
	if cards != 1 {
		word = "cards"
	}
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("%d %s will be added, deck: %s", cards, word, m.cfg.Deck)))

	if m.cfg.Dict != nil {
		b.WriteString("\n\n")
		b.WriteString(m.ps.renderPreviews(m.sentence))
	}

	if m.state == kRefine && m.refineIdx < len(m.ps.phrases) {
		b.WriteString(renderRefinePrompt(m.ps.phraseText(m.sentence, m.ps.phrases[m.refineIdx]), m.refineInput))
	}

	return "\n" + b.String() + "\n"
}

func (m KindleModel) renderKindleEditSentence() string {
	return "\n" + helpStyle.Render("fix typos in the sentence, then confirm — applies to every word from this sentence") + "\n\n" + m.sentenceInput.View() + "\n"
}

// helpText mirrors Model.helpText: a single non-wrapping hint, with the
// bindings themselves living in the `?` modal.
func (m KindleModel) helpText() string {
	switch m.state {
	case kEditSentence:
		return "enter save  esc discard"
	case kRefine:
		return "enter apply  esc cancel"
	case kSubmitting:
		return "submitting..."
	default:
		return "? help"
	}
}

// helpSections is the modal's content for the Kindle review model.
func (m KindleModel) helpSections() []helpSection {
	_, refine := refineAvailable(m.cfg.Dict)
	return kindleHelpSections(refine)
}

// kindleDefResultMsg carries a definition lookup result back for the phrase
// at idx, tagged with the lookup generation it was issued under (gen) so a
// stale result for a phrase that's since changed can be ignored. refined
// marks a result that came from a user correction rather than a plain
// lookup.
type kindleDefResultMsg struct {
	idx        int
	gen        int
	definition string
	lemma      string
	err        error
	refined    bool
}

func kindleDefCmd(p dict.Provider, phrase, sentence string, idx, gen int) tea.Cmd {
	return func() tea.Msg {
		def, lemma, err := p.Define(phrase, sentence)
		return kindleDefResultMsg{idx: idx, gen: gen, definition: def, lemma: lemma, err: err}
	}
}

// kindleRefineCmd re-asks r for a definition the user wasn't happy with,
// passing the answer being corrected plus their instruction.
func kindleRefineCmd(r dict.Refiner, phrase, sentence, definition, lemma, instruction string, idx, gen int) tea.Cmd {
	return func() tea.Msg {
		def, l, err := r.Refine(phrase, sentence, definition, lemma, instruction)
		return kindleDefResultMsg{idx: idx, gen: gen, definition: def, lemma: l, err: err, refined: true}
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
			note := kindle.BuildNote(cfg.Templates, cfg.Deck, cfg.Tags, sel.entries[0], sentence, sel.start, sel.end, back, sel.lemma)

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
