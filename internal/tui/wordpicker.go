package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joshgummersall/ankix/internal/anki"
)

// token is a slice of the working sentence: either a word (matched by
// wordRe) or a separator (whitespace/punctuation) run between words.
type token struct {
	start, end int
	isWord     bool
}

var wordRe = regexp.MustCompile(`[\p{L}]+`)

func tokenize(s string) []token {
	var tokens []token
	last := 0
	for _, loc := range wordRe.FindAllStringIndex(s, -1) {
		if loc[0] > last {
			tokens = append(tokens, token{start: last, end: loc[0], isWord: false})
		}
		tokens = append(tokens, token{start: loc[0], end: loc[1], isWord: true})
		last = loc[1]
	}
	if last < len(s) {
		tokens = append(tokens, token{start: last, end: len(s), isWord: false})
	}
	return tokens
}

// wpDebounce is how long expand mode waits after the last tab/shift+tab
// before kicking off a gloss lookup for the phrase's current bounds, so
// rapid-fire growing doesn't fire a lookup per keystroke.
const wpDebounce = 350 * time.Millisecond

// wpPhrase is a live, editable word/phrase within the sentence currently
// being reviewed, mirroring internal/tui/kindlereview.go's kindlePhrase.
// lo/hi are word-token indices (into m.wordTokens), inclusive. defaultLo/Hi
// record its original single-word position so deleting and later
// re-adding it resets cleanly. deleted fully clears the word's selection
// state — excluded from the batch, rendered with no highlight, as if it
// had never been added. mergedInto is -1 for a standalone phrase;
// otherwise it's the index of another phrase this one was absorbed into
// after an expansion made the two overlap, and lo/hi/deleted are stale.
type wpPhrase struct {
	lo, hi               int
	defaultLo, defaultHi int
	deleted              bool
	mergedInto           int

	// Gloss lookup for the phrase's current text, so a preview can be shown
	// before submitting. glossText is the phrase text the lookup below
	// applies to — once lo/hi/text changes, it no longer matches and a
	// fresh lookup is due (see refreshGlosses).
	glossText    string
	gloss        string
	glossErr     error
	glossPending bool
}

// enterWordPick loads the selected transcript words into a fresh sentence
// with no phrases yet — every word must be added explicitly with v, unlike
// the Kindle review flow, which starts from already-looked-up candidates.
func (m *Model) enterWordPick() {
	var parts []string
	for i := m.selWordStart; i <= m.selWordEnd; i++ {
		parts = append(parts, m.words[i].Text)
	}
	m.sentence = strings.Join(parts, " ")
	m.cueStart = m.cfg.Transcript.Cues[m.words[m.selWordStart].CueIndex].Start
	m.tokens = tokenize(m.sentence)
	m.phrases = nil
	m.setStatus("", false)

	m.wordTokens = m.wordTokens[:0]
	for i, t := range m.tokens {
		if t.isWord {
			m.wordTokens = append(m.wordTokens, i)
		}
	}
	m.wordCursor = 0
	m.state = stateWordPick
}

// phraseAt returns the index of the standalone (non-deleted, non-merged)
// phrase whose range contains pos, or (-1, false) if there isn't one.
func (m *Model) phraseAt(pos int) (int, bool) {
	for i, p := range m.phrases {
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		if pos >= p.lo && pos <= p.hi {
			return i, true
		}
	}
	return -1, false
}

func (m *Model) phraseAtCursor() (int, bool) {
	return m.phraseAt(m.wordCursor)
}

// deletedPhraseAtCursor returns the index of a deleted, standalone phrase
// whose original (pre-deletion) word position is under the cursor, or
// (-1, false) if there isn't one — used so v on a deleted word restores it
// instead of creating a brand new phrase alongside it.
func (m *Model) deletedPhraseAtCursor() (int, bool) {
	for i, p := range m.phrases {
		if p.mergedInto != -1 || !p.deleted {
			continue
		}
		if m.wordCursor >= p.defaultLo && m.wordCursor <= p.defaultHi {
			return i, true
		}
	}
	return -1, false
}

// nearestPhrase returns the index into m.phrases whose range is closest to
// m.wordCursor (0 if the cursor is already within it), ties broken toward
// the lowest index. Deleted/merged phrases are ignored.
func (m *Model) nearestPhrase() int {
	best, bestDist := -1, -1
	for i, p := range m.phrases {
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		dist := 0
		switch {
		case m.wordCursor < p.lo:
			dist = p.lo - m.wordCursor
		case m.wordCursor > p.hi:
			dist = m.wordCursor - p.hi
		}
		if bestDist == -1 || dist < bestDist {
			best, bestDist = i, dist
		}
	}
	if best == -1 {
		return 0
	}
	return best
}

