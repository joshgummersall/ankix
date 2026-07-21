package tui

import "strings"

// Line is one browsable/selectable unit of content, with an optional
// unselectable gutter label shown to its left (e.g. a timestamp) — blank
// for sources with no time axis.
type Line struct {
	Label string
	Text  string
}

// Document is the full browsable content for one TUI session, regardless of
// source.
type Document struct {
	// SourceID is an opaque per-source identity (video ID, URL, file path);
	// tui never interprets it, only carries it for the source's own
	// BuildNote/PreviewLink closures.
	SourceID string
	Lines    []Line
}

// word is a single word within a Document, tagged with the index of the
// Line it came from. Line boundaries don't align with sentence
// boundaries — a source's segmentation breaks wherever it happened to
// break — so selecting a full sentence means being able to work below line
// granularity.
type word struct {
	Text      string
	LineIndex int
}

// flattenWords splits every line's text into words, in order, each tagged
// with the index of the line it came from.
func flattenWords(lines []Line) []word {
	var words []word
	for i, l := range lines {
		for _, w := range strings.Fields(l.Text) {
			words = append(words, word{Text: w, LineIndex: i})
		}
	}
	return words
}
