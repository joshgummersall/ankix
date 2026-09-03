// Command ankix generates Anki cards from Kindle vocabulary builder
// highlights, YouTube video transcripts, podcast transcripts, web articles,
// and local files.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/joshgummersall/ankix/internal/anki"
)

// Shared flags used across subcommands, defined once on the root command so
// every subcommand inherits the same names, defaults, and help text.
var (
	deck           string
	ankiConnectURL string
	ollamaURL      string
	ollamaModel    string
	noGloss        bool

	// cardTemplates renders every note's Front/Back fields; compiled once
	// in main() from the config file's [card] section (or the built-in
	// defaults if unset).
	cardTemplates *anki.Templates
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

	cardTemplates, err = anki.NewTemplates(cfg.Card.Front, cfg.Card.Back)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: card template:", err)
		os.Exit(1)
	}

	root := &cobra.Command{
		Use:   "ankix",
		Short: "Generate Anki cards from Kindle vocab, YouTube transcripts, podcast transcripts, web articles, and local files",
	}
	root.PersistentFlags().StringVar(&deck, "deck", strOr(cfg.Deck, "AnkiX"), "Anki deck name")
	root.PersistentFlags().StringVar(&ankiConnectURL, "ankiconnect-url", strOr(cfg.AnkiConnectURL, "http://localhost:8765"), "AnkiConnect URL")
	root.PersistentFlags().StringVar(&ollamaURL, "ollama-url", strOr(cfg.OllamaURL, "http://localhost:11434"), "Ollama URL")
	root.PersistentFlags().StringVar(&ollamaModel, "ollama-model", strOr(cfg.OllamaModel, "ankix"), "Ollama gloss model name")
	root.PersistentFlags().BoolVar(&noGloss, "no-gloss", cfg.NoGloss, "skip Ollama gloss lookups")

	root.AddCommand(newInstallCmd())
	root.AddCommand(newKindleCmd(cfg))
	root.AddCommand(newYouTubeCmd(cfg))
	root.AddCommand(newPodcastCmd())
	root.AddCommand(newWebCmd())
	root.AddCommand(newFileCmd())
	root.AddCommand(newVersionCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
