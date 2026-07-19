// Package anki is a minimal client for the AnkiConnect add-on's HTTP API.
package anki

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrDuplicate is returned by AddNote when AnkiConnect rejects the note
// as a duplicate (its own duplicate check, based on the first field,
// scoped per Note.Options).
var ErrDuplicate = errors.New("duplicate note")

// Client talks to a running AnkiConnect instance.
type Client struct {
	URL        string
	HTTPClient *http.Client
}

// New returns a Client pointed at the given AnkiConnect URL
// (e.g. "http://localhost:8765").
func New(url string) *Client {
	return &Client{URL: url, HTTPClient: http.DefaultClient}
}

type request struct {
	Action  string `json:"action"`
	Version int    `json:"version"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *string         `json:"error"`
}

func (c *Client) invoke(action string, params, result any) error {
	body, err := json.Marshal(request{Action: action, Version: 6, Params: params})
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Post(c.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("anki-connect %s: %w (is Anki running with AnkiConnect installed?)", action, err)
	}
	defer resp.Body.Close()

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("anki-connect %s: decode response: %w", action, err)
	}
	if r.Error != nil {
		return fmt.Errorf("anki-connect %s: %s", action, *r.Error)
	}
	if result != nil && r.Result != nil {
		if err := json.Unmarshal(r.Result, result); err != nil {
			return fmt.Errorf("anki-connect %s: decode result: %w", action, err)
		}
	}
	return nil
}

// Note is a single Anki note to add.
type Note struct {
	DeckName  string            `json:"deckName"`
	ModelName string            `json:"modelName"`
	Fields    map[string]string `json:"fields"`
	Tags      []string          `json:"tags"`
	Options   *NoteOptions      `json:"options,omitempty"`
}

// NoteOptions controls AnkiConnect's own duplicate handling for AddNote.
type NoteOptions struct {
	AllowDuplicate bool   `json:"allowDuplicate"`
	DuplicateScope string `json:"duplicateScope"`
}

// CreateDeck creates a deck if it doesn't already exist.
func (c *Client) CreateDeck(name string) error {
	return c.invoke("createDeck", map[string]string{"deck": name}, nil)
}

// FindNotes returns note IDs matching an Anki search query.
func (c *Client) FindNotes(query string) ([]int64, error) {
	var ids []int64
	if err := c.invoke("findNotes", map[string]string{"query": query}, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// ModelNames returns the names of all note types installed in Anki.
func (c *Client) ModelNames() ([]string, error) {
	var names []string
	if err := c.invoke("modelNames", nil, &names); err != nil {
		return nil, err
	}
	return names, nil
}

// AddNote adds a single note and returns its note ID. If AnkiConnect
// rejects it as a duplicate, it returns ErrDuplicate.
func (c *Client) AddNote(n Note) (int64, error) {
	var id int64
	if err := c.invoke("addNote", map[string]Note{"note": n}, &id); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return 0, ErrDuplicate
		}
		return 0, err
	}
	return id, nil
}
