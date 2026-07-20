package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/kindle"
	"github.com/joshgummersall/ankix/internal/tui"
)

// ollamaURL is the default Ollama endpoint used to write definitions.
const ollamaURL = "http://localhost:11434"

type syncOptions struct {
	dbPath   string
	lang     string
	deck     string
	ankiURL  string
	tags     []string
	dryRun   bool
	mastered bool
	full     bool
	model    string
	limit    int
	headless bool
}

func newKindleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kindle",
		Short: "Manage Kindle vocabulary builder words",
	}
	cmd.AddCommand(newKindleVocabCmd())
	return cmd
}

func newKindleVocabCmd() *cobra.Command {
	o := &syncOptions{}

	cmd := &cobra.Command{
		Use:   "vocab",
		Short: "Read vocab.db and sync new words to Anki via AnkiConnect",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(o)
		},
	}

	cmd.Flags().StringVar(&o.dbPath, "db", "", "path to Kindle vocab.db (required)")
	cmd.Flags().StringVar(&o.lang, "lang", "en", "language prefix to filter words by, matched against the dictionary used for each lookup (e.g. en, es); empty for all")
	cmd.Flags().StringVar(&o.deck, "deck", "Kindle Vocab", "Anki deck name to sync into")
	cmd.Flags().StringVar(&o.ankiURL, "ankiconnect-url", "http://localhost:8765", "AnkiConnect endpoint")
	cmd.Flags().StringSliceVar(&o.tags, "tag", []string{"kindle"}, "tags to apply to new notes")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "print what would be synced without writing to Anki (only applies with --headless; the interactive review lets you inspect/skip each word before it's added)")
	cmd.Flags().BoolVar(&o.mastered, "mastered", false, "filter out words already marked Mastered in vocab.db (default: include them), and mark words that end up in Anki as Mastered (opens --db read-write)")
	cmd.Flags().BoolVar(&o.full, "full", false, "ignore the sync watermark stored in vocab.db and consider every lookup again")
	cmd.Flags().StringVar(&o.model, "model", "ankindle", "Ollama model to use")
	cmd.Flags().IntVar(&o.limit, "limit", 0, "limit to the N most recently looked-up words (0 for no limit)")
	cmd.Flags().BoolVar(&o.headless, "headless", false, "sync every word straight through without the interactive review TUI (e.g. for cron/automation)")
	cmd.MarkFlagRequired("db")

	return cmd
}

