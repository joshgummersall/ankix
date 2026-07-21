// Package kindle reads vocabulary lookups out of a Kindle vocab.db file.
package kindle

import (
	"database/sql"
	"fmt"
	"io"
	"os"

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
// "en" even though it's a foreign-language edition looked up with a
// foreign-language dictionary). An empty lang returns lookups for all
// languages. Words
// already marked as Mastered are excluded unless includeMastered is true.
func Entries(db *sql.DB, lang string, includeMastered bool) ([]Entry, error) {
	query := `
		SELECT w.id, w.word, w.stem, w.lang, l.usage, COALESCE(b.title, ''), COALESCE(b.authors, ''), l.timestamp
		FROM WORDS w
		JOIN LOOKUPS l ON l.word_key = w.id
		LEFT JOIN BOOK_INFO b ON b.id = l.book_key
		LEFT JOIN DICT_INFO d ON d.id = l.dict_key
		WHERE (? = '' OR d.langin = ? OR d.langin LIKE ? || '%')
		AND (? OR w.category != ?)
		ORDER BY w.timestamp DESC
	`
	rows, err := db.Query(query, lang, lang, lang, includeMastered, masteredCategory)
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
