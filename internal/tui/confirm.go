package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
)

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateWordPick
		return m, nil
	case "enter":
		m.state = stateSubmitting
		notes := make([]anki.Note, len(m.markedWords))
		for i, sel := range m.markedWords {
			notes[i] = anki.BuildWordNote(m.cfg.Deck, m.cfg.VideoTitle, m.cfg.Transcript.VideoID,
				m.cueStart, m.sentence, sel)
		}
		return m, addWordNotesCmd(m.cfg.AnkiClient, m.cfg.Deck, notes)
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) renderConfirm() string {
	ordered := make([]anki.WordSelection, len(m.markedWords))
	copy(ordered, m.markedWords)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })

	var b strings.Builder
	b.WriteString("\n")
	last := 0
	for _, s := range ordered {
		b.WriteString(m.sentence[last:s.Start])
		b.WriteString(markedWordStyle.Render(m.sentence[s.Start:s.End]))
		last = s.End
	}
	b.WriteString(m.sentence[last:])
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("preview only — each word above becomes its own card, bolded on its own front") + "\n\n")

	b.WriteString(fmt.Sprintf("%d card(s), deck: %s\n", len(ordered), m.cfg.Deck))
	if link := anki.VideoLink(m.cfg.Transcript.VideoID, m.cueStart); link != "" {
		b.WriteString("link: " + link + "\n")
	}
	b.WriteString("\n")

	for _, s := range ordered {
		word := m.sentence[s.Start:s.End]
		gloss := s.Gloss
		if gloss == "" && m.glossPending > 0 {
			gloss = "looking up..."
		}
		b.WriteString(fmt.Sprintf("%s: %s\n", word, gloss))
	}

	if m.state == stateSubmitting {
		b.WriteString("\nsubmitting...\n")
	}
	return b.String()
}
