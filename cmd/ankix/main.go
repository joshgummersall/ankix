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

// strOr returns cfgVal if it's set, otherwise fallback. Used to let a config
// file value override a flag's built-in default.
func strOr(cfgVal, fallback string) string {
	if cfgVal != "" {
		return cfgVal
	}
	return fallback
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: reading config:", err)
		os.Exit(1)
	}

	root := &cobra.Command{
		Use:   "ankix",
		Short: "Generate Anki cards from Kindle vocab, YouTube transcripts, web articles, and local files",
	}
	root.PersistentFlags().StringVar(&deck, "deck", strOr(cfg.Deck, "AnkiX"), "Anki deck name")
	root.PersistentFlags().StringVar(&ankiConnectURL, "ankiconnect-url", strOr(cfg.AnkiConnectURL, "http://localhost:8765"), "AnkiConnect URL")
	root.PersistentFlags().StringVar(&ollamaURL, "ollama-url", strOr(cfg.OllamaURL, "http://localhost:11434"), "Ollama URL")
	root.PersistentFlags().StringVar(&ollamaModel, "ollama-model", strOr(cfg.OllamaModel, "ankix"), "Ollama gloss model name")
	root.PersistentFlags().BoolVar(&noGloss, "no-gloss", cfg.NoGloss, "skip Ollama gloss lookups")

	root.AddCommand(newInstallCmd())
	root.AddCommand(newKindleCmd(cfg))
	root.AddCommand(newYouTubeCmd(cfg))
	root.AddCommand(newWebCmd())
	root.AddCommand(newFileCmd())
	root.AddCommand(newVersionCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