// addPhraseAtCursor adds a new single-word phrase for the word under the
// cursor and returns its index (always the new last element of m.phrases).
func (m *Model) addPhraseAtCursor() int {
	m.phrases = append(m.phrases, wpPhrase{lo: m.wordCursor, hi: m.wordCursor, defaultLo: m.wordCursor, defaultHi: m.wordCursor, mergedInto: -1})
	return len(m.phrases) - 1
}

// mergeOverlaps folds any other standalone phrase whose range now overlaps
// phrases[i] into phrases[i], growing i's range to cover the union. Called
// after every expansion so merges happen live as soon as two words' phrases
// touch.
func (m *Model) mergeOverlaps(i int) {
	for j := range m.phrases {
		if j == i || m.phrases[j].mergedInto != -1 || m.phrases[j].deleted {
			continue
		}
		if m.phrases[j].hi < m.phrases[i].lo || m.phrases[j].lo > m.phrases[i].hi {
			continue
		}
		if m.phrases[j].lo < m.phrases[i].lo {
			m.phrases[i].lo = m.phrases[j].lo
		}
		if m.phrases[j].hi > m.phrases[i].hi {
			m.phrases[i].hi = m.phrases[j].hi
		}
		for k := range m.phrases {
			if m.phrases[k].mergedInto == j {
				m.phrases[k].mergedInto = i
			}
		}
		m.phrases[j].mergedInto = i
	}
}

// refreshGlosses kicks off a gloss lookup for every non-deleted, standalone
// phrase whose current text hasn't been looked up yet (or has changed
// since it last was, e.g. after an expansion or merge), so a preview of
// what will be saved is visible before submitting.
func (m *Model) refreshGlosses() tea.Cmd {
	if m.cfg.Translator == nil {
		return nil
	}
	var cmds []tea.Cmd
	for i := range m.phrases {
		p := &m.phrases[i]
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		text := m.sentence[m.tokens[m.wordTokens[p.lo]].start:m.tokens[m.wordTokens[p.hi]].end]
		if p.glossText == text {
			continue
		}
		p.glossText = text
		p.gloss = ""
		p.glossErr = nil
		p.glossPending = true
		cmds = append(cmds, fetchGlossCmd(m.cfg.Translator, text, m.sentence, i, text))
	}
	return tea.Batch(cmds...)
}

func (m *Model) debounceGlossRefresh() tea.Cmd {
	m.debounceGen++
	gen := m.debounceGen
	return tea.Tick(wpDebounce, func(time.Time) tea.Msg {
		return wpDebounceMsg{gen: gen}
	})
}

type wpDebounceMsg struct {
	gen int
}

