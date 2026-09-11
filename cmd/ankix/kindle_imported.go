package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joshgummersall/ankix/internal/kindle"
)

func newKindleVocabImportedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "imported",
		Short: "Inspect and edit ankix's durable log of already-reviewed words",
		Long: `Inspect and edit ankix's durable log of already-reviewed words.

Sync normally skips a word once it has been reviewed, using this log rather
than vocab.db's own Mastered flag: the Kindle resets that flag on its own,
which puts already-imported sentences back in the review queue.

The log is append-only JSON Lines keyed by vocab.db's word id, so it can be
read — and pruned, to put words back in the queue — with ordinary tools.`,
	}
	cmd.AddCommand(newKindleVocabImportedStatusCmd())
	cmd.AddCommand(newKindleVocabImportedMarkAllCmd())
	return cmd
}

func newKindleVocabImportedStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print the import log's path and size",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			log, err := openLog()
			if err != nil {
				return err
			}
			defer log.Close()
			fmt.Printf("%s\n%d word(s) recorded as imported\n", log.Path(), log.Len())
			return nil
		},
	}
}

func newKindleVocabImportedMarkAllCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "mark-all <vocab.db>",
		Short: "Record every word in vocab.db as already imported",
		Long: `Record every word in vocab.db as already imported.

This retires the whole file in one go: no word currently in vocab.db will be
offered for review again, whether or not it ever reached Anki. Use it to
declare a clean slate after importing outside ankix, or after the Kindle has
reset Mastered flags on words you know you already have.

Words looked up after this runs are unaffected — they aren't in the file yet.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := kindle.Open(args[0])
			if err != nil {
				return err
			}
			defer db.Close()

			words, err := kindle.AllWords(db)
			if err != nil {
				return err
			}

			log, err := openLog()
			if err != nil {
				return err
			}
			defer log.Close()

			var unknown int
			for _, w := range words {
				if !log.Has(w.ID) {
					unknown++
				}
			}
			if dryRun {
				fmt.Printf("would record %d of %d word(s) in %s (%d already logged): %s\n",
					unknown, len(words), args[0], len(words)-unknown, log.Path())
				return nil
			}

			for _, w := range words {
				if err := log.Record(w, "", "mark-all"); err != nil {
					return err
				}
			}
			fmt.Printf("recorded %d new word(s) of %d in %s; %d total in %s\n",
				unknown, len(words), args[0], log.Len(), log.Path())
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be recorded without writing to the log")

	return cmd
}

func openLog() (*kindle.ImportLog, error) {
	path, err := kindle.DefaultImportLogPath()
	if err != nil {
		return nil, err
	}
	return kindle.OpenImportLog(path)
}
