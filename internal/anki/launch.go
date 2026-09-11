package anki

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// probeTimeout bounds a single "is AnkiConnect answering?" request. It only
// has to cover a local round trip to an app that is already running, so it
// is short: the case worth being quick about is Anki not running at all,
// where the connection is refused long before this expires.
const probeTimeout = 2 * time.Second

// startTimeout is how long Start waits for AnkiConnect to answer after it
// launches Anki, and pollInterval how often it asks in the meantime. Anki
// opens its collection before the add-on's HTTP server comes up, which on a
// large collection is several seconds, so the ceiling is generous; it costs
// nothing when Anki is quick, because Start returns on the first successful
// probe rather than waiting out the clock.
const (
	startTimeout = 60 * time.Second
	pollInterval = 250 * time.Millisecond
)

// Reachable reports whether AnkiConnect is answering. It sends the cheapest
// call in the API with its own short timeout rather than Client.HTTPClient's
// — which has none — so probing an app that isn't running can't hang.
func (c *Client) Reachable() bool {
	probe := &Client{URL: c.URL, HTTPClient: &http.Client{Timeout: probeTimeout}}
	var version int
	return probe.invoke("version", nil, &version) == nil
}

// Local reports whether the client's URL points at this machine. Starting
// the Anki app here can only make a local URL answer; a remote one is
// someone else's machine to fix.
func (c *Client) Local() bool {
	u, err := url.Parse(c.URL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Start makes sure Anki is running and its AnkiConnect add-on is answering,
// launching the desktop app if it isn't, and returns once it responds.
//
// It is safe to call when Anki is already up: that is the common case, and
// it costs one local request. Anki itself is left running afterwards — it
// is the user's app, not a subprocess of this command.
func (c *Client) Start() error {
	if c.Reachable() {
		return nil
	}
	if !c.Local() {
		return fmt.Errorf("AnkiConnect at %s isn't answering, and it isn't on this machine -- start Anki there yourself", c.URL)
	}
	if err := launchApp(); err != nil {
		return err
	}

	deadline := time.Now().Add(startTimeout)
	for {
		if c.Reachable() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("started Anki, but AnkiConnect at %s didn't answer within %s; is the AnkiConnect add-on installed? (Tools > Add-ons > Get Add-ons..., code 2055492159)", c.URL, startTimeout)
		}
		time.Sleep(pollInterval)
	}
}
