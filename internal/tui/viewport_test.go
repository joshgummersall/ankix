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
