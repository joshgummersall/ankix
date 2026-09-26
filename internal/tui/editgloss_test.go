package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// clearInput deletes everything in a prefilled single-line input, one
// backspace at a time, the way a user would.
func clearInput[M tea.Model](t *testing.T, m M, n int) M {
	t.Helper()
	for range n {
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = mi.(M)
	}
	return m
}

func TestEditGloss_ReplacesThePreview(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key"}
	m := markedWordPickModel(t, "Compré unas llaves nuevas.", 2, d)

	m = send(t, m, "e")
	if m.state != stateEditGloss {
		t.Fatalf("state = %v after e, want stateEditGloss", m.state)
	}
	if got := m.glossInput.Value(); got != "new keys" {
		t.Fatalf("editor prefilled with %q, want the current back %q", got, "new keys")
	}

	m = clearInput(t, m, len("new keys"))
	m = typeRunes(t, m, "keys")
	m = send(t, m, "enter")

	if m.state != stateWordPick {
		t.Errorf("state = %v after enter, want stateWordPick", m.state)
	}
	p := m.ps.phrases[0]
	if p.preview != "keys" {
		t.Errorf("preview = %q, want %q", p.preview, "keys")
	}
	if p.previewLemma != "key" {
		t.Errorf("lemma = %q, want it kept as %q", p.previewLemma, "key")
	}
	if !p.refined {
		t.Error("an edited back must be marked refined so a refresh doesn't re-fetch over it")
	}
	if d.defineCalls != 1 {
		t.Errorf("Define called %d times, want 1 — editing must not trigger a lookup", d.defineCalls)
	}
}

func TestEditGloss_EscDiscards(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key"}
	m := markedWordPickModel(t, "Compré unas llaves nuevas.", 2, d)

	m = send(t, m, "e")
	m = typeRunes(t, m, " and more")
	m = send(t, m, "esc")

	if m.state != stateWordPick {
		t.Errorf("state = %v after esc, want stateWordPick", m.state)
	}
	if got := m.ps.phrases[0].preview; got != "new keys" {
		t.Errorf("preview = %q after esc, want the original %q", got, "new keys")
	}
}

func TestEditGloss_NeedsAMarkedWord(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key"}
	m := markedWordPickModel(t, "Compré unas llaves nuevas.", 2, d)

	m = send(t, m, "h", "e")
	if m.state != stateWordPick {
		t.Errorf("state = %v, want to stay in stateWordPick off a marked word", m.state)
	}
	if !m.statusErr {
		t.Error("expected an error status explaining why e did nothing")
	}
}

func TestEditGloss_InertWithoutLookups(t *testing.T) {
	m := markedWordPickModel(t, "Compré unas llaves nuevas.", 2, nil)

	m = send(t, m, "e")
	if m.state != stateWordPick {
		t.Errorf("state = %v, want e to be inert with lookups off", m.state)
	}
}

func TestHelpSections_OmitEditGlossWithoutLookups(t *testing.T) {
	for _, sections := range [][]helpSection{documentHelpSections(false, false), kindleHelpSections(false, false)} {
		body := strings.ToLower(strings.Join(helpContentLines(sections), "\n"))
		if strings.Contains(body, "card back") {
			t.Errorf("help advertises e with lookups off:\n%s", body)
		}
	}
	for _, sections := range [][]helpSection{documentHelpSections(true, false), kindleHelpSections(true, false)} {
		body := strings.ToLower(strings.Join(helpContentLines(sections), "\n"))
		if !strings.Contains(body, "card back") {
			t.Errorf("help omits e even though lookups are on:\n%s", body)
		}
	}
}

// Kindle quits on a bare q outside the text prompts, so the editor must be
// excluded from that guard like the refine prompt is.
func TestKindleEditGloss_ReplacesThePreviewAndKeepsQ(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key"}
	m := newTestKindleModel(t, "Compré unas llaves nuevas para la puerta.", "llaves", d)

	mi, _ := m.Update(key("e"))
	m = mi.(KindleModel)
	if m.state != kEditGloss {
		t.Fatalf("state = %v after e, want kEditGloss", m.state)
	}

	m = clearInput(t, m, len("new keys"))
	m = typeRunes(t, m, "quick keys")
	if m.state != kEditGloss {
		t.Fatalf("state = %v after typing q, want to still be in kEditGloss", m.state)
	}
	mi, _ = m.Update(key("enter"))
	m = mi.(KindleModel)

	if got := m.ps.phrases[0].preview; got != "quick keys" {
		t.Errorf("preview = %q, want %q", got, "quick keys")
	}
	if m.state != kPicking {
		t.Errorf("state = %v after enter, want kPicking", m.state)
	}
}
