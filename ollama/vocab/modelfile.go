// Package vocab embeds the Modelfile used to build the "ankix" Ollama model,
// so the ankix binary can create/update the model without needing the repo
// checked out (e.g. `ankix install`).
package vocab

import (
	"bytes"
	_ "embed"
	"fmt"
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
