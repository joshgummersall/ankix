package tui

import (
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// phraseStyle picks between the two alternating phrase colors (so adjacent
// but separately-selected phrases stay visually distinct) and their cursor
// variants.
func phraseStyle(toggle, isCursor bool) lipgloss.Style {
	switch {
	case isCursor && toggle:
		return markedWordCursorStyleB
	case isCursor:
		return markedWordCursorStyleA
	case toggle:
		return markedWordStyleB
	default:
		return markedWordStyleA
	}
}

// expandDebounce is how long expand mode waits after the last grow/shrink
// key before kicking off a lookup for the phrase's current bounds, so
// rapid-fire growing doesn't fire a lookup per keystroke.
const expandDebounce = 350 * time.Millisecond

// phrase is a live, editable word/phrase within the sentence currently being
// reviewed, shared by both the YouTube and Kindle sentence editors. lo/hi
// are word-token indices (into phraseSet.wordTokens), inclusive. defaultLo/Hi
// record its original single-word position so deleting and later re-adding
// it resets cleanly. deleted fully clears the word's selection state —
// excluded from the batch, rendered with no highlight, as if it had never
// been added. mergedInto is -1 for a standalone phrase; otherwise it's the
// index of another phrase this one was absorbed into after an expansion made
// the two overlap, and lo/hi/deleted are stale. payload carries whatever
// caller-specific data a phrase needs (e.g. a kindle.Entry); callers with no
// such data use struct{}.
type phrase[T any] struct {
	lo, hi               int
	defaultLo, defaultHi int
	deleted              bool
	mergedInto           int
	payload              T

	// previewText is the phrase text the preview below applies to — once
	// lo/hi/text changes, it no longer matches and a fresh lookup is due
	// (see refreshPreviews).
	previewText    string
	preview        string
	previewErr     error
	previewPending bool
}

// phraseSet is the phrase-selection engine shared by the YouTube and Kindle
// sentence editors: tokenizing a sentence, tracking a word cursor over it,
// and growing/shrinking/merging a set of live phrases as the user edits
// their selection.
type phraseSet[T any] struct {
	tokens     []token
	wordTokens []int // indices into tokens that are words
	wordCursor int   // index into wordTokens

	phrases []phrase[T]

	// expandIdx is the phrase currently being grown/shrunk (valid only
	// while expanding); expandOrigLo/Hi is its pre-expansion range, restored
	// on cancelExpand. expandIsNew marks a phrase created fresh by
	// beginExpand, so cancelExpand can remove it entirely instead of just
	// reverting its range.
	expandIdx                  int
	expandOrigLo, expandOrigHi int
	expandIsNew                bool
	debounceGen                int // bumped on every expand-mode edit; a stale debounce fire is ignored
}

// reset (re)tokenizes sentence and clears every phrase, positioning the
// cursor at the first word. Used when a sentence is set or edited from
// scratch (contrast with Kindle's resetPhrasesForSentence, which rebuilds
// phrases from known word positions instead of clearing them).
func (ps *phraseSet[T]) reset(sentence string) {
	ps.tokens = tokenize(sentence)
	ps.phrases = nil
	ps.setWordTokens()
	ps.wordCursor = 0
}

// setWordTokens rebuilds wordTokens from tokens; callers that reset phrases
// themselves (e.g. Kindle's resetPhrasesForSentence) call this directly
// instead of reset.
func (ps *phraseSet[T]) setWordTokens() {
	ps.wordTokens = ps.wordTokens[:0]
	for i, t := range ps.tokens {
		if t.isWord {
			ps.wordTokens = append(ps.wordTokens, i)
		}
	}
}

// phraseAt returns the index of the standalone (non-deleted, non-merged)
// phrase whose range contains pos, or (-1, false) if there isn't one.
func (ps *phraseSet[T]) phraseAt(pos int) (int, bool) {
	for i, p := range ps.phrases {
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		if pos >= p.lo && pos <= p.hi {
			return i, true
		}
	}
	return -1, false
}

func (ps *phraseSet[T]) phraseAtCursor() (int, bool) {
	return ps.phraseAt(ps.wordCursor)
}

// deletedPhraseAtCursor returns the index of a deleted, standalone phrase
// whose original (pre-deletion) word position is under the cursor, or
// (-1, false) if there isn't one — used so starting an expand on a deleted
// word restores it instead of creating a brand new phrase alongside it.
func (ps *phraseSet[T]) deletedPhraseAtCursor() (int, bool) {
	for i, p := range ps.phrases {
		if p.mergedInto != -1 || !p.deleted {
			continue
		}
		if ps.wordCursor >= p.defaultLo && ps.wordCursor <= p.defaultHi {
			return i, true
		}
	}
	return -1, false
}

// nearestPhrase returns the index into phrases whose range is closest to
// wordCursor (0 if the cursor is already within it), ties broken toward the
// lowest index. Deleted/merged phrases are ignored.
func (ps *phraseSet[T]) nearestPhrase() int {
	best, bestDist := -1, -1
	for i, p := range ps.phrases {
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		dist := 0
		switch {
		case ps.wordCursor < p.lo:
			dist = p.lo - ps.wordCursor
		case ps.wordCursor > p.hi:
			dist = ps.wordCursor - p.hi
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
// cursor, carrying payload, and returns its index (always the new last
// element of phrases).
func (ps *phraseSet[T]) addPhraseAtCursor(payload T) int {
	ps.phrases = append(ps.phrases, phrase[T]{
		lo: ps.wordCursor, hi: ps.wordCursor,
		defaultLo: ps.wordCursor, defaultHi: ps.wordCursor,
		payload: payload, mergedInto: -1,
	})
	return len(ps.phrases) - 1
}

// mergeOverlaps folds any other standalone phrase whose range now overlaps
// phrases[i] into phrases[i], growing i's range to cover the union. Called
// after every expansion so merges happen live as soon as two words' phrases
// touch.
func (ps *phraseSet[T]) mergeOverlaps(i int) {
	for j := range ps.phrases {
		if j == i || ps.phrases[j].mergedInto != -1 || ps.phrases[j].deleted {
			continue
		}
		if ps.phrases[j].hi < ps.phrases[i].lo || ps.phrases[j].lo > ps.phrases[i].hi {
			continue
		}
		if ps.phrases[j].lo < ps.phrases[i].lo {
			ps.phrases[i].lo = ps.phrases[j].lo
		}
		if ps.phrases[j].hi > ps.phrases[i].hi {
			ps.phrases[i].hi = ps.phrases[j].hi
		}
		for k := range ps.phrases {
			if ps.phrases[k].mergedInto == j {
				ps.phrases[k].mergedInto = i
			}
		}
		ps.phrases[j].mergedInto = i
	}
}

// moveCursorRight/moveCursorLeft implement h/l navigation in pick mode:
// jumping past whichever phrase the cursor starts on (if any), then snapping
// onto the far edge of whichever phrase it lands on — so a single keypress
// exits or enters a whole phrase instead of stepping word by word through
// it.
func (ps *phraseSet[T]) moveCursorRight() {
	next := ps.wordCursor + 1
	if i, ok := ps.phraseAt(ps.wordCursor); ok {
		next = ps.phrases[i].hi + 1
	}
	if next > len(ps.wordTokens)-1 {
		next = len(ps.wordTokens) - 1
	}
	if i, ok := ps.phraseAt(next); ok && ps.phrases[i].hi > ps.phrases[i].lo {
		next = ps.phrases[i].hi
	}
	ps.wordCursor = next
}

func (ps *phraseSet[T]) moveCursorLeft() {
	next := ps.wordCursor - 1
	if i, ok := ps.phraseAt(ps.wordCursor); ok {
		next = ps.phrases[i].lo - 1
	}
	if next < 0 {
		next = 0
	}
	if i, ok := ps.phraseAt(next); ok && ps.phrases[i].hi > ps.phrases[i].lo {
		next = ps.phrases[i].lo
	}
	ps.wordCursor = next
}

// growRight/growLeft/shrinkRight/shrinkLeft implement l/h/L/H in expand
// mode, operating on phrases[expandIdx].
func (ps *phraseSet[T]) growRight() {
	if ps.phrases[ps.expandIdx].hi < len(ps.wordTokens)-1 {
		ps.phrases[ps.expandIdx].hi++
	}
	ps.mergeOverlaps(ps.expandIdx)
	ps.wordCursor = ps.phrases[ps.expandIdx].hi
}

func (ps *phraseSet[T]) shrinkRight() {
	if ps.phrases[ps.expandIdx].hi > ps.phrases[ps.expandIdx].lo {
		ps.phrases[ps.expandIdx].hi--
	}
	ps.wordCursor = ps.phrases[ps.expandIdx].hi
}

func (ps *phraseSet[T]) growLeft() {
	if ps.phrases[ps.expandIdx].lo > 0 {
		ps.phrases[ps.expandIdx].lo--
	}
	ps.mergeOverlaps(ps.expandIdx)
	ps.wordCursor = ps.phrases[ps.expandIdx].lo
}

func (ps *phraseSet[T]) shrinkLeft() {
	if ps.phrases[ps.expandIdx].lo < ps.phrases[ps.expandIdx].hi {
		ps.phrases[ps.expandIdx].lo++
	}
	ps.wordCursor = ps.phrases[ps.expandIdx].lo
}

// beginExpand starts expand mode for the word under the cursor: restoring a
// deleted phrase, expanding an existing one, or adding a new single-word
// phrase carrying payload (used only if a new phrase is created). Returns
// the phrase's index.
func (ps *phraseSet[T]) beginExpand(payload T) int {
	var i int
	var onPhrase bool
	if i, onPhrase = ps.deletedPhraseAtCursor(); onPhrase {
		ps.phrases[i].deleted = false
		ps.phrases[i].lo, ps.phrases[i].hi = ps.phrases[i].defaultLo, ps.phrases[i].defaultHi
	} else if i, onPhrase = ps.phraseAtCursor(); !onPhrase {
		i = ps.addPhraseAtCursor(payload)
	}
	ps.expandIsNew = !onPhrase
	ps.expandIdx = i
	ps.expandOrigLo, ps.expandOrigHi = ps.phrases[i].lo, ps.phrases[i].hi
	return i
}

// cancelExpand reverts the phrase being expanded to its pre-expand state (or
// removes it entirely if it was newly added), un-merging anything absorbed
// into it along the way.
func (ps *phraseSet[T]) cancelExpand() {
	for j := range ps.phrases {
		if ps.phrases[j].mergedInto == ps.expandIdx {
			ps.phrases[j].mergedInto = -1
		}
	}
	if ps.expandIsNew {
		ps.phrases = ps.phrases[:ps.expandIdx]
	} else {
		ps.phrases[ps.expandIdx].lo = ps.expandOrigLo
		ps.phrases[ps.expandIdx].hi = ps.expandOrigHi
	}
}

// deleteNearestPhrase clears the selection state of the phrase nearest the
// cursor, as if it had never been marked.
func (ps *phraseSet[T]) deleteNearestPhrase() {
	if len(ps.phrases) == 0 {
		return
	}
	i := ps.nearestPhrase()
	ps.phrases[i].deleted = true
	ps.phrases[i].previewText = ""
	ps.phrases[i].preview = ""
	ps.phrases[i].previewErr = nil
	ps.phrases[i].previewPending = false
}

// refreshPreviews kicks off a lookup for every non-deleted, standalone
// phrase whose current text hasn't been looked up yet (or has changed since
// it last was, e.g. after an expansion or merge), so a preview of what will
// be saved is visible before submitting. text extracts a phrase's current
// sentence text from its lo/hi range; lookup issues the actual command.
func (ps *phraseSet[T]) refreshPreviews(text func(p *phrase[T]) string, lookup func(i int, text string) tea.Cmd) tea.Cmd {
	var cmds []tea.Cmd
	for i := range ps.phrases {
		p := &ps.phrases[i]
		if p.mergedInto != -1 || p.deleted {
			continue
		}
		t := text(p)
		if p.previewText == t {
			continue
		}
		p.previewText = t
		p.preview = ""
		p.previewErr = nil
		p.previewPending = true
		cmds = append(cmds, lookup(i, t))
	}
	return tea.Batch(cmds...)
}

// debounceExpandMsg fires expandDebounce after an expand-mode edit; gen ties
// it to the edit that scheduled it, so an edit that happened afterward
// (which bumped debounceGen again) makes it a no-op.
type debounceExpandMsg struct {
	gen int
}

func (ps *phraseSet[T]) debounceRefresh() tea.Cmd {
	ps.debounceGen++
	gen := ps.debounceGen
	return tea.Tick(expandDebounce, func(time.Time) tea.Msg {
		return debounceExpandMsg{gen: gen}
	})
}

// render renders sentence with every phrase's background highlight spanning
// its full text (including the separators between its words) and the
// cursor's style applied to whichever phrase (or bare word) it's currently
// on.
func (ps *phraseSet[T]) render(sentence string) string {
	var b strings.Builder

	// nextWordPhrase looks ahead to the word position that follows a given
	// token index, so separators between two words of the same phrase can
	// be highlighted too (otherwise a multi-word phrase renders as
	// disjoint per-word chips instead of one continuous highlight).
	nextWordPhrase := func(fromToken, fromWordPos int) (int, bool) {
		wp := fromWordPos
		for _, t := range ps.tokens[fromToken:] {
			if t.isWord {
				wp++
				return ps.phraseAt(wp)
			}
		}
		return 0, false
	}

	// cursorPhrase is the phrase (if any) the cursor currently sits in, so
	// the whole phrase gets the cursor style rather than just the one word
	// the cursor happens to be on.
	cursorPhrase, cursorInPhrase := ps.phraseAt(ps.wordCursor)

	wordPos := -1
	lastPhrase, toggle := -1, false
	for idx, t := range ps.tokens {
		text := sentence[t.start:t.end]
		if t.isWord {
			wordPos++
			if i, ok := ps.phraseAt(wordPos); ok {
				if i != lastPhrase {
					toggle = !toggle
					lastPhrase = i
				}
				text = phraseStyle(toggle, cursorInPhrase && i == cursorPhrase).Render(text)
			} else if wordPos == ps.wordCursor {
				text = wordCursorStyle.Render(text)
			}
		} else if prevPhrase, ok := ps.phraseAt(wordPos); ok {
			if nextPhrase, ok := nextWordPhrase(idx+1, wordPos); ok && nextPhrase == prevPhrase {
				text = phraseStyle(toggle, cursorInPhrase && prevPhrase == cursorPhrase).Render(text)
			}
		}
		b.WriteString(text)
	}
	return b.String()
}
