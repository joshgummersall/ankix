// Package vocab embeds the Modelfile used to build the "ankix" Ollama model,
// so the ankix binary can create/update the model without needing the repo
// checked out (e.g. `ankix install`).
package vocab

import _ "embed"

//go:embed Modelfile
var Modelfile string
