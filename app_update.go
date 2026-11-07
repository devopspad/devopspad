package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	latestReleaseAPI = "https://api.github.com/repos/devopspad/devopspad/releases/latest"
	releasePath      = "/devopspad/devopspad/releases/"
)

type UpdateInfo struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Available   bool   `json:"available"`
	URL         string `json:"url"`
	DownloadURL string `json:"downloadUrl"`
	Notes       string `json:"notes"`
}

// CheckForUpdate compares the running version with the latest published GitHub release.
func (a *App) CheckForUpdate() (UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return checkUpdate(ctx, http.DefaultClient, latestReleaseAPI, productVersion(), goruntime.GOARCH)
}

// OpenUpdate opens a release page or asset of this project in the default browser.
func (a *App) OpenUpdate(link string) error {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil ||
		!strings.HasPrefix(path.Clean(u.Path), releasePath) || strings.Contains(u.Path, "..") {
		return errors.New("Only DevOpsPad release links can be opened")
	}
	runtime.BrowserOpenURL(a.ctx, u.String())
	return nil
}

func checkUpdate(ctx context.Context, client *http.Client, api, current, arch string) (UpdateInfo, error) {
	info := UpdateInfo{Current: current}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "DevOpsPad/"+current)
	res, err := client.Do(req)
	if err != nil {
		return info, fmt.Errorf("Cannot reach GitHub releases: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return info, fmt.Errorf("GitHub releases returned HTTP %d", res.StatusCode)
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&release); err != nil {
		return info, fmt.Errorf("Cannot read the latest release: %w", err)
	}
	info.Latest = strings.TrimPrefix(release.TagName, "v")
	info.URL = release.HTMLURL
	info.Notes = release.Body
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, "-macos-"+arch+".dmg") {
			info.DownloadURL = asset.URL
		}
	}
	info.Available = newerVersion(info.Latest, current)
	return info, nil
}

// newerVersion reports whether latest is a higher major.minor.patch than current.
// Unparseable versions (such as "unknown" in development builds) never prompt.
func newerVersion(latest, current string) bool {
	l, ok1 := parseVersion(latest)
	c, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
