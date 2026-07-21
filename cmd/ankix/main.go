// Command ankix generates Anki cards from Kindle vocabulary builder
// highlights, YouTube video transcripts, web articles, and local files.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Shared flags used across subcommands, defined once on the root command so
// every subcommand inherits the same names, defaults, and help text.
var (
	deck           string
	ankiConnectURL string
	ollamaURL      string
	ollamaModel    string
	noGloss        bool
)

func main() {
	root := &cobra.Command{
		Use:   "ankix",
		Short: "Generate Anki cards from Kindle vocab, YouTube transcripts, web articles, and local files",
	}
	root.PersistentFlags().StringVar(&deck, "deck", "AnkiX", "Anki deck name")
	root.PersistentFlags().StringVar(&ankiConnectURL, "ankiconnect-url", "http://localhost:8765", "AnkiConnect URL")
	root.PersistentFlags().StringVar(&ollamaURL, "ollama-url", "http://localhost:11434", "Ollama URL")
	root.PersistentFlags().StringVar(&ollamaModel, "ollama-model", "ankix", "Ollama gloss model name")
	root.PersistentFlags().BoolVar(&noGloss, "no-gloss", false, "skip Ollama gloss lookups")

	root.AddCommand(newInstallCmd())
	root.AddCommand(newKindleCmd())
	root.AddCommand(newYouTubeCmd())
	root.AddCommand(newWebCmd())
	root.AddCommand(newFileCmd())
	root.AddCommand(newVersionCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
