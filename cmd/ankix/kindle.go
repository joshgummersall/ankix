package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/kindle"
	"github.com/joshgummersall/ankix/internal/tui"
)

type syncOptions struct {
	dbPath   string
	lang     string
	tags     []string
	dryRun   bool
	limit    int
	headless bool
	eject    bool
}

func newKindleCmd(cfg config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kindle",
		Short: "Manage Kindle vocabulary builder words",
	}
	cmd.AddCommand(newKindleVocabCmd(cfg))
	return cmd
}

func newKindleVocabCmd(cfg config) *cobra.Command {
	o := &syncOptions{}

	cmd := &cobra.Command{
		Use:   "vocab <vocab.db>",
		Short: "Read vocab.db and sync new words to Anki via AnkiConnect",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.dbPath = args[0]
			return runSync(o)
		},
	}

	cmd.Flags().StringVar(&o.lang, "lang", strOr(cfg.Kindle.Lang, strOr(cfg.Lang, "en")), "language prefix to filter words by, matched against the dictionary used for each lookup (e.g. en, es); empty for all")
	cmd.Flags().StringSliceVar(&o.tags, "tag", []string{anki.SourceTag("Kindle")}, "tags to apply to new notes")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "print what would be synced without writing to Anki (only applies with --headless; the interactive review lets you inspect/skip each word before it's added)")
	cmd.Flags().IntVar(&o.limit, "limit", 0, "limit to the N most recently looked-up words (0 for no limit)")
	cmd.Flags().BoolVar(&o.headless, "headless", false, "sync every word straight through without the interactive review TUI (e.g. for cron/automation)")
	cmd.Flags().BoolVar(&o.eject, "eject", false, "eject the Kindle's volume after a successful sync (macOS only)")

	cmd.AddCommand(newKindleVocabDbCmd())

	return cmd
}

func runSync(o *syncOptions) (err error) {
	model, err := resolveModel(ollamaURL, ollamaModel)
	if err != nil {
		return err
	}
	provider := ollama.New(ollamaURL, model)

	// A headless dry run never writes anything, including Mastered markers,
	// so a read-only handle is enough; every other run (including every
	// interactive review, which marks words Mastered as it goes) needs a
	// read-write handle.
	openDB := kindle.Open
	if !o.headless || !o.dryRun {
		openDB = kindle.OpenRW
	}
	db, err := openDB(o.dbPath)
	if err != nil {
		return err
	}
	// Registered before db.Close so it runs after (defers are LIFO), and
	// only ejects once the volume's file is safely closed and the sync
	// itself succeeded.
	defer func() {
		if err == nil && o.eject {
			err = ejectVolume(o.dbPath)
		}
	}()
	defer db.Close()

	entries, err := kindle.Entries(db, o.lang, false)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("no new vocab entries found for the given language")
		return nil
	}

	seen, words := dedupeEntries(entries)
	if o.limit > 0 && o.limit < len(words) {
		words = words[:o.limit]
	}

	client := anki.New(ankiConnectURL)

	if !o.headless {
		return runKindleReview(o, db, client, provider, seen, words)
	}

	if !o.dryRun {
		if err := client.CreateDeck(deck); err != nil {
			return err
		}
	}

	var added, skippedExisting, skippedNoDefinition int
	for _, key := range words {
		e := seen[key]

		exists, err := noteExists(client, deck, key)
		if err != nil {
			return err
		}
		if exists {
			skippedExisting++
			if !o.dryRun {
				if err := kindle.MarkMastered(db, e.ID); err != nil {
					return err
				}
			}
			continue
		}

		definition, lemma, err := provider.Define(e.Word, e.Usage)
		if err != nil {
			return fmt.Errorf("define %q: %w", e.Word, err)
		}
		if definition == "" {
			skippedNoDefinition++
			fmt.Printf("skip %q: no definition found\n", e.Word)
			if !o.dryRun {
				if err := kindle.MarkMastered(db, e.ID); err != nil {
					return err
				}
			}
			continue
		}

		start, end := kindle.FindPhrase(e.Usage, e.Word)
		note := kindle.BuildNote(cardTemplates, deck, o.tags, e, e.Usage, start, end, kindle.FormatDefinition(e.Word, definition), lemma)

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
		if err := kindle.MarkMastered(db, e.ID); err != nil {
			return err
		}
		fmt.Printf("added %q\n", e.Word)
		added++
	}

	fmt.Printf("\ndone: %d added, %d already in Anki, %d skipped (no definition)\n", added, skippedExisting, skippedNoDefinition)
	return nil
}

// ejectVolume ejects the removable volume containing path via diskutil.
// Kindle mounts as a USB volume under /Volumes on macOS; it errors on any
// other OS or any path not under a mounted volume.
func ejectVolume(path string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("--eject is only supported on macOS")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	const volumesDir = "/Volumes/"
	if !strings.HasPrefix(abs, volumesDir) {
		return fmt.Errorf("%s is not on a mounted volume under %s, skipping --eject", abs, volumesDir)
	}
	volume := volumesDir + strings.SplitN(strings.TrimPrefix(abs, volumesDir), "/", 2)[0]

	out, err := exec.Command("diskutil", "eject", volume).CombinedOutput()
	if err != nil {
		return fmt.Errorf("eject %s: %w: %s", volume, err, strings.TrimSpace(string(out)))
	}
	fmt.Printf("ejected %s\n", volume)
	return nil
}

func noteExists(client *anki.Client, deck, phrase string) (bool, error) {
	query := fmt.Sprintf(`deck:%q Front:%q`, deck, "<b>"+phrase+"</b>*")
	ids, err := client.FindNotes(query)
	if err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}

// dedupeEntries collapses entries (most-recently-looked-up first) down to
// one per word, keeping the most recent lookup's Entry.
func dedupeEntries(entries []kindle.Entry) (seen map[string]kindle.Entry, words []string) {
	seen = make(map[string]kindle.Entry, len(entries))
	for _, e := range entries {
		key := strings.ToLower(e.Word)
		if _, ok := seen[key]; !ok {
			seen[key] = e
			words = append(words, key)
		}
	}
	return seen, words
}

// runReview opens the interactive TUI so each word can be reviewed in its
// usage sentence and the highlighted word/phrase adjusted (e.g. to capture a
// reflexive form or multi-word phrase Kindle's own lookup can't select)
// before syncing to Anki.
func runKindleReview(o *syncOptions, db *sql.DB, client *anki.Client, provider *ollama.Provider, seen map[string]kindle.Entry, words []string) error {
	entries := make([]kindle.Entry, len(words))
	for i, key := range words {
		entries[i] = seen[key]
	}

	m := tui.NewKindleReview(tui.KindleConfig{
		Entries:    entries,
		Deck:       deck,
		Tags:       o.tags,
		AnkiClient: client,
		Dict:       provider,
		DB:         db,
		Templates:  cardTemplates,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
