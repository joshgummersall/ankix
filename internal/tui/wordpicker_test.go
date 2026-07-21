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

func TestWordPick_LMovesCursorForward(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja de mi abuela")

	mi, _ := m.Update(key("l"))
	m = mi.(Model)
	if m.ps.wordCursor != 1 {
		t.Fatalf("after 1 l: wordCursor=%d, want 1", m.ps.wordCursor)
	}

	mi, _ = m.Update(key("l"))
	m = mi.(Model)
	if m.ps.wordCursor != 2 {
		t.Fatalf("after 2 l's: wordCursor=%d, want 2", m.ps.wordCursor)
	}
}

func TestWordPick_HMovesCursorBack(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja de mi abuela")

	for range 3 {
		mi, _ := m.Update(key("l"))
		m = mi.(Model)
	}
	if m.ps.wordCursor != 3 {
		t.Fatalf("wordCursor = %d, want 3", m.ps.wordCursor)
	}

	mi, _ := m.Update(key("h"))
	m = mi.(Model)
	if m.ps.wordCursor != 2 {
		t.Fatalf("after h: wordCursor=%d, want 2", m.ps.wordCursor)
	}
}

// activePhraseCount counts phrases that will actually be added as cards:
// standalone (not merged into another) and not deleted.
func activePhraseCount(m Model) int {
	n := 0
	for _, p := range m.ps.phrases {
		if p.mergedInto == -1 && !p.deleted {
			n++
		}
	}
	return n
}

func TestWordPick_VMarksCurrentWord(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja")

	mi, _ := m.Update(key("l")) // move to "casa"
	m = mi.(Model)
	mi, _ = m.Update(key("v")) // start expanding a new phrase for "casa"
	m = mi.(Model)
	mi, _ = m.Update(key("enter")) // confirm it without extending
	m = mi.(Model)

	if got := activePhraseCount(m); got != 1 {
		t.Fatalf("expected 1 marked word, got %d", got)
	}
	p := m.ps.phrases[0]
	got := m.sentence[m.ps.tokens[m.ps.wordTokens[p.lo]].start:m.ps.tokens[m.ps.wordTokens[p.hi]].end]
	if got != "casa" {
		t.Errorf("marked word text = %q, want %q", got, "casa")
	}

	// d on the same word deletes it.
	mi, _ = m.Update(key("d"))
	m = mi.(Model)
	if got := activePhraseCount(m); got != 0 {
		t.Errorf("expected word to be deleted, got %d marked", got)
	}
}

func TestWordPick_MultipleVMarksMultipleWords(t *testing.T) {
	m := newTestWordPickModel(t, "la casa vieja")

	mi, _ := m.Update(key("v")) // mark "la"
	m = mi.(Model)
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)
	mi, _ = m.Update(key("l"))
	m = mi.(Model)
	mi, _ = m.Update(key("l"))
	m = mi.(Model)
	mi, _ = m.Update(key("v")) // mark "vieja"
	m = mi.(Model)
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)

	if got := activePhraseCount(m); got != 2 {
		t.Fatalf("expected 2 marked words, got %d", got)
	}
}
