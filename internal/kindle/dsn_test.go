package kindle

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func makeWordsDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(path, "rwc"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE WORDS(id TEXT, word TEXT, category INT); INSERT INTO WORDS VALUES('es:a','a',0)`); err != nil {
		t.Fatal(err)
	}
}

// A file name is not a URI: "#", "?" and "%" in it must open that file and
// no other.
func TestOpenTreatsPathLiterally(t *testing.T) {
	for _, name := range []string{"my#1.db", "100%25.db", "what?.db", "a b.db"} {
		dir := t.TempDir()
		path := filepath.Join(dir, name)
		makeWordsDB(t, path)

		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open(%q): %v", name, err)
		}
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM WORDS`).Scan(&n); err != nil || n != 1 {
			t.Errorf("Open(%q): rows=%d err=%v", name, n, err)
		}
		db.Close()

		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 {
			t.Errorf("Open(%q) left %d files in the directory, want 1", name, len(entries))
		}
	}
}

func TestOpenReadOnlyCannotBeOverriddenByPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocab.db")
	makeWordsDB(t, path)

	if db, err := Open(path + "?mode=rw&x="); err == nil {
		db.Close()
		t.Error("Open accepted a path carrying its own query string")
	}
}
