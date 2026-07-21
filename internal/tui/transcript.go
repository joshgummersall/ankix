package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q", "ctrl+c":
			m.showHelp = false
		}
		return m, nil
	}

	if m.searching {
		return m.handleSearchKey(msg)
	}

	if msg.String() == "?" && (m.state == stateBrowse || m.state == stateVisual) {
		m.showHelp = true
		return m, nil
	}

	switch m.state {
	case stateBrowse, stateVisual:
		return m.handleBrowseKey(msg)
	case stateWordPick:
		return m.handleWordPickKey(msg)
	case stateWordExpand:
		return m.handleWordExpandKey(msg)
	case stateEditSentence:
		return m.handleEditSentenceKey(msg)
	}
	return m, nil
}

func (m Model) handleBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// gg needs two-key lookahead.
	if m.pendingG {
		m.pendingG = false
		if k == "g" {
			m.cursorWord = 0
			m.syncViewport()
			return m, nil
		}
	}

	switch k {
	case "q", "ctrl+c":
		if m.state == stateBrowse {
			return m, tea.Quit
		}
		m.state = stateBrowse
		m.syncViewport()
		return m, nil
	case "esc":
		m.state = stateBrowse
		m.syncViewport()
		return m, nil
	case "l", "right":
		m.moveCursorWord(1)
		return m, nil
	case "h", "left":
		m.moveCursorWord(-1)
		return m, nil
	case "j", "down":
		m.moveCursorLine(1)
		return m, nil
	case "k", "up":
		m.moveCursorLine(-1)
		return m, nil
	case "ctrl+d":
		m.moveCursorLine(m.halfPageLines())
		return m, nil
	case "ctrl+u":
		m.moveCursorLine(-m.halfPageLines())
		return m, nil
	case "g":
		m.pendingG = true
		return m, nil
	case "G":
		m.cursorWord = len(m.words) - 1
		m.syncViewport()
		return m, nil
	case "/":
		m.searching = true
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, nil
	case "n":
		m.jumpToNextMatch(1)
		return m, nil
	case "N":
		m.jumpToNextMatch(-1)
		return m, nil
	case "v":
		if m.state == stateVisual {
			m.state = stateBrowse
		} else {
			m.state = stateVisual
			m.visualAnchor = m.cursorWord
		}
		m.syncViewport()
		return m, nil
	case "enter":
		if m.state == stateVisual {
			m.selWordStart, m.selWordEnd = m.visualBounds()
		} else {
			m.selWordStart, m.selWordEnd = m.cursorWord, m.cursorWord
		}
		m.enterWordPick()
		return m, nil
	}
	return m, nil
}

// visualBounds returns the current visual selection as an inclusive [lo, hi]
// word range: anchored at m.visualAnchor (set when visual mode began) with
// the cursor at the other end, vim-visual-mode style — so moving the cursor
// past the anchor flips which side it's on rather than needing a separate
// shrink key. Valid only while m.state == stateVisual.
func (m Model) visualBounds() (int, int) {
	lo, hi := m.visualAnchor, m.cursorWord
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func (m *Model) moveCursorWord(delta int) {
	m.cursorWord += delta
	if m.cursorWord < 0 {
		m.cursorWord = 0
	}
	if max := len(m.words) - 1; m.cursorWord > max {
		m.cursorWord = max
	}
	m.syncViewport()
}

// moveCursorLine jumps the cursor to the first word of the next/previous
// line, for fast coarse navigation (h/l moves word-by-word instead).
func (m *Model) moveCursorLine(delta int) {
	line := m.lineOfCursor() + delta
	if line < 0 {
		line = 0
	}
	if max := len(m.cueFirstWord) - 1; line > max {
		line = max
	}
	m.cursorWord = m.cueFirstWord[line]
	m.syncViewport()
}

// halfPageLines returns half the viewport's height in lines (at least 1),
// for ctrl+u/ctrl+d half-page scrolling.
func (m Model) halfPageLines() int {
	if half := m.viewport.Height / 2; half > 0 {
		return half
	}
	return 1
}

func (m Model) lineOfCursor() int {
	return m.lineOfWord(m.cursorWord)
}

func (m Model) lineOfWord(wi int) int {
	if len(m.words) == 0 {
		return 0
	}
	return m.words[wi].CueIndex
}

// syncViewport refreshes the viewport's content (cursor/selection highlight
// depends on state, so it can't just be set once) and scrolls it so the
// cursor's line stays visible. This must run as part of Update, not View:
// View has a value receiver, so mutating m.viewport there (SetContent,
// SetYOffset, ...) would only mutate a throwaway copy and silently fail to
// persist between frames — the transcript would never actually scroll.
func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderTranscript())

	line := m.lineOfCursor()
	top := m.viewport.YOffset
	bottom := top + m.viewport.Height
	if line < top {
		m.viewport.SetYOffset(line)
	} else if line >= bottom {
		m.viewport.SetYOffset(line - m.viewport.Height + 1)
	}
}

func (m *Model) jumpToNextMatch(dir int) {
	if m.searchTerm == "" {
		return
	}
	cues := m.cfg.Transcript.Cues
	n := len(cues)
	curLine := m.lineOfCursor()
	for i := 1; i <= n; i++ {
		idx := ((curLine+dir*i)%n + n) % n
		if strings.Contains(strings.ToLower(cues[idx].Text), m.searchTerm) {
			m.cursorWord = m.cueFirstWord[idx]
			m.syncViewport()
			return
		}
	}
	m.setStatus(fmt.Sprintf("no match for %q", m.searchTerm), true)
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searching = false
		m.searchTerm = strings.ToLower(m.searchInput.Value())
		m.jumpToNextMatch(1)
		return m, nil
	case "esc":
		m.searching = false
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m Model) renderTranscript() string {
	var b strings.Builder
	cues := m.cfg.Transcript.Cues
	curLine := m.lineOfCursor()

	selLo, selHi := -1, -1
	if m.state == stateVisual {
		selLo, selHi = m.visualBounds()
	}

	for i, c := range cues {
		marker := "  "
		switch {
		case m.cardedLines[i]:
			marker = cardedMarkerStyle.Render("✓ ")
		case i == curLine:
			marker = currentLineMarkerStyle.Render("› ")
		}
		ts := timestampStyle.Render(fmt.Sprintf("%s ", formatTS(c.Start)))

		start := m.cueFirstWord[i]
		end := len(m.words)
		if i+1 < len(m.cueFirstWord) {
			end = m.cueFirstWord[i+1]
		}

		var words strings.Builder
		for wi := start; wi < end; wi++ {
			if wi > start {
				// Style the separator too when both neighboring words share
				// the same highlight, so a multi-word selection (or a
				// carded line) reads as one continuous background block
				// instead of disjoint per-word chips.
				sep := " "
				switch {
				case selLo != -1 && wi-1 >= selLo && wi <= selHi:
					sep = activeSelectionStyle.Render(sep)
				case m.cardedLines[i]:
					sep = cardedWordStyle.Render(sep)
				}
				words.WriteString(sep)
			}
			text := m.words[wi].Text
			switch {
			case selLo != -1 && wi >= selLo && wi <= selHi:
				if wi == m.cursorWord {
					text = activeSelectionCursorStyle.Render(text)
				} else {
					text = activeSelectionStyle.Render(text)
				}
			case wi == m.cursorWord:
				text = wordCursorStyle.Render(text)
			case m.cardedLines[i]:
				text = cardedWordStyle.Render(text)
			}
			words.WriteString(text)
		}

		b.WriteString(marker + ts + words.String())
		b.WriteString("\n")
	}
	return b.String()
}

func formatTS(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}
