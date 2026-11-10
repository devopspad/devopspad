package release

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func desktopObject() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "svc-user", "namespace": "default", "uid": "uid-1", "resourceVersion": "17", "generation": int64(3)}, "spec": map[string]any{"replicas": int64(2), "template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "svc-user"}}, "spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": "old:1"}}}}}, "status": map[string]any{"observedGeneration": int64(3), "updatedReplicas": int64(2), "availableReplicas": int64(2), "replicas": int64(2)}}}
}
func TestDesktopPlanIsDryRunAndRejectsStaleApply(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), desktopObject())
	updates := 0
	cl.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		a := action.(ktesting.UpdateActionImpl)
		updates++
		// The client-go dynamic fake drops UpdateOptions; HTTP tests cover dryRun.
		if updates == 1 {
			return true, a.GetObject(), nil
		}
		return false, nil, nil
	})
	p, err := PrepareDesktop(context.Background(), cl, "default", "svc-user", "app", "registry.local/app@sha256:"+strings.Repeat("a", 64), "app=svc-user")
	if err != nil {
		t.Fatal(err)
	}
	current, _ := cl.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "svc-user", metav1.GetOptions{})
	images, _, _ := unstructured.NestedSlice(current.Object, "spec", "template", "spec", "containers")
	if images[0].(map[string]any)["image"] != "old:1" {
		t.Fatal("plan mutated deployment")
	}
	current.SetResourceVersion("18")
	if err = cl.Tracker().Update(deploymentGVR, current, "default"); err != nil {
		t.Fatal(err)
	}
	err = ApplyDesktop(context.Background(), cl, p, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale apply: %v", err)
	}
	if updates != 1 {
		t.Fatal("stale deployment updated")
	}
}
func TestDesktopLeaseAndRollout(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), desktopObject())
	p := DesktopPlan{ID: "first", Namespace: "default", Workload: "svc-user"}
	unlock, err := acquireDesktopLease(context.Background(), cl, p)
	if err != nil {
		t.Fatal(err)
	}
	p.ID = "second"
	if _, err = acquireDesktopLease(context.Background(), cl, p); err == nil {
		t.Fatal("second client acquired held lease")
	}
	unlock()
	unlock, err = acquireDesktopLease(context.Background(), cl, p)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	obj := desktopObject()
	if !desktopRolloutReady(obj) {
		t.Fatal("ready deployment rejected")
	}
	_ = unstructured.SetNestedField(obj.Object, int64(1), "status", "updatedReplicas")
	if desktopRolloutReady(obj) {
		t.Fatal("old replicas marked complete")
	}
	_ = unstructured.SetNestedField(obj.Object, int64(2), "status", "updatedReplicas")
	obj.SetGeneration(4)
	if desktopRolloutReady(obj) {
		t.Fatal("unobserved generation marked complete")
	}
	p.ExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339)
	if err = ApplyDesktop(context.Background(), cl, p, func(string) error { return nil }); err == nil {
		t.Fatal("expired plan accepted")
	}
}
func TestDesktopPrepareRejectsWrongApplicationAndContainer(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), desktopObject())
	for _, tc := range []struct{ container, selector string }{{"app", "app=other"}, {"missing", "app=svc-user"}} {
		if _, err := PrepareDesktop(context.Background(), cl, "default", "svc-user", tc.container, "registry.local/app@sha256:"+strings.Repeat("a", 64), tc.selector); err == nil {
			t.Fatal("accepted invalid target", tc)
		}
	}
}

func TestDesktopPlanSendsServerDryRun(t *testing.T) {
	updates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			updates++
			if r.URL.Query().Get("dryRun") != "All" {
				t.Error("plan sent a real mutation")
			}
			var obj map[string]any
			if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(obj)
			return
		}
		_ = json.NewEncoder(w).Encode(desktopObject().Object)
	}))
	defer server.Close()
	cl, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	p, err := PrepareDesktop(context.Background(), cl, "default", "svc-user", "app", "registry.local/app@sha256:"+strings.Repeat("a", 64), "app=svc-user")
	if err != nil {
		t.Fatal(err)
	}
	if updates != 1 || p.BeforeImage != "old:1" {
		t.Fatalf("updates=%d plan=%+v", updates, p)
	}
}

