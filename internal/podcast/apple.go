// Package podcast resolves an Apple Podcasts episode URL to its transcript,
// via iTunes's lookup API and the show's own RSS feed (podcasts are hosted
// by their own feed, e.g. Buzzsprout/Libsyn/Megaphone — Apple only indexes
// them).
package podcast

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const httpTimeout = 30 * time.Second

// userAgent identifies requests to podcast host feeds/transcripts, several
// of which (e.g. Buzzsprout) return 403 for Go's default empty User-Agent.
const userAgent = "Mozilla/5.0 (compatible; ankix/1.0; +https://github.com/joshgummersall/ankix)"

// getWithUserAgent issues a GET with a browser-like User-Agent set, since
// some podcast hosts block requests with no/bot-looking User-Agent headers.
func getWithUserAgent(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: httpTimeout}
	return client.Do(req)
}

var (
	showIDRe    = regexp.MustCompile(`/id(\d+)`)
	episodeIDRe = regexp.MustCompile(`[?&]i=(\d+)`)
)

// ParseAppleURL extracts the podcast (show) ID and episode ID from an Apple
// Podcasts episode URL, e.g.
// https://podcasts.apple.com/us/podcast/some-show/id1776743104?i=1000778667267.
func ParseAppleURL(rawURL string) (podcastID, episodeID string, err error) {
	m := showIDRe.FindStringSubmatch(rawURL)
	if m == nil {
		return "", "", fmt.Errorf("no podcast id (idNNNNNNN) found in %s", rawURL)
	}
	podcastID = m[1]

	m = episodeIDRe.FindStringSubmatch(rawURL)
	if m == nil {
		return "", "", fmt.Errorf("no episode id (?i=NNNNNNN) found in %s", rawURL)
	}
	episodeID = m[1]

	return podcastID, episodeID, nil
}

type lookupResponse struct {
	Results []lookupResult `json:"results"`
}

type lookupResult struct {
	FeedURL        string `json:"feedUrl"`
	TrackID        int64  `json:"trackId"`
	TrackName      string `json:"trackName"`
	EpisodeGUID    string `json:"episodeGuid"`
	EpisodeURL     string `json:"episodeUrl"`
	CollectionName string `json:"collectionName"`
}

func lookup(query string) (lookupResponse, error) {
	var out lookupResponse

	resp, err := getWithUserAgent("https://itunes.apple.com/lookup?" + query)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("itunes lookup: unexpected status %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decode itunes lookup response: %w", err)
	}
	return out, nil
}

// LookupFeedURL resolves a podcast (show) ID to its RSS feed URL.
func LookupFeedURL(podcastID string) (string, error) {
	res, err := lookup("id=" + podcastID)
	if err != nil {
		return "", fmt.Errorf("lookup show %s: %w", podcastID, err)
	}
	if len(res.Results) == 0 || res.Results[0].FeedURL == "" {
		return "", fmt.Errorf("no feed url found for podcast id %s", podcastID)
	}
	return res.Results[0].FeedURL, nil
}

// Episode identifies a single podcast episode as reported by iTunes: its RSS
// guid (to match against the feed's <item><guid>), its enclosure/audio URL,
// and its title.
type Episode struct {
	GUID  string
	URL   string
	Title string
}

// LookupEpisode resolves an episode ID (scoped to podcastID) to its guid,
// audio URL, and title. The iTunes Lookup API only returns a bounded window
// of the show's most recent ~200 episodes, so this can come up empty for
// older back-catalog episodes.
func LookupEpisode(podcastID, episodeID string) (Episode, error) {
	var ep Episode

	res, err := lookup(fmt.Sprintf("id=%s&entity=podcastEpisode&limit=200", podcastID))
	if err != nil {
		return ep, fmt.Errorf("lookup episodes for show %s: %w", podcastID, err)
	}

	wantID, err := strconv.ParseInt(episodeID, 10, 64)
	if err != nil {
		return ep, fmt.Errorf("invalid episode id %q: %w", episodeID, err)
	}

	for _, r := range res.Results {
		if r.TrackID == wantID {
			return Episode{GUID: r.EpisodeGUID, URL: r.EpisodeURL, Title: r.TrackName}, nil
		}
	}

	return ep, fmt.Errorf("episode id %s not found in the show's ~200 most recent episodes (iTunes Lookup doesn't page further back)", episodeID)
}
