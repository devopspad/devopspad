package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKubeContextMetadataAndNamespaceRBAC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			w.WriteHeader(403)
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403,"message":"namespaces is forbidden"}`)
			return
		}
		if r.URL.Path != "/api/v1/namespaces/team/pods" || r.Method != "GET" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","items":[]}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config")
	cfg := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: team
clusters:
- name: local
  cluster:
    server: %s
contexts:
- name: team
  context:
    cluster: local
    user: reader
    namespace: team
- name: exec
  context:
    cluster: local
    user: plugin
users:
- name: reader
  user:
    token: do-not-expose
- name: plugin
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: never-run-this-plugin
      interactiveMode: Never
`, server.URL)
	if e := os.WriteFile(path, []byte(cfg), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := ReadKubeconfig(path)
	if e != nil || len(info.Contexts) != 2 || info.CurrentContext != "team" {
		t.Fatalf("%+v %v", info, e)
	}
	raw, _ := json.Marshal(info)
	if strings.Contains(string(raw), "do-not-expose") || strings.Contains(string(raw), "never-run") {
		t.Fatal("exposed auth info")
	}
	if info.Contexts[0].Name != "exec" || !info.Contexts[0].ExecAuth || info.Contexts[1].Namespace != "team" {
		t.Fatal(info)
	}
	c := Connection{Kind: "k8s", Kubeconfig: path, Context: "team", Database: "team"}
	if _, e = ListDatabases(context.Background(), c); e == nil || !strings.Contains(e.Error(), "saved in this connection") {
		t.Fatal(e)
	}
	if _, e = Execute(context.Background(), c, "get pods"); e != nil {
		t.Fatal("namespace read should still work", e)
	}
	c.Context = "exec"
	if _, e = kubeClient(c); e == nil || !strings.Contains(e.Error(), "Exec-based") {
		t.Fatal(e)
	}
	c.Context = "missing"
	if _, e = kubeClient(c); e == nil {
		t.Fatal("missing context silently used default")
	}
}
