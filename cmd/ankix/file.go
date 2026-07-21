package main

import "github.com/spf13/cobra"

type fileFlags struct {
	deck        string
	ankiConnect string
	ollamaURL   string
	ollamaModel string
	noGloss     bool
}

func newFileCmd() *cobra.Command {
	f := &fileFlags{}

	cmd := &cobra.Command{
		Use:   "file",
		Short: "Browse a local text or markdown file and generate Anki cards",
	}

	cmd.PersistentFlags().StringVar(&f.deck, "deck", "AnkiX", "Anki deck name")
	cmd.PersistentFlags().StringVar(&f.ankiConnect, "ankiconnect-url", "http://localhost:8765", "AnkiConnect URL")
	cmd.PersistentFlags().StringVar(&f.ollamaURL, "ollama-url", "http://localhost:11434", "Ollama URL")
	cmd.PersistentFlags().StringVar(&f.ollamaModel, "ollama-model", "ankix", "Ollama gloss model name")
	cmd.PersistentFlags().BoolVar(&f.noGloss, "no-gloss", false, "skip Ollama gloss lookups")

	cmd.AddCommand(newFileOpenCmd(f))

	return cmd
}

func newFileOpenCmd(f *fileFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "open <path>",
		Short: "Open a local text or markdown file and browse it in the TUI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFileOpen(f, args[0])
		},
	}
}
