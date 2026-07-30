package podcast

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/joshgummersall/ankix/internal/subtitle"
	"github.com/joshgummersall/ankix/internal/textutil"
)

// FetchTranscript downloads the transcript at item.TranscriptURL and parses
// it into cues, based on item.TranscriptType. VTT and SRT cues carry their
// original timestamps; JSON (podcast-namespace segments) cues do too. HTML
// and plain text have no time axis, so each cue is a paragraph with Start 0.
func FetchTranscript(item *Item) ([]subtitle.Cue, error) {
	resp, err := getWithUserAgent(item.TranscriptURL)
	if err != nil {
		return nil, fmt.Errorf("fetch transcript %s: %w", item.TranscriptURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch transcript %s: unexpected status %s", item.TranscriptURL, resp.Status)
	}

	switch {
	case strings.Contains(item.TranscriptType, "vtt"):
		t, err := subtitle.ParseVTTReader(resp.Body, item.TranscriptURL)
		if err != nil {
			return nil, fmt.Errorf("parse vtt transcript: %w", err)
		}
		return t.Cues, nil
	case strings.Contains(item.TranscriptType, "srt"):
		return parseSRT(resp.Body)
	case strings.Contains(item.TranscriptType, "json"):
		return parseJSONTranscript(resp.Body)
	default:
		return parseHTMLTranscript(resp.Body)
	}
}

var srtTimestampRe = regexp.MustCompile(`^(\d{2}:\d{2}:\d{2}),(\d{3}) --> (\d{2}:\d{2}:\d{2}),(\d{3})`)

func parseSRT(r io.Reader) ([]subtitle.Cue, error) {
	var cues []subtitle.Cue
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		m := srtTimestampRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start, err := parseSRTTime(m[1], m[2])
		if err != nil {
			continue
		}
		end, err := parseSRTTime(m[3], m[4])
		if err != nil {
			continue
		}

		var textLines []string
		for scanner.Scan() {
			l := strings.TrimSpace(scanner.Text())
			if l == "" {
				break
			}
			textLines = append(textLines, l)
		}
		text := strings.TrimSpace(strings.Join(textLines, " "))
		if text == "" {
			continue
		}
		cues = append(cues, subtitle.Cue{Start: start, End: end, Text: text})
	}
	return cues, scanner.Err()
}

func parseSRTTime(hms, ms string) (time.Duration, error) {
	t, err := time.Parse("15:04:05.000", hms+"."+ms)
	if err != nil {
		return 0, err
	}
	return t.Sub(time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)), nil
}

// jsonTranscript is the podcast namespace's JSON transcript schema:
// https://github.com/Podcastindex-org/podcast-namespace/blob/main/transcripts/transcripts.md#json
type jsonTranscript struct {
	Segments []struct {
		StartTime float64 `json:"startTime"`
		EndTime   float64 `json:"endTime"`
		Body      string  `json:"body"`
	} `json:"segments"`
}

func parseJSONTranscript(r io.Reader) ([]subtitle.Cue, error) {
	var t jsonTranscript
	if err := json.NewDecoder(r).Decode(&t); err != nil {
		return nil, fmt.Errorf("decode json transcript: %w", err)
	}

	cues := make([]subtitle.Cue, 0, len(t.Segments))
	for _, s := range t.Segments {
		text := strings.TrimSpace(s.Body)
		if text == "" {
			continue
		}
		cues = append(cues, subtitle.Cue{
			Start: time.Duration(s.StartTime * float64(time.Second)),
			End:   time.Duration(s.EndTime * float64(time.Second)),
			Text:  text,
		})
	}
	return cues, nil
}

var htmlTagRe = regexp.MustCompile(`(?i)</p>|<br\s*/?>|</li>|</h[1-6]>`)
var anyTagRe = regexp.MustCompile(`<[^>]*>`)

func parseHTMLTranscript(r io.Reader) ([]subtitle.Cue, error) {
	buf := new(strings.Builder)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		buf.WriteString(scanner.Text())
		buf.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Turn block-level closing tags into paragraph breaks before stripping
	// all remaining tags, so text/html transcripts don't collapse into one
	// giant run-on line.
	text := htmlTagRe.ReplaceAllString(buf.String(), "\n\n")
	text = anyTagRe.ReplaceAllString(text, "")

	var cues []subtitle.Cue
	for _, p := range textutil.Paragraphs(text) {
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			cues = append(cues, subtitle.Cue{Text: p})
		}
	}
	if len(cues) == 0 {
		return nil, fmt.Errorf("no transcript text found")
	}
	return cues, nil
}
