package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Card overrides the Go text/template used to render every note's Front
	// and Back fields (see internal/anki.CardData for the fields available
	// to them). Empty strings keep ankix's built-in formatting. TOML's
	// multiline string syntax ("""...""") is the natural way to write
	// these; a leading/trailing newline from that syntax is trimmed.
	Card struct {
		Front string `toml:"front"`
		Back  string `toml:"back"`
	} `toml:"card"`
}

// configPaths returns every location ankix looks for its config file, in
// priority order: $XDG_CONFIG_HOME/ankix/config.toml (or ~/.config/ankix
// if that's unset) is checked first, since it's the conventional location
// on every OS including macOS; os.UserConfigDir()'s OS-specific default
// (e.g. ~/Library/Application Support/ankix on macOS) is checked next, for
// anyone with a config file already there.
func configPaths() ([]string, error) {
	var paths []string

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "ankix", "config.toml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "ankix", "config.toml"))
	}

	if dir, err := os.UserConfigDir(); err == nil {
		if p := filepath.Join(dir, "ankix", "config.toml"); len(paths) == 0 || paths[0] != p {
			paths = append(paths, p)
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("could not determine config file location: %w", os.ErrNotExist)
	}
	return paths, nil
}

// loadConfig reads the config file if present, trying each of configPaths
// in order and using the first one that exists. A missing file at every
// candidate path is not an error; every field simply keeps its zero value
// and the built-in flag defaults apply.
func loadConfig() (config, error) {
	paths, err := configPaths()
	if err != nil {
		return config{}, err
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return loadConfigFrom(p)
		}
	}
	return config{}, nil
}

// loadConfigFrom is loadConfig with an explicit path, split out so it can be
// exercised directly in tests without touching os.UserConfigDir.
func loadConfigFrom(path string) (config, error) {
	var cfg config

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}

	cfg.Card.Front = strings.Trim(cfg.Card.Front, "\n")
	cfg.Card.Back = strings.Trim(cfg.Card.Back, "\n")
	return cfg, nil
}
