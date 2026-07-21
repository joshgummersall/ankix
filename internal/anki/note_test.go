package anki

import (
	"strings"
	"testing"
	"time"
)

func TestBuildWordNote_BoldsWordInSentence(t *testing.T) {
	cases := []struct {
		name         string
		sentence     string
		sel          WordSelection
		wantFrontSub string
	}{
		{
			name:         "word at start",
			sentence:     "casa grande",
			sel:          WordSelection{Start: 0, End: 4},
			wantFrontSub: "<h1>casa</h1><b><i>casa</i></b> grande",
		},
		{
			name:         "word at end",
			sentence:     "casa grande",
			sel:          WordSelection{Start: 5, End: 11},
			wantFrontSub: "<h1>grande</h1>casa <b><i>grande</i></b>",
		},
		{
			name:     "punctuation adjacent word",
			sentence: "¿qué casa?",
			sel: WordSelection{
				Start: strings.Index("¿qué casa?", "casa"),
				End:   strings.Index("¿qué casa?", "casa") + len("casa"),
			},
			wantFrontSub: "¿qué <b><i>casa</i></b>?",
		},
		{
			name:     "repeated word only bolds the selected occurrence",
			sentence: "la casa y la casa vieja",
			sel: WordSelection{
				Start: strings.LastIndex("la casa y la casa vieja", "casa"),
				End:   strings.LastIndex("la casa y la casa vieja", "casa") + len("casa"),
			},
			wantFrontSub: "la casa y la <b><i>casa</i></b> vieja",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := BuildYouTubeNote("Deck", "Title", "vid1", 5*time.Second, c.sentence, c.sel)
			if got := n.Fields["Front"]; !strings.Contains(got, c.wantFrontSub) {
				t.Errorf("Front = %q, want it to contain %q", got, c.wantFrontSub)
			}
			if n.ModelName != "Basic" {
				t.Errorf("ModelName = %q, want Basic", n.ModelName)
			}
		})
	}
}

func TestBuildWordNote_HeaderLetsSameSentenceYieldDistinctCards(t *testing.T) {
	sentence := "la casa vieja"
	n1 := BuildYouTubeNote("Deck", "Title", "vid1", 0, sentence, WordSelection{Start: 3, End: 7})
	n2 := BuildYouTubeNote("Deck", "Title", "vid1", 0, sentence, WordSelection{Start: 8, End: 13})

	if n1.Fields["Front"] == n2.Fields["Front"] {
		t.Errorf("expected different Front fields (different headwords) for different marked words, got identical: %q", n1.Fields["Front"])
	}
	if !strings.HasPrefix(n1.Fields["Front"], "<h1>casa</h1>") {
		t.Errorf("Front = %q, want it to start with the headword", n1.Fields["Front"])
	}
	if !strings.HasPrefix(n2.Fields["Front"], "<h1>vieja</h1>") {
		t.Errorf("Front = %q, want it to start with the headword", n2.Fields["Front"])
	}
}

func TestBuildWordNote_BackHasGlossAndVideoLink(t *testing.T) {
	n := BuildYouTubeNote("Deck", "Title", "dQw4w9WgXcQ", 65*time.Second, "la casa vieja", WordSelection{Start: 3, End: 7, Gloss: "house"})
	back := n.Fields["Back"]
	if !strings.Contains(back, "house") {
		t.Errorf("Back = %q, want it to contain the gloss", back)
	}
	if !strings.Contains(back, `<a href="https://youtu.be/dQw4w9WgXcQ?t=64">watch</a>`) {
		t.Errorf("Back = %q, want it to contain the video link", back)
	}
}

func TestBuildWordNote_NoLinkForNonYouTubeVideoID(t *testing.T) {
	// The review command passes a local file path as videoID; no link
	// should be added since it isn't a real YouTube video ID.
	n := BuildYouTubeNote("Deck", "Title", "/tmp/sample.es.vtt", 65*time.Second, "la casa vieja", WordSelection{Start: 3, End: 7})
	if strings.Contains(n.Fields["Back"], "<a href") {
		t.Errorf("Back = %q, should not contain a link for a non-YouTube videoID", n.Fields["Back"])
	}
}

func TestBuildWordNote_VideoLinkClampsToZero(t *testing.T) {
	n := BuildYouTubeNote("Deck", "Title", "dQw4w9WgXcQ", 0, "la casa vieja", WordSelection{Start: 3, End: 7})
	if !strings.Contains(n.Fields["Back"], "t=0") {
		t.Errorf("Back = %q, want it to contain t=0 (clamped, not negative)", n.Fields["Back"])
	}
}

func TestBuildWordNote_TagsIncludeWord(t *testing.T) {
	n := BuildYouTubeNote("Deck", "Title", "vid1", 0, "la Casa vieja", WordSelection{Start: 3, End: 7})
	found := false
	for _, tag := range n.Tags {
		if tag == "AnkiX::Word::casa" {
			found = true
		}
	}
	if !found {
		t.Errorf("Tags = %v, want a lowercase AnkiX::Word::casa tag", n.Tags)
	}
}
