package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/subtitle"
)

func TestRenderConfirm_ShowsVideoLinkForRealYouTubeID(t *testing.T) {
	cues := []subtitle.Cue{{Text: "la casa vieja"}}
	m := New(Config{Transcript: &subtitle.Transcript{VideoID: "dQw4w9WgXcQ", Cues: cues}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)

	m.selWordStart, m.selWordEnd = 0, len(m.words)-1
	m.enterWordPick()
	mi, _ = m.Update(key("x"))
	m = mi.(Model)
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)

	if m.state != stateConfirm {
		t.Fatalf("state = %v, want stateConfirm", m.state)
	}
	out := m.renderConfirm()
	if !strings.Contains(out, "https://youtu.be/dQw4w9WgXcQ") {
		t.Errorf("renderConfirm output missing video link:\n%s", out)
	}
}

func TestRenderConfirm_NoLinkForReviewLoadedTranscript(t *testing.T) {
	cues := []subtitle.Cue{{Text: "la casa vieja"}}
	// runReview passes the file path as VideoID, not a real YouTube ID.
	m := New(Config{Transcript: &subtitle.Transcript{VideoID: "/tmp/sample.es.vtt", Cues: cues}})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)

	m.selWordStart, m.selWordEnd = 0, len(m.words)-1
	m.enterWordPick()
	mi, _ = m.Update(key("x"))
	m = mi.(Model)
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)

	out := m.renderConfirm()
	if strings.Contains(out, "link:") {
		t.Errorf("renderConfirm output should not show a link for a non-YouTube videoID:\n%s", out)
	}
}
