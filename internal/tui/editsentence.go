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

// applyEditedSentence swaps in the edited sentence. Editing can shift every
// byte offset in the sentence, so — unlike Kindle's sentence editor, which
// can cheaply recompute each known word's position from its lookup text —
// every phrase here is dropped rather than remapped, since there's no
// source of truth to recompute them against.
func (m *Model) applyEditedSentence(edited string) tea.Cmd {
	if edited == m.sentence {
		m.state = stateWordPick
		m.setStatus("", false)
		return nil
	}

	m.sentence = edited
	m.ps.reset(m.sentence)
	m.state = stateWordPick
	m.setStatus("sentence edited — previous word marks were cleared", false)
	return nil
}
