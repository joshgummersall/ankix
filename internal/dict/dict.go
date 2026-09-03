// Package dict defines a pluggable interface for hydrating vocab words
// with definitions, so the source (a local LLM, a web API, ...) can be
// swapped without touching sync or TUI logic. Every ankix input source
// (Kindle, YouTube, podcast, web article, local file) looks up words
// through the same Provider, so cards from any of them share the same
// Definition/Lemma card template fields.
package dict

// Provider looks up a definition for a single word.
type Provider interface {
	// Define returns the definition text for word, using usage (the
	// sentence the word appeared in, if any) as context, plus the
	// dictionary/base-form lemma when it differs from definition (e.g.
	// "to realize" for "realized"). Implementations that don't need
	// context may ignore usage, and one with no separate lemma concept
	// always returns "" for it. It returns ("", "", nil) if the word has
	// no entry, and a non-nil error only on an unexpected failure (e.g.
	// the lookup process couldn't run).
	Define(word, usage string) (definition, lemma string, err error)
}
