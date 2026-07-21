package main

import "github.com/spf13/cobra"

type fileFlags struct{}

func newFileCmd() *cobra.Command {
	f := &fileFlags{}

	cmd := &cobra.Command{
		Use:   "file",
		Short: "Browse a local text or markdown file and generate Anki cards",
	}

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
