package anki

// SourceTag returns the AnkiX tag identifying which import pipeline
// produced a note (e.g. "Kindle", "YouTube").
func SourceTag(source string) string {
	return "AnkiX::Source::" + source
}