func TestDesktopApplyRecordsStepsAndReleasesLease(t *testing.T) {
	obj := desktopObject()
	var lease *unstructured.Unstructured
	updates := 0
	var pods []any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pods") {
			_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "PodList", "metadata": map[string]any{}, "items": pods})
			return
		}
		if strings.Contains(r.URL.Path, "/leases") {
			switch r.Method {
			case http.MethodGet:
				if lease == nil {
					w.WriteHeader(404)
					fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
					return
				}
			case http.MethodPost, http.MethodPut:
				lease = &unstructured.Unstructured{}
				if err := json.NewDecoder(r.Body).Decode(&lease.Object); err != nil {
					t.Error(err)
				}
				lease.SetResourceVersion("1")
			default:
				t.Errorf("unexpected lease method %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(lease.Object)
			return
		}
		if r.Method == http.MethodPut {
			candidate := &unstructured.Unstructured{}
			if err := json.NewDecoder(r.Body).Decode(&candidate.Object); err != nil {
				t.Error(err)
			}
			if r.URL.Query().Get("dryRun") == "All" {
				_ = json.NewEncoder(w).Encode(candidate.Object)
				return
			}
			if candidate.GetResourceVersion() != obj.GetResourceVersion() {
				t.Error("resourceVersion precondition missing")
			}
			updates++
			candidate.SetGeneration(4)
			candidate.SetResourceVersion("18")
			candidate.Object["status"] = map[string]any{"observedGeneration": int64(4), "updatedReplicas": int64(2), "availableReplicas": int64(2), "replicas": int64(2)}
			obj = candidate
		}
		_ = json.NewEncoder(w).Encode(obj.Object)
	}))
	defer server.Close()
	cl, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	p, err := PrepareDesktop(context.Background(), cl, "default", "svc-user", "app", "registry.local/app@sha256:"+strings.Repeat("a", 64), "app=svc-user")
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	pods = []any{releasedPod("svc-user-b", p.ID, "docker.io/registry.local/app@"+digest), releasedPod("svc-user-a", p.ID, "registry.local/app@"+digest), releasedPod("svc-user-old", "previous", "old@sha256:"+strings.Repeat("0", 64))}
	steps := []string{}
	if err = ApplyDesktop(context.Background(), cl, p, func(s string) error { steps = append(steps, s); return nil }); err != nil {
		t.Fatal(err)
	}
	if updates != 1 || len(steps) != 5 || steps[4] != "image-check: svc-user-a, svc-user-b run "+digest {
		t.Fatalf("updates=%d steps=%v", updates, steps)
	}
	holder, _, _ := unstructured.NestedString(lease.Object, "spec", "holderIdentity")
	if holder != "" {
		t.Fatal("lease not released")
	}
	if err = ApplyDesktop(context.Background(), cl, p, func(string) error { return nil }); err == nil {
		t.Fatal("replayed stale plan accepted")
	}
}

func releasedPod(name, release, imageID string) map[string]any {
	return workloadPod(name, "svc-user", "app", release, imageID)
}

func workloadPod(name, app, container, release, imageID string) map[string]any {
	return map[string]any{"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": name, "namespace": "default", "labels": map[string]any{"app": app}, "annotations": map[string]any{"devopspad.io/release": release}},
		"spec":     map[string]any{"containers": []any{map[string]any{"name": container}}},
		"status":   map[string]any{"phase": "Running", "containerStatuses": []any{map[string]any{"name": container, "imageID": imageID}}}}
}

func TestVerifyReleasedDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	p := DesktopPlan{ID: "rel-1", Namespace: "default", Container: "app", Image: "ghcr.io/x/app@" + digest}
	pods := func(items ...map[string]any) dynamic.Interface {
		objs := []runtime.Object{}
		for _, it := range items {
			objs = append(objs, &unstructured.Unstructured{Object: it})
		}
		return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{podGVR: "PodList"}, objs...)
	}
	all := labels.Everything()
	if _, err := verifyReleasedDigest(context.Background(), pods(releasedPod("a", "rel-1", "ghcr.io/x/app@sha256:"+strings.Repeat("c", 64))), p, all, 1); err == nil || !strings.Contains(err.Error(), "pod a runs") {
		t.Fatalf("mismatch accepted: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := verifyReleasedDigest(ctx, pods(releasedPod("old", "other", "x@"+digest)), p, all, 2); err == nil || !strings.Contains(err.Error(), "annotation") {
		t.Fatalf("missing pods accepted: %v", err)
	}
	if msg, err := verifyReleasedDigest(context.Background(), pods(), p, all, 0); err != nil || !strings.Contains(msg, "0 replicas") {
		t.Fatalf("scaled-to-zero: %q %v", msg, err)
	}
	tag := p
	tag.Image = "ghcr.io/x/app:1.2"
	if msg, _ := verifyReleasedDigest(context.Background(), pods(), tag, all, 1); !strings.Contains(msg, "skipped") {
		t.Fatalf("tag image: %q", msg)
	}
}

