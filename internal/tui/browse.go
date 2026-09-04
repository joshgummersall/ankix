package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/joshgummersall/ankix/internal/position"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		sections := m.helpSections()
		rows, _ := helpViewport(m.width, m.height)
		m.helpScroll, m.showHelp = handleHelpKey(msg.String(), m.helpScroll, helpMaxScroll(m.width, m.height, sections), rows)
		return m, nil
	}

	if m.searching {
		return m.handleSearchKey(msg)
	}

	// The two text-entry states swallow `?` as a literal character, so help
	// is reachable from every other state.
	if msg.String() == "?" && m.state != stateEditSentence && m.state != stateRefine {
		m.showHelp = true
		m.helpScroll = 0
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
	case stateRefine:
		return m.handleRefineKey(msg)
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
	case ")":
		m.jumpToSentence(1)
		return m, nil
	case "(":
		m.jumpToSentence(-1)
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
	case "V":
		m.state = stateVisual
		m.visualAnchor, m.cursorWord = m.sentenceBounds(m.cursorWord)
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

// moveCursorLine moves the cursor delta visual rows down (or up), like
// vim's gj/gk: a long document line wraps across several rows on a narrow
// (or zoomed-in) terminal, and j should step through those rows rather than
// skip the whole line. Within the target row the cursor keeps as close to
// its current column as the words there allow.
func (m *Model) moveCursorLine(delta int) {
	rows := m.layout.wordRow
	if len(m.words) == 0 || len(rows) != len(m.words) {
		m.moveCursorDocLine(delta) // no layout yet — nothing has been rendered
		return
	}

	target := rows[m.cursorWord] + delta
	col := m.layout.wordCol[m.cursorWord]

	// rows is non-decreasing in word order, so the first word at or past
	// the target row is a binary search away. A row can hold no word at all
	// (the tail of a hard-broken long word), hence the nearest row in the
	// direction of travel rather than an exact match.
	i := sort.Search(len(rows), func(j int) bool { return rows[j] >= target })
	switch {
	case i == len(rows):
		i = len(rows) - 1
	case rows[i] > target && delta < 0 && i > 0:
		i--
	}

	lo, hi := i, i
	for lo > 0 && rows[lo-1] == rows[i] {
		lo--
	}
	for hi+1 < len(rows) && rows[hi+1] == rows[i] {
		hi++
	}
	best := lo
	for j := lo + 1; j <= hi; j++ {
		if absInt(m.layout.wordCol[j]-col) < absInt(m.layout.wordCol[best]-col) {
			best = j
		}
	}
	m.cursorWord = best
	m.syncViewport()
}

// moveCursorDocLine is the pre-layout fallback for moveCursorLine: jump to
// the first word of the next/previous document line.
func (m *Model) moveCursorDocLine(delta int) {
	line := m.lineOfCursor() + delta
	if line < 0 {
		line = 0
	}
	if max := len(m.lineFirstWord) - 1; line > max {
		line = max
	}
	if len(m.lineFirstWord) == 0 {
		return
	}
	m.cursorWord = m.lineFirstWord[line]
	m.syncViewport()
}

func absInt(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// halfPageLines returns half the viewport's height in lines (at least 1),
// for ctrl+u/ctrl+d half-page scrolling.
func (m Model) halfPageLines() int {
	if half := m.viewport.Height / 2; half > 0 {
		return half
	}
	return 1
}

// jumpToSentence moves the cursor to the next (dir > 0) or previous
// (dir < 0) sentence start — mirroring vim's )/( sentence motions. Line
// boundaries don't align with sentence boundaries here (a source's
// segmentation can break mid-sentence), so this walks m.words looking for
// the word after one ending in ./!/?, rather than jumping by line.
func (m *Model) jumpToSentence(dir int) {
	if len(m.words) == 0 {
		return
	}
	i := m.cursorWord
	for {
		i += dir
		if i <= 0 {
			i = 0
			break
		}
		if i >= len(m.words)-1 {
			i = len(m.words) - 1
			break
		}
		if m.isSentenceStart(i) {
			break
		}
	}
	m.cursorWord = i
	m.syncViewport()
}

// sentenceBounds returns the [start, end] word-index range (inclusive) of
// the sentence containing word i, so V can select a whole sentence in one
// keypress the same way vim's V selects a whole line.
func (m Model) sentenceBounds(i int) (start, end int) {
	start = i
	for start > 0 && !m.isSentenceStart(start) {
		start--
	}
	end = i
	for end < len(m.words)-1 && !endsSentence(m.words[end].Text) {
		end++
	}
	return start, end
}

// isSentenceStart reports whether word i begins a sentence: either it's the
// very first word, or the word before it ends a sentence.
func (m Model) isSentenceStart(i int) bool {
	if i <= 0 {
		return true
	}
	return endsSentence(m.words[i-1].Text)
}

// endsSentence reports whether w (a word as captured verbatim from the
// document, punctuation and all) ends a sentence: it ends in ./!/? once any
// trailing closing quote/bracket is stripped.
func endsSentence(w string) bool {
	w = strings.TrimRight(w, `"')]”’`)
	if w == "" {
		return false
	}
	switch w[len(w)-1] {
	case '.', '!', '?':
		return true
	}
	return false
}

func (m Model) lineOfCursor() int {
	return m.lineOfWord(m.cursorWord)
}

func (m Model) lineOfWord(wi int) int {
	if len(m.words) == 0 {
		return 0
	}
	return m.words[wi].LineIndex
}

// syncViewport refreshes the viewport's content (cursor/selection highlight
// depends on state, so it can't just be set once) and scrolls it so the
// cursor's line stays visible. This must run as part of Update, not View:
// View has a value receiver, so mutating m.viewport there (SetContent,
// SetYOffset, ...) would only mutate a throwaway copy and silently fail to
// persist between frames — the document would never actually scroll.
func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	content, layout := m.renderDocument()
	m.viewport.SetContent(content)
	m.layout = layout

	m.savePosition()

	if m.cursorWord < len(layout.wordRow) {
		m.scrollTo(layout.wordRow[m.cursorWord])
	}
}

// scrollTo scrolls the viewport so the cursor's visual line keeps at least
// half a screen of context above and below it — vim's `scrolloff` set high
// enough to pin the cursor mid-screen while the text moves past it. Near
// either end of the document there is no more content to scroll in, so
// SetYOffset's clamp takes over and the cursor walks to the top or bottom
// edge instead.
func (m *Model) scrollTo(line int) {
	if m.viewport.Height <= 0 {
		return
	}
	margin := (m.viewport.Height - 1) / 2
	top := m.viewport.YOffset
	// margin <= (Height-1)/2, so the two bounds never cross.
	top = max(top, line+margin-m.viewport.Height+1)
	top = min(top, line-margin)
	m.viewport.SetYOffset(top)
}

// savePosition persists the cursor's exact word as the resume point for
// this document, so quitting out (accidentally or otherwise) doesn't lose
// the reader's place. It writes on every cursor move rather than only at
// quit time, since a killed terminal or crash never gets a chance to run
// quit handling.
func (m *Model) savePosition() {
	if m.cfg.Document.SourceID == "" || m.cursorWord == m.lastSavedWord {
		return
	}
	m.lastSavedWord = m.cursorWord
	line := m.lineOfCursor()
	pos := position.Position{Line: line, Word: m.cursorWord - m.lineFirstWord[line]}
	_ = position.Save(m.cfg.Document.SourceID, pos) // best-effort; losing the resume point isn't worth surfacing an error over
}

func (m *Model) jumpToNextMatch(dir int) {
	if m.searchTerm == "" {
		return
	}
	lines := m.cfg.Document.Lines
	n := len(lines)
	curLine := m.lineOfCursor()
	for i := 1; i <= n; i++ {
		idx := ((curLine+dir*i)%n + n) % n
		if strings.Contains(strings.ToLower(lines[idx].Text), m.searchTerm) {
			m.cursorWord = m.lineFirstWord[idx]
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

// renderDocument renders every line and returns the full content along with
// the layout it produced — where each word ended up once the content was
// wrapped to the viewport width, which j/k and scrolling both navigate by.
func (m Model) renderDocument() (string, docLayout) {
	var b strings.Builder
	lines := m.cfg.Document.Lines
	curLine := m.lineOfCursor()
	layout := docLayout{
		wordRow: make([]int, len(m.words)),
		wordCol: make([]int, len(m.words)),
	}
	visual := 0

	selLo, selHi := -1, -1
	if m.state == stateVisual {
		selLo, selHi = m.visualBounds()
	}

	for i, l := range lines {
		start := m.lineFirstWord[i]
		end := len(m.words)
		if i+1 < len(m.lineFirstWord) {
			end = m.lineFirstWord[i+1]
		}

		lineCarded := false
		for wi := start; wi < end; wi++ {
			if m.cardedWords[wi] {
				lineCarded = true
				break
			}
		}

		markerText := "  "
		switch {
		case lineCarded:
			markerText = "✓ "
		case i == curLine:
			markerText = "› "
		}
		marker := markerText
		switch {
		case lineCarded:
			marker = cardedMarkerStyle.Render(markerText)
		case i == curLine:
			marker = currentLineMarkerStyle.Render(markerText)
		}
		tsText, ts := "", ""
		if l.Label != "" {
			tsText = l.Label + " "
			ts = timestampStyle.Render(tsText)
		}

		// plain mirrors the styled line without any escape sequences, and
		// wordOff records where each word starts in it, so the wrapped
		// render can be walked back onto individual words below.
		var plain strings.Builder
		plain.WriteString(markerText)
		plain.WriteString(tsText)
		wordOff := make([]int, 0, end-start)

		var words strings.Builder
		for wi := start; wi < end; wi++ {
			if wi > start {
				// Style the separator too when both neighboring words share
				// the same highlight, so a multi-word selection (or a
				// carded run) reads as one continuous background block
				// instead of disjoint per-word chips.
				sep := " "
				switch {
				case selLo != -1 && wi-1 >= selLo && wi <= selHi:
					sep = activeSelectionStyle.Render(sep)
				case m.cardedWords[wi-1] && m.cardedWords[wi]:
					sep = cardedWordStyle.Render(sep)
				}
				words.WriteString(sep)
				plain.WriteString(" ")
			}
			wordOff = append(wordOff, plain.Len())
			plain.WriteString(m.words[wi].Text)
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
			case m.cardedWords[wi]:
				text = cardedWordStyle.Render(text)
			}
			words.WriteString(text)
		}

		line := marker + ts + words.String()
		if m.viewport.Width > 0 {
			line = lipgloss.NewStyle().Width(m.viewport.Width).Render(line)
		}

		layout.assign(ansi.Strip(line), plain.String(), wordOff, start, visual)
		visual += strings.Count(line, "\n") + 1

		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), layout
}

// docLayout records where the rendered document put each word: the visual
// (post-wrap) row it starts on and the column within that row. One document
// line can fill a screenful of rows once wrapped, so this — not the line
// index — is what j/k steps through and what the viewport centers on.
type docLayout struct {
	wordRow []int
	wordCol []int
}

// assign walks one line's wrapped, ANSI-stripped render alongside its plain
// text and records the row and column each of its words landed on. The two
// strings only stay in step if the wrap's edits are skipped: breaking at a
// space consumes it, and a fixed Width pads every row out with spaces that
// were never in the text.
func (d docLayout) assign(wrapped, plain string, wordOff []int, firstWord, firstRow int) {
	p, row, col, next := 0, firstRow, 0, 0
	place := func() {
		for next < len(wordOff) && wordOff[next] == p {
			d.wordRow[firstWord+next] = row
			d.wordCol[firstWord+next] = col
			next++
		}
	}
	place()

	for _, r := range wrapped {
		if r == '\n' {
			row, col = row+1, 0
			for p < len(plain) && plain[p] == ' ' {
				p++
			}
			place()
			continue
		}
		pr, size := utf8.DecodeRuneInString(plain[p:])
		if size == 0 || r != pr {
			continue // padding, not text
		}
		p += size
		col++
		place()
	}
}
