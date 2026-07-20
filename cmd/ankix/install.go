package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/joshgummersall/ankix/ollama/vocab"
)

func newInstallCmd() *cobra.Command {
	var model string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Build the local Ollama model ankix uses for definitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installModel(model)
		},
	}
	cmd.Flags().StringVar(&model, "model", "ankix", "name to give the Ollama model")

	return cmd
}

// installModel writes the embedded Modelfile (ollama/vocab/Modelfile) to a
// temp file and hands it to the ollama CLI, which is the only supported way
// to build a model from a Modelfile (the HTTP /api/create endpoint takes the
// same content but shells out to ollama is simpler and matches what users
// would otherwise run by hand).
func installModel(model string) error {
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama not found on PATH: install it from https://ollama.com, then re-run `ankix install`")
	}

	tmp, err := os.CreateTemp("", "ankix-modelfile-*")
	if err != nil {
		return fmt.Errorf("write modelfile: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(vocab.Modelfile); err != nil {
		tmp.Close()
		return fmt.Errorf("write modelfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write modelfile: %w", err)
	}

	fmt.Printf("building Ollama model %q...\n", model)
	c := exec.Command("ollama", "create", model, "-f", tmp.Name())
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("ollama create: %w", err)
	}

	fmt.Printf("model %q ready\n", model)
	return nil
}
