// Package translate defines a pluggable interface for glossing a word or
// phrase in the source language being studied, so the source (a local LLM,
// a web API, ...) can be swapped without touching the TUI.
package translate

// Provider glosses a single word or phrase.
type Provider interface {
	// Gloss returns a short English gloss for word as used in sentence.
	// Implementations that don't need context may ignore sentence. It
	// returns ("", nil) if no gloss could be produced, and a non-nil error
	// only on an unexpected failure (e.g. the lookup process couldn't run).
	Gloss(word, sentence string) (string, error)
}
