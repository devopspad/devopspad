package workspace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClusterListsAreConcurrentAndWorkloadsSkipPods(t *testing.T) {
	for _, mode := range []string{"cluster", "workloads", "workbench"} {
		t.Run(mode, func(t *testing.T) {
			want := int32(3)
			if mode == "cluster" {
				want = 4
			}
			var arrived atomic.Int32
			var once sync.Once
			ready := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/namespaces/default/pods" && mode != "cluster" {
					t.Error("workload listing must not request pods")
				}
				if arrived.Add(1) == want {
					once.Do(func() { close(ready) })
				}
				select {
				case <-ready:
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","items":[]}`)
				case <-r.Context().Done():
				}
			}))
			defer srv.Close()
			path := filepath.Join(t.TempDir(), "config")
			cfg := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: local\nclusters:\n- name: local\n  cluster:\n    server: %s\ncontexts:\n- name: local\n  context:\n    cluster: local\n", srv.URL)
			if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c := Connection{Kind: "k3s", Kubeconfig: path}
			var err error
			switch mode {
			case "cluster":
				_, err = ReadOpsCluster(ctx, c, "default", "")
			case "workloads":
				_, err = ReadOpsWorkloads(ctx, c, "default", "")
			case "workbench":
				_, err = ListWorkloads(ctx, c, "default")
			}
			if err != nil || ctx.Err() != nil || arrived.Load() != want {
				t.Fatalf("lists did not run concurrently: requests=%d want=%d err=%v context=%v", arrived.Load(), want, err, ctx.Err())
			}
		})
	}
}

func TestOpsClusterUsesTemplateLabelsAndMetricsPods(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/default/deployments":
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"svc-user","namespace":"default","uid":"dep-1"},"spec":{"replicas":2,"selector":{"matchLabels":{"app":"svc-user"},"matchExpressions":[{"key":"role","operator":"In","values":["api"]}]},"template":{"metadata":{"labels":{"app":"svc-user"}},"spec":{"containers":[{"name":"app","image":"app:1"}]}}},"status":{"readyReplicas":2}}]}`)
		case "/apis/apps/v1/namespaces/default/statefulsets", "/apis/apps/v1/namespaces/default/daemonsets":
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"List","items":[]}`)
		case "/api/v1/namespaces/default/pods":
			if r.URL.Query().Get("labelSelector") != "app=svc-user" {
				t.Error("missing application scope")
			}
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","items":[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"user-1","namespace":"default","uid":"pod-1","labels":{"app":"svc-user","role":"api"}},"spec":{"containers":[{"name":"app","image":"app:1","ports":[{"containerPort":9001}]}]},"status":{"phase":"Running","podIP":"10.0.0.1","containerStatuses":[{"name":"app","ready":true,"restartCount":2,"state":{"running":{}}}]}}]}`)
		case "/apis/metrics.k8s.io/v1beta1/namespaces/default/pods":
			fmt.Fprint(w, `{"apiVersion":"metrics.k8s.io/v1beta1","kind":"PodMetricsList","items":[{"metadata":{"name":"user-1"},"timestamp":"2026-10-23T01:00:00Z","containers":[{"usage":{"cpu":"125m","memory":"64Mi"}},{"usage":{"cpu":"25000000n","memory":"16Mi"}}]},{"metadata":{"name":"other-app"},"containers":[{"usage":{"cpu":"1","memory":"1Gi"}}]}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cfg := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: local\nclusters:\n- name: local\n  cluster:\n    server: %s\ncontexts:\n- name: local\n  context:\n    cluster: local\n    user: reader\nusers:\n- name: reader\n  user:\n    token: test\n", srv.URL)
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	c := Connection{Kind: "k3s", Kubeconfig: path, Context: "local"}
	out, err := ReadOpsCluster(context.Background(), c, "default", "app=svc-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Workloads) != 1 || len(out.Pods) != 1 || out.Pods[0].Workload != "svc-user" || out.Pods[0].Restarts != 2 || out.Pods[0].Ready != 1 {
		t.Fatalf("%+v", out)
	}
	metrics, err := ReadOpsMetrics(context.Background(), c, "default", "app=svc-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 || metrics[0].CPU != 150 || metrics[0].Memory != 80 {
		t.Fatalf("metrics=%+v", metrics)
	}
}

func TestReleaseConnectionPinsClusterEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	config := func(host string) string {
		return fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: local\nclusters:\n- name: local\n  cluster:\n    server: %s\ncontexts:\n- name: local\n  context:\n    cluster: local\n", host)
	}
	if err := os.WriteFile(path, []byte(config("https://before.invalid")), 0600); err != nil {
		t.Fatal(err)
	}
	pinned, err := PinClusterConnection(Connection{Kind: "k3s", Kubeconfig: path})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(config("https://after.invalid")), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := kubeConfig(pinned)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://before.invalid" {
		t.Fatalf("release redirected to %s", cfg.Host)
	}
}
