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
	// markedWordStyleA/B alternate between adjacent, separately-selected
	// phrases so two cards sitting right next to each other in the
	// sentence (e.g. "jamás" then "hayas oído") are visually distinct
	// instead of blurring into what looks like one longer phrase. Each is
	// a solid background block with an explicit high-contrast foreground
	// (not left to the terminal's default) so it reads clearly regardless
	// of theme. markedWordCursorStyleA/B mark the cursor's exact position
	// within a phrase by reversing that block's own colors, rather than
	// underlining — still a colored block, just inverted.
	markedWordStyleA = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "215", Dark: "208"}).
				Foreground(lipgloss.Color("0")).Bold(true)
	markedWordStyleB = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "51", Dark: "37"}).
				Foreground(lipgloss.Color("0")).Bold(true)
	markedWordCursorStyleA = markedWordStyleA.Reverse(true)
	markedWordCursorStyleB = markedWordStyleB.Reverse(true)
	titleStyle             = lipgloss.NewStyle().
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
