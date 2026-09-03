package anki

import (
	"fmt"
	"io"
	"strings"
	"text/template"
)

// CardData is the data a Front/Back card template renders from. Sentence
// text is split around the marked word/phrase rather than given whole,
// since each builder needs to wrap only that phrase (e.g. in <i>...</i>).
// Fields that don't apply to a given source (e.g. Timestamp for a web
// article) are the empty string, and the default templates degrade
// gracefully when that happens.
type CardData struct {
	Word        string // the marked headword/phrase
	Before      string // sentence text before Word, valid only if Highlighted
	After       string // sentence after Word if Highlighted, else the whole (unmarked) sentence
	Highlighted bool   // whether Before/Word/After is an actual split of a sentence; false when there's no sentence to mark Word within (e.g. Kindle couldn't find the headword in its usage example)
	Definition  string // formatted definition or gloss (HTML), "" if none
	Lemma       string // dictionary/base-form lemma, "" if none or same as Definition (e.g. Kindle only)
	Source      string // video/episode/page title, "" for Kindle
	Timestamp   string // "3:41", "" if not applicable
	Link        string // deep link URL, "" if none
	LinkLabel   string // "watch" / "listen" / "read", "" if Link is ""

	// Attribution is Source, Timestamp and Link pre-combined into one
	// HTML fragment (e.g. `Title (3:41) — <a href="...">watch</a>`), for
	// templates that just want to place it as a unit. It's "" if Source
	// is "".
	Attribution string
}

// formatAttribution combines source/timestamp/link into the ready-to-use
// Attribution field.
func formatAttribution(source, timestamp, link, linkLabel string) string {
	if source == "" {
		return ""
	}
	s := source
	if timestamp != "" {
		s += " (" + timestamp + ")"
	}
	if link != "" {
		s += fmt.Sprintf(` — <a href="%s">%s</a>`, link, linkLabel)
	}
	return s
}

// DefaultFrontTemplate and DefaultBackTemplate reproduce ankix's built-in
// card formatting: a bold headword above the sentence with the marked
// phrase italicized, and the definition/gloss (with its lemma in
// parentheses, when there is one) above an attribution line.
const (
	DefaultFrontTemplate = `<b>{{.Word}}</b>` +
		`{{if or .Highlighted .After}}<br><br>{{end}}` +
		`{{if .Highlighted}}{{.Before}}<i>{{.Word}}</i>{{.After}}{{else}}{{.After}}{{end}}`
	DefaultBackTemplate = `{{.Definition}}{{if .Lemma}} ({{.Lemma}}){{end}}` +
		`{{if and .Definition .Attribution}}<br><br>{{end}}{{.Attribution}}`
)

// Templates holds the compiled Front/Back card templates used to render
// every note ankix creates.
type Templates struct {
	front *template.Template
	back  *template.Template
}

// sampleCardData are fully-populated CardData values used to validate a
// template at load time, so a typo (e.g. {{.Wrod}}) is reported immediately
// instead of surfacing mid-sync or mid-review. Both cover the Highlighted
// branches a template might take.
var sampleCardData = []CardData{
	{
		Word:        "word",
		Before:      "before ",
		After:       " after",
		Highlighted: true,
		Definition:  "definition",
		Lemma:       "lemma",
		Source:      "source",
		Timestamp:   "0:00",
		Link:        "https://example.com",
		LinkLabel:   "watch",
		Attribution: formatAttribution("source", "0:00", "https://example.com", "watch"),
	},
	{
		Word:       "word",
		After:      "sentence with no marked occurrence",
		Definition: "definition",
	},
}

// NewTemplates compiles front and back as Go text/template strings over
// CardData (text/template, not html/template, since the templates produce
// literal HTML that isn't meant to be escaped). An empty string falls back
// to the matching default above.
func NewTemplates(front, back string) (*Templates, error) {
	if front == "" {
		front = DefaultFrontTemplate
	}
	if back == "" {
		back = DefaultBackTemplate
	}

	ft, err := template.New("front").Parse(front)
	if err != nil {
		return nil, fmt.Errorf("parse front card template: %w", err)
	}
	bt, err := template.New("back").Parse(back)
	if err != nil {
		return nil, fmt.Errorf("parse back card template: %w", err)
	}
	for _, sample := range sampleCardData {
		if err := ft.Execute(io.Discard, sample); err != nil {
			return nil, fmt.Errorf("render front card template: %w", err)
		}
		if err := bt.Execute(io.Discard, sample); err != nil {
			return nil, fmt.Errorf("render back card template: %w", err)
		}
	}

	return &Templates{front: ft, back: bt}, nil
}

// defaultTemplates is used whenever a builder is called with a nil
// *Templates, so callers that don't care about custom formatting (and
// existing tests) don't need to construct one.
var defaultTemplates = mustNewTemplates("", "")

func mustNewTemplates(front, back string) *Templates {
	t, err := NewTemplates(front, back)
	if err != nil {
		panic(err)
	}
	return t
}

// Render executes the front and back templates against data. It only
// returns an error a caller can hit if it builds a *Templates itself and
// skips NewTemplates' validation; every ankix code path uses NewTemplates,
// so this is unreachable in practice.
func (t *Templates) Render(data CardData) (front, back string) {
	if t == nil {
		t = defaultTemplates
	}

	var fb, bb strings.Builder
	if err := t.front.Execute(&fb, data); err != nil {
		panic(fmt.Sprintf("card template: render front: %v", err))
	}
	if err := t.back.Execute(&bb, data); err != nil {
		panic(fmt.Sprintf("card template: render back: %v", err))
	}
	return fb.String(), bb.String()
}
