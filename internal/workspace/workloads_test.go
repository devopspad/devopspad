package workspace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestListWorkloadsReadsReplicaAndImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/default/deployments":
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"heartie-server","namespace":"default","uid":"dep-1","creationTimestamp":"2026-10-01T00:00:00Z"},"spec":{"replicas":2,"selector":{"matchLabels":{"app":"heartie"}},"template":{"spec":{"containers":[{"image":"heartie:1.4.2"},{"image":"sidecar:1"}]}}},"status":{"readyReplicas":1,"conditions":[{"lastUpdateTime":"2026-10-20T03:04:05Z"}]}}]}`)
		case "/apis/apps/v1/namespaces/default/statefulsets":
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"StatefulSetList","items":[]}`)
		case "/apis/apps/v1/namespaces/default/daemonsets":
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DaemonSetList","items":[{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{"name":"agent","namespace":"default","uid":"ds-1","creationTimestamp":"2026-10-02T00:00:00Z"},"spec":{"template":{"spec":{"containers":[{"image":"agent:9"}]}}},"status":{"desiredNumberScheduled":3,"numberReady":3}}]}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "kubeconfig")
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
    namespace: default
users:
- name: reader
  user:
    token: do-not-expose
`, server.URL)
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := ListWorkloads(context.Background(), Connection{Kind: "k3s", Kubeconfig: path, Context: "team", Database: "default"}, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d workloads", len(items))
	}
	if items[1].Name != "heartie-server" || items[1].Desired != 2 || items[1].Ready != 1 || items[1].Image != "heartie:1.4.2" || items[1].ExtraImages != 1 || items[1].Selector != "app=heartie" || items[1].UpdatedAt != "2026-10-20T03:04:05Z" {
		t.Fatalf("%+v", items[1])
	}
	if items[0].Kind != "DaemonSet" || items[0].Desired != 3 || items[0].Ready != 3 {
		t.Fatalf("%+v", items[0])
	}
}
