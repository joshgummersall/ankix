package subtitle

import (
	"strings"
	"testing"
)

func TestParseVTT_DedupesRollingCaptions(t *testing.T) {
	tr, err := ParseVTT("../../testdata/sample.es.vtt", "vid123")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"hoy tenemos competencia de birria va a competir México contra Estados Unidos",
		"estoy en Los Ángeles y voy a ir a las",
		"mejores tres birrierías que pueda",
		"encontrar aquí y en México Vamos a ir a",
		"otras tres birrerías van a competir las",
	}
	if len(tr.Cues) != len(want) {
		t.Fatalf("got %d cues, want %d: %+v", len(tr.Cues), len(want), tr.Cues)
	}
	for i, w := range want {
		if tr.Cues[i].Text != w {
			t.Errorf("cue %d: got %q, want %q", i, tr.Cues[i].Text, w)
		}
	}

	// The reconstructed, deduped transcript should read as one continuous
	// stream with no repeated words at the seams.
	var full []string
	for _, c := range tr.Cues {
		full = append(full, c.Text)
	}
	got := strings.Join(full, " ")
	want2 := "hoy tenemos competencia de birria va a competir México contra Estados Unidos estoy en Los Ángeles y voy a ir a las mejores tres birrierías que pueda encontrar aquí y en México Vamos a ir a otras tres birrerías van a competir las"
	if got != want2 {
		t.Errorf("reconstructed transcript =\n%q\nwant\n%q", got, want2)
	}
}

func TestDedupeCues_NonOverlappingCuesPassThrough(t *testing.T) {
	cues := []Cue{
		{Text: "hola que tal"},
		{Text: "todo bien gracias"},
		{Text: "nos vemos luego"},
	}
	got := dedupeCues(cues)
	if len(got) != len(cues) {
		t.Fatalf("got %d cues, want %d: %+v", len(got), len(cues), got)
	}
	for i, c := range cues {
		if got[i].Text != c.Text {
			t.Errorf("cue %d: got %q, want %q", i, got[i].Text, c.Text)
		}
	}
}
