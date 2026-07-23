package kindle

import (
	"os"
	"path/filepath"
	"testing"
)

// isolateConfigDir redirects os.UserConfigDir() (and thus backupRoot) into a
// fresh temp directory for the duration of the test, so backup logs from
// different tests (and the developer's real machine) never collide.
func isolateConfigDir(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
}

func writeVocabFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListBackupsNoneYet(t *testing.T) {
	isolateConfigDir(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.db")
	writeVocabFile(t, path, "v1")

	backups, err := ListBackups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected no backups, got %d", len(backups))
	}
}

func TestBackupAndList(t *testing.T) {
	isolateConfigDir(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.db")
	writeVocabFile(t, path, "v1")

	if err := backup(path); err != nil {
		t.Fatal(err)
	}
	writeVocabFile(t, path, "v2")
	if err := backup(path); err != nil {
		t.Fatal(err)
	}

	backups, err := ListBackups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}
	if !backups[0].Time.After(backups[1].Time) && backups[0].Time != backups[1].Time {
		t.Fatalf("expected backups newest-first, got %v then %v", backups[0].Time, backups[1].Time)
	}
}

func TestRestoreBackup(t *testing.T) {
	isolateConfigDir(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.db")

	writeVocabFile(t, path, "v1")
	if err := backup(path); err != nil {
		t.Fatal(err)
	}
	writeVocabFile(t, path, "v2")
	if err := backup(path); err != nil {
		t.Fatal(err)
	}
	writeVocabFile(t, path, "v3 (corrupted)")

	// Backups are newest-first: index 1 is the "v2" snapshot, index 2 is "v1".
	if err := RestoreBackup(path, 2); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1" {
		t.Fatalf("expected restored content %q, got %q", "v1", got)
	}

	// Restoring must itself have snapshotted the pre-restore ("v3
	// (corrupted)") state, so it's undoable too.
	backups, err := ListBackups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 3 {
		t.Fatalf("expected 3 backups after restore, got %d", len(backups))
	}
	preRestore, err := os.ReadFile(backups[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(preRestore) != "v3 (corrupted)" {
		t.Fatalf("expected pre-restore backup to hold %q, got %q", "v3 (corrupted)", preRestore)
	}
}

func TestRestoreBackupIndexOutOfRange(t *testing.T) {
	isolateConfigDir(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.db")
	writeVocabFile(t, path, "v1")

	if err := RestoreBackup(path, 1); err == nil {
		t.Fatal("expected an error restoring from an empty backup log")
	}
}

func TestSourceDirNamespacesBySourcePath(t *testing.T) {
	isolateConfigDir(t)
	dirA, dirB := t.TempDir(), t.TempDir()
	pathA := filepath.Join(dirA, "vocab.db")
	pathB := filepath.Join(dirB, "vocab.db")
	writeVocabFile(t, pathA, "a")
	writeVocabFile(t, pathB, "b")

	if err := backup(pathA); err != nil {
		t.Fatal(err)
	}

	backupsA, err := ListBackups(pathA)
	if err != nil {
		t.Fatal(err)
	}
	if len(backupsA) != 1 {
		t.Fatalf("expected 1 backup for pathA, got %d", len(backupsA))
	}

	backupsB, err := ListBackups(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if len(backupsB) != 0 {
		t.Fatalf("expected 0 backups for distinct pathB, got %d", len(backupsB))
	}
}
