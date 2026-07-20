package kindle

import "strings"

// SentenceGroup collects every entry that shares the same usage sentence, so
// the review UI can show the sentence once with all of its candidate words,
// instead of repeating the sentence once per word.
type SentenceGroup struct {
	Usage   string
	Entries []Entry
}

// GroupBySentence groups entries by their Usage text (trimmed of leading and
// trailing whitespace), preserving the order in which each distinct sentence
// first appears.
func GroupBySentence(entries []Entry) []SentenceGroup {
	pos := make(map[string]int)
	var groups []SentenceGroup
	for _, e := range entries {
		key := strings.TrimSpace(e.Usage)
		if i, ok := pos[key]; ok {
			groups[i].Entries = append(groups[i].Entries, e)
			continue
		}
		pos[key] = len(groups)
		groups = append(groups, SentenceGroup{Usage: e.Usage, Entries: []Entry{e}})
	}
	return groups
}
