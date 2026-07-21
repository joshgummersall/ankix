package web

import (
	"fmt"
	"testing"
)

func TestDebugSplit(t *testing.T) {
	a, err := Fetch("https://asisejuega.com/guias/liga-de-futbol-mexicano/")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("title:", a.Title)
	fmt.Println("paragraphs:", len(a.Paragraphs))
	for i := 9; i < 20 && i < len(a.Paragraphs); i++ {
		p := a.Paragraphs[i]
		if len(p) > 250 {
			p = p[:250]
		}
		fmt.Println("---", i, "---")
		fmt.Println(p)
	}
}
