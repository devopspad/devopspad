package main

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
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/workspace"
	"github.com/google/uuid"
)

func TestOpsActionsBridgePersistsFailureAndConsumesPlan(t *testing.T) {
	var received []string
	image := "registry.local/app@sha256:" + strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/leases") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","message":"leases forbidden","code":403}`)
			return
		}
		if r.Method == "PUT" {
			if r.URL.Query().Get("dryRun") != "All" {
				t.Error("unexpected real deployment write")
			}
			var obj map[string]any
			_ = json.NewDecoder(r.Body).Decode(&obj)
			_ = json.NewEncoder(w).Encode(obj)
			return
		}
		fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"svc-user","namespace":"default","uid":"w1","resourceVersion":"1","generation":1},"spec":{"replicas":2,"template":{"metadata":{"labels":{"app":"svc-user"}},"spec":{"containers":[{"name":"app","image":"old:1"}]}}}}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	kube := filepath.Join(dir, "kubeconfig")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: local\nclusters:\n- name: local\n  cluster:\n    server: %s\ncontexts:\n- name: local\n  context:\n    cluster: local\n", server.URL)
	if err := os.WriteFile(kube, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.ctx = context.Background()
	var err error
	a.store, err = workspace.NewStore(filepath.Join(dir, "workspace"), testVault{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.SaveConnection(workspace.Connection{Name: "local", Kind: "k8s", Kubeconfig: kube}); err != nil {
		t.Fatal(err)
	}
	a.ops, err = ops.New(filepath.Join(dir, "project"))
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := a.GetOpsProject()
	yaml := "apiVersion: devopspad.project/v1\nname: Test\nenvironments:\n- name: pro\n  connection: local\n  namespace: default\n- name: dev\n  connection: local\n  namespace: dev\napplications:\n- name: svc-user\n  selector: app=svc-user\n"
	if _, err = a.SaveOpsProject(yaml, snap.Revision); err != nil {
		t.Fatal(err)
	}
	plan, err := a.PlanOpsWorkloadAction("pro", "svc-user", "svc-user", "scale", 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != "scale" || plan.Replicas != 0 || plan.BeforeReplicas != 2 {
		t.Fatalf("bad plan: %+v", plan)
	}
	result, err := a.ApplyOpsRelease(plan.ID)
	if err != nil || result.Status != "failed" || result.Action != "scale" || !strings.Contains(result.Message, "leases") {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	snap, err = a.GetOpsProject()
	if err != nil || len(snap.Releases) != 1 || snap.Releases[0].Replicas == nil || *snap.Releases[0].Replicas != 0 {
		t.Fatalf("record not persisted: %+v %v", snap.Releases, err)
	}
	count := len(received)
	if _, err = a.ApplyOpsRelease(plan.ID); err == nil {
		t.Fatal("plan replayed")
	}
	if len(received) != count {
		t.Fatal("replay reached cluster")
	}
	record := ops.Record{ID: uuid.NewString(), Application: "svc-user", Environment: "pro", Connection: "local", Namespace: "default", Workload: "svc-user", Container: "app", Image: image, Version: "1.0", Status: "succeeded", StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if err = a.ops.WriteRecord(record); err != nil {
		t.Fatal(err)
	}
	if _, err = a.PlanOpsRollback("dev", "svc-user", record.ID); err == nil {
		t.Fatal("cross-environment rollback accepted")
	}
	rollback, err := a.PlanOpsRollback("pro", "svc-user", record.ID)
	if err != nil || rollback.Action != "rollback" || rollback.Image != image || rollback.BeforeImage != "old:1" {
		t.Fatalf("rollback=%+v %v", rollback, err)
	}
}
