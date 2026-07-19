package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var helpBindings = [][2]string{
	{"tab / shift+tab", "move forward / back a word"},
	{"h / l, ← / →", "move by word (same as tab/shift+tab)"},
	{"j / k, ↓ / ↑", "move by line"},
	{"gg", "jump to first word"},
	{"G", "jump to last word"},
	{"/", "search transcript"},
	{"n / N", "next / previous match"},
	{"x", "start a sentence selection"},
	{"enter", "complete selection → mark words for cards"},
	{"tab / shift+tab (word pick)", "move the word cursor"},
	{"x (word pick)", "mark/unmark current word — each becomes its own card"},
	{"esc", "cancel / back out"},
	{"?", "toggle this help"},
	{"q, ctrl+c", "quit"},
}

// overlayHelp renders the keybinding popover on top of background, centered,
// leaving the surrounding transcript visible around it rather than blanking
// the whole screen. Background lines under the popover are stripped of
// their own styling (color would otherwise bleed past the popover's edges
// once spliced) but their text stays visible.
func (m Model) overlayHelp(background string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Keybindings"))
	b.WriteString("\n\n")
	for _, kb := range helpBindings {
		b.WriteString(helpKeyStyle.Render(padRight(kb[0], 16)))
		b.WriteString(kb[1])
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("press ? or esc to close"))

	popoverLines := strings.Split(helpPopoverStyle.Render(b.String()), "\n")

	popW := 0
	for _, l := range popoverLines {
		if w := ansi.StringWidth(l); w > popW {
			popW = w
		}
	}
	popH := len(popoverLines)

	bgLines := strings.Split(background, "\n")
	for len(bgLines) < popH {
		bgLines = append(bgLines, "")
	}

	top := max((len(bgLines)-popH)/2, 0)
	left := max((m.width-popW)/2, 0)

	for i, pl := range popoverLines {
		bgIdx := top + i
		if bgIdx >= len(bgLines) {
			break
		}
		bgLines[bgIdx] = spliceLine(ansi.Strip(bgLines[bgIdx]), pl, left)
	}

	return strings.Join(bgLines, "\n")
}

// spliceLine overlays styled (possibly narrower/wider) content onto a plain
// background line starting at column left, padding the background with
// spaces if it's too short and preserving anything past the overlay's
// right edge.
func spliceLine(plainBackground, overlay string, left int) string {
	bgRunes := []rune(plainBackground)
	for len(bgRunes) < left {
		bgRunes = append(bgRunes, ' ')
	}
	leftPart := string(bgRunes[:left])

	rightStart := left + ansi.StringWidth(overlay)
	var rightPart string
	if rightStart < len(bgRunes) {
		rightPart = string(bgRunes[rightStart:])
	}
	return leftPart + overlay + rightPart
}

func padRight(s string, n int) string {
	w := len([]rune(s))
	if w >= n {
		return s + " "
	}
	return s + strings.Repeat(" ", n-w)
}
