package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/joshgummersall/ankix/internal/dict"
	"github.com/joshgummersall/ankix/internal/kindle"
)

func helpTestModel(t *testing.T, w, h int) Model {
	t.Helper()
	lines := []Line{
		{Text: "compré unas llaves nuevas para la puerta de la casa"},
		{Text: "y yo creo que algo muy importante en este tema pasa por la capacidad"},
	}
	m := New(Config{Document: &Document{Lines: lines}, Deck: "scratch"})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return mi.(Model)
}

func send(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		mi, _ := m.Update(key(k))
		m = mi.(Model)
	}
	return m
}

// The whole point of the modal: on a small (zoomed-in) terminal the box must
// stay inside the screen instead of wrapping, which is what the old footer
// keybinding list did.
func TestHelpOverlay_StaysInsideTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{60, 12}, {40, 10}, {100, 30}, {24, 8}} {
		w, h := size[0], size[1]
		m := send(t, helpTestModel(t, w, h), "?")
		if !m.showHelp {
			t.Fatalf("%dx%d: ? did not open help", w, h)
		}

		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > h {
			t.Errorf("%dx%d: view is %d lines, want <= %d", w, h, len(lines), h)
		}
		for i, l := range lines {
			if got := ansi.StringWidth(l); got > w {
				t.Errorf("%dx%d: line %d is %d cells wide, want <= %d: %q", w, h, i, got, w, l)
			}
		}
	}
}

func TestHelpOverlay_ScrollsAndClamps(t *testing.T) {
	m := helpTestModel(t, 60, 12)
	sections := m.helpSections()
	maxScroll := helpMaxScroll(60, 12, sections)
	if maxScroll == 0 {
		t.Fatalf("expected the help content to overflow a 60x12 terminal")
	}

	m = send(t, m, "?")
	if m.helpScroll != 0 {
		t.Errorf("helpScroll = %d on open, want 0", m.helpScroll)
	}

	m = send(t, m, "k")
	if m.helpScroll != 0 {
		t.Errorf("helpScroll = %d after scrolling up from the top, want 0", m.helpScroll)
	}

	m = send(t, m, "j", "j")
	if m.helpScroll != 2 {
		t.Errorf("helpScroll = %d after two j, want 2", m.helpScroll)
	}

	m = send(t, m, "G")
	if m.helpScroll != maxScroll {
		t.Errorf("helpScroll = %d after G, want %d", m.helpScroll, maxScroll)
	}
	m = send(t, m, "j", "j", "j")
	if m.helpScroll != maxScroll {
		t.Errorf("helpScroll = %d past the bottom, want it clamped to %d", m.helpScroll, maxScroll)
	}

	m = send(t, m, "g")
	if m.helpScroll != 0 {
		t.Errorf("helpScroll = %d after g, want 0", m.helpScroll)
	}

	// Scrolling never leaks a key through to the document underneath.
	if m.cursorWord != 0 {
		t.Errorf("cursorWord = %d, want the document cursor untouched while help is open", m.cursorWord)
	}

	m = send(t, m, "esc")
	if m.showHelp {
		t.Errorf("esc did not close help")
	}
}

func TestHelp_OpensFromEveryNonTextState(t *testing.T) {
	// Each entry drives the model into a state, then checks ? opens help.
	for name, tc := range map[string]struct {
		keys  []string
		state state
	}{
		"browse":      {nil, stateBrowse},
		"visual":      {[]string{"v"}, stateVisual},
		"word pick":   {[]string{"enter"}, stateWordPick},
		"word expand": {[]string{"enter", "v"}, stateWordExpand},
	} {
		t.Run(name, func(t *testing.T) {
			m := send(t, helpTestModel(t, 100, 24), tc.keys...)
			if m.state != tc.state {
				t.Fatalf("state = %v, want %v", m.state, tc.state)
			}
			m = send(t, m, "?")
			if !m.showHelp {
				t.Fatalf("? did not open help in %v", tc.state)
			}
			m = send(t, m, "?")
			if m.showHelp {
				t.Errorf("? did not toggle help closed in %v", tc.state)
			}
			if m.state != tc.state {
				t.Errorf("state = %v after help, want it unchanged (%v)", m.state, tc.state)
			}
		})
	}
}

// In the text prompts `?` is a character the user is typing, so it must
// reach the input rather than opening the modal.
func TestHelp_IsALiteralInTextPrompts(t *testing.T) {
	m := send(t, helpTestModel(t, 100, 24), "enter", "e")
	if m.state != stateEditSentence {
		t.Fatalf("state = %v, want stateEditSentence", m.state)
	}
	m = send(t, m, "?")
	if m.showHelp {
		t.Fatalf("? opened help in the sentence editor instead of being typed")
	}
	if !strings.HasSuffix(m.sentenceInput.Value(), "?") {
		t.Errorf("sentence input = %q, want it to end in the typed ?", m.sentenceInput.Value())
	}
}

func TestHelpText_ListsNoKeybindings(t *testing.T) {
	m := helpTestModel(t, 100, 24)
	for _, st := range []state{stateBrowse, stateVisual, stateWordPick, stateWordExpand} {
		m.state = st
		if got := m.helpText(); got != "? help" {
			t.Errorf("helpText() in %v = %q, want %q", st, got, "? help")
		}
	}
}

func TestKindleHelp_OpensAndStaysInsideTheTerminal(t *testing.T) {
	entries := []kindle.Entry{{ID: "1", Word: "llaves", Usage: "compré unas llaves nuevas para la puerta", BookTitle: "Un libro"}}
	km := NewKindleReview(KindleConfig{Entries: entries, Deck: "scratch"})
	mi, _ := km.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	km = mi.(KindleModel)
	km = drain(t, km, km.Init())

	mi, _ = km.Update(key("?"))
	km = mi.(KindleModel)
	if !km.showHelp {
		t.Fatalf("? did not open the Kindle help")
	}

	lines := strings.Split(km.View(), "\n")
	if len(lines) > 12 {
		t.Errorf("view is %d lines, want <= 12", len(lines))
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got > 60 {
			t.Errorf("line %d is %d cells wide, want <= 60: %q", i, got, l)
		}
	}

	mi, _ = km.Update(key("esc"))
	km = mi.(KindleModel)
	if km.showHelp {
		t.Errorf("esc did not close the Kindle help")
	}
	// The picker must still be live — help swallowed the keys, not the state.
	if km.state != kPicking {
		t.Errorf("state = %v, want kPicking", km.state)
	}
}

// The help must not advertise `r` when the configured dict can't refine —
// the same invariant the old footer text carried.
func TestHelpSections_OmitRefineWithoutARefiner(t *testing.T) {
	for name, d := range map[string]dict.Provider{
		"no dict":    nil,
		"plain dict": plainDict{},
	} {
		t.Run(name, func(t *testing.T) {
			_, refine := refineAvailable(d)
			if refine {
				t.Fatalf("refineAvailable = true, want false for %s", name)
			}
			for _, sections := range [][]helpSection{documentHelpSections(refine), kindleHelpSections(refine)} {
				body := strings.ToLower(strings.Join(helpContentLines(sections), "\n"))
				if strings.Contains(body, "refine") {
					t.Errorf("help advertises a dead key:\n%s", body)
				}
			}
		})
	}

	for _, sections := range [][]helpSection{documentHelpSections(true), kindleHelpSections(true)} {
		body := strings.ToLower(strings.Join(helpContentLines(sections), "\n"))
		if !strings.Contains(body, "refine") {
			t.Errorf("help omits refine even though the dict supports it:\n%s", body)
		}
	}
}
