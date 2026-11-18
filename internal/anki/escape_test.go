package anki

import (
	"strings"
	"testing"
	"time"
)

// Text from a page, feed or model reply must reach a card as text, never
// as markup.
func TestBuildNoteEscapesUntrustedText(t *testing.T) {
	sentence := `Texto <img src=x onerror=alert(1)> fin`
	n := BuildNote(nil, "D", `T <script>x()</script>`, `https://e/?a="b"`, "Web", sentence,
		WordSelection{Start: 0, End: 5, Gloss: `<b>gloss</b>`, Lemma: `<i>lemma</i>`})

	for name, field := range n.Fields {
		for _, raw := range []string{"<img", "<script", "<b>gloss", "<i>lemma", `"b"`} {
			if strings.Contains(field, raw) {
				t.Errorf("%s contains unescaped %q: %s", name, raw, field)
			}
		}
	}
	if want := `<b>Texto</b><br><br><i>Texto</i> &lt;img src=x onerror=alert(1)&gt; fin`; n.Fields["Front"] != want {
		t.Errorf("Front = %s, want %s", n.Fields["Front"], want)
	}
}

func TestPodcastNoteLinkCannotLeaveHref(t *testing.T) {
	n := BuildPodcastNote(nil, "D", "Ep", `http://e/a.mp3" onmouseover="alert(1)`, 0, "hola mundo",
		WordSelection{Start: 0, End: 4, Gloss: "hi"})
	if strings.Contains(n.Fields["Back"], `" onmouseover="`) {
		t.Errorf("link broke out of its href: %s", n.Fields["Back"])
	}
}

func TestAudioLinkOnlyAllowsWebURLs(t *testing.T) {
	if got := AudioLink("javascript:alert(1)", time.Minute); got != "" {
		t.Errorf("AudioLink(javascript:) = %q, want empty", got)
	}
	if got := AudioLink("https://e/a.mp3", 10*time.Second); got != "https://e/a.mp3#t=9" {
		t.Errorf("AudioLink(https) = %q", got)
	}
}
