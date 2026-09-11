package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/ollama/vocab"
)

func TestLoadConfigFrom_TrimsMultilineCardTemplates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, `
[card]
front = """
<b>{{.Word}}</b>
"""
back = """
{{.Definition}}
"""
`)

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("loadConfigFrom() error = %v", err)
	}
	if want := "<b>{{.Word}}</b>"; cfg.Card.Front != want {
		t.Errorf("Card.Front = %q, want %q", cfg.Card.Front, want)
	}
	if want := "{{.Definition}}"; cfg.Card.Back != want {
		t.Errorf("Card.Back = %q, want %q", cfg.Card.Back, want)
	}

	if _, err := anki.NewTemplates(cfg.Card.Front, cfg.Card.Back); err != nil {
		t.Errorf("anki.NewTemplates() error = %v", err)
	}
}

func TestLoadConfigFrom_MissingFileReturnsZeroValue(t *testing.T) {
	cfg, err := loadConfigFrom(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("loadConfigFrom() error = %v", err)
	}
	if cfg.Card.Front != "" || cfg.Card.Back != "" {
		t.Errorf("cfg.Card = %+v, want zero value", cfg.Card)
	}
}

func TestLoadConfig_PrefersDotConfigOverOSDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	dotConfigPath := filepath.Join(home, ".config", "ankix", "config.toml")
	mustWriteConfig(t, dotConfigPath, `deck = "from-dot-config"`)

	osDefaultPath := osDefaultConfigPath(t)
	if osDefaultPath != dotConfigPath {
		mustWriteConfig(t, osDefaultPath, `deck = "from-os-default"`)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Deck != "from-dot-config" {
		t.Errorf("Deck = %q, want %q (should prefer ~/.config/ankix)", cfg.Deck, "from-dot-config")
	}
}

func TestLoadConfig_FallsBackToOSDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	mustWriteConfig(t, osDefaultConfigPath(t), `deck = "from-os-default"`)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Deck != "from-os-default" {
		t.Errorf("Deck = %q, want %q (should fall back to the OS default location)", cfg.Deck, "from-os-default")
	}
}

func TestLoadConfig_RespectsXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	mustWriteConfig(t, filepath.Join(xdg, "ankix", "config.toml"), `deck = "from-xdg"`)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Deck != "from-xdg" {
		t.Errorf("Deck = %q, want %q", cfg.Deck, "from-xdg")
	}
}

func osDefaultConfigPath(t *testing.T) string {
	t.Helper()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("os.UserConfigDir() error = %v", err)
	}
	return filepath.Join(dir, "ankix", "config.toml")
}

func mustWriteConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	writeFile(t, path, content)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// Every upgrade re-runs `ankix install` (the model is pinned to the
// Modelfile checksum), so a base model picked once on the command line
// would revert to the default on the next rebuild. It has to be durable.
func TestBaseModelFor_UsesTheConfiguredBaseModel(t *testing.T) {
	if got := baseModelFor(config{BaseModel: "qwen2.5:14b"}); got != "qwen2.5:14b" {
		t.Errorf("baseModelFor = %q, want the configured base_model", got)
	}
}

func TestBaseModelFor_FallsBackToTheDefaultBaseModel(t *testing.T) {
	if got := baseModelFor(config{}); got != vocab.DefaultBaseModel {
		t.Errorf("baseModelFor = %q, want %q", got, vocab.DefaultBaseModel)
	}
}

// install's inputs are both durable config: the name to build is the global
// --ollama-model (the same one every command looks up), and the base model
// comes from `base_model`. A flag for either would be a transient input
// producing a persistent artifact — build once with it, and the next
// `ankix install`, which every upgrade asks for, silently reverts.
func TestInstallCmd_TakesNoFlagsOfItsOwn(t *testing.T) {
	newInstallCmd(config{}).Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "help" {
			t.Errorf("install defines --%s; both its inputs must come from config", f.Name)
		}
	})
}

func TestLoadConfigFrom_ReadsBaseModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, "base_model = \"qwen2.5:14b\"\n")

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("loadConfigFrom() error = %v", err)
	}
	if cfg.BaseModel != "qwen2.5:14b" {
		t.Errorf("BaseModel = %q, want %q", cfg.BaseModel, "qwen2.5:14b")
	}
}

// Ejecting is a property of how you sync (plugged in over USB, then
// unplugged), not of a single run, so `[kindle].eject` has to seed the flag
// default.
func TestKindleVocabCmd_EjectDefaultsToTheConfiguredValue(t *testing.T) {
	var cfg config
	cfg.Kindle.Eject = true

	if got := newKindleVocabCmd(cfg).Flags().Lookup("eject").DefValue; got != "true" {
		t.Errorf("--eject default = %q, want %q", got, "true")
	}
	if got := newKindleVocabCmd(config{}).Flags().Lookup("eject").DefValue; got != "false" {
		t.Errorf("--eject default = %q with no config, want %q", got, "false")
	}
}

func TestLoadConfigFrom_ReadsKindleEject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, "[kindle]\neject = true\n")

	cfg, err := loadConfigFrom(path)
	if err != nil {
		t.Fatalf("loadConfigFrom() error = %v", err)
	}
	if !cfg.Kindle.Eject {
		t.Error("Kindle.Eject = false, want true")
	}
}
