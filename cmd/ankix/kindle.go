package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/kindle"
)

// ankiModelName is the Anki note type used for synced notes; it must have
// "Front" and "Back" fields (Anki's built-in "Basic" note type does).
const ankiModelName = "Basic"

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
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "print what would be synced without writing to Anki")
	cmd.Flags().BoolVar(&o.mastered, "mastered", false, "filter out words already marked Mastered in vocab.db (default: include them), and mark words that end up in Anki as Mastered (opens --db read-write)")
	cmd.Flags().BoolVar(&o.full, "full", false, "ignore the sync watermark stored in vocab.db and consider every lookup again")
	cmd.Flags().StringVar(&o.model, "model", "ankindle", "Ollama model to use")
	cmd.Flags().IntVar(&o.limit, "limit", 0, "limit to the N most recently looked-up words (0 for no limit)")
	cmd.MarkFlagRequired("db")

	return cmd
}

func runSync(o *syncOptions) error {
	provider := ollama.New(ollamaURL, o.model)

	// A dry run never writes anything, including the sync watermark, so a
	// read-only handle is enough; every other run persists the watermark
	// (and, with --mastered, word categories) so it needs read-write.
	openDB := kindle.Open
	if !o.dryRun {
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

	// Kindle can log multiple lookups for the same word; keep the most
	// recent (entries is already ordered most-recent-first), but track the
	// newest lookup timestamp across all of a word's lookups so the sync
	// watermark accounts for every lookup considered this run.
	seen := make(map[string]kindle.Entry, len(entries))
	maxLookupTimestamp := make(map[string]int64, len(entries))
	var words []string
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
	if o.limit > 0 && o.limit < len(words) {
		words = words[:o.limit]
	}

	client := anki.New(o.ankiURL)
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

		note := buildNote(o.deck, o.tags, e, definition)

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

// wordTag returns a per-word tag used to detect notes already synced for
// a given word, independent of how the Front/Back fields are rendered.
func wordTag(word string) string {
	return "kindle-word::" + strings.ToLower(word)
}

func noteExists(client *anki.Client, deck, word string) (bool, error) {
	query := fmt.Sprintf(`deck:%q tag:%q`, deck, wordTag(word))
	ids, err := client.FindNotes(query)
	if err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}

func buildNote(deck string, tags []string, e kindle.Entry, definition string) anki.Note {
	front := "<h1>" + e.Word + "</h1>"
	if sentence := boldWordInSentence(e.Usage, e.Word); sentence != "" {
		front += sentence
	}

	allTags := append(append([]string{}, tags...), wordTag(e.Word))

	back := formatDefinition(e.Word, definition)

	return anki.Note{
		DeckName:  deck,
		ModelName: ankiModelName,
		Fields: map[string]string{
			"Front": front,
			"Back":  back,
		},
		Tags: allTags,
		Options: &anki.NoteOptions{
			AllowDuplicate: false,
			DuplicateScope: "deck",
		},
	}
}

var (
	pronunciationRe    = regexp.MustCompile(`\s*\|[^|]*\|\s*`)
	senseBeforeParenRe = regexp.MustCompile(`\s([1-9]\d?)\s(\()`)
	senseAfterPunctRe  = regexp.MustCompile(`([\.\!\?])\s([1-9]\d?)\s`)
	exampleGapRe       = regexp.MustCompile(`\s*▸\s*`)

	// Recognized part-of-speech labels Apple Dictionary prints right
	// after the headword (and an optional homonym number, e.g. "perro
	// 1 adjective ..."). Longer phrases are listed first so e.g.
	// "transitive verb" matches before the bare "verb" fallback.
	posRe = regexp.MustCompile(`(?i)^(?:[1-9]\s+)?(transitive & intransitive verb|transitive verb|intransitive verb|reflexive verb|phrasal verb|feminine noun|masculine noun|plural noun|verb|noun|adjective|adverb|pronoun|interjection|conjunction|preposition|article)\b`)
)

// formatDefinition turns Apple Dictionary's flattened, single-line
// definition text into something readable on an Anki card: it drops the
// pronunciation guide and leading headword, sets the part of speech off
// on its own italic line, breaks out numbered senses onto their own
// lines, and renders "▸" example sentences as an indented, italicized
// list.
func formatDefinition(word, raw string) string {
	def := pronunciationRe.ReplaceAllString(raw, " ")
	def = strings.TrimSpace(def)

	headwordRe := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(word) + `\s*`)
	def = headwordRe.ReplaceAllString(def, "")

	var pos string
	if m := posRe.FindStringSubmatch(def); m != nil {
		pos = m[1]
		def = strings.TrimSpace(def[len(m[0]):])
	}

	parts := exampleGapRe.Split(def, -1)
	main := strings.TrimSpace(parts[0])
	main = senseBeforeParenRe.ReplaceAllString(main, "<br>$1 $2")
	main = senseAfterPunctRe.ReplaceAllString(main, "$1<br>$2 ")
	main = strings.TrimPrefix(main, "<br>")

	var b strings.Builder
	if pos != "" {
		b.WriteString("<i>")
		b.WriteString(pos)
		b.WriteString("</i><br>")
	}
	b.WriteString(main)
	for _, example := range parts[1:] {
		example = strings.TrimSpace(example)
		if example == "" {
			continue
		}
		b.WriteString("<br>▸ <i>")
		b.WriteString(example)
		b.WriteString("</i>")
	}
	return b.String()
}

// boldWordInSentence wraps the first case-insensitive occurrence of word
// in <b><i> tags within sentence.
func boldWordInSentence(sentence, word string) string {
	if sentence == "" {
		return ""
	}
	idx := strings.Index(strings.ToLower(sentence), strings.ToLower(word))
	if idx == -1 {
		return sentence
	}
	return sentence[:idx] + "<b><i>" + sentence[idx:idx+len(word)] + "</i></b>" + sentence[idx+len(word):]
}
