package kindle

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ImportRecord is one reviewed word in the import log.
type ImportRecord struct {
	ID      string    `json:"id"`   // WORDS.id, e.g. "es:pájaros"
	Word    string    `json:"word"` // for reading the log by eye; ID is the key
	Deck    string    `json:"deck,omitempty"`
	Outcome string    `json:"outcome"` // added, duplicate, skipped, no-definition
	At      time.Time `json:"at"`
}

// ImportLog is ankix's own durable record of which Kindle words have been
// reviewed already.
//
// It exists because vocab.db's own Mastered flag (WORDS.category, see
// MarkMastered) is not durable: the Kindle resets words back to category 0
// on its own — 58 words marked Mastered by one sync were back in the review
// queue eight days later, only one of which had been looked up again — so
// the device's flag alone lets already-imported sentences reappear. The
// Mastered marking is still written, since it's what stops the Kindle's own
// Vocabulary Builder quizzing; this log is what stops ankix re-reviewing.
//
// The file is append-only JSON Lines, one ImportRecord per line, so a run
// that's interrupted keeps everything it had already recorded. Words are
// keyed by WORDS.id alone, not by deck: the log answers "have I reviewed
// this word?", including words deliberately skipped, which belong to no
// deck.
type ImportLog struct {
	path string

	mu  sync.Mutex
	ids map[string]bool
	f   *os.File
}

// DefaultImportLogPath is the import log's durable home, alongside the
// vocab.db backups (see backupRoot) rather than in a cache directory.
func DefaultImportLogPath() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "imported.jsonl"), nil
}

// OpenImportLog opens (creating if needed) the import log at path, reading
// every record already in it. Malformed lines are skipped rather than
// failing the run — a torn write from a killed process must not lock the
// user out of importing.
func OpenImportLog(path string) (*ImportLog, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("open import log: %w", err)
	}

	l := &ImportLog{path: path, ids: map[string]bool{}}

	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("open import log: %w", err)
	}
	if err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for s.Scan() {
			var r ImportRecord
			if err := json.Unmarshal(s.Bytes(), &r); err != nil || r.ID == "" {
				continue
			}
			l.ids[r.ID] = true
		}
		if err := s.Err(); err != nil {
			return nil, fmt.Errorf("read import log: %w", err)
		}
	}

	return l, nil
}

// Path is where the log lives on disk.
func (l *ImportLog) Path() string { return l.path }

// Has reports whether id has been reviewed already. A nil log has nothing.
func (l *ImportLog) Has(id string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ids[id]
}

// Len is how many distinct words the log holds.
func (l *ImportLog) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.ids)
}

// Record appends e to the log. Words with no vocab.db row (manually added
// during review) have no id to key on and are ignored, as is a nil log.
// Re-recording a word already in the log is a no-op, so a re-imported word
// doesn't grow the file every run.
func (l *ImportLog) Record(e Entry, deck, outcome string) error {
	if l == nil || e.ID == "" {
		return nil
	}
	return l.append(ImportRecord{ID: e.ID, Word: e.Word, Deck: deck, Outcome: outcome, At: time.Now()})
}

// RecordIDs appends ids that carry no Entry of their own — used to seed the
// log from words already marked Mastered (see SeedFromMastered).
func (l *ImportLog) RecordIDs(ids []string, outcome string) error {
	if l == nil {
		return nil
	}
	for _, id := range ids {
		if err := l.append(ImportRecord{ID: id, Outcome: outcome, At: time.Now()}); err != nil {
			return err
		}
	}
	return nil
}

func (l *ImportLog) append(r ImportRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if r.ID == "" || l.ids[r.ID] {
		return nil
	}

	if l.f == nil {
		f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("write import log: %w", err)
		}
		l.f = f
	}

	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("write import log: %w", err)
	}
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write import log: %w", err)
	}
	l.ids[r.ID] = true
	return nil
}

// Close releases the log's file handle. A log that was only read never
// opened one.
func (l *ImportLog) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// FilterImported drops every entry already in the log, returning what's
// left along with how many were dropped.
func FilterImported(entries []Entry, l *ImportLog) (kept []Entry, dropped int) {
	if l == nil {
		return entries, 0
	}
	kept = make([]Entry, 0, len(entries))
	for _, e := range entries {
		if l.Has(e.ID) {
			dropped++
			continue
		}
		kept = append(kept, e)
	}
	return kept, dropped
}
