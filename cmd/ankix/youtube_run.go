package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict"
	"github.com/joshgummersall/ankix/internal/subtitle"
	"github.com/joshgummersall/ankix/internal/tui"
)

func formatTS(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func runFetch(f *youtubeFlags, url string) error {
	startAnki()
	provider, err := newDictProvider()
	if err != nil {
		return err
	}

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

	return launchYouTubeTUI(f, provider, transcript, title)
}

func runReview(f *youtubeFlags, path string) error {
	startAnki()
	provider, err := newDictProvider()
	if err != nil {
		return err
	}

	transcript, err := subtitle.ParseVTT(path, path)
	if err != nil {
		return fmt.Errorf("parse subtitles: %w", err)
	}
	if len(transcript.Cues) == 0 {
		return fmt.Errorf("no transcript lines found in %s", path)
	}
	return launchYouTubeTUI(f, provider, transcript, path)
}

func launchYouTubeTUI(f *youtubeFlags, provider dict.Provider, transcript *subtitle.Transcript, title string) error {
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
		Dict:       provider,
		BuildNote: func(lineIndex int, sentence string, sel anki.WordSelection) anki.Note {
			return anki.BuildYouTubeNote(cardTemplates, deck, title, videoID, cues[lineIndex].Start, sentence, sel)
		},
		PreviewLink: func(lineIndex int) string {
			return anki.VideoLink(videoID, cues[lineIndex].Start)
		},
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
