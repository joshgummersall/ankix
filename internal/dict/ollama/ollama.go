// Package ollama defines vocab words (and, via the translate adapter, glosses
// subtitle words) by asking a local "ankindle" Ollama model (see
// ollama/vocab/Modelfile) for a contextual translation and dictionary lemma
// of a word as used in a sentence. The model's system prompt and few-shot
// examples are baked into the Modelfile itself, so this package only needs
// to send the per-word chat message it expects.
package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Provider defines words by querying a local Ollama chat model built from
// ollama/Modelfile.
type Provider struct {
	URL        string // e.g. "http://localhost:11434"
	Model      string // e.g. "ankindle"
	HTTPClient *http.Client
}

// New returns a Provider that queries the Ollama model at url.
func New(url, model string) *Provider {
	return &Provider{
		URL:        strings.TrimSuffix(url, "/"),
		Model:      model,
		HTTPClient: http.DefaultClient,
	}
}

// Define implements dict.Provider. It returns the model's contextual
// translation, with the dictionary/base-form lemma in parentheses when it
// differs (e.g. "realized (to realize)").
func (p *Provider) Define(word, usage string) (string, error) {
	content := fmt.Sprintf("Word: %s | Sentence: %s", word, usage)
	resp, err := p.chat(content)
	if err != nil {
		return "", err
	}

	translation, lemma, ok := parseReply(resp)
	if !ok {
		return "", fmt.Errorf("ollama chat: unexpected reply %q", resp)
	}
	if lemma == "" || strings.EqualFold(lemma, translation) {
		return translation, nil
	}
	return fmt.Sprintf("%s (%s)", translation, lemma), nil
}

// parseReply extracts translation and lemma from a reply formatted as
// "TRANSLATION: <word> | LEMMA: <meaning>".
func parseReply(reply string) (translation, lemma string, ok bool) {
	before, after, found := strings.Cut(reply, "|")
	if !found {
		return "", "", false
	}
	_, translation, found = strings.Cut(before, ":")
	if !found {
		return "", "", false
	}
	_, lemma, found = strings.Cut(after, ":")
	if !found {
		return "", "", false
	}
	translation = strings.TrimSpace(translation)
	lemma = strings.TrimSpace(lemma)
	if translation == "" {
		return "", "", false
	}
	return translation, lemma, true
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

func (p *Provider) chat(content string) (string, error) {
	req := chatRequest{
		Model:    p.Model,
		Messages: []chatMessage{{Role: "user", Content: content}},
		Stream:   false,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	resp, err := p.HTTPClient.Post(p.URL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama chat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama chat: unexpected status %s", resp.Status)
	}

	var r chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("ollama chat: decode response: %w", err)
	}

	result := strings.TrimSpace(r.Message.Content)
	if result == "" {
		return "", fmt.Errorf("ollama chat: empty response")
	}
	return result, nil
}
