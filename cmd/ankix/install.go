package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshgummersall/ankix/ollama/vocab"
)

// newInstallCmd deliberately defines no flags of its own. Both inputs come
// from config: the name to build is the global --ollama-model, the same one
// every other command looks up, and the base model to build FROM is
// `base_model`.
//
// A --base-model flag would be incoherent rather than merely redundant.
// install's output is the durable artifact, so there is no "just this once"
// for it: building on a base passed once on the command line leaves that
// base installed until the next `ankix install` silently reverts it — which
// every upgrade asks you to run. Config is the only place the choice can
// actually survive.
func newInstallCmd(cfg config) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Build the local Ollama model ankix uses for definitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installModel(ollamaModel, baseModelFor(cfg))
		},
	}
}

// baseModelFor is the Ollama model `ankix install` builds the gloss model
// FROM.
func baseModelFor(cfg config) string {
	return strOr(cfg.BaseModel, vocab.DefaultBaseModel)
}

// installModel renders the embedded Modelfile (ollama/vocab/Modelfile) with
// the requested base model, writes it to a temp file, and hands it to the
// ollama CLI, which is the only supported way to build a model from a
// Modelfile (the HTTP /api/create endpoint takes the same content but
// shelling out to ollama is simpler and matches what users would otherwise
// run by hand).
//
// The model is tagged with the Modelfile's checksum rather than :latest, so
// the name itself pins the prompt — see vocab.Tag. Every ankix command asks
// for that exact tag, which is why an upgrade can't quietly keep using the
// model an older release built.
func installModel(model, baseModel string) error {
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama not found on PATH: install it from https://ollama.com, then re-run `ankix install`")
	}

	rendered, err := vocab.Render(baseModel)
	if err != nil {
		return fmt.Errorf("render modelfile: %w", err)
	}

	tmp, err := os.CreateTemp("", "ankix-modelfile-*")
	if err != nil {
		return fmt.Errorf("write modelfile: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(rendered); err != nil {
		tmp.Close()
		return fmt.Errorf("write modelfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write modelfile: %w", err)
	}

	tagged := vocab.Tag(model)
	fmt.Printf("building Ollama model %q...\n", tagged)
	c := exec.Command("ollama", "create", tagged, "-f", tmp.Name())
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("ollama create: %w", err)
	}

	fmt.Printf("model %q ready\n", tagged)
	reportOldBuilds(model, tagged)
	return nil
}

// reportOldBuilds points out models an earlier ankix left behind. They cost
// almost nothing — every build of the same base model shares its weights,
// and only the small prompt layers differ — so they're worth mentioning
// rather than deleting: an older tag is a working rollback target, and
// ankix shouldn't remove something the user may have pinned.
func reportOldBuilds(model, tagged string) {
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return
	}

	var old []string
	for line := range strings.Lines(string(out)) {
		name, _, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || name == tagged {
			continue
		}
		if repo, _, hasTag := strings.Cut(name, ":"); hasTag && repo == model {
			old = append(old, name)
		}
	}
	if len(old) == 0 {
		return
	}

	fmt.Printf("\nearlier builds still installed: %s\n", strings.Join(old, ", "))
	fmt.Printf("they share their weights with the new one, so they cost little; remove one with `ollama rm <name>`\n")
}
