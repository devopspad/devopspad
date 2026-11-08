package opsrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/release"
	"devopspad/internal/workspace"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

const digestImage = "ghcr.io/x/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestSaveAndTakePlan(t *testing.T) {
	dir := t.TempDir()
	p := SavedPlan{ID: uuid.NewString(), Environment: "pro", Image: digestImage, ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
		Targets: []SavedTarget{{Application: "api", Workload: "api", Container: "app"}}}
	if err := SavePlan(dir, p); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "plans", p.ID+".json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("plan file: %v %v", info, err)
	}
	got, err := TakePlan(dir, p.ID)
	if err != nil || got.Targets[0].Workload != "api" {
		t.Fatalf("take: %+v %v", got, err)
	}
	if _, err := TakePlan(dir, p.ID); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("second take: %v", err)
	}
	p.ID = uuid.NewString()
	p.ExpiresAt = time.Now().Add(-time.Second).UTC().Format(time.RFC3339)
	_ = SavePlan(dir, p)
	if _, err := TakePlan(dir, p.ID); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired: %v", err)
	}
	if _, err := TakePlan(dir, "../../etc/passwd"); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestBatchRecordAndApplyStopsAtFirstFailure(t *testing.T) {
	store, err := ops.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saved := SavedPlan{ID: uuid.NewString(), Image: digestImage, Version: "1.2.0", Targets: []SavedTarget{
		{Application: "api", Workload: "api", Container: "app", BeforeImage: "old@sha256:1"},
		{Application: "worker", Workload: "worker", Container: "worker", BeforeImage: "old@sha256:1"},
	}}
	r := BatchRecord(ops.Environment{Name: "pro", Connection: "hk", Namespace: "default"}, saved)
	if r.Workload != "api, worker" || r.Container != "app, worker" || r.BeforeImage != "old@sha256:1" || len(r.Targets) != 2 {
		t.Fatalf("record: %+v", r)
	}
	old := openCluster
	defer func() { openCluster = old }()
	openCluster = func(context.Context, workspace.Connection) (dynamic.Interface, func(), error) {
		return fake.NewSimpleDynamicClient(runtime.NewScheme()), func() {}, nil
	}
	// Plans without a prepared candidate fail as expired before touching the cluster.
	plans := []release.DesktopPlan{{ID: saved.ID, Workload: "api"}, {ID: saved.ID, Workload: "worker"}}
	out, err := ApplyBatch(context.Background(), store, workspace.Connection{}, plans, r, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "failed" || !strings.HasPrefix(out.Message, "api: ") || out.Targets[0].Status != "failed" || out.Targets[1].Status != "not-started" {
		t.Fatalf("result: %+v", out)
	}
	ran := false
	out, _ = ApplyBatch(context.Background(), store, workspace.Connection{}, plans, r, func(_ context.Context, _ dynamic.Interface, step func(string) error) error {
		ran = true
		_ = step("created Job migrate-1")
		return errors.New("Job migrate-1 failed")
	}, nil)
	if !ran || out.Status != "failed" || !strings.Contains(out.Message, "no image was changed") || out.Targets[0].Status != "not-started" || out.Steps[0] != "migrate: created Job migrate-1" {
		t.Fatalf("migration failure: %+v", out)
	}
	snap, _ := store.Read()
	if len(snap.Releases) != 1 || snap.Releases[0].Status != "failed" {
		t.Fatalf("records on disk: %+v", snap.Releases)
	}
}

func TestRenderJob(t *testing.T) {
	id := "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0"
	manifest := `apiVersion: batch/v1
kind: Job
metadata:
  name: api-migrate
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: ghcr.io/x/api:latest
          args: ["--job=migrate"]
        - name: sidecar
          image: busybox:1
`
	job, err := RenderJob([]byte(manifest), digestImage, "default", id)
	if err != nil {
		t.Fatal(err)
	}
	cs, _, _ := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
	if cs[0].(map[string]any)["image"] != digestImage || cs[1].(map[string]any)["image"] != "busybox:1" {
		t.Fatalf("images: %v", cs)
	}
	if job.GetName() != "api-migrate-0f1e2d3c" || job.GetNamespace() != "default" || job.GetAnnotations()["devopspad.io/release"] != id {
		t.Fatalf("metadata: %v %v %v", job.GetName(), job.GetNamespace(), job.GetAnnotations())
	}
	placeholder := strings.Replace(manifest, "ghcr.io/x/api:latest", ImagePlaceholder, 1)
	placeholder = strings.Replace(placeholder, "ghcr.io", "other.io", 1)
	if job, err := RenderJob([]byte(placeholder), digestImage, "default", id); err != nil || !strings.Contains(string(mustJSON(job)), digestImage) {
		t.Fatalf("placeholder: %v", err)
	}
	for name, bad := range map[string]string{
		"kind":      strings.Replace(manifest, "kind: Job", "kind: Deployment", 1),
		"namespace": strings.Replace(manifest, "name: api-migrate", "name: api-migrate\n  namespace: prod", 1),
		"two docs":  manifest + "---\n" + manifest,
		"no match":  strings.Replace(manifest, "ghcr.io/x/api:latest", "ghcr.io/x/other:1", 1),
		"bad yaml":  "kind: [",
	} {
		if _, err := RenderJob([]byte(bad), digestImage, "default", id); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func mustJSON(u *unstructured.Unstructured) []byte {
	b, _ := u.MarshalJSON()
	return b
}

func TestRunJob(t *testing.T) {
	job, err := RenderJob([]byte("apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: m\nspec:\n  template:\n    spec:\n      containers:\n        - name: c\n          image: "+ImagePlaceholder+"\n"), digestImage, "default", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"Complete", "Failed"} {
		cl := fake.NewSimpleDynamicClient(runtime.NewScheme())
		cl.PrependReactor("get", "jobs", func(action ktesting.Action) (bool, runtime.Object, error) {
			done := job.DeepCopy()
			done.Object["status"] = map[string]any{"conditions": []any{map[string]any{"type": outcome, "status": "True", "reason": "BackoffLimitExceeded"}}}
			return true, done, nil
		})
		var steps []string
		err := RunJob(context.Background(), cl, job, 10*time.Second, func(s string) error { steps = append(steps, s); return nil })
		if outcome == "Complete" && (err != nil || len(steps) != 2 || !strings.Contains(steps[1], "completed")) {
			t.Fatalf("complete: %v %v", err, steps)
		}
		if outcome == "Failed" && (err == nil || !strings.Contains(err.Error(), "BackoffLimitExceeded")) {
			t.Fatalf("failed: %v", err)
		}
	}
	stuck := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": job.GetName() + "-x", "namespace": "default", "labels": map[string]any{"job-name": job.GetName()}},
		"status":   map[string]any{"containerStatuses": []any{map[string]any{"name": "c", "image": digestImage, "state": map[string]any{"waiting": map[string]any{"reason": "ImagePullBackOff"}}}}}}}
	cl := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Version: "v1", Resource: "pods"}: "PodList"}, stuck)
	err = RunJob(context.Background(), cl, job, 2*time.Second, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "did not complete") || !strings.Contains(err.Error(), "pods waiting: ImagePullBackOff for "+digestImage) {
		t.Fatalf("timeout: %v", err)
	}
}

func TestReprepareRefusesChangedDeployment(t *testing.T) {
	dep := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "api", "namespace": "default", "uid": "u1", "resourceVersion": "5"},
		"spec":     map[string]any{"replicas": int64(1), "template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "api"}}, "spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": "old:1"}}}}}}}
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), dep)
	cl.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, action.(ktesting.UpdateActionImpl).GetObject(), nil
	})
	target := Target{Environment: ops.Environment{Namespace: "default"}, Application: ops.Application{Name: "api", Selector: "app=api"}}
	saved := SavedPlan{ID: uuid.NewString(), Image: digestImage}
	p, err := Reprepare(context.Background(), cl, target, saved, SavedTarget{Workload: "api", Container: "app", UID: "u1", ResourceVersion: "5"})
	if err != nil || p.ID != saved.ID {
		t.Fatalf("unchanged: %v %s", err, p.ID)
	}
	if _, err := Reprepare(context.Background(), cl, target, saved, SavedTarget{Workload: "api", Container: "app", UID: "u1", ResourceVersion: "4"}); err == nil || !strings.Contains(err.Error(), "changed since the plan") {
		t.Fatalf("changed deployment accepted: %v", err)
	}
}
