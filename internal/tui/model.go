// Package tui implements the Bubble Tea interface for browsing a source
// document (a YouTube transcript, a web article, a local file, ...) with
// vim-style navigation and generating Anki cards from it.
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict"
	"github.com/joshgummersall/ankix/internal/position"
)

type state int

const (
	stateBrowse state = iota
	stateVisual
	stateWordPick
	stateWordExpand
	stateEditSentence
	stateRefine
	stateSubmitting
)

// Config holds everything the TUI needs to run. BuildNote and PreviewLink
// are supplied by the calling source (youtube/web/file) and let it decide
// what a submitted card and its optional preview link look like, without
// the TUI needing to know that those shapes differ.
type Config struct {
	Document    *Document
	Title       string
	Deck        string
	AnkiClient  *anki.Client
	Dict        dict.Provider // nil if glossing is disabled
	BuildNote   func(lineIndex int, sentence string, sel anki.WordSelection) anki.Note
	PreviewLink func(lineIndex int) string // nil, or returns "" for no link
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg Config

	state    state
	viewport viewport.Model
	width    int
	height   int
	ready    bool

	words          []word // every word in the document, in order, tagged with its source line
	lineFirstWord  []int  // lineFirstWord[i] = index into words of line i's first word
	lineVisualLine []int  // lineVisualLine[i] = wrapped viewport line that line i starts on, set by syncViewport

	cursorWord    int
	pendingG      bool
	lastSavedWord int // cursorWord last written via position.Save; -1 until the first save, so word 0 still gets persisted once

	// visualAnchor is the fixed end of the in-progress document selection
	// (stateVisual), set to cursorWord when visual mode began; cursorWord is
	// the other, moving end — see visualBounds.
	visualAnchor int

	searching   bool
	searchInput textinput.Model
	searchTerm  string

	showHelp   bool
	helpScroll int

	selWordStart, selWordEnd int // confirmed word selection, inclusive

	sentence      string
	sentenceInput textarea.Model // pre-filled with sentence while editing, for fixing typos; wraps long lines
	ps            phraseSet[struct{}]

	// refineInput collects a free-form correction for the gloss of
	// phrases[refineIdx] — see enterRefine.
	refineInput textinput.Model
	refineIdx   int

	selLineIndex int // line the current sentence was picked from, passed to Config.BuildNote/PreviewLink

	cardedWords map[int]bool // word indices included in a submitted card

	status    string
	statusErr bool
}

func New(cfg Config) Model {
	si := textinput.New()
	si.Prompt = "/"

	sei := textarea.New()
	sei.Prompt = "edit: "
	sei.ShowLineNumbers = false
	sei.SetWidth(120)
	sei.SetHeight(3)

	words := flattenWords(cfg.Document.Lines)
	lineFirstWord := make([]int, len(cfg.Document.Lines))
	last := -1
	for i, w := range words {
		if w.LineIndex != last {
			lineFirstWord[w.LineIndex] = i
			last = w.LineIndex
		}
	}

	m := Model{
		cfg:           cfg,
		state:         stateBrowse,
		searchInput:   si,
		sentenceInput: sei,
		refineInput:   newRefineInput(),
		cardedWords:   make(map[int]bool),
		words:         words,
		lineFirstWord: lineFirstWord,
		lastSavedWord: -1,
	}

	if cfg.Document.SourceID != "" {
		if pos, ok := position.Load(cfg.Document.SourceID); ok && pos.Line >= 0 && pos.Line < len(lineFirstWord) {
			lineStart := lineFirstWord[pos.Line]
			lineEnd := len(words)
			if pos.Line+1 < len(lineFirstWord) {
				lineEnd = lineFirstWord[pos.Line+1]
			}
			m.cursorWord = lineStart + pos.Word
			if max := lineEnd - 1; m.cursorWord > max {
				m.cursorWord = max
			}
			if m.cursorWord < lineStart {
				m.cursorWord = lineStart
			}
		}
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		headerH, footerH := 2, 2
		vpHeight := msg.Height - headerH - footerH
		if vpHeight < 0 {
			vpHeight = 0
		}
		if !m.ready {
			m.viewport = viewport.New(msg.Width, vpHeight)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpHeight
		}
		m.sentenceInput.SetWidth(msg.Width)
		m.refineInput.Width = msg.Width
		m.syncViewport()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case debounceExpandMsg:
		if msg.gen != m.ps.debounceGen {
			return m, nil
		}
		return m, m.refreshGlosses()

	case glossResultMsg:
		m.ps.applyPreview(msg.idx, msg.gen, msg.gloss, msg.lemma, msg.err, msg.refined)
		return m, nil

	case submitResultMsg:
		switch {
		case msg.err != nil && msg.added == 0 && msg.duplicates == 0:
			// Total failure (e.g. Anki isn't running) — stay on the word
			// picker so the selection isn't lost and the user can retry.
			m.state = stateWordPick
			m.setStatus(fmt.Sprintf("failed to add cards: %v", msg.err), true)
			return m, nil
		case msg.err != nil:
			m.setStatus(fmt.Sprintf("%d card(s) added, %d duplicate(s) skipped, then hit an error: %v", msg.added, msg.duplicates, msg.err), true)
		case msg.added == 0:
			m.setStatus(fmt.Sprintf("%d duplicate(s) — already in deck", msg.duplicates), true)
		default:
			m.setStatus(fmt.Sprintf("%d card(s) added", msg.added), false)
		}
		m.state = stateBrowse
		for i := m.selWordStart; i <= m.selWordEnd; i++ {
			m.cardedWords[i] = true
		}
		m.syncViewport()
		return m, nil
	}

	return m, nil
}

