package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/subtitle"
)

// TestScrolling_KeepsCursorInView guards against a regression where View's
// value receiver silently dropped viewport.SetContent, so maxYOffset was
// always 0 and the viewport could never actually scroll.
func TestScrolling_KeepsCursorInView(t *testing.T) {
	var cues []subtitle.Cue
	for i := 0; i < 60; i++ {
		cues = append(cues, subtitle.Cue{Start: time.Duration(i) * time.Second, Text: "line"})
	}
	m := New(Config{Transcript: &subtitle.Transcript{Cues: cues}})
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
