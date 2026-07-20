package kindle

import "testing"

func TestGroupBySentence(t *testing.T) {
	entries := []Entry{
		{Word: "casa", Usage: "la casa vieja"},
		{Word: "perro", Usage: "un perro grande"},
		{Word: "vieja", Usage: "la casa vieja"},
	}

	groups := GroupBySentence(entries)
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}
	if got := len(groups[0].Entries); got != 2 {
		t.Errorf("groups[0].Entries has %d entries, want 2 (casa, vieja share a sentence)", got)
	}
	if got := groups[0].Entries[0].Word; got != "casa" {
		t.Errorf("groups[0].Entries[0].Word = %q, want %q", got, "casa")
	}
	if got := groups[0].Entries[1].Word; got != "vieja" {
		t.Errorf("groups[0].Entries[1].Word = %q, want %q", got, "vieja")
	}
	if got := groups[1].Entries[0].Word; got != "perro" {
		t.Errorf("groups[1].Entries[0].Word = %q, want %q", got, "perro")
	}
}
