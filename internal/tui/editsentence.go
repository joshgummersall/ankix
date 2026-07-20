package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// enterEditSentence opens a text input pre-filled with the current sentence
// so typos introduced by auto-generated captions can be fixed before words
// are picked for cards.
func (m *Model) enterEditSentence() tea.Cmd {
	m.sentenceInput.SetValue(m.sentence)
	m.sentenceInput.CursorEnd()
	cmd := m.sentenceInput.Focus()
	m.state = stateEditSentence
	m.setStatus("", false)
	return cmd
}

func (m Model) handleEditSentenceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sentenceInput.Blur()
		m.state = stateWordPick
		m.setStatus("", false)
		return m, nil
	case "enter":
		m.applyEditedSentence(m.sentenceInput.Value())
		m.sentenceInput.Blur()
		m.state = stateWordPick
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.sentenceInput, cmd = m.sentenceInput.Update(msg)
	return m, cmd
}

// applyEditedSentence swaps in the edited sentence. Since editing can shift
// every byte offset in the sentence, any marks (and their in-flight gloss
// lookups) are dropped rather than tried to be remapped onto the new text —
// the word cursor is repositioned as closely as possible by word index
// instead of byte offset.
func (m *Model) applyEditedSentence(edited string) {
	if edited == m.sentence {
		return
	}

	prevWordCursor := m.wordCursor

	m.sentence = edited
	m.tokens = tokenize(m.sentence)
	m.markedWords = nil
	m.glossPending = 0

	m.wordTokens = m.wordTokens[:0]
	for i, t := range m.tokens {
		if t.isWord {
			m.wordTokens = append(m.wordTokens, i)
		}
	}

	m.wordCursor = 0
	if len(m.wordTokens) > 0 {
		m.wordCursor = min(prevWordCursor, len(m.wordTokens)-1)
	}

	m.setStatus("sentence edited — previous word marks were cleared", false)
}
