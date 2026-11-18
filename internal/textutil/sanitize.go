package textutil

import "strings"

// StripControl removes control characters from text that came from outside
// ankix (a web page, a subtitle track, a podcast feed, a vocab.db, a model
// reply) before it is shown in the terminal. Left in, an ESC byte lets that
// text drive the terminal: retitle the window, clear the screen, or write
// the clipboard. Newlines and tabs are kept; every other C0 control, DEL,
// and the C1 range (which some terminals read as 8-bit escape introducers)
// is dropped.
func StripControl(s string) string {
	clean := func(r rune) bool {
		return r == '\n' || r == '\t' || !(r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f))
	}
	if strings.IndexFunc(s, func(r rune) bool { return !clean(r) }) == -1 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if clean(r) {
			return r
		}
		return -1
	}, s)
}
