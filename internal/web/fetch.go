// Package web fetches a web page and extracts its readable article content.
package web

import (
	"fmt"
	"io"
	"net/http"
	nurl "net/url"
	"strings"
	"time"

	"github.com/go-shiori/dom"
	readability "github.com/go-shiori/go-readability"

	"github.com/joshgummersall/ankix/internal/textutil"
)

// Article is the extracted, readable content of a web page.
type Article struct {
	Title      string
	URL        string
	Paragraphs []string
}

const fetchTimeout = 30 * time.Second

// maxPageBytes caps how much of a response is read. An article is a few
// hundred kilobytes; without a cap a server can keep ankix allocating for
// the whole of fetchTimeout.
const maxPageBytes = 16 << 20

// get downloads url, refusing anything that isn't a successful HTML
// response of a sane size. readability.FromURL does none of the three: it
// parses an error page as if it were the article and reads without limit.
func get(url string) ([]byte, *nurl.URL, error) {
	parsed, err := nurl.ParseRequestURI(url)
	if err != nil {
		return nil, nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, nil, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}

	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		return nil, nil, fmt.Errorf("not an HTML document (Content-Type %q)", ct)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > maxPageBytes {
		return nil, nil, fmt.Errorf("page is larger than %d MB", maxPageBytes>>20)
	}
	return body, parsed, nil
}

// Fetch downloads url and runs it through readability extraction, returning
// the article title and body split into paragraphs.
func Fetch(url string) (*Article, error) {
	body, pageURL, err := get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	parsed, err := readability.FromReader(strings.NewReader(string(body)), pageURL)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", url, err)
	}

	// TextContent strips all tags without inserting separators between
	// block-level elements, so paragraphs run together on some sites.
	// Re-parse the extracted HTML and pull text out per block element
	// instead, which preserves the page's actual paragraph breaks.
	doc, err := dom.FastParse(strings.NewReader(parsed.Content))
	if err != nil {
		return nil, fmt.Errorf("parse extracted content for %s: %w", url, err)
	}

	var paragraphs []string
	for _, node := range dom.QuerySelectorAll(doc, "p, li, h1, h2, h3, h4, h5, h6, blockquote") {
		text := strings.TrimSpace(textutil.StripControl(dom.TextContent(node)))
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("no readable content found at %s", url)
	}

	title := strings.TrimSpace(textutil.StripControl(parsed.Title))
	if title == "" {
		title = url
	}

	return &Article{Title: title, URL: url, Paragraphs: paragraphs}, nil
}
