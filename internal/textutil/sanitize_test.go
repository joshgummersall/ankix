package textutil

import "testing"

func TestStripControl(t *testing.T) {
	cases := map[string]string{
		"plain pájaros":                  "plain pájaros",
		"keeps\ttabs\nand newlines":      "keeps\ttabs\nand newlines",
		"osc \x1b]52;c;cHduZWQ=\x07 end": "osc ]52;c;cHduZWQ= end",
		"clear \x1b[2J":                  "clear [2J",
		"c1 \u009b31m del \x7f cr \r":    "c1 31m del  cr ",
	}
	for in, want := range cases {
		if got := StripControl(in); got != want {
			t.Errorf("StripControl(%q) = %q, want %q", in, got, want)
		}
	}
}
