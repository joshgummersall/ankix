package main

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// config holds defaults loaded from the user's config file, letting flags
// like --deck or --lang be set once instead of passed on every invocation.
type config struct {
	Deck           string `toml:"deck"`
	AnkiConnectURL string `toml:"ankiconnect_url"`
	OllamaURL      string `toml:"ollama_url"`
	OllamaModel    string `toml:"ollama_model"`
	NoGloss        bool   `toml:"no_gloss"`
	// Lang is the target language being studied. It seeds Kindle's --lang
	// filter and YouTube's --sub-lang unless a command-specific value below
	// overrides it.
	Lang string `toml:"lang"`

	Kindle struct {
		Lang string `toml:"lang"`
	} `toml:"kindle"`

	YouTube struct {
		SubLang  string `toml:"sub_lang"`
		CacheDir string `toml:"cache_dir"`
	} `toml:"youtube"`
}

// configPath returns the location ankix reads its config from:
// $XDG_CONFIG_HOME/ankix/config.toml (or the OS equivalent).
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ankix", "config.toml"), nil
}

// loadConfig reads the config file if present. A missing file is not an
// error; every field simply keeps its zero value and the built-in flag
// defaults apply.
func loadConfig() (config, error) {
	var cfg config

	path, err := configPath()
	if err != nil {
		return cfg, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	_, err = toml.DecodeFile(path, &cfg)
	return cfg, err
}
