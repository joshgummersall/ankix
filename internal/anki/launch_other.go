//go:build !darwin && !linux

package anki

import (
	"fmt"
	"runtime"
)

// launchApp reports that there's no reliable way to start Anki on this OS.
// Anki runs here, but ankix has no dependable way to find it -- there's no
// `open -a` equivalent and no command on PATH to count on -- so the user
// starts it, the way they would without --launch-anki.
func launchApp() error {
	return fmt.Errorf("starting Anki automatically isn't supported on %s; start Anki yourself and re-run", runtime.GOOS)
}
