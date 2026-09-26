package tui

import "github.com/charmbracelet/lipgloss"

// modeKind groups the TUI states by what the keys do in them, so the footer
// badge can tell "moving around" apart from "shaping a selection" and
// "typing text" at a glance, vim-style.
type modeKind int

const (
	modeNavigate modeKind = iota // browsing the transcript
	modePick                     // working on the marked words of one sentence
	modeSelect                   // growing/shrinking a selection or phrase
	modeInput                    // keys are typed into a text field
	modeBusy                     // waiting on Anki
)

// modeBadgeColors gives each kind its own background. modeSelect reuses the
// marked-phrase orange so the badge matches the highlight it describes.
var modeBadgeColors = map[modeKind]lipgloss.AdaptiveColor{
	modeNavigate: {Light: "250", Dark: "244"},
	modePick:     {Light: "33", Dark: "39"},
	modeSelect:   {Light: "215", Dark: "208"},
	modeInput:    {Light: "170", Dark: "170"},
	modeBusy:     {Light: "250", Dark: "244"},
}

// renderModeBadge is the label that opens the footer hint line, e.g.
// " EXPAND ", followed by a space before whatever hint comes after it.
func renderModeBadge(label string, kind modeKind) string {
	return lipgloss.NewStyle().
		Background(modeBadgeColors[kind]).
		Foreground(lipgloss.Color("0")).
		Bold(true).
		Padding(0, 1).
		Render(label) + " "
}

// modeBadge names the Model's current mode for the footer.
func (m Model) modeBadge() string {
	if m.searching {
		return renderModeBadge("SEARCH", modeInput)
	}
	switch m.state {
	case stateVisual:
		return renderModeBadge("VISUAL", modeSelect)
	case stateWordPick:
		return renderModeBadge("PICK", modePick)
	case stateWordExpand:
		return renderModeBadge("EXPAND", modeSelect)
	case stateRefine:
		return renderModeBadge("REFINE", modeInput)
	case stateEditGloss:
		return renderModeBadge("EDIT BACK", modeInput)
	case stateSubmitting:
		return renderModeBadge("ADDING", modeBusy)
	default:
		return renderModeBadge("BROWSE", modeNavigate)
	}
}

// modeBadge names the KindleModel's current mode for the footer.
func (m KindleModel) modeBadge() string {
	switch m.state {
	case kExpanding:
		return renderModeBadge("EXPAND", modeSelect)
	case kRefine:
		return renderModeBadge("REFINE", modeInput)
	case kEditGloss:
		return renderModeBadge("EDIT BACK", modeInput)
	case kSubmitting:
		return renderModeBadge("ADDING", modeBusy)
	default:
		return renderModeBadge("PICK", modePick)
	}
}
