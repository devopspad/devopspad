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
	"k8s.io/apimachinery/pkg/runtime"
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
	if err == nil || !strings.Contains(err.Error(), "已变化") {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
	steps := []string{}
	if err = ApplyDesktop(context.Background(), cl, p, func(s string) error { steps = append(steps, s); return nil }); err != nil {
		t.Fatal(err)
	}
	if updates != 1 || len(steps) != 4 {
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
