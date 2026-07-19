package ollama

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGloss_TrimsReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatResponse{
			Message: chatMessage{Role: "assistant", Content: "  bench  \n"},
		})
	}))
	defer srv.Close()

	p := New(srv.URL, "ankitube")
	gloss, err := p.Gloss("banco", "Nos sentamos en el banco del parque.")
	if err != nil {
		t.Fatal(err)
	}
	if gloss != "bench" {
		t.Errorf("Gloss = %q, want %q", gloss, "bench")
	}
}

func TestGloss_EmptyReplyIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Content: "   "}})
	}))
	defer srv.Close()

	p := New(srv.URL, "ankitube")
	if _, err := p.Gloss("banco", "sentence"); err == nil {
		t.Error("expected error for empty reply")
	}
}
