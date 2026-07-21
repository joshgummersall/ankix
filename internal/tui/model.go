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
	"github.com/joshgummersall/ankix/internal/translate"
)

type state int

const (
	stateBrowse state = iota
	stateVisual
	stateWordPick
	stateWordExpand
	stateEditSentence
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
	Translator  translate.Provider // nil if glossing is disabled
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

	cursorWord int
	pendingG   bool

	// visualAnchor is the fixed end of the in-progress document selection
	// (stateVisual), set to cursorWord when visual mode began; cursorWord is
	// the other, moving end — see visualBounds.
	visualAnchor int

	searching   bool
	searchInput textinput.Model
	searchTerm  string

	showHelp bool

	selWordStart, selWordEnd int // confirmed word selection, inclusive

	sentence      string
	sentenceInput textarea.Model // pre-filled with sentence while editing, for fixing typos; wraps long lines
	ps            phraseSet[struct{}]

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

	return Model{
		cfg:           cfg,
		state:         stateBrowse,
		searchInput:   si,
		sentenceInput: sei,
		cardedWords:   make(map[int]bool),
		words:         words,
		lineFirstWord: lineFirstWord,
	}
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
		if msg.idx < len(m.ps.phrases) && m.ps.phrases[msg.idx].previewText == msg.text {
			m.ps.phrases[msg.idx].previewPending = false
			m.ps.phrases[msg.idx].preview = msg.gloss
			m.ps.phrases[msg.idx].previewErr = msg.err
		}
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

func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}

	header := titleStyle.Render(m.cfg.Title) +
		"  " + helpStyle.Render(fmt.Sprintf("%d lines loaded", len(m.cfg.Document.Lines)))
	if m.searching {
		header = m.searchInput.View()
	}

	var body string
	switch m.state {
	case stateWordPick, stateWordExpand, stateSubmitting:
		body = m.renderWordPicker()
	case stateEditSentence:
		body = m.renderEditSentence()
	default:
		// Content is kept in sync by syncViewport (called from Update, not
		// here — View has a value receiver, so mutating m.viewport here
		// wouldn't persist to the next frame).
		body = m.viewport.View()
	}

	if (m.state == stateWordPick || m.state == stateWordExpand || m.state == stateSubmitting) && m.width > 0 {
		body = lipgloss.NewStyle().Width(m.width).Render(body)
	}

	statusLine := statusStyle
	if m.statusErr {
		statusLine = errStatusStyle
	}
	footer := statusLine.Render(m.status) + "\n" + helpStyle.Render(m.helpText())

	view := header + "\n" + body + "\n" + footer

	if m.showHelp {
		view = m.overlayHelp(view)
	}

	return view
}

func (m Model) helpText() string {
	switch m.state {
	case stateVisual:
		return "h/l/j/k extend selection  enter complete selection  esc cancel"
	case stateWordPick:
		return "h/l move  v expand/add word  d delete word  e edit sentence  enter add all  esc cancel"
	case stateWordExpand:
		return "h/l extend selection  enter confirm  esc cancel"
	case stateEditSentence:
		return "enter save  esc discard changes"
	case stateSubmitting:
		return "submitting..."
	default:
		return "h/l word  j/k line  ctrl+d/ctrl+u half page  )/( sentence  gg/G top/bottom  v select  V select sentence  / search  enter confirm  ? help  q quit"
	}
}
