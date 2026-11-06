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
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestScopedWorkloadIndexPreservesTemplateAndMetadataMatches(t *testing.T) {
	for _, mode := range []string{"indexed", "list-only", "unsupported", "deleted", "truncated", "changed", "broad", "missing-column", "unsafe-name"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			gets, fullLists := []string{}, 0
			items := make([]map[string]any, 8)
			table := metav1.Table{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "Table"}, ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Selector"}}}
			for i := range items {
				name, app := fmt.Sprintf("w%d", i), fmt.Sprintf("other-%d", i)
				selector := "app=" + app
				metadata := map[string]any{"name": name, "namespace": "default", "uid": name}
				if i == 0 {
					app, selector = "svc-user", "app=svc-user"
				}
				if i == 1 {
					metadata["labels"] = map[string]string{"app": "svc-user"}
				}
				if i == 2 {
					app, selector = "svc-user", "role=api"
				}
				if i == 0 && mode == "changed" {
					app = "changed-app"
				}
				items[i] = map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": metadata, "spec": map[string]any{"replicas": int64(1), "selector": map[string]any{"matchLabels": map[string]string{"app": app}}, "template": map[string]any{"metadata": map[string]any{"labels": map[string]string{"app": app}}, "spec": map[string]any{"containers": []map[string]string{{"name": "web", "image": "app:1"}}}}}}
				table.Rows = append(table.Rows, metav1.TableRow{Cells: []any{name, selector}})
			}
			if mode == "truncated" {
				table.Continue = "next-page"
			}
			if mode == "broad" {
				for i := range table.Rows {
					table.Rows[i].Cells[1] = "role=api"
				}
			}
			if mode == "missing-column" {
				table.ColumnDefinitions[1].Name = "Unknown"
			}
			if mode == "unsafe-name" {
				table.Rows[0].Cells[0] = "../secrets"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				base := "/apis/apps/v1/namespaces/default/deployments"
				if strings.HasPrefix(r.URL.Path, base+"/") {
					name := strings.TrimPrefix(r.URL.Path, base+"/")
					mu.Lock()
					gets = append(gets, name)
					mu.Unlock()
					if mode == "list-only" {
						http.Error(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403}`, 403)
						return
					}
					if mode == "deleted" && name == "w1" {
						http.Error(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`, 404)
						return
					}
					for _, obj := range items {
						if obj["metadata"].(map[string]any)["name"] == name {
							_ = json.NewEncoder(w).Encode(obj)
							return
						}
					}
					t.Errorf("unexpected object fetch: %s", name)
					return
				}
				if r.URL.Path != base {
					fmt.Fprint(w, `{"kind":"Table","apiVersion":"meta.k8s.io/v1","rows":[]}`)
					return
				}
				if strings.Contains(r.Header.Get("Accept"), "as=Table") && mode != "unsupported" {
					if r.URL.Query().Get("includeObject") != "None" {
						t.Error("index must not embed full metadata")
					}
					result := table
					if r.URL.Query().Get("labelSelector") != "" {
						result.Rows = table.Rows[1:2]
						result.Continue = ""
					}
					_ = json.NewEncoder(w).Encode(result)
					return
				}
				mu.Lock()
				fullLists++
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "apps/v1", "kind": "DeploymentList", "items": items})
			}))
			defer srv.Close()
			path := filepath.Join(t.TempDir(), "config")
			cfg := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: local\nclusters:\n- name: local\n  cluster:\n    server: %s\ncontexts:\n- name: local\n  context:\n    cluster: local\n", srv.URL)
			if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := ReadOpsWorkloads(context.Background(), Connection{Kind: "k3s", Kubeconfig: path}, "default", "app=svc-user")
			if err != nil || len(out.Warnings) != 0 {
				t.Fatalf("result=%+v err=%v", out, err)
			}
			want := 3
			if mode == "deleted" || mode == "changed" {
				want = 2
			}
			if len(out.Workloads) != want {
				t.Fatalf("workloads=%+v", out.Workloads)
			}
			if mode == "truncated" && !out.Truncated {
				t.Fatal("lost pagination warning")
			}
			mu.Lock()
			defer mu.Unlock()
			if mode == "indexed" || mode == "truncated" || mode == "changed" || mode == "deleted" {
				if fullLists != 0 || len(gets) != 3 {
					t.Fatalf("must only fetch three possible matches: gets=%v fullLists=%d", gets, fullLists)
				}
			} else if fullLists == 0 {
				t.Fatal("missing full-list fallback")
			}
		})
	}
}

func TestWorkloadIndexOnlyExcludesProvenContradictions(t *testing.T) {
	for _, tc := range []struct {
		application, workload string
		possible              bool
	}{
		{"app=user", "app=other", false},
		{"app=user", "app=user", true},
		{"app=user", "role=api", true},
		{"app=user", "app in (user,other)", true},
		{"app in (user,admin)", "app=other", false},
		{"!app", "app=user", false},
		{"tier=web", "app=user", true},
		{"app!=user", "app=other", true},
		{"app=user", "", true},
	} {
		application, err := labels.Parse(tc.application)
		if err != nil {
			t.Fatal(err)
		}
		workload, err := labels.Parse(tc.workload)
		if err != nil {
			t.Fatal(err)
		}
		if got := couldMatchTemplate(application, workload); got != tc.possible {
			t.Errorf("application=%q workload=%q got=%v", tc.application, tc.workload, got)
		}
	}
}
