package anki

import (
	"strings"
	"testing"
)

func TestNewTemplates_EmptyStringsUseDefaults(t *testing.T) {
	tmpl, err := NewTemplates("", "")
	if err != nil {
		t.Fatalf("NewTemplates() error = %v", err)
	}
	front, back := tmpl.Render(CardData{Word: "casa", Before: "la ", After: " vieja", Highlighted: true, Definition: "house"})
	if front != "<b>casa</b><br><br>la <i>casa</i> vieja" {
		t.Errorf("front = %q", front)
	}
	if back != "house" {
		t.Errorf("back = %q", back)
	}
}

func TestNewTemplates_CustomOverride(t *testing.T) {
	tmpl, err := NewTemplates(`{{.Word}}!`, `def: {{.Definition}}`)
	if err != nil {
		t.Fatalf("NewTemplates() error = %v", err)
	}
	front, back := tmpl.Render(CardData{Word: "casa", Definition: "house"})
	if front != "casa!" {
		t.Errorf("front = %q, want %q", front, "casa!")
	}
	if back != "def: house" {
		t.Errorf("back = %q, want %q", back, "def: house")
	}
}

func TestNewTemplates_RejectsBadSyntax(t *testing.T) {
	if _, err := NewTemplates(`{{.Word`, ""); err == nil {
		t.Error("NewTemplates() with unclosed action, want error")
	}
}

func TestNewTemplates_RejectsUnknownField(t *testing.T) {
	// Caught at load time via the sample-data dry run, not left to blow up
	// mid-sync.
	if _, err := NewTemplates(`{{.Wrod}}`, ""); err == nil {
		t.Error("NewTemplates() with unknown field, want error")
	}
}

func TestNewTemplates_NilTemplatesRendersDefaults(t *testing.T) {
	var tmpl *Templates
	front, _ := tmpl.Render(CardData{Word: "casa", Before: "la ", After: " vieja", Highlighted: true})
	if !strings.Contains(front, "<b>casa</b>") {
		t.Errorf("front = %q, want default formatting", front)
	}
}

func TestDefaultFrontTemplate_NoHighlightedOccurrence(t *testing.T) {
	tmpl, _ := NewTemplates("", "")

	front, _ := tmpl.Render(CardData{Word: "gato", After: "un gato salió corriendo."})
	if front != "<b>gato</b><br><br>un gato salió corriendo." {
		t.Errorf("front = %q", front)
	}

	front, _ = tmpl.Render(CardData{Word: "gato"})
	if front != "<b>gato</b>" {
		t.Errorf("front (no sentence) = %q", front)
	}
}

func TestDefaultBackTemplate_AttributionOnly(t *testing.T) {
	tmpl, _ := NewTemplates("", "")
	_, back := tmpl.Render(CardData{
		Source:      "My Video",
		Timestamp:   "1:05",
		Link:        "https://example.com",
		LinkLabel:   "watch",
		Attribution: formatAttribution("My Video", "1:05", "https://example.com", "watch"),
	})
	want := `My Video (1:05) — <a href="https://example.com">watch</a>`
	if back != want {
		t.Errorf("back = %q, want %q", back, want)
	}
}
