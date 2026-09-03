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

// Refiner is an optional Provider capability: re-deriving a definition from
// a user's free-form correction of a previous answer ("drop the adjective",
// "it's a verb here"). Not every source can take a correction — a plain
// dictionary lookup has nothing to re-ask — so it's a separate interface,
// and callers type-assert for it and offer no refinement when it's absent.
type Refiner interface {
	// Refine re-answers word as used in usage, given the definition and
	// lemma already returned for it and an instruction describing what's
	// wrong with them. lemma follows Define's convention on the way in and
	// out: "" when it doesn't differ from the definition, so a refined
	// answer is shaped exactly like a fresh one and refining a refinement
	// round-trips.
	Refine(word, usage, definition, lemma, instruction string) (newDefinition, newLemma string, err error)
}
