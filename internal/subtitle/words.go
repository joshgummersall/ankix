package subtitle

import "strings"

// Word is a single word from the transcript, tagged with the index of the
// cue (display line) it came from. Cue boundaries don't align with
// sentence boundaries — auto-caption sliding windows break wherever the
// window happened to end — so selecting a full sentence means being able
// to work below line granularity.
type Word struct {
	Text     string
	CueIndex int
}

// FlattenWords splits every cue's text into words, in order, each tagged
// with the index of the cue it came from.
func FlattenWords(cues []Cue) []Word {
	var words []Word
	for i, c := range cues {
		for _, w := range strings.Fields(c.Text) {
			words = append(words, Word{Text: w, CueIndex: i})
		}
	}
	return words
}
