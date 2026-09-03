package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/dict"
	"github.com/joshgummersall/ankix/internal/kindle"
)

// fakeDict answers every lookup with canned values and records what it was
// asked, so a test can assert that a correction reached the provider (or
// that none did).
type fakeDict struct {
	definition, lemma string
	refined           string // what Refine returns as the definition
	refineErr         error
	instructions      []string // every instruction Refine was called with
	defineCalls       int
}

func (f *fakeDict) Define(word, usage string) (string, string, error) {
	f.defineCalls++
	return f.definition, f.lemma, nil
}

func (f *fakeDict) Refine(word, usage, definition, lemma, instruction string) (string, string, error) {
	f.instructions = append(f.instructions, instruction)
	if f.refineErr != nil {
		return "", "", f.refineErr
	}
	return f.refined, "", nil
}

// plainDict implements Define but not Refine, standing in for a provider
// with no correction support.
type plainDict struct{}

func (plainDict) Define(word, usage string) (string, string, error) { return "gloss", "", nil }

// typeRunes feeds s into the model one keypress at a time, the way a real
// terminal delivers it.
func typeRunes[M tea.Model](t *testing.T, m M, s string) M {
	t.Helper()
	for _, r := range s {
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mi.(M)
	}
	return m
}

// markedWordPickModel returns a word picker over sentence with the word at
// wordIdx marked and its preview already settled.
func markedWordPickModel(t *testing.T, sentence string, wordIdx int, d dict.Provider) Model {
	t.Helper()
	lines := []Line{{Text: sentence}}
	m := New(Config{Document: &Document{Lines: lines}, Dict: d})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(Model)
	m.selWordStart, m.selWordEnd = 0, len(m.words)-1
	m.enterWordPick()

	for range wordIdx {
		mi, _ = m.Update(key("l"))
		m = mi.(Model)
	}
	mi, _ = m.Update(key("v"))
	m = mi.(Model)
	mi, cmd := m.Update(key("enter"))
	m = mi.(Model)
	return drain(t, m, cmd)
}

// drain runs cmd (and anything it batches) and feeds the resulting messages
// back through the model, so lookups issued by a keypress settle before the
// test inspects the result.
func drain[M tea.Model](t *testing.T, m M, cmd tea.Cmd) M {
	t.Helper()
	for range 10 {
		if cmd == nil {
			return m
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			var next tea.Cmd
			for _, c := range batch {
				mi, cc := m.Update(c())
				m = mi.(M)
				if cc != nil {
					next = cc
				}
			}
			cmd = next
			continue
		}
		mi, cc := m.Update(msg)
		m = mi.(M)
		cmd = cc
	}
	return m
}

func TestRefine_ReplacesThePreview(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	if got := m.ps.phrases[0].preview; got != "new keys" {
		t.Fatalf("preview before refining = %q, want %q", got, "new keys")
	}

	mi, _ := m.Update(key("r"))
	m = mi.(Model)
	if m.state != stateRefine {
		t.Fatalf("state = %v after r, want stateRefine", m.state)
	}

	m = typeRunes(t, m, "drop the adjective")
	mi, cmd := m.Update(key("enter"))
	m = drain(t, mi.(Model), cmd)

	if got := d.instructions; len(got) != 1 || got[0] != "drop the adjective" {
		t.Fatalf("instructions sent = %v, want [drop the adjective]", got)
	}
	if got := m.ps.phrases[0].preview; got != "keys" {
		t.Errorf("preview after refining = %q, want %q", got, "keys")
	}
	if !m.ps.phrases[0].refined {
		t.Error("phrase not marked refined after a successful refine")
	}
}

// The answer being corrected has to stay on screen while the model thinks —
// blanking it would leave the user with nothing to compare against, and
// nothing at all if the refine then failed.
func TestRefine_KeepsThePreviousAnswerVisibleWhileInFlight(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(Model), "shorter")
	mi, _ = m.Update(key("enter"))
	m = mi.(Model)

	if !m.ps.phrases[0].previewPending {
		t.Fatal("phrase not pending after dispatching a refine")
	}
	if got := m.ps.phrases[0].preview; got != "new keys" {
		t.Errorf("preview during refine = %q, want the previous answer %q", got, "new keys")
	}
	if out := m.renderWordPicker(); !strings.Contains(out, "new keys (refining...)") {
		t.Errorf("preview list should show the answer being corrected:\n%s", out)
	}
}

