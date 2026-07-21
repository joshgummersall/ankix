// Package textutil holds small text-segmentation helpers shared across
// sources.
package textutil

import "strings"

// Paragraphs splits text on blank lines into paragraphs, trimming
// whitespace and dropping empty entries.
func Paragraphs(text string) []string {
	var paragraphs []string
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	return paragraphs
}
