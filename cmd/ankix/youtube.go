package main

import (
	"os"

	"github.com/spf13/cobra"
)

type youtubeFlags struct {
	deck        string
	ankiConnect string
	ollamaURL   string
	ollamaModel string
	subLang     string
	cacheDir    string
	noGloss     bool
}

func newYouTubeCmd() *cobra.Command {
	f := &youtubeFlags{}

	cmd := &cobra.Command{
		Use:   "youtube",
		Short: "Browse YouTube subtitles and generate Anki cards",
	}

	defaultCacheDir, err := os.UserCacheDir()
	if err != nil {
		defaultCacheDir = os.TempDir()
	}
	defaultCacheDir += "/ankix/youtube"

	cmd.PersistentFlags().StringVar(&f.deck, "deck", "Spanish::YouTube", "Anki deck name")
	cmd.PersistentFlags().StringVar(&f.ankiConnect, "ankiconnect-url", "http://localhost:8765", "AnkiConnect URL")
	cmd.PersistentFlags().StringVar(&f.ollamaURL, "ollama-url", "http://localhost:11434", "Ollama URL")
	cmd.PersistentFlags().StringVar(&f.ollamaModel, "ollama-model", "ankitube", "Ollama gloss model name")
	cmd.PersistentFlags().StringVar(&f.subLang, "sub-lang", "es", "subtitle language code")
	cmd.PersistentFlags().StringVar(&f.cacheDir, "cache-dir", defaultCacheDir, "subtitle cache directory")
	cmd.PersistentFlags().BoolVar(&f.noGloss, "no-gloss", false, "skip Ollama gloss lookups")

	cmd.AddCommand(newYouTubeFetchCmd(f))
	cmd.AddCommand(newYouTubeReviewCmd(f))

	return cmd
}

func newYouTubeFetchCmd(f *youtubeFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "fetch <youtube-url>",
		Short: "Fetch subtitles for a YouTube video and browse them in the TUI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFetch(f, args[0])
		},
	}
}

func newYouTubeReviewCmd(f *youtubeFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "review <transcript-file>",
		Short: "Open an existing VTT transcript file directly in the TUI, skipping yt-dlp",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(f, args[0])
		},
	}
}
