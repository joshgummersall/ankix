package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// helpSection is one feature's worth of keybindings in the help modal —
// the transcript viewer, the word picker, and so on. Bindings are pairs of
// {keys, description}.
type helpSection struct {
	title    string
	bindings [][2]string
}

// documentHelpSections is the keymap for the document browser TUI (Model).
// refine reports whether the configured dict can take a correction — when
// it can't, `r` is inert (see refineAvailable) and must not be advertised.
func documentHelpSections(refine bool) []helpSection {
	sections := []helpSection{
		{"Transcript", [][2]string{
			{"h / l, ← / →", "move by word"},
			{"j / k, ↓ / ↑", "move by line"},
			{"ctrl+d / ctrl+u", "move by half page"},
			{"gg / G", "jump to first / last word"},
			{") / (", "jump to next / previous sentence"},
			{"/", "search the transcript"},
			{"n / N", "jump to next / previous match"},
			{"v", "start a selection, anchored at the cursor"},
			{"V", "select the whole current sentence"},
			{"enter", "pick words from the cursor's sentence"},
		}},
		{"Visual selection", [][2]string{
			{"h / l, j / k", "extend the selection, vim-visual-mode style"},
			{"enter", "confirm → mark words for cards"},
			{"v, esc", "cancel the selection"},
		}},
		{"Word picker", [][2]string{
			{"h / l, ← / →", "move the word cursor"},
			{") / (", "jump to next / previous marked word"},
			{"v", "expand/add the word under the cursor as a phrase"},
			{"d", "delete the nearest word/phrase"},
			{"e", "edit the sentence to fix typos"},
		}},
		{"Phrase expand", [][2]string{
			{"h / l, ← / →", "extend the phrase"},
			{"enter", "confirm the phrase"},
			{"esc", "revert the phrase"},
		}},
		{"Sentence editor", [][2]string{
			{"enter", "save the edited sentence"},
			{"esc", "discard the edit"},
		}},
		{"Help", [][2]string{
			{"j / k, ↓ / ↑", "scroll this help"},
			{"ctrl+d / ctrl+u", "scroll by half page"},
			{"g / G", "jump to top / bottom"},
			{"?, esc, q", "close this help"},
		}},
		{"Anywhere", [][2]string{
			{"?", "show this help"},
			{"esc", "back out of the current mode"},
			{"q, ctrl+c", "quit"},
		}},
	}
	return withRefine(sections, "Word picker", refine)
}

// kindleHelpSections is the keymap for the Kindle vocabulary review TUI
// (KindleModel), which shares the phrase/expand/refine machinery but browses
// sentence groups instead of a document.
func kindleHelpSections(refine bool) []helpSection {
	sections := []helpSection{
		{"Review", [][2]string{
			{"h / l, ← / →", "move the word cursor"},
			{"v", "expand/add the word under the cursor as a phrase"},
			{"d", "delete the nearest word/phrase"},
			{"e", "edit the sentence to fix typos"},
			{"enter", "add every marked word/phrase, then next sentence"},
		}},
		{"Phrase expand", [][2]string{
			{"h / l, ← / →", "extend the phrase"},
			{"enter", "confirm the phrase"},
			{"esc", "revert the phrase"},
		}},
		{"Sentence editor", [][2]string{
			{"enter", "save the edited sentence"},
			{"esc", "discard the edit"},
		}},
		{"Help", [][2]string{
			{"j / k, ↓ / ↑", "scroll this help"},
			{"ctrl+d / ctrl+u", "scroll by half page"},
			{"g / G", "jump to top / bottom"},
			{"?, esc, q", "close this help"},
		}},
		{"Anywhere", [][2]string{
			{"?", "show this help"},
			{"q, ctrl+c", "quit"},
		}},
	}
	return withRefine(sections, "Review", refine)
}

// refineHelp is listed under the picker section, and only when the dict can
// actually refine — otherwise the help would advertise a dead key. Its
// description names the Refine prompt so the key points at the section
// listing that prompt's own bindings.
var refineHelp = [2]string{"r", "open the Refine prompt for the translation"}

var refineSection = helpSection{"Refine prompt", [][2]string{
	{"enter", "apply the correction"},
	{"esc", "cancel the correction"},
}}

func withRefine(sections []helpSection, pickerTitle string, refine bool) []helpSection {
	if !refine {
		return sections
	}
	out := make([]helpSection, 0, len(sections)+1)
	for _, s := range sections {
		if s.title == pickerTitle {
			s.bindings = append(append([][2]string{}, s.bindings...), refineHelp)
			out = append(out, s, refineSection)
			continue
		}
		out = append(out, s)
	}
	return out
}

// helpContentLines renders the sections to styled lines, one binding per
// line, with a blank line between sections.
func helpContentLines(sections []helpSection) []string {
	var lines []string
	for i, s := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, titleStyle.Render(s.title))
		for _, kb := range s.bindings {
			lines = append(lines, "  "+helpKeyStyle.Render(padRight(kb[0], 18))+kb[1])
		}
	}
	return lines
}

