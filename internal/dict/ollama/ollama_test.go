package ollama

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestProvider stands up a fake Ollama /api/chat that replies with reply
// and records the messages it was sent.
func newTestProvider(t *testing.T, reply string) (*Provider, *[]chatMessage) {
	t.Helper()
	var got []chatMessage

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		var req chatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		got = req.Messages
		json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Role: "assistant", Content: reply}})
	}))
	t.Cleanup(srv.Close)

	p := New(srv.URL, "ankix")
	return p, &got
}

func TestDefine_SendsOneUserMessage(t *testing.T) {
	p, sent := newTestProvider(t, "TRANSLATION: keys | LEMMA: key")

	def, lemma, err := p.Define("llaves", "Compré unas llaves nuevas.")
	if err != nil {
		t.Fatalf("Define: %v", err)
	}
	if def != "keys" || lemma != "key" {
		t.Errorf("Define = (%q, %q), want (\"keys\", \"key\")", def, lemma)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d messages, want 1: %+v", len(*sent), *sent)
	}
	if (*sent)[0].Role != "user" {
		t.Errorf("message role = %q, want \"user\"", (*sent)[0].Role)
	}
	if want := "Word: llaves | Sentence: Compré unas llaves nuevas."; (*sent)[0].Content != want {
		t.Errorf("message content = %q, want %q", (*sent)[0].Content, want)
	}
}

func TestDefine_CollapsesLemmaMatchingTranslation(t *testing.T) {
	p, _ := newTestProvider(t, "TRANSLATION: bench | LEMMA: Bench")

	def, lemma, err := p.Define("banco", "Nos sentamos en el banco.")
	if err != nil {
		t.Fatalf("Define: %v", err)
	}
	if def != "bench" {
		t.Errorf("definition = %q, want %q", def, "bench")
	}
	if lemma != "" {
		t.Errorf("lemma = %q, want \"\" for a lemma matching the translation", lemma)
	}
}

func TestRefine_ReplaysQuestionAndAnswerBeforeTheCorrection(t *testing.T) {
	p, sent := newTestProvider(t, "TRANSLATION: keys | LEMMA: key")

	def, lemma, err := p.Refine("llaves", "Compré unas llaves nuevas.", "new keys", "key", "drop the adjective")
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if def != "keys" || lemma != "key" {
		t.Errorf("Refine = (%q, %q), want (\"keys\", \"key\")", def, lemma)
	}

	if len(*sent) != 3 {
		t.Fatalf("sent %d messages, want 3: %+v", len(*sent), *sent)
	}
	want := []chatMessage{
		{Role: "user", Content: "Word: llaves | Sentence: Compré unas llaves nuevas."},
		{Role: "assistant", Content: "TRANSLATION: new keys | LEMMA: key"},
		{Role: "user", Content: "Correction: drop the adjective"},
	}
	for i, w := range want {
		if (*sent)[i] != w {
			t.Errorf("message %d = %+v, want %+v", i, (*sent)[i], w)
		}
	}
}

// Define reports a lemma matching the translation as "", so Refine has to
// put it back — the model never omits the LEMMA half of its own answers,
// and an assistant turn that did wouldn't match the format it's primed on.
func TestRefine_RestoresCollapsedLemmaInTheAssistantTurn(t *testing.T) {
	p, sent := newTestProvider(t, "TRANSLATION: bench | LEMMA: bench")

	if _, _, err := p.Refine("banco", "Nos sentamos en el banco.", "bank", "", "it's furniture here"); err != nil {
		t.Fatalf("Refine: %v", err)
	}

	if want := "TRANSLATION: bank | LEMMA: bank"; (*sent)[1].Content != want {
		t.Errorf("assistant turn = %q, want %q", (*sent)[1].Content, want)
	}
}

func TestRefine_CollapsesLemmaMatchingTranslation(t *testing.T) {
	p, _ := newTestProvider(t, "TRANSLATION: goal | LEMMA: goal")

	def, lemma, err := p.Refine("arco", "Entre el arco y yo.", "arch", "", "this is about soccer")
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if def != "goal" {
		t.Errorf("definition = %q, want %q", def, "goal")
	}
	if lemma != "" {
		t.Errorf("lemma = %q, want \"\" so a refined answer is shaped like a fresh one", lemma)
	}
}

// Too low a num_predict truncates the reply before the "|", as does a model
// that wasn't built from an ankix Modelfile at all.
func TestRefine_UnparseableReplyNamesTheExpectedFormat(t *testing.T) {
	p, _ := newTestProvider(t, "TRANSLATION: whoever you are")

	_, _, err := p.Refine("llaves", "Compré unas llaves.", "new keys", "key", "shorter")
	if err == nil {
		t.Fatal("Refine succeeded on a truncated reply, want an error")
	}
	if !strings.Contains(err.Error(), "TRANSLATION") {
		t.Errorf("error = %q, want it to name the expected reply format", err)
	}
}

func TestParseReply(t *testing.T) {
	tests := []struct {
		name               string
		reply              string
		translation, lemma string
		ok                 bool
	}{
		{"well formed", "TRANSLATION: keys | LEMMA: key", "keys", "key", true},
		{"extra whitespace", "TRANSLATION:  keys  |  LEMMA:  key ", "keys", "key", true},
		{"truncated before the pipe", "TRANSLATION: whoever you are", "", "", false},
		{"no lemma label", "TRANSLATION: keys | key", "", "", false},
		{"empty translation", "TRANSLATION:  | LEMMA: key", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translation, lemma, ok := parseReply(tt.reply)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if translation != tt.translation || lemma != tt.lemma {
				t.Errorf("= (%q, %q), want (%q, %q)", translation, lemma, tt.translation, tt.lemma)
			}
		})
	}
}

// Warm has to look like a real lookup, not a bare load: the point is to make
// Ollama evaluate the Modelfile's system prompt and few-shot examples, which
// only happens for a request carrying an actual question.
func TestWarm_SendsALookupShapedLikeARealOne(t *testing.T) {
	p, sent := newTestProvider(t, "TRANSLATION: bench | LEMMA: bench")

	if err := p.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d messages, want 1: %+v", len(*sent), *sent)
	}
	if (*sent)[0].Role != "user" {
		t.Errorf("message role = %q, want \"user\"", (*sent)[0].Role)
	}
	if want := askContent(warmWord, warmUsage); (*sent)[0].Content != want {
		t.Errorf("message content = %q, want %q", (*sent)[0].Content, want)
	}
}

// Callers fire Warm in the background and ignore it, so it must never panic
// or block on an Ollama that isn't there — the first real lookup reports that.
func TestWarm_ReturnsTheErrorFromAnUnreachableOllama(t *testing.T) {
	p := New("http://127.0.0.1:1", "ankix")

	if err := p.Warm(); err == nil {
		t.Fatal("Warm succeeded against an unreachable Ollama")
	}
}
