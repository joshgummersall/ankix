package anki

import (
	"fmt"
	"os/exec"
	"syscall"
)

// launchApp starts the Anki desktop app from PATH. Setpgid puts it in its
// own process group so a Ctrl-C in the terminal running ankix doesn't also
// kill Anki, and Release drops the handle: Anki is the user's app and
// outlives this command rather than being reaped with it.
func launchApp() error {
	cmd := exec.Command("anki")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting Anki: %w (is the `anki` command on PATH?)", err)
	}
	return cmd.Process.Release()
}