func (m *Model) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
}

// progressText is the dim header readout of where the cursor sits in the
// document: the current line of the total, plus that as a percentage, which
// answers "how much is left?" at a glance.
func (m Model) progressText() string {
	n := len(m.cfg.Document.Lines)
	if n == 0 {
		return "0 / 0, 0%"
	}
	line := m.lineOfCursor()
	pct := 100
	if n > 1 {
		pct = line * 100 / (n - 1)
	}
	return fmt.Sprintf("%d / %d, %d%%", line+1, n, pct)
}

func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}

	header := titleStyle.Render(m.cfg.Title) +
		"  " + helpStyle.Render(m.progressText())
	if m.searching {
		header = m.searchInput.View()
	}

	var body string
	switch m.state {
	case stateWordPick, stateWordExpand, stateRefine, stateSubmitting:
		body = m.renderWordPicker()
	case stateEditSentence:
		body = m.renderEditSentence()
	default:
		// Content is kept in sync by syncViewport (called from Update, not
		// here — View has a value receiver, so mutating m.viewport here
		// wouldn't persist to the next frame).
		body = m.viewport.View()
	}

	if (m.state == stateWordPick || m.state == stateWordExpand || m.state == stateRefine || m.state == stateSubmitting) && m.width > 0 {
		body = lipgloss.NewStyle().Width(m.width).Render(body)
	}

	statusLine := statusStyle
	if m.statusErr {
		statusLine = errStatusStyle
	}
	footer := statusLine.Render(m.status) + "\n" + helpStyle.Render(m.helpText())

	view := header + "\n" + body + "\n" + footer

	if m.showHelp {
		view = overlayHelp(view, m.width, m.height, m.helpSections(), m.helpScroll)
	}

	return view
}

// helpText is the one-line footer hint. It deliberately does not list
// keybindings — a full list wraps on a narrow (or zoomed-in) terminal, so
// the bindings live in the `?` modal instead. The two text-entry states are
// the exception: `?` is typed into the input there, so help can't be opened
// and the two keys that do work are named outright.
func (m Model) helpText() string {
	switch m.state {
	case stateEditSentence:
		return "enter save  esc discard"
	case stateRefine:
		return "enter apply  esc cancel"
	case stateSubmitting:
		return "submitting..."
	default:
		return "? help"
	}
}

// helpSections is the modal's content for this model, with the refine keys
// included only when the configured dict can actually refine.
func (m Model) helpSections() []helpSection {
	_, refine := refineAvailable(m.cfg.Dict)
	return documentHelpSections(refine)
}
