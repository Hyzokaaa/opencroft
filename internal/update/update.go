// Package update replaces the running binary with the latest release.
//
// The trust anchor is TLS to GitHub — the checksum is published beside the
// binary, so it catches a truncated or corrupted download, not a compromised
// release. Saying otherwise would be dishonest.
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repo      = "Hyzokaaa/opencroft"
	userAgent = "croft-update"
)

type Release struct {
	Tag   string `json:"tag_name"`
	Notes string `json:"body"`
}

func client() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func Latest() (Release, error) {
	req, _ := http.NewRequest(http.MethodGet,
		"https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	res, err := client().Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("asking GitHub for the latest release: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub answered %s", res.Status)
	}

	var release Release
	if err := json.NewDecoder(res.Body).Decode(&release); err != nil {
		return Release{}, err
	}
	return release, nil
}

// Newer reports whether the release differs from what is running. Versions are
// compared as strings on purpose: any difference is worth acting on, and a
// version parser is a thing that can be wrong.
func Newer(current string, release Release) bool {
	return strings.TrimPrefix(release.Tag, "v") != strings.TrimPrefix(current, "v")
}

func assetName() (string, error) {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return "croft-linux-" + runtime.GOARCH, nil
	default:
		return "", fmt.Errorf("no published binary for %s", runtime.GOARCH)
	}
}

// Apply downloads the release and replaces the binary at path.
//
// The new file is written beside the old one and renamed into place: writing
// over a running binary fails with ETXTBSY, and a rename is atomic, so an
// interrupted update never leaves half a binary behind.
func Apply(tag, path string) error {
	asset, err := assetName()
	if err != nil {
		return err
	}

	base := "https://github.com/" + repo + "/releases/download/" + tag + "/"

	expected, err := checksumFor(base+"SHA256SUMS", asset)
	if err != nil {
		return err
	}

	temporary := path + ".new"
	sum, err := download(base+asset, temporary)
	if err != nil {
		os.Remove(temporary)
		return err
	}

	if expected != "" && sum != expected {
		os.Remove(temporary)
		return fmt.Errorf("the download does not match its published checksum:\n  expected %s\n  got      %s", expected, sum)
	}

	if err := os.Chmod(temporary, 0o755); err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}

func download(url, into string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", userAgent)

	res, err := client().Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", filepath.Base(url), err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: %s", filepath.Base(url), res.Status)
	}

	file, err := os.Create(into)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), res.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// checksumFor returns an empty string when the release predates SHA256SUMS,
// rather than refusing to update at all.
func checksumFor(url, asset string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", userAgent)

	res, err := client().Do(req)
	if err != nil {
		return "", nil
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", nil
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	if err != nil {
		return "", nil
	}

	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0], nil
		}
	}
	return "", nil
}
