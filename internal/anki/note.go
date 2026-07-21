package anki

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// WordSelection is a word or phrase marked within a sentence, given as a
// byte range [Start,End) into that sentence, plus its English gloss (may
// be empty if lookup is disabled or still pending).
type WordSelection struct {
	Start, End int
	Gloss      string
}

// BuildYouTubeNote constructs a Basic (Front/Back) note for a single marked
// word or phrase within sentence. Front is an <h1> headword followed by
// the sentence with that word bolded/italicized — the headword is what
// lets the same sentence generate several distinct cards (one per marked
// word) without Anki's duplicate check (which compares the first field)
// treating them as the same note. Back is the English gloss plus a link
// to roughly where the word is spoken in the source video.
func BuildYouTubeNote(deck, videoTitle, videoID string, cueStart time.Duration, sentence string, sel WordSelection) Note {
	word := sentence[sel.Start:sel.End]

	front := "<h1>" + word + "</h1>" +
		sentence[:sel.Start] + "<b><i>" + word + "</i></b>" + sentence[sel.End:]

	var back strings.Builder
	if sel.Gloss != "" {
		back.WriteString(sel.Gloss)
		back.WriteString("<br><br>")
	}
	fmt.Fprintf(&back, "%s (%s)", videoTitle, formatTimestamp(cueStart))
	if link := VideoLink(videoID, cueStart); link != "" {
		fmt.Fprintf(&back, ` — <a href="%s">watch</a>`, link)
	}

	return Note{
		DeckName:  deck,
		ModelName: "Basic",
		Fields: map[string]string{
			"Front": front,
			"Back":  back.String(),
		},
		Tags: []string{SourceTag("YouTube"), "AnkiX::Video::" + videoID, WordTag(word)},
		Options: &NoteOptions{
			AllowDuplicate: false,
			DuplicateScope: "deck",
		},
	}
}

// BuildNote is BuildYouTubeNote's counterpart for sources with no time
// axis (web articles, local files): no timestamp or deep link, just a
// title and an optional plain link to the source.
func BuildNote(deck, title, url, sourceTag, sentence string, sel WordSelection) Note {
	word := sentence[sel.Start:sel.End]

	front := "<h1>" + word + "</h1>" +
		sentence[:sel.Start] + "<b><i>" + word + "</i></b>" + sentence[sel.End:]

	var back strings.Builder
	if sel.Gloss != "" {
		back.WriteString(sel.Gloss)
		back.WriteString("<br><br>")
	}
	back.WriteString(title)
	if url != "" {
		fmt.Fprintf(&back, ` — <a href="%s">read</a>`, url)
	}

	return Note{
		DeckName:  deck,
		ModelName: "Basic",
		Fields: map[string]string{
			"Front": front,
			"Back":  back.String(),
		},
		Tags: []string{SourceTag(sourceTag), WordTag(word)},
		Options: &NoteOptions{
			AllowDuplicate: false,
			DuplicateScope: "deck",
		},
	}
}

func formatTimestamp(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	return fmt.Sprintf("%d:%02d", m, s)
}

// youtubeIDRe matches a bare 11-character YouTube video ID. Cards built
// from a locally-loaded transcript file (ankix youtube review) carry the file
// path as videoID instead, which fails this check, so no (broken) link is
// added for those.
var youtubeIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// VideoLink returns a YouTube deep link that jumps to roughly cueStart, or
// "" if videoID isn't a real YouTube video ID. It backs up one second from
// the caption timestamp, since auto-caption timing tends to lag slightly
// behind when a word is actually spoken.
func VideoLink(videoID string, cueStart time.Duration) string {
	if !youtubeIDRe.MatchString(videoID) {
		return ""
	}
	seconds := int(cueStart.Round(time.Second).Seconds()) - 1
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("https://youtu.be/%s?t=%d", videoID, seconds)
}
