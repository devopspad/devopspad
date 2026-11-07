package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.1.2", "0.1.1", true},
		{"0.2.0", "0.1.9", true},
		{"1.0.0", "0.9.9", true},
		{"0.1.10", "0.1.9", true},
		{"0.1.1", "0.1.1", false},
		{"0.1.0", "0.1.1", false},
		{"v0.1.2", "0.1.1", true},
		{"0.1.2", "unknown", false},
		{"0.1.2-rc1", "0.1.1", false},
		{"", "0.1.1", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.latest, c.current); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestCheckUpdatePicksArchitectureDMG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "DevOpsPad/0.1.1" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(`{"tag_name":"v0.1.2","html_url":"https://github.com/devopspad/devopspad/releases/tag/v0.1.2","body":"notes","assets":[
			{"name":"devopspadctl-0.1.2-darwin-arm64.tar.gz","browser_download_url":"https://example.test/cli"},
			{"name":"DevOpsPad-0.1.2-macos-amd64.dmg","browser_download_url":"https://example.test/amd64"},
			{"name":"DevOpsPad-0.1.2-macos-arm64.dmg","browser_download_url":"https://example.test/arm64"}]}`))
	}))
	defer srv.Close()
	info, err := checkUpdate(context.Background(), srv.Client(), srv.URL, "0.1.1", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Latest != "0.1.2" || info.Current != "0.1.1" || info.DownloadURL != "https://example.test/arm64" || info.Notes != "notes" {
		t.Fatalf("unexpected update info: %+v", info)
	}
}

func TestCheckUpdateReportsHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	info, err := checkUpdate(context.Background(), srv.Client(), srv.URL, "0.1.1", "arm64")
	if err == nil || info.Available {
		t.Fatalf("expected an error without an update, got %+v, %v", info, err)
	}
}

func TestOpenUpdateRejectsForeignLinks(t *testing.T) {
	a := &App{ctx: context.Background()}
	for _, url := range []string{"https://evil.test/devopspad.dmg", "https://github.com/devopspad/devopspad/../other", "file:///etc/passwd",
		"https://github.com/devopspad/devopspad/releases/../../other", "http://github.com/devopspad/devopspad/releases/tag/v1.0.0",
		"https://github.com.evil.test/devopspad/devopspad/releases/tag/v1.0.0"} {
		if err := a.OpenUpdate(url); err == nil {
			t.Errorf("OpenUpdate(%q) accepted a foreign link", url)
		}
	}
}
