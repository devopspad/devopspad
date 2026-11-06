package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
)

func TestDesktopActionsDryRunAndApply(t *testing.T) {
	for _, action := range []string{"scale", "restart"} {
		t.Run(action, func(t *testing.T) {
			original := desktopObject()
			obj := original.DeepCopy()
			var lease *unstructured.Unstructured
			dry, real := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "/leases") {
					if r.Method == http.MethodGet && lease == nil {
						w.WriteHeader(404)
						fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
						return
					}
					if r.Method != http.MethodGet {
						lease = &unstructured.Unstructured{}
						if err := json.NewDecoder(r.Body).Decode(lease); err != nil {
							t.Error(err)
						}
						lease.SetResourceVersion("1")
					}
					_ = json.NewEncoder(w).Encode(lease.Object)
					return
				}
				if r.Method == http.MethodPut {
					candidate := &unstructured.Unstructured{}
					if err := json.NewDecoder(r.Body).Decode(candidate); err != nil {
						t.Error(err)
					}
					// JSON numbers decoded into Unstructured retain their integer types.
					if candidate.GetResourceVersion() != "17" || candidate.GetUID() != "uid-1" {
						t.Error("lost concurrency preconditions")
					}
					oldTemplate, _, _ := unstructured.NestedMap(original.Object, "spec", "template")
					newTemplate, _, _ := unstructured.NestedMap(candidate.Object, "spec", "template")
					replicas, _, _ := unstructured.NestedInt64(candidate.Object, "spec", "replicas")
					if action == "scale" {
						if !reflect.DeepEqual(oldTemplate, newTemplate) || replicas != 0 {
							t.Error("scale changed template or lost zero replicas")
						}
					} else {
						spec, _, _ := unstructured.NestedMap(candidate.Object, "spec", "template", "spec")
						before, _, _ := unstructured.NestedMap(original.Object, "spec", "template", "spec")
						annotation, _, _ := unstructured.NestedString(candidate.Object, "spec", "template", "metadata", "annotations", "infradex.io/release")
						if !reflect.DeepEqual(spec, before) || replicas != 2 || annotation == "" {
							t.Error("restart changed containers/replicas or omitted annotation")
						}
					}
					if r.URL.Query().Get("dryRun") == "All" {
						dry++
						_ = json.NewEncoder(w).Encode(candidate.Object)
						return
					}
					real++
					candidate.SetGeneration(4)
					candidate.SetResourceVersion("18")
					candidate.Object["status"] = map[string]any{"observedGeneration": int64(4), "updatedReplicas": replicas, "availableReplicas": replicas, "replicas": replicas}
					obj = candidate
				}
				_ = json.NewEncoder(w).Encode(obj.Object)
			}))
			defer server.Close()
			cl, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := PrepareDesktopAction(context.Background(), cl, "default", "svc-user", "app=svc-user", action, 0)
			if err != nil {
				t.Fatal(err)
			}
			if dry != 1 || real != 0 || plan.Action != action || plan.BeforeReplicas != 2 {
				t.Fatalf("plan mutated resource: dry=%d real=%d plan=%+v", dry, real, plan)
			}
			if err := ApplyDesktop(context.Background(), cl, plan, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if real != 1 {
				t.Fatalf("real updates: %d", real)
			}
			holder, _, _ := unstructured.NestedString(lease.Object, "spec", "holderIdentity")
			if holder != "" {
				t.Fatal("lease retained")
			}
			if err := ApplyDesktop(context.Background(), cl, plan, func(string) error { return nil }); err == nil {
				t.Fatal("stale action replayed")
			}
		})
	}
}

func TestDesktopActionsRejectInvalidTargets(t *testing.T) {
	for _, tc := range []struct {
		action, selector string
		replicas         int64
	}{
		{"delete", "app=svc-user", 1}, {"scale", "app=svc-user", -1}, {"scale", "app=svc-user", 1001},
		{"scale", "app=svc-user", 2}, {"restart", "app=other", 0},
	} {
		cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), desktopObject())
		if _, err := PrepareDesktopAction(context.Background(), cl, "default", "svc-user", tc.selector, tc.action, tc.replicas); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
		for _, a := range cl.Actions() {
			if a.GetVerb() == "update" {
				t.Fatal("invalid action updated workload")
			}
		}
	}
	obj := desktopObject()
	_ = unstructured.SetNestedField(obj.Object, true, "spec", "paused")
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	if _, err := PrepareDesktopAction(context.Background(), cl, "default", "svc-user", "app=svc-user", "restart", 0); err == nil {
		t.Fatal("restarted paused Deployment")
	}
	// A failed history must not look ready just because replica totals happen to match.
	obj.SetGeneration(4)
	_ = unstructured.SetNestedField(obj.Object, int64(3), "status", "observedGeneration")
	if desktopRolloutReady(obj) {
		t.Fatal("stale controller status accepted")
	}
}
