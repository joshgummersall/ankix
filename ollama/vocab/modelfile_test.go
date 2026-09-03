package vocab

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	got, err := Render("qwen2.5:14b")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "FROM qwen2.5:14b") {
		t.Errorf("rendered Modelfile missing FROM line for base model:\n%s", got)
	}
	if strings.Contains(got, "{{.BaseModel}}") {
		t.Errorf("rendered Modelfile still contains unrendered placeholder:\n%s", got)
	}
}

func TestRenderDefaultBaseModel(t *testing.T) {
	got, err := Render(DefaultBaseModel)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "FROM "+DefaultBaseModel) {
		t.Errorf("rendered Modelfile missing FROM line for default base model:\n%s", got)
	}
}

func TestChecksum_IsStableAndShort(t *testing.T) {
	got := Checksum()
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(got) {
		t.Errorf("Checksum() = %q, want 12 lowercase hex characters", got)
	}
	if got != Checksum() {
		t.Error("Checksum() is not stable across calls")
	}
}

// The checksum names the prompt, not the build: a command looking the model
// up at runtime has no idea which base model the install used, so the base
// model must not affect the tag.
func TestChecksum_IndependentOfBaseModel(t *testing.T) {
	before := Checksum()
	if _, err := Render("qwen2.5:14b"); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if after := Checksum(); after != before {
		t.Errorf("Checksum changed after rendering a different base model: %q -> %q", before, after)
	}
}

func TestTag(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"bare name gets the checksum", "ankix", "ankix:" + Checksum()},
		{"explicit tag is left alone", "myfork:v1", "myfork:v1"},
		{"latest is left alone", "ankix:latest", "ankix:latest"},
		{"digest is left alone", "ankix@sha256:abc", "ankix@sha256:abc"},
		{"namespaced bare name gets the checksum", "me/ankix", "me/ankix:" + Checksum()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Tag(tt.model); got != tt.want {
				t.Errorf("Tag(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

// The generated tag has to be a name `ollama create` will accept.
func TestTag_ProducesAValidOllamaName(t *testing.T) {
	got := Tag("ankix")
	repo, tag, ok := strings.Cut(got, ":")
	if !ok || repo == "" || tag == "" {
		t.Fatalf("Tag(\"ankix\") = %q, want a repo:tag pair", got)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`).MatchString(tag) {
		t.Errorf("tag %q is not a valid Ollama tag", tag)
	}
}

// ollama/vocab/mise.toml's create task computes the same tag in shell, with
// `shasum -a 256 Modelfile | cut -c1-12`. A model built under any other name
// isn't found at runtime, so the two must not drift.
func TestChecksum_MatchesShasumOfTheModelfile(t *testing.T) {
	raw, err := os.ReadFile("Modelfile")
	if err != nil {
		t.Fatalf("read Modelfile: %v", err)
	}
	sum := sha256.Sum256(raw)
	if want := hex.EncodeToString(sum[:])[:12]; Checksum() != want {
		t.Errorf("Checksum() = %q, but sha256(Modelfile)[:12] = %q — the embedded copy and the file on disk disagree", Checksum(), want)
	}
}