func runSync(o *syncOptions) error {
	provider := ollama.New(ollamaURL, o.model)

	// A headless dry run never writes anything, including the sync
	// watermark, so a read-only handle is enough; every other run (including
	// every interactive review, which always persists progress as you go)
	// needs a read-write handle.
	openDB := kindle.Open
	if !o.headless || !o.dryRun {
		openDB = kindle.OpenRW
	}
	db, err := openDB(o.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	var since int64
	if !o.full {
		since, err = kindle.LastSynced(db)
		if err != nil {
			return err
		}
	}

	entries, err := kindle.Entries(db, o.lang, !o.mastered, since)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("no new vocab entries found for the given language")
		return nil
	}

	seen, maxLookupTimestamp, words := dedupeEntries(entries)
	if o.limit > 0 && o.limit < len(words) {
		words = words[:o.limit]
	}

	client := anki.New(o.ankiURL)

	if !o.headless {
		return runKindleReview(o, db, client, provider, seen, maxLookupTimestamp, words)
	}

	if !o.dryRun {
		if err := client.CreateDeck(o.deck); err != nil {
			return err
		}
	}

	var added, skippedExisting, skippedNoDefinition int
	for _, key := range words {
		e := seen[key]

		exists, err := noteExists(client, o.deck, key)
		if err != nil {
			return err
		}
		if exists {
			skippedExisting++
			if o.mastered && !o.dryRun {
				if err := kindle.MarkMastered(db, e.ID); err != nil {
					return err
				}
			}
			continue
		}

		definition, err := provider.Define(e.Word, e.Usage)
		if err != nil {
			return fmt.Errorf("define %q: %w", e.Word, err)
		}
		if definition == "" {
			skippedNoDefinition++
			fmt.Printf("skip %q: no definition found\n", e.Word)
			continue
		}

		start, end := kindle.FindPhrase(e.Usage, e.Word)
		note := kindle.BuildNote(o.deck, o.tags, e, e.Usage, start, end, kindle.FormatDefinition(e.Word, definition))

		if o.dryRun {
			fmt.Printf("would add %q\n  front: %s\n  back:  %s\n", e.Word, note.Fields["Front"], note.Fields["Back"])
			added++
			continue
		}

		if _, err := client.AddNote(note); err != nil {
			if errors.Is(err, anki.ErrDuplicate) {
				skippedExisting++
				fmt.Printf("skip %q: already exists in Anki (duplicate front field)\n", e.Word)
				continue
			}
			return fmt.Errorf("add note %q: %w", e.Word, err)
		}
		if o.mastered {
			if err := kindle.MarkMastered(db, e.ID); err != nil {
				return err
			}
		}
		fmt.Printf("added %q\n", e.Word)
		added++
	}

	if !o.dryRun {
		// Every processed word was considered, whether added, already in
		// Anki, or skipped for lacking a definition, so it's safe to move
		// the watermark up to the newest lookup among them.
		var maxTimestamp int64
		for _, key := range words {
			if ts := maxLookupTimestamp[key]; ts > maxTimestamp {
				maxTimestamp = ts
			}
		}
		if err := kindle.SetLastSynced(db, maxTimestamp); err != nil {
			return err
		}
	}

	fmt.Printf("\ndone: %d added, %d already in Anki, %d skipped (no definition)\n", added, skippedExisting, skippedNoDefinition)
	return nil
}

func noteExists(client *anki.Client, deck, phrase string) (bool, error) {
	query := fmt.Sprintf(`deck:%q tag:%q`, deck, kindle.WordTag(phrase))
	ids, err := client.FindNotes(query)
	if err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}

// dedupeEntries collapses entries (most-recently-looked-up first) down to
// one per word, keeping the most recent lookup's Entry but tracking the
// newest lookup timestamp across every lookup of that word, so the sync
// watermark can account for all of them even though only one Entry survives.
func dedupeEntries(entries []kindle.Entry) (seen map[string]kindle.Entry, maxLookupTimestamp map[string]int64, words []string) {
	seen = make(map[string]kindle.Entry, len(entries))
	maxLookupTimestamp = make(map[string]int64, len(entries))
	for _, e := range entries {
		key := strings.ToLower(e.Word)
		if _, ok := seen[key]; !ok {
			seen[key] = e
			words = append(words, key)
		}
		if e.Timestamp > maxLookupTimestamp[key] {
			maxLookupTimestamp[key] = e.Timestamp
		}
	}
	return seen, maxLookupTimestamp, words
}

// runReview opens the interactive TUI so each word can be reviewed in its
// usage sentence and the highlighted word/phrase adjusted (e.g. to capture a
// reflexive form or multi-word phrase Kindle's own lookup can't select)
// before syncing to Anki.
func runKindleReview(o *syncOptions, db *sql.DB, client *anki.Client, provider *ollama.Provider, seen map[string]kindle.Entry, maxLookupTimestamp map[string]int64, words []string) error {
	entries := make([]kindle.Entry, len(words))
	for i, key := range words {
		entries[i] = seen[key]
	}

	m := tui.NewKindleReview(tui.KindleConfig{
		Entries:            entries,
		MaxLookupTimestamp: maxLookupTimestamp,
		Deck:               o.deck,
		Tags:               o.tags,
		AnkiClient:         client,
		Dict:               provider,
		DB:                 db,
		Mastered:           o.mastered,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
