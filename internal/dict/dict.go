// Package dict defines a pluggable interface for hydrating vocab words
// with definitions, so the source (Apple Dictionary, a web API, a local
// file, ...) can be swapped without touching sync logic.
package dict

// Provider looks up a definition for a single word.
type Provider interface {
	// Define returns the definition text for word, using usage (the
	// sentence the word appeared in, if any) as context. Implementations
	// that don't need context may ignore usage. It returns ("", nil) if
	// the word has no entry, and a non-nil error only on an unexpected
	// failure (e.g. the lookup process couldn't run).
	Define(word, usage string) (string, error)
}
