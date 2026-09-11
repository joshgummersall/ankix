package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict"
	"github.com/joshgummersall/ankix/internal/tui"
	"github.com/joshgummersall/ankix/internal/web"
)

func runWebFetch(f *webFlags, url string) error {
	startAnki()
	provider, err := newDictProvider()
	if err != nil {
		return err
	}

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

	return launchWebTUI(f, provider, doc, article.Title, article.URL)
}

func launchWebTUI(f *webFlags, provider dict.Provider, doc *tui.Document, title, url string) error {
	client, err := newAnkiClient()
	if err != nil {
		return err
	}
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
		Dict:       provider,
		BuildNote: func(lineIndex int, sentence string, sel anki.WordSelection) anki.Note {
			return anki.BuildNote(cardTemplates, deck, title, url, "Web", sentence, sel)
		},
		PreviewLink: func(lineIndex int) string {
			return url
		},
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
