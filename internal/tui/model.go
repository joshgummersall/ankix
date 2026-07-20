// Package tui implements the Bubble Tea interface for browsing a transcript
// with vim-style navigation and generating Anki cards from it.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/subtitle"
	"github.com/joshgummersall/ankix/internal/translate"
)

type state int

const (
	stateBrowse state = iota
	stateVisual
	stateWordPick
	stateEditSentence
	stateConfirm
	stateSubmitting
)

// Config holds everything the TUI needs to run.
type Config struct {
	Transcript *subtitle.Transcript
	VideoTitle string
	Deck       string
	AnkiClient *anki.Client
	Translator translate.Provider // nil if glossing is disabled
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg Config

	state    state
	viewport viewport.Model
	width    int
	height   int
	ready    bool

	words        []subtitle.Word // every word in the transcript, in order, tagged with its source line
	cueFirstWord []int           // cueFirstWord[i] = index into words of cue i's first word

	cursorWord       int
	visualAnchorWord int
	pendingG         bool

	searching   bool
	searchInput textinput.Model
	searchTerm  string

	showHelp bool

	selWordStart, selWordEnd int // confirmed word selection, inclusive

	sentence      string
	sentenceInput textarea.Model // pre-filled with sentence while editing, for fixing transcript typos; wraps long lines
	tokens        []token
	wordTokens    []int // indices into tokens that are words
	wordCursor    int   // index into wordTokens

	markedWords  []anki.WordSelection // words marked with x, each becomes its own card
	glossPending int                  // count of in-flight gloss lookups
	cueStart     time.Duration

	cardedLines map[int]bool // cue indices with at least one submitted card

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

	words := subtitle.FlattenWords(cfg.Transcript.Cues)
	cueFirstWord := make([]int, len(cfg.Transcript.Cues))
	last := -1
	for i, w := range words {
		if w.CueIndex != last {
			cueFirstWord[w.CueIndex] = i
			last = w.CueIndex
		}
	}

	return Model{
		cfg:           cfg,
		state:         stateBrowse,
		searchInput:   si,
		sentenceInput: sei,
		cardedLines:   make(map[int]bool),
		words:         words,
		cueFirstWord:  cueFirstWord,
		status:        fmt.Sprintf("%d lines loaded — tab/shift+tab word, j/k line, x select, enter confirm, q quit", len(cfg.Transcript.Cues)),
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

	case glossResultMsg:
		if m.glossPending > 0 {
			m.glossPending--
		}
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("gloss lookup failed: %v", msg.err), true)
			return m, nil
		}
		for i, s := range m.markedWords {
			if s.Start == msg.start && s.End == msg.end {
				m.markedWords[i].Gloss = msg.gloss
				break
			}
		}
		return m, nil

	case submitResultMsg:
		m.state = stateConfirm
		switch {
		case msg.err != nil && msg.added == 0 && msg.duplicates == 0:
			// Total failure (e.g. Anki isn't running) — stay put so the
			// selection isn't lost and the user can retry.
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
			m.cardedLines[m.words[i].CueIndex] = true
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

	header := titleStyle.Render(fmt.Sprintf("ankitube — %s", m.cfg.VideoTitle))
	if m.searching {
		header = m.searchInput.View()
	}

	var body string
	switch m.state {
	case stateWordPick:
		body = m.renderWordPicker()
	case stateEditSentence:
		body = m.renderEditSentence()
	case stateConfirm, stateSubmitting:
		body = m.renderConfirm()
	default:
		// Content is kept in sync by syncViewport (called from Update, not
		// here — View has a value receiver, so mutating m.viewport here
		// wouldn't persist to the next frame).
		body = m.viewport.View()
	}

	if m.state == stateWordPick && m.width > 0 {
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
		return "tab/shift+tab extend by word  j/k extend by line  enter complete selection  esc cancel"
	case stateWordPick:
		return "tab/shift+tab move  x mark word  e edit sentence  enter confirm  esc cancel"
	case stateEditSentence:
		return "enter save  esc discard changes"
	case stateConfirm:
		return "enter submit  esc back to word picker"
	default:
		return "tab/shift+tab word  j/k line  gg/G top/bottom  x start selection  / search  enter confirm  ? help  q quit"
	}
}
