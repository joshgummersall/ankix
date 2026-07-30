package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/podcast"
	"github.com/joshgummersall/ankix/internal/subtitle"
	"github.com/joshgummersall/ankix/internal/translate"
	"github.com/joshgummersall/ankix/internal/tui"
)

func runPodcastAppleFetch(url string) error {
	podcastID, episodeID, err := podcast.ParseAppleURL(url)
	if err != nil {
		return err
	}

	fmt.Println("resolving show feed...")
	feedURL, err := podcast.LookupFeedURL(podcastID)
	if err != nil {
		return err
	}

	fmt.Println("resolving episode...")
	ep, err := podcast.LookupEpisode(podcastID, episodeID)
	if err != nil {
		return err
	}

	fmt.Println("fetching feed and matching episode...")
	item, err := podcast.FindItem(feedURL, ep)
	if err != nil {
		return err
	}

	fmt.Println("fetching transcript...")
	cues, err := podcast.FetchTranscript(item)
	if err != nil {
		return err
	}
	if len(cues) == 0 {
		return fmt.Errorf("no transcript lines found for %q", item.Title)
	}

	return launchPodcastTUI(cues, item.Title, item.AudioURL)
}

func launchPodcastTUI(cues []subtitle.Cue, title, audioURL string) error {
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

	lines := make([]tui.Line, len(cues))
	for i, c := range cues {
		if c.Start > 0 || c.End > 0 {
			lines[i] = tui.Line{Label: formatTS(c.Start), Text: c.Text}
		} else {
			lines[i] = tui.Line{Text: c.Text}
		}
	}

	m := tui.New(tui.Config{
		Document:   &tui.Document{SourceID: title, Lines: lines},
		Title:      title,
		Deck:       deck,
		AnkiClient: client,
		Translator: translator,
		BuildNote: func(lineIndex int, sentence string, sel anki.WordSelection) anki.Note {
			return anki.BuildPodcastNote(deck, title, audioURL, cues[lineIndex].Start, sentence, sel)
		},
		PreviewLink: func(lineIndex int) string {
			return anki.AudioLink(audioURL, cues[lineIndex].Start)
		},
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
