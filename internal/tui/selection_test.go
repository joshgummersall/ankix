package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/subtitle"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestWordSelection_SpansAcrossLineBoundary(t *testing.T) {
	cues := []subtitle.Cue{
		{Text: "hoy tenemos competencia de birria va a competir México contra Estados Unidos"},
		{Text: "estoy en Los Ángeles y voy a ir a las"},
	}
	m := New(Config{Transcript: &subtitle.Transcript{Cues: cues}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)

	// Move cursor to "Estados" (word index 10, 0-based) via 10 "l" presses.
	for i := 0; i < 10; i++ {
		mi, _ = m.Update(key("l"))
		m = mi.(Model)
	}
	if m.cursorWord != 10 {
		t.Fatalf("cursorWord = %d, want 10", m.cursorWord)
	}
	if word := m.words[m.cursorWord].Text; word != "Estados" {
		t.Fatalf("cursor word = %q, want %q", word, "Estados")
	}

	mi, _ = m.Update(key("v"))
	m = mi.(Model)
	if m.state != stateVisual {
		t.Fatalf("state = %v, want stateVisual", m.state)
	}

	// Extend 5 more words to the right, crossing into line 2.
	for i := 0; i < 5; i++ {
		mi, _ = m.Update(key("l"))
		m = mi.(Model)
	}

	mi, _ = m.Update(key("enter"))
	m = mi.(Model)
	if m.state != stateWordPick {
		t.Fatalf("state = %v, want stateWordPick", m.state)
	}

	want := "Estados Unidos estoy en Los Ángeles"
	if m.sentence != want {
		t.Errorf("sentence = %q, want %q", m.sentence, want)
	}
}

func TestWordSelection_TabMovesAndVStartsSelection(t *testing.T) {
	cues := []subtitle.Cue{{Text: "la casa vieja de mi abuela"}}
	m := New(Config{Transcript: &subtitle.Transcript{Cues: cues}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)

	mi, _ = m.Update(key("tab"))
	m = mi.(Model)
	mi, _ = m.Update(key("tab"))
	m = mi.(Model)
	if m.cursorWord != 2 {
		t.Fatalf("cursorWord = %d, want 2 (tab should move forward by word)", m.cursorWord)
	}

	mi, _ = m.Update(key("v"))
	m = mi.(Model)
	if m.state != stateVisual {
		t.Fatalf("state = %v, want stateVisual (v should start a selection)", m.state)
	}

	mi, _ = m.Update(key("tab"))
	m = mi.(Model)
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)

	if m.state != stateWordPick {
		t.Fatalf("state = %v, want stateWordPick", m.state)
	}
	if want := "vieja de"; m.sentence != want {
		t.Errorf("sentence = %q, want %q", m.sentence, want)
	}
}
