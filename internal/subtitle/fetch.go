package subtitle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Fetch downloads a Spanish subtitle track for url using yt-dlp, caching the
// result under cacheDir, and returns the path to the downloaded VTT file
// and the video's ID. It prefers manually-authored subtitles and falls
// back to auto-generated captions if none exist.
func Fetch(url, lang, cacheDir string) (path, videoID string, err error) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return "", "", fmt.Errorf("yt-dlp not found in PATH; install it with `brew install yt-dlp`")
	}

	videoID, err = GetVideoID(url)
	if err != nil {
		return "", "", fmt.Errorf("resolve video id: %w", err)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create cache dir: %w", err)
	}

	if p, err := findSubFile(cacheDir, videoID, lang); err == nil {
		return p, videoID, nil
	}

	out := filepath.Join(cacheDir, "%(id)s")
	_ = runYtDlp(url, lang, out, false)
	if p, err := findSubFile(cacheDir, videoID, lang); err == nil {
		return p, videoID, nil
	}

	if err := runYtDlp(url, lang, out, true); err != nil {
		return "", "", fmt.Errorf("no Spanish subtitles (manual or auto-generated) found for this video: %w", err)
	}

	p, err := findSubFile(cacheDir, videoID, lang)
	if err != nil {
		return "", "", fmt.Errorf("yt-dlp reported success but no subtitle file was found: %w", err)
	}
	return p, videoID, nil
}

// GetTitle returns the video's title via yt-dlp.
func GetTitle(url string) (string, error) {
	cmd := exec.Command("yt-dlp", "--get-title", url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func runYtDlp(url, lang, out string, auto bool) error {
	args := []string{"--skip-download", "--sub-lang", lang, "--sub-format", "vtt", "-o", out}
	if auto {
		args = append(args, "--write-auto-sub")
	} else {
		args = append(args, "--write-subs")
	}
	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yt-dlp: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

var videoIDRe = regexp.MustCompile(`(?:v=|youtu\.be/|shorts/)([A-Za-z0-9_-]{11})`)

// GetVideoID extracts the video ID from url, resolving via yt-dlp if it
// can't be parsed directly (e.g. shortened/non-standard URLs).
func GetVideoID(url string) (string, error) {
	if m := videoIDRe.FindStringSubmatch(url); m != nil {
		return m[1], nil
	}
	cmd := exec.Command("yt-dlp", "--get-id", url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// findSubFile globs for a previously-downloaded subtitle file for videoID
// and lang, since yt-dlp's exact suffix (e.g. ".es.vtt" vs ".es-en.vtt" for
// translated auto-captions) can vary.
func findSubFile(cacheDir, videoID, lang string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(cacheDir, videoID+"*."+lang+"*.vtt"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no cached subtitle file for %s", videoID)
	}
	return matches[0], nil
}
