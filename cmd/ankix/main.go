// Command ankix generates Anki cards from Kindle vocabulary builder
// highlights, YouTube video transcripts, podcast transcripts, web articles,
// and local files.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/joshgummersall/ankix/ollama/vocab"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
)

// Shared flags used across subcommands, defined once on the root command so
// every subcommand inherits the same names, defaults, and help text.
var (
	deck            string
	ankiConnectURL  string
	ollamaURL       string
	ollamaModel     string
	ollamaKeepAlive string
	noGloss         bool
	launchAnki      bool

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
		// main below is the single place errors are printed; without this
		// cobra prints its own copy first.
		SilenceErrors: true,
		// Cobra validates args before running this, so anything failing
		// from here on is a runtime failure (Ollama unreachable, model not
		// installed, Anki not running), not misuse. Those errors already
		// say what to do and dumping the flag list underneath buries it —
		// but a genuine usage error, caught earlier, still gets usage.
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			return nil
		},
	}
	root.PersistentFlags().StringVar(&deck, "deck", strOr(cfg.Deck, "AnkiX"), "Anki deck name")
	root.PersistentFlags().StringVar(&ankiConnectURL, "ankiconnect-url", strOr(cfg.AnkiConnectURL, "http://localhost:8765"), "AnkiConnect URL")
	root.PersistentFlags().StringVar(&ollamaURL, "ollama-url", strOr(cfg.OllamaURL, "http://localhost:11434"), "Ollama URL")
	root.PersistentFlags().StringVar(&ollamaModel, "ollama-model", strOr(cfg.OllamaModel, "ankix"), "Ollama gloss model -- both the name ankix install builds and the one every other command looks up; a bare name is pinned to this build's Modelfile checksum (e.g. ankix:"+vocab.Checksum()+"), a name with an explicit :tag is used as-is")
	root.PersistentFlags().StringVar(&ollamaKeepAlive, "ollama-keep-alive", strOr(cfg.OllamaKeepAlive, ollama.DefaultKeepAlive), "how long Ollama keeps the gloss model loaded after a lookup -- a duration, seconds, 0 to unload immediately, or a negative value to keep it loaded indefinitely")
	root.PersistentFlags().BoolVar(&noGloss, "no-gloss", cfg.NoGloss, "skip Ollama gloss lookups")
	root.PersistentFlags().BoolVar(&launchAnki, "launch-anki", cfg.LaunchAnki, "start the Anki desktop app if AnkiConnect isn't answering (local AnkiConnect URLs only; macOS and Linux)")

	root.AddCommand(newInstallCmd(cfg))
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
