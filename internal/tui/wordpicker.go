package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
)

// token is a slice of the working sentence: either a word (matched by
// wordRe) or a separator (whitespace/punctuation) run between words.
type token struct {
	start, end int
	isWord     bool
}

var wordRe = regexp.MustCompile(`[\p{L}]+`)

func tokenize(s string) []token {
	var tokens []token
	last := 0
	for _, loc := range wordRe.FindAllStringIndex(s, -1) {
		if loc[0] > last {
			tokens = append(tokens, token{start: last, end: loc[0], isWord: false})
		}
		tokens = append(tokens, token{start: loc[0], end: loc[1], isWord: true})
		last = loc[1]
	}
	if last < len(s) {
		tokens = append(tokens, token{start: last, end: len(s), isWord: false})
	}
	return tokens
}

func (m *Model) enterWordPick() {
	var parts []string
	for i := m.selWordStart; i <= m.selWordEnd; i++ {
		parts = append(parts, m.words[i].Text)
	}
	m.sentence = strings.Join(parts, " ")
	m.cueStart = m.cfg.Transcript.Cues[m.words[m.selWordStart].CueIndex].Start
	m.tokens = tokenize(m.sentence)
	m.markedWords = nil
	m.glossPending = 0
	m.setStatus("", false)

	m.wordTokens = m.wordTokens[:0]
	for i, t := range m.tokens {
		if t.isWord {
			m.wordTokens = append(m.wordTokens, i)
		}
	}
	m.wordCursor = 0
	m.state = stateWordPick
}

// markedIndexAt returns the index into m.markedWords of a word covering
// [start,end), or -1 if none has been marked yet.
func (m Model) markedIndexAt(start, end int) int {
	for i, s := range m.markedWords {
		if s.Start == start && s.End == end {
			return i
		}
	}
	return -1
}

func (m Model) handleWordPickKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateBrowse
		m.setStatus("", false)
		m.syncViewport()
		return m, nil
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
	case "x":
		if len(m.wordTokens) == 0 {
			return m, nil
		}
		t := m.tokens[m.wordTokens[m.wordCursor]]
		if i := m.markedIndexAt(t.start, t.end); i >= 0 {
			m.markedWords = append(m.markedWords[:i], m.markedWords[i+1:]...)
			return m, nil
		}
		m.markedWords = append(m.markedWords, anki.WordSelection{Start: t.start, End: t.end})
		if m.cfg.Translator == nil {
			return m, nil
		}
		m.glossPending++
		return m, fetchGlossCmd(m.cfg.Translator, m.sentence[t.start:t.end], m.sentence, t.start, t.end)
	case "enter":
		if len(m.markedWords) == 0 {
			m.setStatus("mark at least one word with x first", true)
			return m, nil
		}
		m.setStatus("", false)
		m.state = stateConfirm
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) renderWordPicker() string {
	var b strings.Builder
	b.WriteString(helpStyle.Render(fmt.Sprintf("%d word(s) marked — each becomes its own card", len(m.markedWords))))
	b.WriteString("\n\n")

	wordPos := -1
	for _, t := range m.tokens {
		text := m.sentence[t.start:t.end]
		if t.isWord {
			wordPos++
		}
		marked := t.isWord && m.markedIndexAt(t.start, t.end) >= 0
		switch {
		case t.isWord && wordPos == m.wordCursor:
			text = wordCursorStyle.Render(text)
		case marked:
			text = markedWordStyle.Render(text)
		}
		b.WriteString(text)
	}
	return "\n" + b.String() + "\n"
}