// helpViewport is the geometry of the help popover for a given terminal
// size: how many content lines fit inside the box, and how wide they may be.
// Both are clamped so the box never spills outside the terminal — the whole
// point of the modal is that it survives a zoomed-in (small) terminal.
func helpViewport(width, height int) (rows, cols int) {
	// Border (2) + vertical padding (2) + blank separator (1) + hint (1).
	rows = height - 6
	// Border (2) + horizontal padding (4).
	cols = width - 6
	return max(rows, 1), max(cols, 8)
}

// helpMaxScroll is the largest scroll offset that still shows content, so
// the key handler and the renderer agree on the bottom of the list.
func helpMaxScroll(width, height int, sections []helpSection) int {
	rows, _ := helpViewport(width, height)
	return max(len(helpContentLines(sections))-rows, 0)
}

// overlayHelp renders the keybinding popover on top of background, centered,
// leaving the surrounding view visible (with its original styling intact,
// via spliceLine/ansi.Cut) around it rather than blanking the whole screen.
// Content taller than the terminal is windowed by scroll rather than
// overflowing.
func overlayHelp(background string, width, height int, sections []helpSection, scroll int) string {
	rows, cols := helpViewport(width, height)
	content := helpContentLines(sections)

	scroll = min(max(scroll, 0), max(len(content)-rows, 0))
	window := content[scroll:min(scroll+rows, len(content))]

	hint := "esc close"
	if len(content) > rows {
		hint = "j/k scroll · esc close"
		if scroll > 0 {
			hint = "↑ " + hint
		}
		if scroll+rows < len(content) {
			hint += " ↓"
		}
	}

	// Size the box against every line, not just the visible window, so it
	// doesn't grow and shrink under the cursor while scrolling.
	boxW := ansi.StringWidth(hint)
	for _, l := range content {
		boxW = max(boxW, ansi.StringWidth(l))
	}
	boxW = min(boxW, cols)

	fit := func(l string) string {
		l = ansi.Truncate(l, boxW, "…")
		if pad := boxW - ansi.StringWidth(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		return l
	}

	var b strings.Builder
	for _, l := range window {
		b.WriteString(fit(l))
		b.WriteString("\n")
	}
	b.WriteString(fit(""))
	b.WriteString("\n")
	b.WriteString(fit(helpStyle.Render(hint)))

	popoverLines := strings.Split(helpPopoverStyle.Render(b.String()), "\n")

	// helpViewport's clamps keep the box inside all but the most extreme
	// terminals; these two are the hard backstop, so a box can never spill
	// past the screen no matter how far the user has zoomed in.
	if height > 0 && len(popoverLines) > height {
		popoverLines = popoverLines[:height]
	}
	for i, l := range popoverLines {
		if width > 0 && ansi.StringWidth(l) > width {
			popoverLines[i] = ansi.Truncate(l, width, "")
		}
	}

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
	left := max((width-popW)/2, 0)

	for i, pl := range popoverLines {
		bgIdx := top + i
		if bgIdx >= len(bgLines) {
			break
		}
		bgLines[bgIdx] = spliceLine(bgLines[bgIdx], pl, left)
	}

	return strings.Join(bgLines, "\n")
}

// handleHelpKey applies a keypress made while the help modal is open,
// returning the new scroll offset and whether the modal stays open. Both
// TUIs share it so the modal scrolls identically in each.
func handleHelpKey(key string, scroll, maxScroll, rows int) (newScroll int, open bool) {
	switch key {
	case "?", "esc", "q", "ctrl+c":
		return 0, false
	case "j", "down":
		scroll++
	case "k", "up":
		scroll--
	case "ctrl+d", "pgdown":
		scroll += max(rows/2, 1)
	case "ctrl+u", "pgup":
		scroll -= max(rows/2, 1)
	case "g", "home":
		scroll = 0
	case "G", "end":
		scroll = maxScroll
	}
	return min(max(scroll, 0), maxScroll), true
}

// spliceLine overlays styled (possibly narrower/wider) content onto a styled
// background line starting at column left, padding the background with
// spaces if it's too short. Uses ansi.Cut (not rune slicing) to pull out the
// background's left/right edges so their original styling survives the
// splice instead of being flattened to plain text.
func spliceLine(background, overlay string, left int) string {
	bgWidth := ansi.StringWidth(background)
	if bgWidth < left {
		background += strings.Repeat(" ", left-bgWidth)
		bgWidth = left
	}
	leftPart := ansi.Cut(background, 0, left)

	rightStart := left + ansi.StringWidth(overlay)
	var rightPart string
	if rightStart < bgWidth {
		rightPart = ansi.Cut(background, rightStart, bgWidth)
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
