package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var helpBindings = [][2]string{
	{"h / l, ← / →", "move by word"},
	{"j / k, ↓ / ↑", "move by line"},
	{"ctrl+d / ctrl+u", "move by half page"},
	{"gg", "jump to first word"},
	{"G", "jump to last word"},
	{"/", "search transcript"},
	{"n / N", "next / previous match"},
	{"v", "start a sentence selection, anchored at the cursor"},
	{"h / l, j / k (visual)", "extend selection — move the cursor, vim-visual-mode style"},
	{"enter", "complete selection → mark words for cards"},
	{"h / l (word pick)", "move the word cursor"},
	{"v (word pick)", "expand/add the word under the cursor as a phrase"},
	{"d (word pick)", "delete the nearest word/phrase"},
	{"e (word pick)", "edit the sentence to fix typos before marking words"},
	{"enter (word pick)", "add every marked word/phrase as its own card"},
	{"h / l (expand)", "extend phrase — move the cursor, vim-visual-mode style"},
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
