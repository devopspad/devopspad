package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"infradex/internal/workspace"
)

type testVault map[string]string

func (v testVault) Set(k, s string) error        { v[k] = s; return nil }
func (v testVault) Get(k string) (string, error) { return v[k], nil }
func (v testVault) Delete(k string) error        { delete(v, k); return nil }

func TestClusterBridgePinsContextAndNamespaceAfterProfileEdit(t *testing.T) {
	server := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" || r.URL.Path != "/api/v1/namespaces/team/pods" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":%q,"namespace":"team"}}]}`, name)
		}))
	}
	east := server("east-pod")
	defer east.Close()
	west := server("west-pod")
	defer west.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: east
clusters:
- name: east
  cluster:
    server: %s
- name: west
  cluster:
    server: %s
contexts:
- name: east
  context:
    cluster: east
- name: west
  context:
    cluster: west
`, east.URL, west.URL)
	if e := os.WriteFile(path, []byte(config), 0600); e != nil {
		t.Fatal(e)
	}
	app := NewApp()
	app.ctx = context.Background()
	var e error
	app.store, e = workspace.NewStore(filepath.Join(dir, "workspace"), testVault{})
	if e != nil {
		t.Fatal(e)
	}
	c, e := app.SaveConnection(workspace.Connection{Name: "Cluster", Kind: "k8s", Kubeconfig: path, Database: "team"})
	if e != nil || c.Context != "east" {
		t.Fatalf("current context was not pinned %+v %v", c, e)
	}
	c.Context = "west"
	c.Database = "staging"
	if _, e = app.SaveConnection(c); e != nil {
		t.Fatal(e)
	}
	r, e := app.ExecuteInContext(c.ID, "team", "east", "get pods", "old-tab")
	if e != nil || len(r.Rows) != 1 || r.Rows[0][0] != "east-pod" {
		t.Fatalf("old tab changed target %+v %v", r, e)
	}
	c.Context = "missing"
	if _, e = app.SaveConnection(c); e == nil {
		t.Fatal("saved a missing context")
	}
}
