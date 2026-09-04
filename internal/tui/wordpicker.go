package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshgummersall/ankix/internal/anki"
)

// enterWordPick loads the selected document words into a fresh sentence
// with no phrases yet — every word must be added explicitly with v, unlike
// the Kindle review flow, which starts from already-looked-up candidates.
func (m *Model) enterWordPick() {
	var parts []string
	for i := m.selWordStart; i <= m.selWordEnd; i++ {
		parts = append(parts, m.words[i].Text)
	}
	m.sentence = strings.Join(parts, " ")
	m.selLineIndex = m.words[m.selWordStart].LineIndex
	m.ps.reset(m.sentence)
	m.ps.wordCursor = m.cursorWord - m.selWordStart
	m.setStatus("", false)
	m.state = stateWordPick
}

// refreshGlosses kicks off a gloss lookup for every phrase whose text is
// not previewed yet, so what will be saved is visible before submitting.
func (m *Model) refreshGlosses() tea.Cmd {
	if m.cfg.Dict == nil {
		return nil
	}
	return m.ps.refreshPreviews(m.sentence, func(i, gen int, text string) tea.Cmd {
		return fetchGlossCmd(m.cfg.Dict, text, m.sentence, i, gen)
	})
}

func (m Model) handleWordPickKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateBrowse
		m.setStatus("", false)
		m.syncViewport()
		return m, nil
	case "l", "right":
		m.ps.moveCursorRight()
		return m, nil
	case "h", "left":
		m.ps.moveCursorLeft()
		return m, nil
	case "(":
		m.ps.moveCursorPrevPhrase()
		return m, nil
	case ")":
		m.ps.moveCursorNextPhrase()
		return m, nil
	case "e":
		return m, m.enterEditSentence()
	case "r":
		cmd := m.enterRefine()
		return m, cmd
	case "v":
		if len(m.ps.wordTokens) == 0 {
			return m, nil
		}
		m.ps.beginExpand(struct{}{})
		m.state = stateWordExpand
		m.setStatus("", false)
		return m, m.ps.debounceRefresh()
	case "d":
		m.ps.deleteNearestPhrase()
		m.setStatus("", false)
		return m, nil
	case "enter":
		return m.submitWordPick()
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// submitWordPick builds a card for every non-deleted phrase in the sentence
// and submits them all in one action.
func (m Model) submitWordPick() (tea.Model, tea.Cmd) {
	if m.ps.previewsPending() {
		m.setStatus("still looking up glosses...", false)
		return m, nil
	}

	var notes []anki.Note
	for _, p := range m.ps.phrases {
		if !p.included() {
			continue
		}
		start, end := m.ps.phraseBounds(p)
		sel := anki.WordSelection{Start: start, End: end, Gloss: p.preview, Lemma: p.previewLemma}
		notes = append(notes, m.cfg.BuildNote(m.selLineIndex, m.sentence, sel))
	}
	if len(notes) == 0 {
		m.setStatus("mark at least one word with v first", true)
		return m, nil
	}

	m.state = stateSubmitting
	m.setStatus("adding...", false)
	return m, addWordNotesCmd(m.cfg.AnkiClient, m.cfg.Deck, notes)
}

func (m Model) handleWordExpandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch action, cmd := m.ps.handleExpandKey(msg); action {
	case expandMoved:
		return m, cmd
	case expandConfirmed, expandCanceled:
		m.state = stateWordPick
		m.setStatus("", false)
		return m, m.refreshGlosses()
	}
	return m, nil
}

func (m Model) renderEditSentence() string {
	return "\n" + helpStyle.Render("fix typos in the sentence, then confirm") + "\n\n" + m.sentenceInput.View() + "\n"
}

func (m Model) renderWordPicker() string {
	var b strings.Builder
	b.WriteString(m.ps.render(m.sentence))

	cards := m.ps.countIncluded()
	word := "card"
	if cards != 1 {
		word = "cards"
	}
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("%d %s will be added, deck: %s", cards, word, m.cfg.Deck)))
	if m.cfg.PreviewLink != nil {
		if link := m.cfg.PreviewLink(m.selLineIndex); link != "" {
			b.WriteString("\n" + helpStyle.Render("link: "+link))
		}
	}
	b.WriteString("\n")

	if m.cfg.Dict != nil {
		b.WriteString(m.ps.renderPreviews(m.sentence))
	}

	if m.state == stateRefine && m.refineIdx < len(m.ps.phrases) {
		b.WriteString(renderRefinePrompt(m.ps.phraseText(m.sentence, m.ps.phrases[m.refineIdx]), m.refineInput))
	}

	if m.state == stateSubmitting {
		b.WriteString("\nsubmitting...\n")
	}
	return "\n" + b.String() + "\n"
}
