package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/subtitle"
	"github.com/joshgummersall/ankix/internal/translate"
	"github.com/joshgummersall/ankix/internal/tui"
)

// glossProvider adapts the shared dict/ollama Provider (which defines words
// for the kindle flow) to translate.Provider, so youtube can reuse the same
// Ollama model instead of maintaining a near-duplicate client.
type glossProvider struct {
	*ollama.Provider
}

func (g glossProvider) Gloss(word, sentence string) (string, error) {
	return g.Define(word, sentence)
}

func runFetch(f *youtubeFlags, url string) error {
	fmt.Println("fetching Spanish subtitles via yt-dlp...")
	path, videoID, err := subtitle.Fetch(url, f.subLang, f.cacheDir)
	if err != nil {
		return err
	}

	title, err := subtitle.GetTitle(url)
	if err != nil || title == "" {
		title = videoID
	}

	transcript, err := subtitle.ParseVTT(path, videoID)
	if err != nil {
		return fmt.Errorf("parse subtitles: %w", err)
	}
	if len(transcript.Cues) == 0 {
		return fmt.Errorf("no transcript lines found in %s", path)
	}

	return launchYouTubeTUI(f, transcript, title)
}

func runReview(f *youtubeFlags, path string) error {
	transcript, err := subtitle.ParseVTT(path, path)
	if err != nil {
		return fmt.Errorf("parse subtitles: %w", err)
	}
	if len(transcript.Cues) == 0 {
		return fmt.Errorf("no transcript lines found in %s", path)
	}
	return launchYouTubeTUI(f, transcript, path)
}

func launchYouTubeTUI(f *youtubeFlags, transcript *subtitle.Transcript, title string) error {
	var translator translate.Provider
	if !f.noGloss {
		translator = glossProvider{ollama.New(f.ollamaURL, f.ollamaModel)}
	}

	client := anki.New(f.ankiConnect)
	if names, err := client.ModelNames(); err == nil {
		found := false
		for _, n := range names {
			if n == "Basic" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Basic note type not found in Anki — this is a default Anki note type; check Tools > Manage Note Types")
		}
	}

	m := tui.New(tui.Config{
		Transcript: transcript,
		VideoTitle: title,
		Deck:       f.deck,
		AnkiClient: client,
		Translator: translator,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
