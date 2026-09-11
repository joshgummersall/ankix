package anki

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReachable_TrueWhenAnkiConnectAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":6,"error":null}`))
	}))
	defer srv.Close()

	if !New(srv.URL).Reachable() {
		t.Error("Reachable() = false against a server answering the version call")
	}
}

// Reachable is the "is Anki up?" question asked before anything is launched,
// so a closed port has to come back false rather than as an error the caller
// has to interpret.
func TestReachable_FalseWhenNothingIsListening(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	if New(url).Reachable() {
		t.Error("Reachable() = true against a closed port")
	}
}

func TestLocal(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"http://localhost:8765", true},
		{"http://127.0.0.1:8765", true},
		{"http://[::1]:8765", true},
		{"http://anki.local:8765", false},
		{"http://192.168.1.20:8765", false},
		{":// not a url", false},
	}
	for _, c := range cases {
		if got := New(c.url).Local(); got != c.want {
			t.Errorf("New(%q).Local() = %v, want %v", c.url, got, c.want)
		}
	}
}

// Starting the app on this machine can't make someone else's AnkiConnect
// answer, so Start says so rather than opening a local Anki the user never
// asked to point at.
func TestStart_RefusesToLaunchForARemoteURL(t *testing.T) {
	err := New("http://192.168.1.20:8765").Start()
	if err == nil {
		t.Fatal("Start() succeeded against an unreachable remote AnkiConnect")
	}
	if !strings.Contains(err.Error(), "isn't on this machine") {
		t.Errorf("Start() error = %v, want it to explain the URL is remote", err)
	}
}

// The common case: Anki is already running, and Start is a no-op that never
// reaches the launcher.
func TestStart_NoOpWhenAnkiConnectAlreadyAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":6,"error":null}`))
	}))
	defer srv.Close()

	if err := New(srv.URL).Start(); err != nil {
		t.Errorf("Start() = %v, want nil when AnkiConnect is already up", err)
	}
}
