// Command ankix generates Anki cards from Kindle vocabulary builder
// highlights, YouTube video transcripts, web articles, and local files.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "ankix",
		Short: "Generate Anki cards from Kindle vocab, YouTube transcripts, web articles, and local files",
	}
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
