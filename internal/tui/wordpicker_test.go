package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/subtitle"
)

func newTestWordPickModel(t *testing.T, sentence string) Model {
	t.Helper()
	cues := []subtitle.Cue{{Text: sentence}}
	m := New(Config{Transcript: &subtitle.Transcript{Cues: cues}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)
	m.selWordStart, m.selWordEnd = 0, len(m.words)-1
	m.enterWordPick()
	return m
}

func TestWordPick_TabMovesCursorForward(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja de mi abuela")

	mi, _ := m.Update(key("tab"))
	m = mi.(Model)
	if m.wordCursor != 1 {
		t.Fatalf("after 1 tab: wordCursor=%d, want 1", m.wordCursor)
	}

	mi, _ = m.Update(key("tab"))
	m = mi.(Model)
	if m.wordCursor != 2 {
		t.Fatalf("after 2 tabs: wordCursor=%d, want 2", m.wordCursor)
	}
}

func TestWordPick_ShiftTabMovesCursorBack(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja de mi abuela")

	for range 3 {
		mi, _ := m.Update(key("tab"))
		m = mi.(Model)
	}
	if m.wordCursor != 3 {
		t.Fatalf("wordCursor = %d, want 3", m.wordCursor)
	}

	mi, _ := m.Update(key("shift+tab"))
	m = mi.(Model)
	if m.wordCursor != 2 {
		t.Fatalf("after shift+tab: wordCursor=%d, want 2", m.wordCursor)
	}
}

func TestWordPick_XMarksCurrentWord(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja")

	mi, _ := m.Update(key("tab")) // move to "casa"
	m = mi.(Model)
	mi, _ = m.Update(key("x"))
	m = mi.(Model)

	if len(m.markedWords) != 1 {
		t.Fatalf("expected 1 marked word, got %d", len(m.markedWords))
	}
	got := m.sentence[m.markedWords[0].Start:m.markedWords[0].End]
	if got != "casa" {
		t.Errorf("marked word text = %q, want %q", got, "casa")
	}

	// x again on the same word unmarks it.
	mi, _ = m.Update(key("x"))
	m = mi.(Model)
	if len(m.markedWords) != 0 {
		t.Errorf("expected word to be unmarked, got %d marked", len(m.markedWords))
	}
}

func TestWordPick_MultipleXMarksMultipleWords(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja")

	mi, _ := m.Update(key("x")) // mark "la"
	m = mi.(Model)
	mi, _ = m.Update(key("tab"))
	m = mi.(Model)
	mi, _ = m.Update(key("tab"))
	m = mi.(Model)
	mi, _ = m.Update(key("x")) // mark "vieja"
	m = mi.(Model)

	if len(m.markedWords) != 2 {
		t.Fatalf("expected 2 marked words, got %d", len(m.markedWords))
	}
}