// A multi-target release stamps one release ID on every target, so the check
// for one Deployment must only look at that Deployment's pods.
func TestVerifyReleasedDigestOnlyChecksTargetWorkload(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	cl := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{podGVR: "PodList"},
		&unstructured.Unstructured{Object: workloadPod("api-1", "api", "api", "rel-1", "")},
		&unstructured.Unstructured{Object: workloadPod("worker-1", "worker", "worker", "rel-1", "ghcr.io/x/app@"+digest)})
	worker := desktopObject()
	_ = unstructured.SetNestedField(worker.Object, map[string]any{"matchLabels": map[string]any{"app": "worker"}}, "spec", "selector")
	p := DesktopPlan{ID: "rel-1", Namespace: "default", Container: "worker", Image: "ghcr.io/x/app@" + digest}
	msg, err := verifyReleasedDigest(context.Background(), cl, p, podSelector(worker), 1)
	if err != nil || msg != "image-check: worker-1 run "+digest {
		t.Fatalf("worker check: %q %v", msg, err)
	}
	// Pods without the target container are skipped, even if a selector overlaps.
	if msg, err = verifyReleasedDigest(context.Background(), cl, p, labels.Everything(), 1); err != nil || msg != "image-check: worker-1 run "+digest {
		t.Fatalf("container filter: %q %v", msg, err)
	}
}

func TestDesktopLeaseRenewTimeIsMicroTime(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), desktopObject())
	var written []string
	record := func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(interface{ GetObject() runtime.Object }).GetObject().(*unstructured.Unstructured)
		if renew, found, _ := unstructured.NestedString(obj.Object, "spec", "renewTime"); found {
			written = append(written, renew)
		}
		return false, nil, nil
	}
	cl.PrependReactor("create", "leases", record)
	cl.PrependReactor("update", "leases", record)
	old := leaseClock
	defer func() { leaseClock = old }()
	p := DesktopPlan{ID: "first", Namespace: "default", Workload: "svc-user"}
	// From the issue: microseconds ending in 0, and a full nanosecond clock.
	for _, at := range []time.Time{time.Date(2026, 11, 9, 5, 21, 38, 950670000, time.UTC), time.Date(2026, 11, 9, 5, 21, 38, 123456789, time.UTC)} {
		leaseClock = func() time.Time { return at }
		unlock, err := acquireDesktopLease(context.Background(), cl, p)
		if err != nil {
			t.Fatal(err)
		}
		unlock()
	}
	if len(written) == 0 {
		t.Fatal("renewTime not written")
	}
	for _, renew := range written {
		// The API server parses MicroTime with exactly six fractional digits.
		if _, frac, _ := strings.Cut(strings.TrimSuffix(renew, "Z"), "."); len(frac) != 6 {
			t.Fatalf("renewTime %q is not a MicroTime", renew)
		}
	}
}

func TestDesktopLeaseIsReentrantForThePlan(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), desktopObject())
	p := DesktopPlan{ID: "plan-1", Namespace: "default", Workload: "svc-user"}
	outer, err := LockDesktop(context.Background(), cl, p)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := acquireDesktopLease(context.Background(), cl, p)
	if err != nil {
		t.Fatalf("same plan could not re-acquire its lock: %v", err)
	}
	other := p
	other.ID = "plan-2"
	if _, err = acquireDesktopLease(context.Background(), cl, other); err == nil {
		t.Fatal("another plan acquired a held lock")
	}
	inner()
	outer()
	if _, err = acquireDesktopLease(context.Background(), cl, other); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}

func TestDesktopPlanIdentityAndRebind(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), desktopObject())
	cl.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, action.(ktesting.UpdateActionImpl).GetObject(), nil
	})
	p, err := PrepareDesktop(context.Background(), cl, "default", "svc-user", "app", "registry.local/app@sha256:"+strings.Repeat("a", 64), "app=svc-user")
	if err != nil {
		t.Fatal(err)
	}
	if p.UID != "uid-1" || p.ResourceVersion != "17" {
		t.Fatalf("identity: %q %q", p.UID, p.ResourceVersion)
	}
	p.Rebind("saved-plan")
	ann, _, _ := unstructured.NestedStringMap(p.candidate.Object, "spec", "template", "metadata", "annotations")
	if p.ID != "saved-plan" || ann["devopspad.io/release"] != "saved-plan" {
		t.Fatalf("rebind: %s %v", p.ID, ann)
	}
}
