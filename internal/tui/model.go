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
	stateWordExpand
	stateEditSentence
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

	cursorWord int
	pendingG   bool

	// visualLo/Hi are the independent word-index boundaries of the
	// in-progress transcript selection (stateVisual), grown/shrunk one at a
	// time by h/l (word) and j/k (line) — mirrors the v/h/l phrase-expansion
	// UX used once inside word pick.
	visualLo, visualHi int

	searching   bool
	searchInput textinput.Model
	searchTerm  string

	showHelp bool

	selWordStart, selWordEnd int // confirmed word selection, inclusive

	sentence      string
	sentenceInput textarea.Model // pre-filled with sentence while editing, for fixing transcript typos; wraps long lines
	ps            phraseSet[struct{}]

	cueStart time.Duration

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
		status:        fmt.Sprintf("%d lines loaded — h/l word, j/k line, v select, enter confirm, q quit", len(cfg.Transcript.Cues)),
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
		return "h/l grow left/right  H/L shrink left/right  j/k grow down/up  J/K shrink down/up  enter complete selection  esc cancel"
	case stateWordPick:
		return "h/l move  v expand/add word  d delete word  e edit sentence  enter add all  esc cancel"
	case stateWordExpand:
		return "h/l grow left/right  H/L shrink left/right  enter confirm  esc cancel"
	case stateEditSentence:
		return "enter save  esc discard changes"
	case stateSubmitting:
		return "submitting..."
	default:
		return "h/l word  j/k line  gg/G top/bottom  v start selection  / search  enter confirm  ? help  q quit"
	}
}
