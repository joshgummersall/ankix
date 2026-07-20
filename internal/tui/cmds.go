package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/translate"
)

// glossResultMsg carries a gloss lookup result back for the phrase at idx,
// tagged with the text it was fetched for (text) so a stale result for a
// phrase that's since changed can be ignored.
type glossResultMsg struct {
	idx   int
	text  string
	gloss string
	err   error
}

type submitResultMsg struct {
	added      int
	duplicates int
	err        error // first non-duplicate error encountered, if any
}

func fetchGlossCmd(p translate.Provider, word, sentence string, idx int, text string) tea.Cmd {
	return func() tea.Msg {
		gloss, err := p.Gloss(word, sentence)
		return glossResultMsg{idx: idx, text: text, gloss: gloss, err: err}
	}
}

// addWordNotesCmd submits one note per marked word. Each word becomes its
// own card, so a failure on one shouldn't lose progress on the rest —
// every note is attempted, and the result tallies how many were added,
// how many were skipped as duplicates, and the first hard error (if any).
func addWordNotesCmd(client *anki.Client, deck string, notes []anki.Note) tea.Cmd {
	return func() tea.Msg {
		if err := client.CreateDeck(deck); err != nil {
			return submitResultMsg{err: err}
		}

		var added, duplicates int
		var firstErr error
		for _, n := range notes {
			_, err := client.AddNote(n)
			switch {
			case errors.Is(err, anki.ErrDuplicate):
				duplicates++
			case err != nil:
				if firstErr == nil {
					firstErr = err
				}
			default:
				added++
			}
		}
		return submitResultMsg{added: added, duplicates: duplicates, err: firstErr}
	}
}