func TestRefine_FailureLeavesThePreviousAnswerInPlace(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refineErr: errors.New("ollama is down")}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(Model), "shorter")
	mi, cmd := m.Update(key("enter"))
	m = drain(t, mi.(Model), cmd)

	if m.ps.phrases[0].previewErr == nil {
		t.Error("expected the refine error to be recorded")
	}
	if got := m.ps.phrases[0].preview; got != "new keys" {
		t.Errorf("preview after a failed refine = %q, want the original %q", got, "new keys")
	}
	if m.ps.phrases[0].refined {
		t.Error("a failed refine should not mark the phrase refined")
	}
}

// The phrase text doesn't change when it's refined, so text equality can't
// tell a refinement apart from an ordinary lookup still in flight for the
// same words — only the generation can.
func TestRefine_StaleLookupDoesNotClobberARefinement(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)
	staleGen := m.ps.phrases[0].previewGen

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(Model), "drop the adjective")
	mi, cmd := m.Update(key("enter"))
	m = drain(t, mi.(Model), cmd)

	// An ordinary Define for the same, unchanged phrase text lands late.
	mi, _ = m.Update(glossResultMsg{idx: 0, gen: staleGen, gloss: "new keys", lemma: "key"})
	m = mi.(Model)

	if got := m.ps.phrases[0].preview; got != "keys" {
		t.Errorf("preview = %q after a stale lookup landed, want the refinement %q", got, "keys")
	}
}

// Cancelling an expansion reverts lo/hi but not previewText, so the follow-up
// refresh would re-fetch a plain gloss over the correction if refinements
// weren't sticky.
func TestRefine_SurvivesAnExpandThenCancel(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(Model), "drop the adjective")
	mi, cmd := m.Update(key("enter"))
	m = drain(t, mi.(Model), cmd)

	// Expand the phrase and back out again.
	mi, _ = m.Update(key("v"))
	m = mi.(Model)
	mi, _ = m.Update(key("l"))
	m = mi.(Model)
	mi, cmd = m.Update(key("esc"))
	m = drain(t, mi.(Model), cmd)

	if got := m.ps.phrases[0].preview; got != "keys" {
		t.Errorf("preview = %q after expand+cancel, want the refinement %q kept", got, "keys")
	}
}

// Growing the phrase really does change what's being translated, so the
// correction no longer applies and a fresh lookup should take over.
func TestRefine_DiscardedWhenThePhraseActuallyGrows(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(Model), "drop the adjective")
	mi, cmd := m.Update(key("enter"))
	m = drain(t, mi.(Model), cmd)

	mi, _ = m.Update(key("v"))
	m = mi.(Model)
	mi, _ = m.Update(key("l"))
	m = mi.(Model)
	mi, cmd = m.Update(key("enter"))
	m = drain(t, mi.(Model), cmd)

	if m.ps.phrases[0].refined {
		t.Error("refinement should be dropped once the phrase covers different words")
	}
	if got := m.ps.phrases[0].preview; got != "new keys" {
		t.Errorf("preview = %q after growing the phrase, want a fresh lookup %q", got, "new keys")
	}
}

func TestRefine_EscCancelsWithoutAskingTheModel(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(Model), "drop the adjective")
	mi, _ = m.Update(key("esc"))
	m = mi.(Model)

	if m.state != stateWordPick {
		t.Errorf("state = %v after esc, want stateWordPick", m.state)
	}
	if len(d.instructions) != 0 {
		t.Errorf("esc sent %v to the model, want nothing", d.instructions)
	}
}

func TestRefine_EmptyInstructionCancels(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(Model), "   ")
	mi, cmd := m.Update(key("enter"))
	m = drain(t, mi.(Model), cmd)

	if m.state != stateWordPick {
		t.Errorf("state = %v, want stateWordPick", m.state)
	}
	if len(d.instructions) != 0 {
		t.Errorf("an empty instruction sent %v to the model, want nothing", d.instructions)
	}
}

