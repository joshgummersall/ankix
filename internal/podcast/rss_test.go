package podcast

import "testing"

func TestParseAppleURL(t *testing.T) {
	url := "https://podcasts.apple.com/us/podcast/some-show/id1776743104?i=1000778667267"
	podcastID, episodeID, err := ParseAppleURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if podcastID != "1776743104" {
		t.Errorf("podcastID = %q, want 1776743104", podcastID)
	}
	if episodeID != "1000778667267" {
		t.Errorf("episodeID = %q, want 1000778667267", episodeID)
	}
}

func TestParseAppleURLMissingIDs(t *testing.T) {
	if _, _, err := ParseAppleURL("https://example.com/not-a-podcast-url"); err == nil {
		t.Fatal("expected error for url with no ids")
	}
	if _, _, err := ParseAppleURL("https://podcasts.apple.com/us/podcast/some-show/id1776743104"); err == nil {
		t.Fatal("expected error for url with no episode id")
	}
}

func TestMatchItemByGUID(t *testing.T) {
	items := []rssItem{
		{Title: "Ep 1", GUID: "guid-1", Enclosure: rssEnclosure{URL: "https://host/1.mp3"}},
		{Title: "Ep 2", GUID: "guid-2", Enclosure: rssEnclosure{URL: "https://host/2.mp3"}},
	}
	got := matchItem(items, Episode{GUID: "guid-2", URL: "https://host/nope.mp3"})
	if got == nil || got.Title != "Ep 2" {
		t.Fatalf("got %+v, want Ep 2", got)
	}
}

func TestMatchItemByEnclosureFallback(t *testing.T) {
	items := []rssItem{
		{Title: "Ep 1", GUID: "guid-1", Enclosure: rssEnclosure{URL: "https://host/1.mp3"}},
		{Title: "Ep 2", GUID: "guid-2", Enclosure: rssEnclosure{URL: "https://host/2.mp3"}},
	}
	got := matchItem(items, Episode{GUID: "no-match-guid", URL: "https://host/2.mp3"})
	if got == nil || got.Title != "Ep 2" {
		t.Fatalf("got %+v, want Ep 2 via enclosure fallback", got)
	}
}

func TestBestTranscriptPrefersVTT(t *testing.T) {
	transcripts := []rssTranscript{
		{URL: "https://host/t.html", Type: "text/html"},
		{URL: "https://host/t.json", Type: "application/json"},
		{URL: "https://host/t.vtt", Type: "text/vtt"},
	}
	url, typ := bestTranscript(transcripts)
	if url != "https://host/t.vtt" || typ != "text/vtt" {
		t.Errorf("got (%q, %q), want vtt", url, typ)
	}
}

func TestBestTranscriptFallsBackToFirst(t *testing.T) {
	transcripts := []rssTranscript{
		{URL: "https://host/t.weird", Type: "application/x-weird"},
	}
	url, typ := bestTranscript(transcripts)
	if url != "https://host/t.weird" || typ != "application/x-weird" {
		t.Errorf("got (%q, %q), want fallback to first entry", url, typ)
	}
}
