// Package web fetches a web page and extracts its readable article content.
package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-shiori/dom"
	readability "github.com/go-shiori/go-readability"
)

// Article is the extracted, readable content of a web page.
type Article struct {
	Title      string
	URL        string
	Paragraphs []string
}

const fetchTimeout = 30 * time.Second

// Fetch downloads url and runs it through readability extraction, returning
// the article title and body split into paragraphs.
func Fetch(url string) (*Article, error) {
	parsed, err := readability.FromURL(url, fetchTimeout)
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
		text := strings.TrimSpace(dom.TextContent(node))
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("no readable content found at %s", url)
	}

	title := parsed.Title
	if title == "" {
		title = url
	}

	return &Article{Title: title, URL: url, Paragraphs: paragraphs}, nil
}
