package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const filler = "<p>Esta es una frase larga de relleno para que readability acepte el articulo como legible, con suficiente texto.</p>"

func serve(t *testing.T, status int, contentType, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestFetchStripsControlCharacters(t *testing.T) {
	page := "<html><head><title>T&#27;]0;x&#7;</title></head><body><article>" + strings.Repeat(filler, 8) +
		"<p>Texto con escape &#27;]52;c;cHduZWQ=&#7; y raw \x1b[2J al final de la frase de prueba.</p></article></body></html>"
	a, err := Fetch(serve(t, http.StatusOK, "text/html; charset=utf-8", page))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range append([]string{a.Title}, a.Paragraphs...) {
		if strings.ContainsAny(s, "\x1b\x07") {
			t.Errorf("control characters survived: %q", s)
		}
	}
}

func TestFetchRejectsErrorPages(t *testing.T) {
	page := "<html><body><article>" + strings.Repeat(filler, 8) + "</article></body></html>"
	if _, err := Fetch(serve(t, http.StatusInternalServerError, "text/html", page)); err == nil {
		t.Error("Fetch accepted an HTTP 500 body as an article")
	}
}

func TestFetchRejectsOversizedPages(t *testing.T) {
	page := "<html><body><article>" + strings.Repeat(filler, maxPageBytes/len(filler)+1) + "</article></body></html>"
	if _, err := Fetch(serve(t, http.StatusOK, "text/html", page)); err == nil {
		t.Error("Fetch accepted a page over maxPageBytes")
	}
}

func TestFetchRejectsNonWebSchemes(t *testing.T) {
	if _, err := Fetch("file:///etc/hosts"); err == nil {
		t.Error("Fetch accepted a file: URL")
	}
}
