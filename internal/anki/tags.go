package anki

import "strings"

// SourceTag returns the AnkiX tag identifying which import pipeline
// produced a note (e.g. "Kindle", "YouTube").
func SourceTag(source string) string {
	return "AnkiX::Source::" + source
}

// WordTag returns the AnkiX tag for the headword or phrase a note tests,
// lowercased so different casings of the same word collapse to one tag.
func WordTag(word string) string {
	return "AnkiX::Word::" + strings.ToLower(word)
}
