// Package kindle reads vocabulary lookups out of a Kindle vocab.db file.
package kindle

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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

// OpenRW backs up a Kindle vocab.db file into a timestamped log (see
// backup), then opens the original read-write, for marking words as
// mastered.
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

// configRoot is ankix's own durable state directory. It's the user's config
// directory (not a cache directory) since what lives here — vocab.db
// backups, the import log — is meant to survive OS cache cleanup.
func configRoot() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "ankix"), nil
}

// backupRoot is the durable directory every vocab.db's backup log lives
// under, namespaced per source file by sourceDir.
func backupRoot() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "kindle-backups"), nil
}

// sourceDir returns the backup directory for path, keyed by a hash of its
// absolute form so relative and absolute spellings of the same file (and
// distinct vocab.db files, e.g. from different Kindles) resolve to stable,
// separate backup histories.
//
// This is a path-based identity, not a device identity: vocab.db carries no
// serial number, account ID, or other durable identifier for the Kindle it
// came from (WORDS.profileid is typically empty; METADATA/VERSION have
// nothing device-specific either — checked directly against a real
// device's file). So if the same Kindle mounts under a different path next
// time (different volume name, different USB port behavior, copied instead
// of synced in place, etc.), or if you copy vocab.db to a new location,
// ListBackups/RestoreBackup will treat it as an unrelated file and start a
// fresh backup history rather than continuing the old one. Always back up
// and restore against the same path you've been using for a given device to
// keep its history intact.
func sourceDir(path string) (string, error) {
	root, err := backupRoot()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	h := fnv.New64a()
	h.Write([]byte(abs))
	return filepath.Join(root, fmt.Sprintf("%x", h.Sum64())), nil
}

// backup copies path into its timestamped backup log (sourceDir(path)),
// named by the current time in nanoseconds so entries sort and stay unique
// without a collision check.
func backup(path string) error {
	dir, err := sourceDir(path)
	if err != nil {
		return fmt.Errorf("back up vocab.db: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("back up vocab.db: %w", err)
	}

	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("back up vocab.db: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(filepath.Join(dir, fmt.Sprintf("%d.db", time.Now().UnixNano())))
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

// Backup is one entry in a vocab.db's timestamped backup log.
type Backup struct {
	Time time.Time
	Path string // absolute path to the backup file on disk
	Size int64
}

// ListBackups returns path's backups, newest first. A path with no backups
// yet returns (nil, nil), not an error.
func ListBackups(path string) ([]Backup, error) {
	dir, err := sourceDir(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	var backups []Backup
	for _, e := range entries {
		name := e.Name()
		nanos, ok := strings.CutSuffix(name, ".db")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(nanos, 10, 64)
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("list backups: %w", err)
		}
		backups = append(backups, Backup{
			Time: time.Unix(0, n),
			Path: filepath.Join(dir, name),
			Size: info.Size(),
		})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Time.After(backups[j].Time) })
	return backups, nil
}

// RestoreBackup overwrites path with the backup at index (1-based, matching
// ListBackups' newest-first order), after first snapshotting path's current
// contents via backup — so a restore is itself always undoable.
func RestoreBackup(path string, index int) error {
	backups, err := ListBackups(path)
	if err != nil {
		return err
	}
	if index < 1 || index > len(backups) {
		return fmt.Errorf("restore vocab.db: index %d out of range (1-%d)", index, len(backups))
	}

	if err := backup(path); err != nil {
		return fmt.Errorf("restore vocab.db: %w", err)
	}

	src, err := os.Open(backups[index-1].Path)
	if err != nil {
		return fmt.Errorf("restore vocab.db: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("restore vocab.db: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("restore vocab.db: %w", err)
	}
	return dst.Close()
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

// MasteredIDs returns the id of every word in db marked Mastered.
func MasteredIDs(db *sql.DB) ([]string, error) {
	return wordIDs(db, `SELECT id FROM WORDS WHERE category = ?`, masteredCategory)
}

// AllWords returns every word in db, whatever its category, as Entries
// carrying only an id and the word itself — enough to record in the import
// log, not enough to build a card from.
func AllWords(db *sql.DB) ([]Entry, error) {
	rows, err := db.Query(`SELECT id, word FROM WORDS`)
	if err != nil {
		return nil, fmt.Errorf("query words: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Word); err != nil {
			return nil, fmt.Errorf("scan word: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func wordIDs(db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query words: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan word: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SeedImportLog backfills l from the words already marked Mastered in db
// and in every backup of it, so a log created after words were already
// synced doesn't re-review them. The backups matter as much as the live
// file: the Kindle resets words out of Mastered on its own (see ImportLog),
// and a backup taken right after a sync is the only surviving record that
// those words were ever synced. Returns how many ids it added.
//
// Seeding treats any Mastered word as reviewed, including words mastered on
// the Kindle itself rather than by ankix — a word the user has retired from
// the device's own quizzing is one they're done with either way.
func SeedImportLog(l *ImportLog, path string, db *sql.DB) (int, error) {
	if l == nil {
		return 0, nil
	}
	before := l.Len()

	ids, err := MasteredIDs(db)
	if err != nil {
		return 0, err
	}
	if err := l.RecordIDs(ids, "seed-mastered"); err != nil {
		return 0, err
	}

	backups, err := ListBackups(path)
	if err != nil {
		return 0, err
	}
	for _, b := range backups {
		old, err := Open(b.Path)
		if err != nil {
			continue // an unreadable backup is not worth failing a sync over
		}
		ids, err := MasteredIDs(old)
		old.Close()
		if err != nil {
			continue
		}
		if err := l.RecordIDs(ids, "seed-backup"); err != nil {
			return 0, err
		}
	}

	return l.Len() - before, nil
}
