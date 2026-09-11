package main

import (
	"sync"

	"github.com/joshgummersall/ankix/internal/anki"
)

// awaitAnki waits for the background launch startAnki kicked off and
// reports how it went. It carries that result from the top of a command's
// run to the point Anki is first used, and is package-level for the same
// reason the flags are: the launch has to start before the source fetch and
// is consumed after it, and threading a channel through every launch*TUI
// signature would say nothing that a single command per process doesn't
// already say. Nil means no launch was asked for.
var awaitAnki func() error

// startAnki starts Anki in the background when --launch-anki is set, so its
// startup overlaps the setup still to come -- fetching subtitles, an
// article, or a vocab.db -- rather than being paid for at the end, once the
// review screen is already open. It returns immediately; newAnkiClient
// collects the result.
//
// Unlike the gloss model's warm-up, that result is not discarded: nothing
// else recovers from Anki being absent, so a failed launch has to reach the
// user. Call this first, next to newDictProvider, for the same reason --
// everything after it is time Anki can spend starting.
func startAnki() {
	if !launchAnki {
		return
	}
	done := make(chan error, 1)
	go func() { done <- anki.New(ankiConnectURL).Start() }()
	// OnceValue so the result survives being asked for more than once: the
	// channel yields it exactly once, and a second caller would block on an
	// empty channel forever.
	awaitAnki = sync.OnceValue(func() error { return <-done })
}

// newAnkiClient returns the AnkiConnect client, waiting first for any
// launch startAnki began. Every command's first Anki call goes through
// here, so the wait lands exactly where Anki is first needed and nowhere
// earlier.
func newAnkiClient() (*anki.Client, error) {
	if awaitAnki != nil {
		if err := awaitAnki(); err != nil {
			return nil, err
		}
	}
	return anki.New(ankiConnectURL), nil
}
