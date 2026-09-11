package anki

import (
	"fmt"
	"os/exec"
	"strings"
)

// launchApp starts the Anki desktop app. -g leaves it in the background:
// what the user is looking at is ankix's own review screen, and Anki only
// has to be running, not in front of it. -a takes the app by name, so a
// relocated Anki.app is still found the way a Dock click would find it.
func launchApp() error {
	out, err := exec.Command("open", "-g", "-a", "Anki").CombinedOutput()
	if err != nil {
		return fmt.Errorf("starting Anki: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