func TestRefine_OnUnmarkedWordReportsWhy(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := markedWordPickModel(t, "compré unas llaves nuevas", 2, d)

	// Move off the marked phrase.
	mi, _ := m.Update(key("h"))
	m = mi.(Model)
	mi, _ = m.Update(key("r"))
	m = mi.(Model)

	if m.state != stateWordPick {
		t.Errorf("state = %v, want to stay in stateWordPick", m.state)
	}
	if !m.statusErr || m.status == "" {
		t.Errorf("expected an error status, got %q (isErr=%v)", m.status, m.statusErr)
	}
}

func TestRefine_InertWithoutARefiner(t *testing.T) {
	for name, d := range map[string]dict.Provider{
		"no dict":    nil,
		"plain dict": plainDict{},
	} {
		t.Run(name, func(t *testing.T) {
			lines := []Line{{Text: "compré unas llaves nuevas"}}
			m := New(Config{Document: &Document{Lines: lines}, Dict: d})
			mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
			m = mi.(Model)
			m.selWordStart, m.selWordEnd = 0, len(m.words)-1
			m.enterWordPick()
			mi, _ = m.Update(key("v"))
			m = mi.(Model)
			mi, _ = m.Update(key("enter"))
			m = mi.(Model)

			mi, _ = m.Update(key("r"))
			m = mi.(Model)
			if m.state != stateWordPick {
				t.Errorf("state = %v, want r to be inert", m.state)
			}
			if strings.Contains(m.helpText(), "refine") {
				t.Errorf("help text advertises a dead key: %q", m.helpText())
			}
		})
	}
}

// --- Kindle review ---

func newTestKindleModel(t *testing.T, sentence, word string, d dict.Provider) KindleModel {
	t.Helper()
	entries := []kindle.Entry{{ID: "1", Word: word, Usage: sentence, BookTitle: "Un libro"}}
	m := NewKindleReview(KindleConfig{Entries: entries, Deck: "scratch", Dict: d})
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = mi.(KindleModel)
	return drain(t, m, m.Init())
}

func TestKindleRefine_ReplacesThePreview(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := newTestKindleModel(t, "Compré unas llaves nuevas para la puerta.", "llaves", d)

	if got := m.ps.phrases[0].preview; got != "new keys" {
		t.Fatalf("preview before refining = %q, want %q", got, "new keys")
	}

	mi, _ := m.Update(key("r"))
	m = mi.(KindleModel)
	if m.state != kRefine {
		t.Fatalf("state = %v after r, want kRefine", m.state)
	}

	m = typeRunes(t, m, "drop the adjective")
	mi, cmd := m.Update(key("enter"))
	m = drain(t, mi.(KindleModel), cmd)

	if got := d.instructions; len(got) != 1 || got[0] != "drop the adjective" {
		t.Fatalf("instructions sent = %v, want [drop the adjective]", got)
	}
	if got := m.ps.phrases[0].preview; got != "keys" {
		t.Errorf("preview after refining = %q, want %q", got, "keys")
	}
}

// Kindle quits on a bare q before dispatching by state, so the refine prompt
// has to be excluded from that guard — plenty of Spanish corrections contain
// a q, and quitting would lose the whole sentence group.
func TestKindleRefine_QIsTypedNotTreatedAsQuit(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := newTestKindleModel(t, "Compré unas llaves nuevas para la puerta.", "llaves", d)

	mi, _ := m.Update(key("r"))
	m = typeRunes(t, mi.(KindleModel), "quitar el adjetivo")

	if m.state != kRefine {
		t.Fatalf("state = %v, want to still be in kRefine", m.state)
	}
	if got := m.refineInput.Value(); got != "quitar el adjetivo" {
		t.Errorf("refine input = %q, want the full instruction typed", got)
	}
}

func TestKindleRefine_CtrlCStillQuits(t *testing.T) {
	d := &fakeDict{definition: "new keys", lemma: "key", refined: "keys"}
	m := newTestKindleModel(t, "Compré unas llaves nuevas para la puerta.", "llaves", d)

	mi, _ := m.Update(key("r"))
	m = mi.(KindleModel)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c in the refine prompt returned no command, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c returned %T, want tea.QuitMsg", cmd())
	}
}