func (m Model) handleWordPickKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateBrowse
		m.setStatus("", false)
		m.syncViewport()
		return m, nil
	case "l", "right", "tab":
		next := m.wordCursor + 1
		if next > len(m.wordTokens)-1 {
			next = len(m.wordTokens) - 1
		}
		if i, ok := m.phraseAt(next); ok && m.phrases[i].hi > m.phrases[i].lo {
			next = m.phrases[i].hi
		}
		m.wordCursor = next
		return m, nil
	case "h", "left", "shift+tab":
		next := m.wordCursor - 1
		if next < 0 {
			next = 0
		}
		if i, ok := m.phraseAt(next); ok && m.phrases[i].hi > m.phrases[i].lo {
			next = m.phrases[i].lo
		}
		m.wordCursor = next
		return m, nil
	case "e":
		return m, m.enterEditSentence()
	case "v":
		if len(m.wordTokens) == 0 {
			return m, nil
		}
		var i int
		var onPhrase bool
		if i, onPhrase = m.deletedPhraseAtCursor(); onPhrase {
			m.phrases[i].deleted = false
			m.phrases[i].lo, m.phrases[i].hi = m.phrases[i].defaultLo, m.phrases[i].defaultHi
		} else if i, onPhrase = m.phraseAtCursor(); !onPhrase {
			i = m.addPhraseAtCursor()
		}
		m.expandIsNew = !onPhrase
		m.expandIdx = i
		m.expandOrigLo, m.expandOrigHi = m.phrases[i].lo, m.phrases[i].hi
		m.state = stateWordExpand
		m.setStatus("tab grow right, shift+tab grow left, enter confirm, esc cancel", false)
		return m, m.debounceGlossRefresh()
	case "d":
		if len(m.phrases) == 0 {
			return m, nil
		}
		i := m.nearestPhrase()
		m.phrases[i].deleted = true
		m.phrases[i].glossText = ""
		m.phrases[i].gloss = ""
		m.phrases[i].glossErr = nil
		m.phrases[i].glossPending = false
		m.setStatus("", false)
		return m, nil
	case "enter":
		return m.submitWordPick()
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// submitWordPick builds a card for every non-deleted phrase in the
// sentence and submits them all in one action.
func (m Model) submitWordPick() (tea.Model, tea.Cmd) {
	for _, p := range m.phrases {
		if p.mergedInto == -1 && !p.deleted && p.glossPending {
			m.setStatus("still looking up glosses...", false)
			return m, nil
		}
	}

	var notes []anki.Note
	for _, p := range m.phrases {
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		start := m.tokens[m.wordTokens[p.lo]].start
		end := m.tokens[m.wordTokens[p.hi]].end
		sel := anki.WordSelection{Start: start, End: end, Gloss: p.gloss}
		notes = append(notes, anki.BuildWordNote(m.cfg.Deck, m.cfg.VideoTitle, m.cfg.Transcript.VideoID, m.cueStart, m.sentence, sel))
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
	case "l", "right", "tab":
		if m.phrases[m.expandIdx].hi < len(m.wordTokens)-1 {
			m.phrases[m.expandIdx].hi++
		}
		m.mergeOverlaps(m.expandIdx)
		m.wordCursor = m.phrases[m.expandIdx].hi
		return m, m.debounceGlossRefresh()
	case "h", "left", "shift+tab":
		if m.phrases[m.expandIdx].lo > 0 {
			m.phrases[m.expandIdx].lo--
		}
		m.mergeOverlaps(m.expandIdx)
		m.wordCursor = m.phrases[m.expandIdx].lo
		return m, m.debounceGlossRefresh()
	case "esc":
		for j := range m.phrases {
			if m.phrases[j].mergedInto == m.expandIdx {
				m.phrases[j].mergedInto = -1
			}
		}
		if m.expandIsNew {
			m.phrases = m.phrases[:m.expandIdx]
		} else {
			m.phrases[m.expandIdx].lo = m.expandOrigLo
			m.phrases[m.expandIdx].hi = m.expandOrigHi
		}
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

	wordPos := -1
	for _, t := range m.tokens {
		text := m.sentence[t.start:t.end]
		if t.isWord {
			wordPos++
			if style, ok := m.styleForWord(wordPos); ok {
				text = style.Render(text)
			}
		}
		b.WriteString(text)
	}

	cards := 0
	for _, p := range m.phrases {
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
	if link := anki.VideoLink(m.cfg.Transcript.VideoID, m.cueStart); link != "" {
		b.WriteString("\n" + helpStyle.Render("link: "+link))
	}
	b.WriteString("\n")

	if m.cfg.Translator != nil {
		for _, p := range m.phrases {
			if p.mergedInto != -1 || p.deleted {
				continue
			}
			phrase := m.sentence[m.tokens[m.wordTokens[p.lo]].start:m.tokens[m.wordTokens[p.hi]].end]
			switch {
			case p.glossPending:
				fmt.Fprintf(&b, "%s: looking up...\n", phrase)
			case p.glossErr != nil:
				fmt.Fprintf(&b, "%s: lookup failed (%v)\n", phrase, p.glossErr)
			case p.gloss == "":
				fmt.Fprintf(&b, "%s: (none)\n", phrase)
			default:
				fmt.Fprintf(&b, "%s: %s\n", phrase, p.gloss)
			}
		}
	}

	if m.state == stateSubmitting {
		b.WriteString("\nsubmitting...\n")
	}
	return "\n" + b.String() + "\n"
}

// styleForWord returns the style (if any) for the word token at wordPos.
// A word that's part of a non-deleted phrase always renders as part of
// that phrase's block, even if the cursor sits on it — an underline marks
// exactly where the cursor is within the block, rather than swapping that
// one word to the plain cursor style, which would visually split the
// phrase in two. Deleted words get no style at all, so they render
// indistinguishably from a word that was never selected.
func (m Model) styleForWord(wordPos int) (lipgloss.Style, bool) {
	for _, p := range m.phrases {
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		if wordPos >= p.lo && wordPos <= p.hi {
			if wordPos == m.wordCursor {
				return markedWordCursorStyle, true
			}
			return markedWordStyle, true
		}
	}
	if wordPos == m.wordCursor {
		return wordCursorStyle, true
	}
	return lipgloss.Style{}, false
}
