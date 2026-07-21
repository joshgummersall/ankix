package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set via -ldflags at build time (see .goreleaser.yaml).
var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the ankix version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(version)
			return nil
		},
	}
}
