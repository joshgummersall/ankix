package main

import "github.com/spf13/cobra"

type webFlags struct{}

func newWebCmd() *cobra.Command {
	f := &webFlags{}

	cmd := &cobra.Command{
		Use:   "web",
		Short: "Browse a web article and generate Anki cards",
	}

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
