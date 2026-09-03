// Package vocab embeds the Modelfile used to build the "ankix" Ollama model,
// so the ankix binary can create/update the model without needing the repo
// checked out (e.g. `ankix install`).
package vocab

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
	"text/template"
)

// modelfileTemplate is Modelfile, a text/template with a {{.BaseModel}}
// placeholder for the Ollama model it's built FROM. Render it before handing
// it to `ollama create`.
//
//go:embed Modelfile
var modelfileTemplate string

// DefaultBaseModel is the Ollama model ankix builds on top of when no other
// base model is requested.
const DefaultBaseModel = "llama3.2:3b"

// Render fills in the Modelfile template's {{.BaseModel}} placeholder,
// producing a Modelfile ready to hand to `ollama create`.
func Render(baseModel string) (string, error) {
	tmpl, err := template.New("Modelfile").Parse(modelfileTemplate)
	if err != nil {
		return "", fmt.Errorf("parse modelfile template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ BaseModel string }{BaseModel: baseModel}); err != nil {
		return "", fmt.Errorf("render modelfile template: %w", err)
	}

	return buf.String(), nil
}

// Checksum is a short hex digest of the Modelfile's own source, identifying
// the prompt a model was built from. It deliberately hashes the unrendered
// template, not the output of Render: the {{.BaseModel}} line is the one
// part that varies per install, and a binary looking a model up at runtime
// has no idea which base model it was built on. So the checksum names the
// prompt, and the base model stays the user's orthogonal choice — visible
// via `ollama show`, and not something ankix needs to agree about.
func Checksum() string {
	sum := sha256.Sum256([]byte(modelfileTemplate))
	return hex.EncodeToString(sum[:])[:12]
}

// Tag resolves a configured model name to the exact Ollama tag ankix should
// ask for. A bare name gets the current Modelfile's checksum appended, so
// the binary can only ever address a model built from the prompt it itself
// embeds — a model built from an older Modelfile isn't stale so much as
// unaddressable, and the lookup fails with "not found" rather than
// answering wrongly. A name that already carries an explicit tag (or a
// digest) is left alone, which is the escape hatch for a hand-built or
// forked model.
func Tag(model string) string {
	if strings.ContainsAny(model, ":@") {
		return model
	}
	return model + ":" + Checksum()
}
