package tui

import "github.com/charmbracelet/lipgloss"

// Colors are defined as AdaptiveColor pairs so the TUI reads correctly on
// both light and dark terminal themes — plain ANSI codes like "252" (near
// white) or "237" (near black) are tuned for one theme and become
// invisible/washed out on the other.
var (
	currentLineMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "117"}).Bold(true)
	selectedLineStyle = lipgloss.NewStyle().Underline(true).
				Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "117"})
	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "243", Dark: "246"})
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"}).
			Padding(0, 1)
	errStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"}).
			Bold(true).Padding(0, 1)
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "243", Dark: "240"})
	wordCursorStyle = lipgloss.NewStyle().Reverse(true).Bold(true)
	markedWordStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"}).Bold(true)
	// markedWordCursorStyle is markedWordStyle plus an underline, used for
	// the cursor's exact position within a marked phrase — the whole
	// phrase still reads as one colored block, with the underline as the
	// only cue for where the cursor sits inside it.
	markedWordCursorStyle = markedWordStyle.Underline(true)
	titleStyle            = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "39"}).Bold(true)
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"}).Bold(true)
	helpPopoverStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.AdaptiveColor{Light: "25", Dark: "39"}).
				Padding(1, 2)
	cardedMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "78"}).Bold(true)
)
