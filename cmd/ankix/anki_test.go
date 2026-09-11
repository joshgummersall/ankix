package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAnkiConnect serves the version call every reachability probe makes.
func fakeAnkiConnect(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":6,"error":null}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// setFlags points the shared flag globals at a test's own values and puts
// them back afterwards, so one test's launch can't leak into the next.
func setFlags(t *testing.T, url string, launch bool) {
	t.Helper()
	oldURL, oldLaunch, oldAwait := ankiConnectURL, launchAnki, awaitAnki
	t.Cleanup(func() { ankiConnectURL, launchAnki, awaitAnki = oldURL, oldLaunch, oldAwait })
	ankiConnectURL, launchAnki, awaitAnki = url, launch, nil
}

// Without --launch-anki nothing is started and nothing is waited on: the
// client is handed back exactly as it was before.
func TestStartAnki_DoesNothingUnlessAsked(t *testing.T) {
	setFlags(t, "http://192.168.1.20:8765", false)

	startAnki()
	if awaitAnki != nil {
		t.Error("startAnki() started a launch with --launch-anki off")
	}

	client, err := newAnkiClient()
	if err != nil {
		t.Fatalf("newAnkiClient: %v", err)
	}
	if client.URL != ankiConnectURL {
		t.Errorf("client.URL = %q, want %q", client.URL, ankiConnectURL)
	}
}

func TestNewAnkiClient_SucceedsWhenAnkiIsAlreadyRunning(t *testing.T) {
	setFlags(t, fakeAnkiConnect(t), true)

	startAnki()
	if _, err := newAnkiClient(); err != nil {
		t.Errorf("newAnkiClient: %v", err)
	}
}

// A launch that failed has to reach the user, unlike the gloss model's
// warm-up: nothing downstream recovers from Anki being absent.
func TestNewAnkiClient_ReportsAFailedLaunch(t *testing.T) {
	setFlags(t, "http://192.168.1.20:8765", true)

	startAnki()
	_, err := newAnkiClient()
	if err == nil {
		t.Fatal("newAnkiClient succeeded with an unreachable AnkiConnect")
	}
	if !strings.Contains(err.Error(), "isn't on this machine") {
		t.Errorf("newAnkiClient error = %v, want the launch failure", err)
	}
}

// The launch result is collected once but may be asked for again; a second
// call has to return the same answer rather than block forever.
func TestNewAnkiClient_CanBeCalledTwice(t *testing.T) {
	setFlags(t, fakeAnkiConnect(t), true)

	startAnki()
	if _, err := newAnkiClient(); err != nil {
		t.Fatalf("newAnkiClient: %v", err)
	}
	if _, err := newAnkiClient(); err != nil {
		t.Errorf("second newAnkiClient: %v", err)
	}
}
