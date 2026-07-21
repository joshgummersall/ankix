package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict/ollama"
	"github.com/joshgummersall/ankix/internal/subtitle"
	"github.com/joshgummersall/ankix/internal/translate"
	"github.com/joshgummersall/ankix/internal/tui"
)

func formatTS(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

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
	fmt.Printf("fetching %q subtitles via yt-dlp...\n", f.subLang)
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

	cues := transcript.Cues
	lines := make([]tui.Line, len(cues))
	for i, c := range cues {
		lines[i] = tui.Line{Label: formatTS(c.Start), Text: c.Text}
	}
	videoID := transcript.VideoID

	m := tui.New(tui.Config{
		Document:   &tui.Document{SourceID: videoID, Lines: lines},
		Title:      title,
		Deck:       deck,
		AnkiClient: client,
		Translator: translator,
		BuildNote: func(lineIndex int, sentence string, sel anki.WordSelection) anki.Note {
			return anki.BuildYouTubeNote(deck, title, videoID, cues[lineIndex].Start, sentence, sel)
		},
		PreviewLink: func(lineIndex int) string {
			return anki.VideoLink(videoID, cues[lineIndex].Start)
		},
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
