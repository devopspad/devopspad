package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	goruntime "runtime"
	"time"

	"devopspad/internal/workspace"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed wails.json
var buildMetadata []byte

func productVersion() string {
	var config struct {
		Info struct {
			Version string `json:"productVersion"`
		} `json:"info"`
	}
	if json.Unmarshal(buildMetadata, &config) != nil || config.Info.Version == "" {
		return "unknown"
	}
	return config.Info.Version
}

func (a *App) GetWorkspaceHealth() workspace.StoreHealth {
	if a.store == nil {
		return workspace.StoreHealth{Status: "unavailable", Message: "Workspace could not be initialized. Check access to the app configuration directory and restart DevOpsPad."}
	}
	return a.store.Health()
}

func (a *App) RetryWorkspace() error {
	if a.store == nil {
		return errors.New("Restart DevOpsPad after restoring access to the configuration directory")
	}
	return a.store.RetryLoad()
}

func (a *App) RestoreWorkspaceBackup() error {
	if a.store == nil {
		return errors.New("Workspace recovery is unavailable")
	}
	return a.store.RestoreBackup()
}

// Build from an explicit allowlist, never from a redacted copy of configuration,
// logs, connection errors, queries, environment variables, or keychain contents.
func (a *App) GetDiagnostics() (string, error) {
	health := a.GetWorkspaceHealth()
	counts := map[string]int{}
	if a.store != nil && health.Status == "ready" {
		for _, c := range a.store.List() {
			switch c.Kind {
			case "mysql", "redis", "mongo", "clickhouse", "kafka", "k8s", "k3s":
				counts[c.Kind]++
			}
		}
	}
	a.mu.Lock()
	activeQueries := len(a.queries)
	a.mu.Unlock()
	report := struct {
		FormatVersion       int                   `json:"formatVersion"`
		Application         string                `json:"application"`
		Version             string                `json:"version"`
		GeneratedAt         string                `json:"generatedAt"`
		OS                  string                `json:"os"`
		Architecture        string                `json:"architecture"`
		GoVersion           string                `json:"goVersion"`
		Workspace           workspace.StoreHealth `json:"workspace"`
		ConnectionsByEngine map[string]int        `json:"connectionsByEngine"`
		ActiveQueries       int                   `json:"activeQueries"`
	}{1, "DevOpsPad", productVersion(), time.Now().UTC().Format(time.RFC3339), goruntime.GOOS, goruntime.GOARCH, goruntime.Version(), health, counts, activeQueries}
	encoded, err := json.MarshalIndent(report, "", "  ")
	return string(encoded), err
}

func (a *App) ExportDiagnostics() (bool, error) {
	report, err := a.GetDiagnostics()
	if err != nil {
		return false, err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Save diagnostics", DefaultFilename: "devopspad-diagnostics.json",
		Filters: []runtime.FileFilter{{DisplayName: "JSON files", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return false, err
	}
	if err = os.WriteFile(path, []byte(report), 0600); err != nil {
		return false, errors.New("Could not save diagnostics to the selected file")
	}
	return true, nil
}
