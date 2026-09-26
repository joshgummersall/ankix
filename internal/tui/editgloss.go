package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// newGlossInput builds the one-line editor both review TUIs use to rewrite
// the back of a card by hand, when a refine round-trip isn't worth it.
func newGlossInput() textinput.Model {
	gi := textinput.New()
	gi.Prompt = "back: "
	return gi
}

// renderGlossEditor is the block appended below the preview list while a
// card back is being edited, mirroring renderRefinePrompt.
func renderGlossEditor(phrase string, in textinput.Model) string {
	return "\n" + helpStyle.Render("edit the back of the card for “"+phrase+"”") +
		"\n" + in.View() + "\n"
}

// glossEditValue is what an edited back is saved as: the input's text with
// surrounding whitespace dropped.
func glossEditValue(in textinput.Model) string {
	return strings.TrimSpace(in.Value())
}

// enterEditGloss opens the editor pre-filled with the back of the phrase
// under the cursor, or reports why it can't.
func (m *Model) enterEditGloss() tea.Cmd {
	if m.cfg.Dict == nil {
		m.setStatus("glosses are off, so there's no card back to edit", true)
		return nil
	}
	idx, why := m.ps.beginEdit()
	if idx == -1 {
		m.setStatus(why, true)
		return nil
	}
	m.glossIdx = idx
	m.glossInput.SetValue(m.ps.phrases[idx].preview)
	m.glossInput.CursorEnd()
	m.state = stateEditGloss
	m.setStatus("", false)
	return m.glossInput.Focus()
}

func (m Model) handleEditGlossKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	// textinput has no ctrl+c handling of its own, so without this the app
	// would be unquittable while the editor is open.
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.glossInput.Blur()
		m.state = stateWordPick
		m.setStatus("", false)
		return m, nil
	case "enter":
		m.ps.setPreview(m.glossIdx, glossEditValue(m.glossInput))
		m.glossInput.Blur()
		m.state = stateWordPick
		m.setStatus("", false)
		return m, nil
	}
	var cmd tea.Cmd
	m.glossInput, cmd = m.glossInput.Update(msg)
	return m, cmd
}

// enterEditGloss is KindleModel's counterpart to Model.enterEditGloss.
func (m *KindleModel) enterEditGloss() tea.Cmd {
	if m.cfg.Dict == nil {
		m.setStatus("definitions are off, so there's no card back to edit", true)
		return nil
	}
	idx, why := m.ps.beginEdit()
	if idx == -1 {
		m.setStatus(why, true)
		return nil
	}
	m.glossIdx = idx
	m.glossInput.SetValue(m.ps.phrases[idx].preview)
	m.glossInput.CursorEnd()
	m.state = kEditGloss
	m.setStatus("", false)
	return m.glossInput.Focus()
}

func (m KindleModel) handleEditGlossKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.glossInput.Blur()
		m.state = kPicking
		m.setStatus("", false)
		return m, nil
	case "enter":
		m.ps.setPreview(m.glossIdx, glossEditValue(m.glossInput))
		m.glossInput.Blur()
		m.state = kPicking
		m.setStatus("", false)
		return m, nil
	}
	var cmd tea.Cmd
	m.glossInput, cmd = m.glossInput.Update(msg)
	return m, cmd
}
