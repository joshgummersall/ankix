package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/dict"
)

// newRefineInput builds the one-line prompt both review TUIs use to collect
// a correction ("drop the adjective", "it's a verb here").
func newRefineInput() textinput.Model {
	ri := textinput.New()
	ri.Prompt = "refine: "
	ri.Placeholder = "what's wrong with it?"
	return ri
}

// renderRefinePrompt is the block appended below the preview list while a
// correction is being typed. The sentence and the preview list stay on
// screen above it, so the phrase and the answer being corrected are both
// still visible.
func renderRefinePrompt(phrase string, in textinput.Model) string {
	return "\n" + helpStyle.Render("tell the model what to fix about “"+phrase+"”") +
		"\n" + in.View() + "\n"
}

// refineAvailable reports whether p can take a correction at all. A nil
// Provider (--no-gloss) fails the assertion just as an implementation
// without Refine does, so this is the single check both the `r` key and the
// help text consult.
func refineAvailable(p dict.Provider) (dict.Refiner, bool) {
	r, ok := p.(dict.Refiner)
	return r, ok
}

// enterRefine opens the correction prompt for the phrase under the cursor,
// or reports why it can't. The two review models differ only in their state
// enum and status text, so the eligibility check and input setup live here.
func (m *Model) enterRefine() tea.Cmd {
	if _, ok := refineAvailable(m.cfg.Dict); !ok {
		return nil
	}
	idx, why := m.ps.beginRefine()
	if idx == -1 {
		m.setStatus(why, true)
		return nil
	}
	m.refineIdx = idx
	m.refineInput.SetValue("")
	m.state = stateRefine
	m.setStatus("", false)
	return m.refineInput.Focus()
}

func (m Model) handleRefineKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	// textinput has no ctrl+c handling of its own, so without this the app
	// would be unquittable while the prompt is open.
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.refineInput.Blur()
		m.state = stateWordPick
		m.setStatus("", false)
		return m, nil
	case "enter":
		cmd := m.submitRefine()
		m.refineInput.Blur()
		return m, cmd
	}
	var cmd tea.Cmd
	m.refineInput, cmd = m.refineInput.Update(msg)
	return m, cmd
}

// submitRefine dispatches the correction, or treats an empty instruction as
// a cancel — there's nothing to ask the model.
func (m *Model) submitRefine() tea.Cmd {
	m.state = stateWordPick
	instruction := strings.TrimSpace(m.refineInput.Value())
	if instruction == "" {
		m.setStatus("", false)
		return nil
	}
	r, ok := refineAvailable(m.cfg.Dict)
	if !ok || m.refineIdx >= len(m.ps.phrases) {
		return nil
	}

	p := m.ps.phrases[m.refineIdx]
	text := m.ps.phraseText(m.sentence, p)
	gen := m.ps.startRefine(m.refineIdx)
	m.setStatus("refining “"+text+"”...", false)
	return refineGlossCmd(r, text, m.sentence, p.preview, p.previewLemma, instruction, m.refineIdx, gen)
}

// enterRefine is KindleModel's counterpart to Model.enterRefine.
func (m *KindleModel) enterRefine() tea.Cmd {
	if _, ok := refineAvailable(m.cfg.Dict); !ok {
		return nil
	}
	idx, why := m.ps.beginRefine()
	if idx == -1 {
		m.setStatus(why, true)
		return nil
	}
	m.refineIdx = idx
	m.refineInput.SetValue("")
	m.state = kRefine
	m.setStatus("", false)
	return m.refineInput.Focus()
}

func (m KindleModel) handleRefineKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.refineInput.Blur()
		m.state = kPicking
		m.setStatus("", false)
		return m, nil
	case "enter":
		cmd := m.submitRefine()
		m.refineInput.Blur()
		return m, cmd
	}
	var cmd tea.Cmd
	m.refineInput, cmd = m.refineInput.Update(msg)
	return m, cmd
}

func (m *KindleModel) submitRefine() tea.Cmd {
	m.state = kPicking
	instruction := strings.TrimSpace(m.refineInput.Value())
	if instruction == "" {
		m.setStatus("", false)
		return nil
	}
	r, ok := refineAvailable(m.cfg.Dict)
	if !ok || m.refineIdx >= len(m.ps.phrases) {
		return nil
	}

	p := m.ps.phrases[m.refineIdx]
	text := m.ps.phraseText(m.sentence, p)
	gen := m.ps.startRefine(m.refineIdx)
	m.setStatus("refining “"+text+"”...", false)
	return kindleRefineCmd(r, text, m.sentence, p.preview, p.previewLemma, instruction, m.refineIdx, gen)
}
