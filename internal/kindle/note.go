package kindle

import (
	"regexp"
	"strings"

	"github.com/joshgummersall/ankix/internal/anki"
)

// ankiModelName is the Anki note type used for synced notes; it must have
// "Front" and "Back" fields (Anki's built-in "Basic" note type does).
const ankiModelName = "Basic"

// FindPhrase returns the byte range of the first case-insensitive
// occurrence of phrase within sentence, or (-1, -1) if it isn't found.
func FindPhrase(sentence, phrase string) (start, end int) {
	if sentence == "" || phrase == "" {
		return -1, -1
	}
	idx := strings.Index(strings.ToLower(sentence), strings.ToLower(phrase))
	if idx == -1 {
		return -1, -1
	}
	return idx, idx + len(phrase)
}

// BuildNote constructs a Basic (Front/Back) note for a headword or phrase
// within sentence. If start is negative, no byte range was found/chosen and
// the sentence (if any) is appended unbolded. definition is expected to
// already be formatted (see FormatDefinition).
func BuildNote(deck string, tags []string, e Entry, sentence string, start, end int, definition string) anki.Note {
	phrase := e.Word
	front := "<h1>" + phrase + "</h1>"
	switch {
	case start >= 0:
		phrase = sentence[start:end]
		front = "<h1>" + phrase + "</h1>" + sentence[:start] + "<b><i>" + phrase + "</i></b>" + sentence[end:]
	case sentence != "":
		front += sentence
	}

	return anki.Note{
		DeckName:  deck,
		ModelName: ankiModelName,
		Fields: map[string]string{
			"Front": front,
			"Back":  definition,
		},
		Tags: tags,
		Options: &anki.NoteOptions{
			AllowDuplicate: false,
			DuplicateScope: "deck",
		},
	}
}

var (
	pronunciationRe    = regexp.MustCompile(`\s*\|[^|]*\|\s*`)
	senseBeforeParenRe = regexp.MustCompile(`\s([1-9]\d?)\s(\()`)
	senseAfterPunctRe  = regexp.MustCompile(`([\.\!\?])\s([1-9]\d?)\s`)
	exampleGapRe       = regexp.MustCompile(`\s*▸\s*`)

	// Recognized part-of-speech labels Apple Dictionary prints right
	// after the headword (and an optional homonym number, e.g. "perro
	// 1 adjective ..."). Longer phrases are listed first so e.g.
	// "transitive verb" matches before the bare "verb" fallback.
	posRe = regexp.MustCompile(`(?i)^(?:[1-9]\s+)?(transitive & intransitive verb|transitive verb|intransitive verb|reflexive verb|phrasal verb|feminine noun|masculine noun|plural noun|verb|noun|adjective|adverb|pronoun|interjection|conjunction|preposition|article)\b`)
)

// FormatDefinition turns Apple Dictionary's flattened, single-line
// definition text into something readable on an Anki card: it drops the
// pronunciation guide and leading headword, sets the part of speech off
// on its own italic line, breaks out numbered senses onto their own
// lines, and renders "▸" example sentences as an indented, italicized
// list.
func FormatDefinition(word, raw string) string {
	def := pronunciationRe.ReplaceAllString(raw, " ")
	def = strings.TrimSpace(def)

	headwordRe := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(word) + `\s*`)
	def = headwordRe.ReplaceAllString(def, "")

	var pos string
	if m := posRe.FindStringSubmatch(def); m != nil {
		pos = m[1]
		def = strings.TrimSpace(def[len(m[0]):])
	}

	parts := exampleGapRe.Split(def, -1)
	main := strings.TrimSpace(parts[0])
	main = senseBeforeParenRe.ReplaceAllString(main, "<br>$1 $2")
	main = senseAfterPunctRe.ReplaceAllString(main, "$1<br>$2 ")
	main = strings.TrimPrefix(main, "<br>")

	var b strings.Builder
	if pos != "" {
		b.WriteString("<i>")
		b.WriteString(pos)
		b.WriteString("</i><br>")
	}
	b.WriteString(main)
	for _, example := range parts[1:] {
		example = strings.TrimSpace(example)
		if example == "" {
			continue
		}
		b.WriteString("<br>▸ <i>")
		b.WriteString(example)
		b.WriteString("</i>")
	}
	return b.String()
}
