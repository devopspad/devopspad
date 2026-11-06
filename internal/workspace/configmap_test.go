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

func TestSaveConfigMap(t *testing.T) {
	for _, scenario := range []string{"save", "stale", "replaced", "immutable", "race", "forbidden", "missing-version", "wrong-resource", "invalid-key"} {
		t.Run(scenario, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/api/v1/namespaces/team/configmaps/settings" {
					t.Errorf("unexpected target %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				obj := map[string]any{
					"apiVersion": "v1", "kind": "ConfigMap",
					"metadata": map[string]any{"name": "settings", "namespace": "team", "uid": "uid-1", "resourceVersion": "12", "labels": map[string]any{"owner": "portal"}, "annotations": map[string]any{"keep": "yes"}},
					"data":     map[string]any{"app.yaml": "port: 80\n", "keep": "unchanged"}, "binaryData": map[string]any{"cert": "YWJj"},
				}
				if scenario == "immutable" {
					obj["immutable"] = true
				}
				switch r.Method {
				case http.MethodGet:
					_ = json.NewEncoder(w).Encode(obj)
				case http.MethodPut:
					writes++
					var updated map[string]any
					if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
						t.Error(err)
					}
					meta := updated["metadata"].(map[string]any)
					if meta["uid"] != "uid-1" || meta["resourceVersion"] != "12" || meta["labels"].(map[string]any)["owner"] != "portal" || meta["annotations"].(map[string]any)["keep"] != "yes" {
						t.Errorf("identity or metadata lost: %v", meta)
					}
					if updated["binaryData"].(map[string]any)["cert"] != "YWJj" {
						t.Error("binary data lost")
					}
					data := updated["data"].(map[string]any)
					if data["app.yaml"] != "port: 8080\n" || data["keep"] != "unchanged" {
						t.Errorf("wrong data: %v", data)
					}
					if scenario == "race" || scenario == "forbidden" {
						code, reason := http.StatusConflict, "Conflict"
						if scenario == "forbidden" {
							code, reason = http.StatusForbidden, "Forbidden"
						}
						w.WriteHeader(code)
						_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": reason, "code": code, "message": reason})
						return
					}
					meta["resourceVersion"] = "13"
					_ = json.NewEncoder(w).Encode(updated)
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			defer srv.Close()
			path := filepath.Join(t.TempDir(), "config")
			cfg := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: wrong\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: selected\n  context:\n    cluster: test\n", srv.URL)
			if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
				t.Fatal(err)
			}
			request := ConfigMapUpdate{Ref: ClusterRef{Context: "selected", Namespace: "team", Resource: "configmaps", Name: "settings", UID: "uid-1"}, ResourceVersion: "12", Data: map[string]string{"app.yaml": "port: 8080\n", "keep": "unchanged"}}
			switch scenario {
			case "stale":
				request.ResourceVersion = "11"
			case "replaced":
				request.Ref.UID = "old-uid"
			case "missing-version":
				request.ResourceVersion = ""
			case "wrong-resource":
				request.Ref.Resource = "secrets"
			case "invalid-key":
				request.Data["../bad"] = "invalid"
			}
			result, err := SaveConfigMap(context.Background(), Connection{Kind: "k3s", Kubeconfig: path}, request)
			if scenario == "save" {
				if err != nil || !strings.Contains(result, `"resourceVersion":"13"`) {
					t.Fatalf("save: %s %v", result, err)
				}
			} else if err == nil {
				t.Fatal("expected failure")
			}
			expectedWrites := 0
			if scenario == "save" || scenario == "race" || scenario == "forbidden" {
				expectedWrites = 1
			}
			if writes != expectedWrites {
				t.Fatalf("writes=%d want=%d", writes, expectedWrites)
			}
			if scenario == "race" && !strings.Contains(err.Error(), "集群配置已变化") {
				t.Fatal(err)
			}
			if scenario == "forbidden" && !strings.Contains(err.Error(), "权限") {
				t.Fatal(err)
			}
		})
	}
}
