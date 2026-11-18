package position

import (
	"os"
	"runtime"
	"testing"
)

func TestSaveIsPrivateAndRoundTrips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)

	if err := Save("a", Position{Line: 1, Word: 2}); err != nil {
		t.Fatal(err)
	}
	if err := Save("b", Position{Line: 3}); err != nil {
		t.Fatal(err)
	}
	if pos, ok := Load("a"); !ok || pos != (Position{Line: 1, Word: 2}) {
		t.Errorf("Load(a) = %+v, %v", pos, ok)
	}

	if runtime.GOOS == "windows" {
		return
	}
	p, _ := path()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("positions.json mode = %o, want 600", perm)
	}
}
