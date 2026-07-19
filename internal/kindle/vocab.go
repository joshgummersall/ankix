// Package kindle reads vocabulary lookups out of a Kindle vocab.db file.
package kindle

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// Entry is a single vocabulary word paired with the sentence it was
// looked up from.
type Entry struct {
	ID        string
	Word      string
	Stem      string
	Lang      string
	Usage     string
	BookTitle string
	Authors   string
	Timestamp int64
}

// Open opens a Kindle vocab.db file read-only.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		return nil, fmt.Errorf("open vocab.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open vocab.db: %w", err)
	}
	return db, nil
}

// OpenRW backs up a Kindle vocab.db file to "<path>.bak" (overwriting any
// existing backup), then opens the original read-write, for marking words
// as mastered.
func OpenRW(path string) (*sql.DB, error) {
	if err := backup(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=rw", path))
	if err != nil {
		return nil, fmt.Errorf("open vocab.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open vocab.db: %w", err)
	}
	return db, nil
}

// backup copies path to "<path>.bak".
func backup(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("back up vocab.db: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(path + ".bak")
	if err != nil {
		return fmt.Errorf("back up vocab.db: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("back up vocab.db: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("back up vocab.db: %w", err)
	}
	return nil
}

// masteredCategory is the WORDS.category value the Kindle Vocabulary
// Builder flashcard app treats as "Mastered", removing the word from the
// review queue. This is a reverse-engineered convention, not documented
// by Amazon.
const masteredCategory = 1

// MarkMastered sets a word's category to mastered so the Kindle
// Vocabulary Builder stops quizzing on it.
func MarkMastered(db *sql.DB, id string) error {
	_, err := db.Exec(`UPDATE WORDS SET category = ? WHERE id = ?`, masteredCategory, id)
	if err != nil {
		return fmt.Errorf("mark %q mastered: %w", id, err)
	}
	return nil
}

// Entries returns lookups for the given language (BCP-47 prefix, e.g. "en"
// or "es"), most recently looked-up word first. The language is matched
// against the dictionary actually used for each lookup (DICT_INFO.langin
// via LOOKUPS.dict_key) rather than WORDS.lang or BOOK_INFO.lang, both of
// which can reflect stale or wrong metadata (e.g. a book's ASIN reporting
// "en" even though it's a Spanish edition looked up with a Spanish
// dictionary). An empty lang returns lookups for all languages. Words
// already marked as Mastered are excluded unless includeMastered is true.
// Only lookups with LOOKUPS.timestamp strictly greater than sinceTimestamp
// are returned; pass 0 to include everything.
func Entries(db *sql.DB, lang string, includeMastered bool, sinceTimestamp int64) ([]Entry, error) {
	query := `
		SELECT w.id, w.word, w.stem, w.lang, l.usage, COALESCE(b.title, ''), COALESCE(b.authors, ''), l.timestamp
		FROM WORDS w
		JOIN LOOKUPS l ON l.word_key = w.id
		LEFT JOIN BOOK_INFO b ON b.id = l.book_key
		LEFT JOIN DICT_INFO d ON d.id = l.dict_key
		WHERE (? = '' OR d.langin = ? OR d.langin LIKE ? || '%')
		AND (? OR w.category != ?)
		AND l.timestamp > ?
		ORDER BY w.timestamp DESC
	`
	rows, err := db.Query(query, lang, lang, lang, includeMastered, masteredCategory, sinceTimestamp)
	if err != nil {
		return nil, fmt.Errorf("query lookups: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Word, &e.Stem, &e.Lang, &e.Usage, &e.BookTitle, &e.Authors, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("scan lookup: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// syncStateID is the single row key used in the SYNC_STATE table.
const syncStateID = "ankindle"

// LastSynced returns the LOOKUPS.timestamp watermark recorded by the most
// recent successful sync, or 0 if ankindle has never synced this vocab.db.
// It works against a read-only db handle; a missing SYNC_STATE table (not
// yet created by SetLastSynced) is treated the same as "never synced".
func LastSynced(db *sql.DB) (int64, error) {
	var ts int64
	err := db.QueryRow(`SELECT timestamp FROM SYNC_STATE WHERE id = ?`, syncStateID).Scan(&ts)
	switch {
	case err == sql.ErrNoRows:
		return 0, nil
	case err != nil && strings.Contains(err.Error(), "no such table"):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("read sync state: %w", err)
	}
	return ts, nil
}

// SetLastSynced records timestamp as the LOOKUPS.timestamp watermark for
// the most recent successful sync, so a future sync can skip lookups
// already processed.
func SetLastSynced(db *sql.DB, timestamp int64) error {
	if err := ensureSyncStateTable(db); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO SYNC_STATE (id, timestamp) VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET timestamp = excluded.timestamp`, syncStateID, timestamp)
	if err != nil {
		return fmt.Errorf("write sync state: %w", err)
	}
	return nil
}

func ensureSyncStateTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS SYNC_STATE (id TEXT PRIMARY KEY NOT NULL, timestamp INTEGER DEFAULT 0)`)
	if err != nil {
		return fmt.Errorf("create sync state table: %w", err)
	}
	return nil
}
