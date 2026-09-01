// Package position persists the last-viewed line for a document across TUI
// sessions, keyed by the source's opaque SourceID, so quitting out of a long
// transcript (accidentally or otherwise) doesn't lose your place.
package position

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// path returns the location ankix stores per-source reading positions:
// $XDG_CONFIG_HOME/ankix/positions.json (or the OS equivalent).
func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ankix", "positions.json"), nil
}

// Load returns the saved line index for sourceID. ok is false if there is
// no saved position (missing file, missing entry, or unreadable file — all
// treated the same: fall back to starting at the top).
func Load(sourceID string) (line int, ok bool) {
	p, err := path()
	if err != nil {
		return 0, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return 0, false
	}
	var positions map[string]int
	if err := json.Unmarshal(data, &positions); err != nil {
		return 0, false
	}
	line, ok = positions[sourceID]
	return line, ok
}

// Save records the current line index for sourceID, merging with whatever
// is already on disk so browsing one source doesn't clobber another's saved
// position. Errors are the caller's to decide whether to surface — this is
// a best-effort convenience, not something worth failing a session over.
func Save(sourceID string, line int) error {
	p, err := path()
	if err != nil {
		return err
	}

	positions := map[string]int{}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &positions) // corrupt file: start fresh rather than fail
	}
	positions[sourceID] = line

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(positions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
