package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestScrolling_KeepsCursorInView guards against a regression where View's
// value receiver silently dropped viewport.SetContent, so maxYOffset was
// always 0 and the viewport could never actually scroll.
func TestScrolling_KeepsCursorInView(t *testing.T) {
	var lines []Line
	for i := 0; i < 60; i++ {
		lines = append(lines, Line{Text: "line"})
	}
	m := New(Config{Document: &Document{Lines: lines}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 15})
	m = mi.(Model)

	for i := 0; i < 59; i++ {
		mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = mi.(Model)
		line := m.lineOfCursor()
		top := m.viewport.YOffset
		bottom := top + m.viewport.Height
		if line < top || line >= bottom {
			t.Fatalf("i=%d: cursor off screen: line=%d not in [%d,%d)", i, line, top, bottom)
		}
	}
	if got := m.lineOfCursor(); got != 59 {
		t.Errorf("lineOfCursor() = %d, want 59 (clamped to last line)", got)
	}
}

// TestScrolling_CentersCursor checks the scrolloff behavior: the cursor
// walks down from the top edge, then stays mid-screen while the text
// scrolls under it, then walks to the bottom edge once the document's end
// is on screen.
func TestScrolling_CentersCursor(t *testing.T) {
	const total = 60
	var lines []Line
	for i := 0; i < total; i++ {
		lines = append(lines, Line{Text: "line"})
	}
	m := New(Config{Document: &Document{Lines: lines}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 15})
	m = mi.(Model)

	h := m.viewport.Height
	middle := (h - 1) / 2
	maxOffset := max(m.viewport.TotalLineCount()-h, 0)
	sawTop, sawMiddle, sawBottom := false, false, false
	for i := 0; i < total-1; i++ {
		mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = mi.(Model)
		line := m.lineVisualLine[m.lineOfCursor()]
		row := line - m.viewport.YOffset
		want := line - min(max(line-middle, 0), maxOffset)
		if row != want {
			t.Fatalf("visual line %d: cursor on screen row %d, want %d", line, row, want)
		}
		switch {
		case row < middle:
			sawTop = true
		case row == middle:
			sawMiddle = true
		default:
			sawBottom = true
		}
	}
	if !sawTop || !sawMiddle || !sawBottom {
		t.Errorf("cursor rows seen: top=%v middle=%v bottom=%v, want all three", sawTop, sawMiddle, sawBottom)
	}
}
