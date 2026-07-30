package podcast

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title     string       `xml:"title"`
	GUID      string       `xml:"guid"`
	Enclosure rssEnclosure `xml:"enclosure"`
	// Unqualified (no namespace prefix in the tag), so this matches a
	// <transcript> child by local name regardless of which namespace URI
	// declares it — podcast hosts are inconsistent about the podcast
	// namespace's URI (http vs https, trailing slashes, stale pre-1.0
	// URIs), and no other namespace realistically defines this element on
	// an RSS item.
	Transcripts []rssTranscript `xml:"transcript"`
}

type rssEnclosure struct {
	URL string `xml:"url,attr"`
}

type rssTranscript struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

// transcriptTypePriority ranks transcript formats by how easy they are to
// render as plain text in a terminal, preferring text/vtt and
// application/srt (already timestamped and simple to strip) over JSON
// (needs schema-specific parsing) and HTML (needs tag-stripping).
var transcriptTypePriority = []string{"text/vtt", "application/srt", "application/json", "text/html", "text/plain"}

// Item is the matched RSS <item> for a looked-up episode, with the best
// available transcript reference.
type Item struct {
	Title          string
	AudioURL       string
	TranscriptURL  string
	TranscriptType string
}

// FindItem fetches feedURL and returns the <item> matching ep by guid (or,
// failing that, by enclosure/audio URL — guids sometimes get reformatted
// between iTunes and the raw feed, so enclosure URL is the more reliable
// key), along with its best transcript reference.
func FindItem(feedURL string, ep Episode) (*Item, error) {
	resp, err := getWithUserAgent(feedURL)
	if err != nil {
		return nil, fmt.Errorf("fetch feed %s: %w", feedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch feed %s: unexpected status %s", feedURL, resp.Status)
	}

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parse feed %s: %w", feedURL, err)
	}

	item := matchItem(feed.Channel.Items, ep)
	if item == nil {
		return nil, fmt.Errorf("no item in feed matching episode guid %q or url %q", ep.GUID, ep.URL)
	}

	transcriptURL, transcriptType := bestTranscript(item.Transcripts)
	if transcriptURL == "" {
		return nil, fmt.Errorf("no transcript published in this feed for %q", item.Title)
	}

	return &Item{
		Title:          item.Title,
		AudioURL:       item.Enclosure.URL,
		TranscriptURL:  transcriptURL,
		TranscriptType: transcriptType,
	}, nil
}

func matchItem(items []rssItem, ep Episode) *rssItem {
	for i := range items {
		if ep.GUID != "" && strings.TrimSpace(items[i].GUID) == strings.TrimSpace(ep.GUID) {
			return &items[i]
		}
	}
	for i := range items {
		if ep.URL != "" && items[i].Enclosure.URL == ep.URL {
			return &items[i]
		}
	}
	return nil
}

func bestTranscript(transcripts []rssTranscript) (url, typ string) {
	for _, wantType := range transcriptTypePriority {
		for _, t := range transcripts {
			if t.Type == wantType {
				return t.URL, t.Type
			}
		}
	}
	if len(transcripts) > 0 {
		return transcripts[0].URL, transcripts[0].Type
	}
	return "", ""
}
