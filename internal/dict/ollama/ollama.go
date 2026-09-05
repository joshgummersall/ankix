// Package ollama defines vocab words (and, via the translate adapter, glosses
// subtitle words) by asking a local "ankix" Ollama model (see
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

// DefaultKeepAlive is how long ankix asks Ollama to hold the model in
// memory after a lookup. Ollama's own default is 5 minutes, which is
// shorter than the gaps between lookups in ordinary use — you read a page,
// then pick a word — so the model unloads mid-session and the next word
// pays the load cost all over again. This is sized to outlast reading, not
// to hold memory forever; see Provider.KeepAlive for the other settings.
const DefaultKeepAlive = "30m"

// Provider defines words by querying a local Ollama chat model built from
// ollama/Modelfile.
type Provider struct {
	URL   string // e.g. "http://localhost:11434"
	Model string // e.g. "ankix"
	// KeepAlive is sent with every request as the Ollama API's keep_alive:
	// a duration ("30m"), a number of seconds ("1800"), "0" to unload as
	// soon as the reply is sent, or any negative value to keep the model
	// loaded indefinitely. Empty leaves the field off the request, which
	// gives Ollama's own default.
	KeepAlive  string
	HTTPClient *http.Client
}

// New returns a Provider that queries the Ollama model at url.
func New(url, model string) *Provider {
	return &Provider{
		URL:        strings.TrimSuffix(url, "/"),
		Model:      model,
		KeepAlive:  DefaultKeepAlive,
		HTTPClient: http.DefaultClient,
	}
}

// Define implements dict.Provider. It returns the model's contextual
// translation as definition, and the dictionary/base-form lemma (e.g. "to
// realize" for "realized") as lemma when it differs from the translation.
func (p *Provider) Define(word, usage string) (definition, lemma string, err error) {
	resp, err := p.chat([]chatMessage{{Role: "user", Content: askContent(word, usage)}})
	if err != nil {
		return "", "", err
	}
	return collapse(resp)
}

// Refine implements dict.Refiner. It replays the original question and the
// answer the model already gave as a two-turn history, then asks for the
// correction — so the model corrects its own previous line rather than
// answering a new question, which is what the Modelfile's "Correction:"
// rule and few-shot examples are written for.
func (p *Provider) Refine(word, usage, definition, lemma, instruction string) (string, string, error) {
	// Define collapses lemma to "" when it matched the translation, so
	// restore it here: the assistant turn has to look exactly like one the
	// model would have produced, and it never omits the LEMMA half.
	if lemma == "" {
		lemma = definition
	}
	resp, err := p.chat([]chatMessage{
		{Role: "user", Content: askContent(word, usage)},
		{Role: "assistant", Content: fmt.Sprintf("TRANSLATION: %s | LEMMA: %s", definition, lemma)},
		{Role: "user", Content: "Correction: " + instruction},
	})
	if err != nil {
		return "", "", err
	}
	return collapse(resp)
}

// askContent formats the per-word question the model expects, as documented
// in the Modelfile's system prompt.
func askContent(word, usage string) string {
	return fmt.Sprintf("Word: %s | Sentence: %s", word, usage)
}

// collapse parses a model reply and applies the dict.Provider convention of
// returning "" for a lemma that doesn't differ from the translation. Shared
// by Define and Refine so a refined answer is shaped exactly like a fresh
// one.
func collapse(resp string) (definition, lemma string, err error) {
	translation, l, ok := parseReply(resp)
	if !ok {
		// ankix pins the model to its Modelfile's checksum, so a stock
		// install can't answer in the wrong format — reaching here means a
		// model chosen with --ollama-model that wasn't built from an ankix
		// Modelfile, or one whose num_predict truncates the reply before
		// the "|".
		return "", "", fmt.Errorf("ollama chat: unexpected reply %q; the model isn't answering in ankix's `TRANSLATION: ... | LEMMA: ...` format", resp)
	}
	if l == "" || strings.EqualFold(l, translation) {
		return translation, "", nil
	}
	return translation, l, nil
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
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	KeepAlive string        `json:"keep_alive,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

func (p *Provider) chat(msgs []chatMessage) (string, error) {
	req := chatRequest{
		Model:     p.Model,
		Messages:  msgs,
		Stream:    false,
		KeepAlive: p.KeepAlive,
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
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if strings.Contains(e.Error, "not found") {
			// Normally caught by the startup preflight (see cmd/ankix's
			// resolveModel); this covers a model removed mid-session.
			return "", fmt.Errorf("ollama chat: model %q not found; run `ankix install` to build it", p.Model)
		}
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

// warmWord and warmUsage are the throwaway lookup Warm sends. Any word
// would do — what matters is that the request is shaped exactly like a real
// one, so Ollama evaluates the same Modelfile system prompt and few-shot
// examples every later lookup starts from.
const (
	warmWord  = "banco"
	warmUsage = "Nos sentamos en el banco del parque."
)

// Warm sends a throwaway lookup so Ollama loads the model and evaluates the
// prompt prefix now, rather than on the user's first real word. With a large
// base model most of a cold lookup is that one-time cost — loading the
// weights, then evaluating the Modelfile's system prompt and few-shot
// examples — and Ollama reuses both for subsequent requests.
//
// Callers run this in the background and ignore its result: a warm-up that
// fails costs nothing, because the failure recurs on the first real lookup,
// where it is reported properly.
func (p *Provider) Warm() error {
	_, err := p.chat([]chatMessage{{Role: "user", Content: askContent(warmWord, warmUsage)}})
	return err
}
