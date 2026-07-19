package subtitle

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	tagRe       = regexp.MustCompile(`<[^>]*>`)
	timestampRe = regexp.MustCompile(`^(\d{2}:\d{2}:\d{2}\.\d{3}) --> (\d{2}:\d{2}:\d{2}\.\d{3})`)
)

// ParseVTT parses a WEBVTT file into a Transcript, deduplicating the
// rolling/overlapping cues that yt-dlp's auto-generated captions produce
// (each new cue re-announces part of the previous cue's text, karaoke-style,
// with inline <00:00:01.500><c>word</c> timing tags).
func ParseVTT(path, videoID string) (*Transcript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	raw, err := parseCues(f)
	if err != nil {
		return nil, err
	}

	return &Transcript{VideoID: videoID, Cues: dedupeCues(raw)}, nil
}

func parseCues(f *os.File) ([]Cue, error) {
	var cues []Cue
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		m := timestampRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start, err := parseVTTTime(m[1])
		if err != nil {
			continue
		}
		end, err := parseVTTTime(m[2])
		if err != nil {
			continue
		}

		var textLines []string
		for scanner.Scan() {
			l := scanner.Text()
			if strings.TrimSpace(l) == "" {
				break
			}
			textLines = append(textLines, l)
		}

		text := cleanText(strings.Join(textLines, " "))
		if text == "" {
			continue
		}
		cues = append(cues, Cue{Start: start, End: end, Text: text})
	}
	return cues, scanner.Err()
}

func cleanText(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func parseVTTTime(s string) (time.Duration, error) {
	t, err := time.Parse("15:04:05.000", s)
	if err != nil {
		return 0, err
	}
	return t.Sub(time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)), nil
}

// dedupeCues reconstructs the underlying transcript from yt-dlp's
// auto-generated captions, which display as a rolling sliding window: each
// new cue repeats a tail chunk of the previous cue's words as its own
// prefix, then adds new words past that. A naive per-cue display therefore
// shows the same words several times over. This walks the cues emitting
// only each cue's new words (the words past the longest overlap between
// what's already been accumulated and the start of the current cue), so
// the output is the original continuous transcript split back into
// per-timestamp lines. Manual (non-auto) subtitles have no such overlap,
// so this is a no-op for them.
func dedupeCues(cues []Cue) []Cue {
	var out []Cue
	var accumulated []string
	for _, c := range cues {
		words := strings.Fields(c.Text)
		overlap := overlapLen(accumulated, words)
		newWords := words[overlap:]
		if len(newWords) == 0 {
			// Cue is fully contained in what's already been emitted —
			// just extend the last line's end time.
			if len(out) > 0 {
				out[len(out)-1].End = c.End
			}
			continue
		}
		accumulated = append(accumulated, newWords...)
		out = append(out, Cue{Start: c.Start, End: c.End, Text: strings.Join(newWords, " ")})
	}
	return out
}

// overlapLen returns the length of the longest suffix of accumulated that
// equals a prefix of words.
func overlapLen(accumulated, words []string) int {
	max := len(accumulated)
	if len(words) < max {
		max = len(words)
	}
	for n := max; n > 0; n-- {
		if slicesEqual(accumulated[len(accumulated)-n:], words[:n]) {
			return n
		}
	}
	return 0
}

func slicesEqual(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
