// Package position persists the last-viewed word within a document across
// TUI sessions, keyed by the source's opaque SourceID, so quitting out of a
// long transcript (accidentally or otherwise) doesn't lose your place.
package position

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Position is the cursor's location within a document: Line is the document
// line index, and Word is the cursor's word offset within that line (0 for
// the line's first word), so restoring lands on the exact word rather than
// just the start of the line.
type Position struct {
	Line int `json:"line"`
	Word int `json:"word"`
}

// path returns the location ankix stores per-source reading positions:
// $XDG_CONFIG_HOME/ankix/positions.json (or the OS equivalent).
func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ankix", "positions.json"), nil
}

// Load returns the saved position for sourceID. ok is false if there is no
// saved position (missing file, missing entry, or unreadable file — all
// treated the same: fall back to starting at the top).
func Load(sourceID string) (pos Position, ok bool) {
	p, err := path()
	if err != nil {
		return Position{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Position{}, false
	}
	var positions map[string]Position
	if err := json.Unmarshal(data, &positions); err != nil {
		return Position{}, false
	}
	pos, ok = positions[sourceID]
	return pos, ok
}

// Save records the current position for sourceID, merging with whatever is
// already on disk so browsing one source doesn't clobber another's saved
// position. Errors are the caller's to decide whether to surface — this is
// a best-effort convenience, not something worth failing a session over.
func Save(sourceID string, pos Position) error {
	p, err := path()
	if err != nil {
		return err
	}

	positions := map[string]Position{}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &positions) // corrupt file: start fresh rather than fail
	}
	positions[sourceID] = pos

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(positions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
