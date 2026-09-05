package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joshgummersall/ankix/ollama/vocab"
)

// fakeOllama serves /api/tags with the given model names.
func fakeOllama(t *testing.T, names ...string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		models := make([]map[string]string, len(names))
		for i, n := range names {
			models[i] = map[string]string{"name": n}
		}
		json.NewEncoder(w).Encode(map[string]any{"models": models})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestResolveModel_ReturnsTheChecksumTagWhenInstalled(t *testing.T) {
	want := vocab.Tag("ankix")
	url := fakeOllama(t, "llama3.2:3b", want)

	got, err := resolveModel(url, "ankix")
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if got != want {
		t.Errorf("resolveModel = %q, want %q", got, want)
	}
}

// The whole point of pinning: a model an older ankix built is still
// installed, but this binary won't use it, and says exactly how to fix that.
func TestResolveModel_OutdatedBuildSaysToReinstall(t *testing.T) {
	url := fakeOllama(t, "llama3.2:3b", "ankix:0123456789ab")

	_, err := resolveModel(url, "ankix")
	if err == nil {
		t.Fatal("resolveModel succeeded with only an outdated build installed")
	}
	msg := err.Error()
	for _, want := range []string{"out of date", "ankix:0123456789ab", vocab.Tag("ankix"), "ankix install"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error is missing %q:\n%s", want, msg)
		}
	}
}

func TestResolveModel_NothingInstalledSaysToInstall(t *testing.T) {
	url := fakeOllama(t, "llama3.2:3b")

	_, err := resolveModel(url, "ankix")
	if err == nil {
		t.Fatal("resolveModel succeeded with no ankix model installed")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ankix install") {
		t.Errorf("error should tell the user to run `ankix install`:\n%s", msg)
	}
	if strings.Contains(msg, "out of date") {
		t.Errorf("nothing was installed, so the error shouldn't claim staleness:\n%s", msg)
	}
}

// A user-supplied tag is their own model; `ankix install` would build
// something else, so it must not be the suggested fix.
func TestResolveModel_ExplicitTagDoesNotSuggestInstall(t *testing.T) {
	url := fakeOllama(t, "llama3.2:3b")

	_, err := resolveModel(url, "myfork:v1")
	if err == nil {
		t.Fatal("resolveModel succeeded with an uninstalled explicit tag")
	}
	msg := err.Error()
	if strings.Contains(msg, "ankix install") {
		t.Errorf("a hand-picked model shouldn't be fixed by `ankix install`:\n%s", msg)
	}
	if !strings.Contains(msg, "myfork:v1") {
		t.Errorf("error should name the model asked for:\n%s", msg)
	}
}

func TestResolveModel_ExplicitTagIsUsedVerbatimWhenInstalled(t *testing.T) {
	url := fakeOllama(t, "myfork:v1")

	got, err := resolveModel(url, "myfork:v1")
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if got != "myfork:v1" {
		t.Errorf("resolveModel = %q, want it used verbatim", got)
	}
}

func TestResolveModel_UnreachableOllamaSaysSo(t *testing.T) {
	// A port nothing is listening on.
	_, err := resolveModel("http://127.0.0.1:1", "ankix")
	if err == nil {
		t.Fatal("resolveModel succeeded against an unreachable Ollama")
	}
	msg := err.Error()
	for _, want := range []string{"can't reach Ollama", "ollama serve"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error is missing %q:\n%s", want, msg)
		}
	}
}

func TestCheckKeepAlive_AcceptsWhatOllamaAccepts(t *testing.T) {
	// Durations, bare seconds, immediate unload, and the negative "keep it
	// loaded indefinitely" form, which is why this isn't just ParseDuration.
	for _, v := range []string{"", "30m", "1h30m", "90s", "1800", "0", "-1", "-1s"} {
		if err := checkKeepAlive(v); err != nil {
			t.Errorf("checkKeepAlive(%q) = %v, want nil", v, err)
		}
	}
}

func TestCheckKeepAlive_RejectsAValueOllamaWouldReject(t *testing.T) {
	for _, v := range []string{"forever", "30 minutes", "30min", "m30"} {
		if err := checkKeepAlive(v); err == nil {
			t.Errorf("checkKeepAlive(%q) = nil, want an error", v)
		}
	}
}
