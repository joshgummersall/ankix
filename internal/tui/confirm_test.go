package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
)

func TestRenderWordPicker_ShowsVideoLinkForRealYouTubeID(t *testing.T) {
	lines := []Line{{Text: "la casa vieja"}}
	m := New(Config{
		Document:    &Document{SourceID: "dQw4w9WgXcQ", Lines: lines},
		PreviewLink: func(int) string { return anki.VideoLink("dQw4w9WgXcQ", 0) },
	})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)

	m.selWordStart, m.selWordEnd = 0, len(m.words)-1
	m.enterWordPick()
	mi, _ = m.Update(key("v"))
	m = mi.(Model)
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)

	out := m.renderWordPicker()
	if !strings.Contains(out, "https://youtu.be/dQw4w9WgXcQ") {
		t.Errorf("renderWordPicker output missing video link:\n%s", out)
	}
}

func TestRenderWordPicker_NoLinkForReviewLoadedTranscript(t *testing.T) {
	lines := []Line{{Text: "la casa vieja"}}
	// runReview passes the file path as the source ID, not a real YouTube ID.
	m := New(Config{
		Document:    &Document{SourceID: "/tmp/sample.es.vtt", Lines: lines},
		PreviewLink: func(int) string { return anki.VideoLink("/tmp/sample.es.vtt", 0) },
	})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)

	m.selWordStart, m.selWordEnd = 0, len(m.words)-1
	m.enterWordPick()
	mi, _ = m.Update(key("v"))
	m = mi.(Model)
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)

	out := m.renderWordPicker()
	if strings.Contains(out, "link:") {
		t.Errorf("renderWordPicker output should not show a link for a non-YouTube videoID:\n%s", out)
	}
}
