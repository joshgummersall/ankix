package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
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

	lines := make([]tui.Line, len(article.Paragraphs))
	for i, p := range article.Paragraphs {
		lines[i] = tui.Line{Text: p}
	}
	doc := &tui.Document{SourceID: article.URL, Lines: lines}

	return launchWebTUI(f, doc, article.Title, article.URL)
}

func launchWebTUI(f *webFlags, doc *tui.Document, title, url string) error {
	var translator translate.Provider
	if !noGloss {
		translator = glossProvider{ollama.New(ollamaURL, ollamaModel)}
	}

	client := anki.New(ankiConnectURL)
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
		Document:   doc,
		Title:      title,
		Deck:       deck,
		AnkiClient: client,
		Translator: translator,
		BuildNote: func(lineIndex int, sentence string, sel anki.WordSelection) anki.Note {
			return anki.BuildNote(deck, title, url, "Web", sentence, sel)
		},
		PreviewLink: func(lineIndex int) string {
			return url
		},
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
