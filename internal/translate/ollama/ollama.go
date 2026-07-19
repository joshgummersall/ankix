// Package ollama glosses Spanish words by asking a local "ankitube" Ollama
// model (see ollama/Modelfile) for a short contextual English translation
// of a word or phrase as used in a sentence. The model's system prompt is
// baked into the Modelfile itself: it's told to reply with the gloss text
// only, so this package doesn't need to parse a structured reply.
package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Provider glosses words by querying a local Ollama chat model built from
// ollama/Modelfile.
type Provider struct {
	URL        string // e.g. "http://localhost:11434"
	Model      string // e.g. "ankitube"
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

// Gloss implements translate.Provider.
func (p *Provider) Gloss(word, sentence string) (string, error) {
	content := fmt.Sprintf("Word: %s | Sentence: %s", word, sentence)
	resp, err := p.chat(content)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp), nil
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
