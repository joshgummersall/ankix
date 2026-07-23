package main

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/joshgummersall/ankix/internal/kindle"
)

func newKindleVocabDbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Manage timestamped vocab.db backups",
		Long: `Manage timestamped vocab.db backups.

Backup history is keyed by the vocab.db path you pass to list/restore, not
by the device itself — vocab.db has no serial number or account ID to key
off. If the same Kindle ever mounts at a different path, or you copy
vocab.db somewhere new, list/restore will treat it as an unrelated file and
start a fresh history. Always point list/restore at the same path you've
been syncing a given device with.`,
	}
	cmd.AddCommand(newKindleVocabDbListCmd())
	cmd.AddCommand(newKindleVocabDbRestoreCmd())
	return cmd
}

func newKindleVocabDbListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <vocab.db>",
		Short: "List timestamped backups for a vocab.db file, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKindleDbList(args[0])
		},
	}
}

func newKindleVocabDbRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <vocab.db> <index>",
		Short: "Restore a backup over vocab.db (index from 'db list'; backs up the current file first)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid index %q: %w", args[1], err)
			}
			return runKindleDbRestore(args[0], idx)
		},
	}
}

func runKindleDbList(path string) error {
	backups, err := kindle.ListBackups(path)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		fmt.Println("no backups found for", path)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "IDX\tTIMESTAMP\tSIZE")
	for i, b := range backups {
		fmt.Fprintf(w, "%d\t%s\t%s\n", i+1, b.Time.Format("2006-01-02 15:04:05"), formatSize(b.Size))
	}
	return w.Flush()
}

func runKindleDbRestore(path string, index int) error {
	if err := kindle.RestoreBackup(path, index); err != nil {
		return err
	}
	fmt.Printf("restored backup %d over %s\n", index, path)
	return nil
}

func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
