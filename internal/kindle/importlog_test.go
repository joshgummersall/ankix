package kindle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportLogRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "imported.jsonl")

	l, err := OpenImportLog(path)
	if err != nil {
		t.Fatalf("OpenImportLog: %v", err)
	}
	if l.Len() != 0 {
		t.Fatalf("new log has %d ids, want 0", l.Len())
	}

	if err := l.Record(Entry{ID: "es:pájaros", Word: "pájaros"}, "Spanish::AnkiX", "added"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// A word with no vocab.db row is not recordable, and re-recording a
	// known word must not grow the log.
	if err := l.Record(Entry{Word: "manual"}, "Spanish::AnkiX", "added"); err != nil {
		t.Fatalf("Record manual: %v", err)
	}
	if err := l.Record(Entry{ID: "es:pájaros", Word: "pájaros"}, "Spanish::AnkiX", "duplicate"); err != nil {
		t.Fatalf("Record repeat: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got := string(data); len(got) == 0 || got[len(got)-1] != '\n' {
		t.Fatalf("log is not newline-terminated JSON lines: %q", got)
	}

	reopened, err := OpenImportLog(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !reopened.Has("es:pájaros") {
		t.Error("recorded word missing after reopen")
	}
	if reopened.Has("es:otra") {
		t.Error("unrecorded word reported as imported")
	}
	if reopened.Len() != 1 {
		t.Errorf("reopened log has %d ids, want 1", reopened.Len())
	}
}

func TestOpenImportLogSkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "imported.jsonl")
	contents := `{"id":"es:uno","word":"uno","outcome":"added"}
not json at all
{"id":"","word":"no id"}
{"id":"es:dos","word":"dos","outcome":"skipped"}
{"id":"es:tres","word":"tr` // torn final write, as a killed process would leave
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	l, err := OpenImportLog(path)
	if err != nil {
		t.Fatalf("OpenImportLog: %v", err)
	}
	if !l.Has("es:uno") || !l.Has("es:dos") {
		t.Error("well-formed records were dropped")
	}
	if l.Has("es:tres") {
		t.Error("torn record was accepted")
	}
	if l.Len() != 2 {
		t.Errorf("log has %d ids, want 2", l.Len())
	}
}

func TestFilterImported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "imported.jsonl")
	l, err := OpenImportLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Record(Entry{ID: "es:dos"}, "deck", "added"); err != nil {
		t.Fatal(err)
	}

	entries := []Entry{{ID: "es:uno"}, {ID: "es:dos"}, {ID: "es:tres"}}
	kept, dropped := FilterImported(entries, l)
	if dropped != 1 {
		t.Errorf("dropped %d, want 1", dropped)
	}
	if len(kept) != 2 || kept[0].ID != "es:uno" || kept[1].ID != "es:tres" {
		t.Errorf("kept %+v, want uno and tres", kept)
	}

	// A nil log filters nothing, so a failure to open one can't silently
	// hide every word.
	kept, dropped = FilterImported(entries, nil)
	if dropped != 0 || len(kept) != 3 {
		t.Errorf("nil log dropped %d of %d entries", dropped, len(entries))
	}
}
