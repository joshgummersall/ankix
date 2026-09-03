package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/joshgummersall/ankix/internal/dict"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/ollama/vocab"
)

// newDictProvider builds the definition provider every source shares, or
// nil when glossing is switched off. Call it before any slow setup work
// (downloading subtitles, fetching an article): it checks the model is
// installed, and a missing one should be reported before a yt-dlp download,
// not after.
func newDictProvider() (dict.Provider, error) {
	if noGloss {
		return nil, nil
	}
	model, err := resolveModel(ollamaURL, ollamaModel)
	if err != nil {
		return nil, err
	}
	return ollama.New(ollamaURL, model), nil
}

// resolveModel returns the exact Ollama tag to use for the configured model
// name and checks that it's actually installed, so a missing or outdated
// model is reported once, up front, with the command that fixes it — rather
// than surfacing as a "lookup failed" on every word once the review screen
// is already open.
func resolveModel(ollamaURL, model string) (string, error) {
	tagged := vocab.Tag(model)

	installed, err := installedModels(ollamaURL)
	if err != nil {
		return "", err
	}
	for _, name := range installed {
		if name == tagged {
			return tagged, nil
		}
	}
	return "", modelNotFoundError(model, tagged, installed)
}

// modelNotFoundError explains what to do about a model that isn't
// installed. The three cases read very differently to a user: an upgrade
// that left an older build behind, a first run with nothing installed at
// all, and a hand-picked model name that simply isn't there.
func modelNotFoundError(model, tagged string, installed []string) error {
	// An explicit tag is the user's own choice of model (see vocab.Tag), so
	// `ankix install` isn't the answer — it would build something else.
	if tagged == model {
		return fmt.Errorf("Ollama model %q not found; pull or build it, or pass --ollama-model to pick another", model)
	}

	var older []string
	for _, name := range installed {
		if repo, _, ok := strings.Cut(name, ":"); ok && repo == model {
			older = append(older, name)
		}
	}
	sort.Strings(older)

	if len(older) > 0 {
		return fmt.Errorf(
			"the %q model is out of date: found %s, but this version of ankix needs %s\n"+
				"run `ankix install` to build it (the older build is left alone, and keeps working with the older ankix)",
			model, strings.Join(older, ", "), tagged)
	}
	return fmt.Errorf("no %q model installed; run `ankix install` to build it", model)
}

// installedModels lists the model tags Ollama has locally.
func installedModels(ollamaURL string) ([]string, error) {
	url := strings.TrimSuffix(ollamaURL, "/") + "/api/tags"
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("can't reach Ollama at %s: %w\nis it running? start it with `ollama serve`, or point --ollama-url elsewhere", ollamaURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing Ollama models at %s: unexpected status %s", url, resp.Status)
	}

	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("listing Ollama models: decode response: %w", err)
	}

	names := make([]string, len(body.Models))
	for i, m := range body.Models {
		names[i] = m.Name
	}
	return names, nil
}
