package tui

import (
	"fmt"
	"sort"
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

// refreshGlosses kicks off a gloss lookup for every non-deleted, standalone
// phrase whose current text hasn't been looked up yet (or has changed since
// it last was, e.g. after an expansion or merge), so a preview of what will
// be saved is visible before submitting.
func (m *Model) refreshGlosses() tea.Cmd {
	if m.cfg.Translator == nil {
		return nil
	}
	text := func(p *phrase[struct{}]) string {
		return m.sentence[m.ps.tokens[m.ps.wordTokens[p.lo]].start:m.ps.tokens[m.ps.wordTokens[p.hi]].end]
	}
	lookup := func(i int, text string) tea.Cmd {
		return fetchGlossCmd(m.cfg.Translator, text, m.sentence, i, text)
	}
	return m.ps.refreshPreviews(text, lookup)
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
	case "e":
		return m, m.enterEditSentence()
	case "v":
		if len(m.ps.wordTokens) == 0 {
			return m, nil
		}
		m.ps.beginExpand(struct{}{})
		m.state = stateWordExpand
		m.setStatus("h/l extend selection, enter confirm, esc cancel", false)
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
	for _, p := range m.ps.phrases {
		if p.mergedInto == -1 && !p.deleted && p.previewPending {
			m.setStatus("still looking up glosses...", false)
			return m, nil
		}
	}

	var notes []anki.Note
	for _, p := range m.ps.phrases {
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		start := m.ps.tokens[m.ps.wordTokens[p.lo]].start
		end := m.ps.tokens[m.ps.wordTokens[p.hi]].end
		sel := anki.WordSelection{Start: start, End: end, Gloss: p.preview}
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
	switch msg.String() {
	case "l", "right":
		m.ps.moveExpandCursor(1)
		return m, m.ps.debounceRefresh()
	case "h", "left":
		m.ps.moveExpandCursor(-1)
		return m, m.ps.debounceRefresh()
	case "esc":
		m.ps.cancelExpand()
		m.state = stateWordPick
		m.setStatus("", false)
		return m, m.refreshGlosses()
	case "enter":
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

	cards := 0
	for _, p := range m.ps.phrases {
		if p.mergedInto == -1 && !p.deleted {
			cards++
		}
	}
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

	if m.cfg.Translator != nil {
		ordered := make([]phrase[struct{}], len(m.ps.phrases))
		copy(ordered, m.ps.phrases)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].lo < ordered[j].lo })
		for _, p := range ordered {
			if p.mergedInto != -1 || p.deleted {
				continue
			}
			text := m.sentence[m.ps.tokens[m.ps.wordTokens[p.lo]].start:m.ps.tokens[m.ps.wordTokens[p.hi]].end]
			switch {
			case p.previewPending:
				fmt.Fprintf(&b, "%s: looking up...\n", text)
			case p.previewErr != nil:
				fmt.Fprintf(&b, "%s: lookup failed (%v)\n", text, p.previewErr)
			case p.preview == "":
				fmt.Fprintf(&b, "%s: (none)\n", text)
			default:
				fmt.Fprintf(&b, "%s: %s\n", text, p.preview)
			}
		}
	}

	if m.state == stateSubmitting {
		b.WriteString("\nsubmitting...\n")
	}
	return "\n" + b.String() + "\n"
}
