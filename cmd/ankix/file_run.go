package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/textutil"
	"github.com/joshgummersall/ankix/internal/translate"
	"github.com/joshgummersall/ankix/internal/tui"
)

func runFileOpen(f *fileFlags, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	paragraphs := textutil.Paragraphs(string(data))
	if len(paragraphs) == 0 {
		return fmt.Errorf("no readable content found in %s", path)
	}

	lines := make([]tui.Line, len(paragraphs))
	for i, p := range paragraphs {
		lines[i] = tui.Line{Text: p}
	}
	doc := &tui.Document{SourceID: path, Lines: lines}

	return launchFileTUI(f, doc, filepath.Base(path))
}

func launchFileTUI(f *fileFlags, doc *tui.Document, title string) error {
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
			return anki.BuildNote(deck, title, "", "File", sentence, sel)
		},
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
