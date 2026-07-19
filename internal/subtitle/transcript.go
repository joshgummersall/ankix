// Package subtitle fetches and parses YouTube subtitle tracks into a
// transcript of timestamped lines.
package subtitle

import "time"

// Cue is a single line of transcript text with its time range in the video.
type Cue struct {
	Start, End time.Duration
	Text       string
}

// Transcript is a full set of cues for one video.
type Transcript struct {
	VideoID string
	Cues    []Cue
}
