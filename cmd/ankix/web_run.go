package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/subtitle"
	"github.com/joshgummersall/ankix/internal/translate"
	"github.com/joshgummersall/ankix/internal/tui"
	"github.com/joshgummersall/ankix/internal/web"
)

func runWebFetch(f *webFlags, url string) error {
	fmt.Println("fetching and extracting article...")
	article, err := web.Fetch(url)
	if err != nil {
		return err
	}

	cues := make([]subtitle.Cue, len(article.Paragraphs))
	for i, p := range article.Paragraphs {
		cues[i] = subtitle.Cue{Text: p}
	}
	transcript := &subtitle.Transcript{VideoID: article.URL, Cues: cues}

	return launchWebTUI(f, transcript, article.Title)
}

func launchWebTUI(f *webFlags, transcript *subtitle.Transcript, title string) error {
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
		Transcript:     transcript,
		VideoTitle:     title,
		Deck:           f.deck,
		AnkiClient:     client,
		Translator:     translator,
		ShowTimestamps: false,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
