package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Every state must name itself in the footer, so browsing the transcript
// and working on a picked sentence can't be mistaken for each other.
func TestModeBadge_NamesEveryState(t *testing.T) {
	m := helpTestModel(t, 100, 24)
	for st, want := range map[state]string{
		stateBrowse:     "BROWSE",
		stateVisual:     "VISUAL",
		stateWordPick:   "PICK",
		stateWordExpand: "EXPAND",
		stateRefine:     "REFINE",
		stateEditGloss:  "EDIT BACK",
		stateSubmitting: "ADDING",
	} {
		m.state = st
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		footer := lines[len(lines)-1]
		if !strings.HasPrefix(strings.TrimSpace(footer), want+" ") {
			t.Errorf("footer in %v = %q, want it to start with %q", st, footer, want)
		}
	}

	m.state = stateBrowse
	m.searching = true
	if got := ansi.Strip(m.modeBadge()); !strings.Contains(got, "SEARCH") {
		t.Errorf("badge while searching = %q, want SEARCH", got)
	}
}

func TestKindleModeBadge_NamesEveryState(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key"}
	m := newTestKindleModel(t, "Compré unas llaves nuevas para la puerta.", "llaves", d)
	for st, want := range map[kState]string{
		kPicking:    "PICK",
		kExpanding:  "EXPAND",
		kRefine:     "REFINE",
		kEditGloss:  "EDIT BACK",
		kSubmitting: "ADDING",
	} {
		m.state = st
		if got := ansi.Strip(m.View()); !strings.Contains(got, " "+want+" ") {
			t.Errorf("view in %v has no %q badge:\n%s", st, want, got)
		}
	}
}
