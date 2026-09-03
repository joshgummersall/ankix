package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
	"github.com/joshgummersall/ankix/internal/dict"
)

// glossResultMsg carries a gloss lookup result back for the phrase at idx,
// tagged with the lookup generation it was issued under (gen) so a stale
// result for a phrase that's since changed can be ignored. refined marks a
// result that came from a user correction rather than a plain lookup.
type glossResultMsg struct {
	idx     int
	gen     int
	gloss   string
	lemma   string
	err     error
	refined bool
}

type submitResultMsg struct {
	added      int
	duplicates int
	err        error // first non-duplicate error encountered, if any
}

func fetchGlossCmd(p dict.Provider, word, sentence string, idx, gen int) tea.Cmd {
	return func() tea.Msg {
		gloss, lemma, err := p.Define(word, sentence)
		return glossResultMsg{idx: idx, gen: gen, gloss: gloss, lemma: lemma, err: err}
	}
}

// refineGlossCmd re-asks r for a gloss the user wasn't happy with, passing
// the answer being corrected plus their instruction. The result comes back
// as an ordinary glossResultMsg so both paths land in one handler.
func refineGlossCmd(r dict.Refiner, word, sentence, gloss, lemma, instruction string, idx, gen int) tea.Cmd {
	return func() tea.Msg {
		g, l, err := r.Refine(word, sentence, gloss, lemma, instruction)
		return glossResultMsg{idx: idx, gen: gen, gloss: g, lemma: l, err: err, refined: true}
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
