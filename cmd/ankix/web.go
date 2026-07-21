package main

import "github.com/spf13/cobra"

type webFlags struct {
	deck        string
	ankiConnect string
	ollamaURL   string
	ollamaModel string
	noGloss     bool
}

func newWebCmd() *cobra.Command {
	f := &webFlags{}

	cmd := &cobra.Command{
		Use:   "web",
		Short: "Browse a web article and generate Anki cards",
	}

	cmd.PersistentFlags().StringVar(&f.deck, "deck", "AnkiX", "Anki deck name")
	cmd.PersistentFlags().StringVar(&f.ankiConnect, "ankiconnect-url", "http://localhost:8765", "AnkiConnect URL")
	cmd.PersistentFlags().StringVar(&f.ollamaURL, "ollama-url", "http://localhost:11434", "Ollama URL")
	cmd.PersistentFlags().StringVar(&f.ollamaModel, "ollama-model", "ankix", "Ollama gloss model name")
	cmd.PersistentFlags().BoolVar(&f.noGloss, "no-gloss", false, "skip Ollama gloss lookups")

	cmd.AddCommand(newWebFetchCmd(f))

	return cmd
}

func newWebFetchCmd(f *webFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "fetch <url>",
		Short: "Fetch a web article and browse it in the TUI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWebFetch(f, args[0])
		},
	}
}
