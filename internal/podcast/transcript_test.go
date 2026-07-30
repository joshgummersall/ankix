package podcast

import (
	"strings"
	"testing"
)

func TestParseSRT(t *testing.T) {
	input := `1
00:00:01,000 --> 00:00:04,000
Hola, ¿cómo estás?

2
00:00:04,500 --> 00:00:07,000
Muy bien, gracias.
`
	cues, err := parseSRT(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("got %d cues, want 2", len(cues))
	}
	if cues[0].Text != "Hola, ¿cómo estás?" {
		t.Errorf("cue 0 text = %q", cues[0].Text)
	}
	if cues[0].Start.Seconds() != 1 {
		t.Errorf("cue 0 start = %v, want 1s", cues[0].Start)
	}
	if cues[1].Text != "Muy bien, gracias." {
		t.Errorf("cue 1 text = %q", cues[1].Text)
	}
}

func TestParseJSONTranscript(t *testing.T) {
	input := `{"segments":[
		{"startTime":0.5,"endTime":2.1,"body":"Hola a todos."},
		{"startTime":2.1,"endTime":4.0,"body":"Bienvenidos al show."}
	]}`
	cues, err := parseJSONTranscript(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("got %d cues, want 2", len(cues))
	}
	if cues[0].Text != "Hola a todos." {
		t.Errorf("cue 0 text = %q", cues[0].Text)
	}
	if cues[0].Start.Seconds() != 0.5 {
		t.Errorf("cue 0 start = %v, want 0.5s", cues[0].Start)
	}
}

func TestParseHTMLTranscript(t *testing.T) {
	input := `<p>Primer párrafo del episodio.</p><p>Segundo párrafo aquí.</p>`
	cues, err := parseHTMLTranscript(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("got %d cues, want 2: %+v", len(cues), cues)
	}
	if cues[0].Text != "Primer párrafo del episodio." {
		t.Errorf("cue 0 text = %q", cues[0].Text)
	}
	if cues[0].Start != 0 {
		t.Errorf("cue 0 start = %v, want 0", cues[0].Start)
	}
}

func TestParseHTMLTranscriptNoText(t *testing.T) {
	_, err := parseHTMLTranscript(strings.NewReader("<p></p>"))
	if err == nil {
		t.Fatal("expected error for empty transcript")
	}
}
