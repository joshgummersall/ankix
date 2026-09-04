package tui

import (
	"strings"
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
		line := m.layout.wordRow[m.cursorWord]
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

// A document line wider than the viewport wraps onto several rows, and j/k
// must step through those rows instead of jumping the whole line — the
// symptom on a zoomed-in terminal, where one sentence fills most of the
// screen.
func TestScrolling_JStepsThroughWrappedRows(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("palabra ", 40)) // ~7 rows at width 48
	lines := []Line{{Text: long}, {Text: "segunda linea corta"}}
	m := New(Config{Document: &Document{Lines: lines}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	m = mi.(Model)

	firstLineRows := 0 // rows the first line wraps onto, beyond its first
	for wi, r := range m.layout.wordRow {
		if m.words[wi].LineIndex == 0 && r > firstLineRows {
			firstLineRows = r
		}
	}
	if firstLineRows < 3 {
		t.Fatalf("test needs a line wrapping over several rows, got %d rows", firstLineRows+1)
	}

	prevRow := m.layout.wordRow[m.cursorWord]
	for i := 0; i < firstLineRows; i++ {
		mi, _ = m.Update(key("j"))
		m = mi.(Model)
		row := m.layout.wordRow[m.cursorWord]
		if row != prevRow+1 {
			t.Fatalf("j #%d: row %d -> %d, want one row down", i+1, prevRow, row)
		}
		prevRow = row
	}
	if got := m.lineOfCursor(); got != 0 {
		t.Errorf("after %d j presses lineOfCursor() = %d, want 0 (still inside the wrapped line)", firstLineRows, got)
	}

	mi, _ = m.Update(key("j"))
	m = mi.(Model)
	if got := m.lineOfCursor(); got != 1 {
		t.Errorf("one more j: lineOfCursor() = %d, want 1 (onto the next document line)", got)
	}

	mi, _ = m.Update(key("k"))
	m = mi.(Model)
	if got := m.layout.wordRow[m.cursorWord]; got != prevRow {
		t.Errorf("k: row = %d, want %d (back one row)", got, prevRow)
	}
}

// j should hold its column across wrapped rows, like vim's gj.
func TestScrolling_JKeepsTheColumn(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("palabra ", 40))
	m := New(Config{Document: &Document{Lines: []Line{{Text: long}}}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	m = mi.(Model)

	for i := 0; i < 3; i++ { // move a few words into the first row
		mi, _ = m.Update(key("l"))
		m = mi.(Model)
	}
	col := m.layout.wordCol[m.cursorWord]

	mi, _ = m.Update(key("j"))
	m = mi.(Model)
	if got := m.layout.wordCol[m.cursorWord]; got < col-8 || got > col+8 {
		t.Errorf("after j column = %d, want near %d", got, col)
	}
}
