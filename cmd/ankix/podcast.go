package main

import "github.com/spf13/cobra"

func newPodcastCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "podcast",
		Short: "Browse podcast transcripts and generate Anki cards",
	}

	cmd.AddCommand(newPodcastAppleCmd())

	return cmd
}

func newPodcastAppleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apple",
		Short: "Fetch podcast transcripts via Apple Podcasts URLs",
	}

	cmd.AddCommand(newPodcastAppleFetchCmd())

	return cmd
}

func newPodcastAppleFetchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch <apple-podcasts-episode-url>",
		Short: "Fetch an episode's transcript and browse it in the TUI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPodcastAppleFetch(args[0])
		},
	}
}
