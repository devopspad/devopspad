package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devopspad/internal/workspace"
)

func TestDiagnosticsAllowlistExcludesConfigurationAndSecrets(t *testing.T) {
	a := NewApp()
	a.ctx = context.Background()
	vault := testVault{}
	var err error
	a.store, err = workspace.NewStore(t.TempDir(), vault)
	if err != nil {
		t.Fatal(err)
	}
	c, err := a.store.Save(workspace.Connection{
		Name: "private-profile-sentinel", Kind: "mysql", Host: "private-host-sentinel", Port: 3306,
		Username: "private-user-sentinel", Password: "private-password-sentinel", Database: "private-db-sentinel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.store.SaveAI(workspace.AISettings{Endpoint: "https://private-provider-sentinel.test", Model: "private-model-sentinel", APIKey: "private-key-sentinel"}); err != nil {
		t.Fatal(err)
	}
	report, err := a.GetDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{"private-", c.ID, "apiKey", "password", "endpoint", "username"} {
		if strings.Contains(report, excluded) {
			t.Fatalf("diagnostics contains excluded field %q", excluded)
		}
	}
	var parsed struct {
		Version     string         `json:"version"`
		Connections map[string]int `json:"connectionsByEngine"`
	}
	if json.Unmarshal([]byte(report), &parsed) != nil || parsed.Version != productVersion() || parsed.Connections["mysql"] != 1 {
		t.Fatal("missing diagnostic facts")
	}
}

func TestBlockedAppCanExportDiagnosticsAndRecover(t *testing.T) {
	dir := t.TempDir()
	vault := testVault{}
	s, _ := workspace.NewStore(dir, vault)
	s.Save(workspace.Connection{Name: "test", Kind: "redis", Host: "localhost", Port: 6379})
	os.WriteFile(filepath.Join(dir, "workspace.json"), []byte("damaged private-error-sentinel"), 0600)
	a := NewApp()
	a.ctx = context.Background()
	a.store, _ = workspace.NewStore(dir, vault)
	if _, err := a.ListConnections(); err == nil {
		t.Fatal("blocked app accepted live operations")
	}
	report, err := a.GetDiagnostics()
	if err != nil || strings.Contains(report, dir) || strings.Contains(report, "private-error-sentinel") {
		t.Fatal("blocked diagnostics leaks original data", err)
	}
	if !a.GetWorkspaceHealth().CanRestore {
		t.Fatal("recovery unavailable")
	}
	if err = a.RestoreWorkspaceBackup(); err != nil {
		t.Fatal(err)
	}
	cs, err := a.ListConnections()
	if err != nil || len(cs) != 1 {
		t.Fatal("recovered app needs a restart", err)
	}
}
