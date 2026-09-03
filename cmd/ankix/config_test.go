package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshgummersall/ankix/internal/anki"
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
