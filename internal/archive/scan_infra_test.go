package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devopspad/internal/workspace"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
)

func workload(kind, ns, name string, labels map[string]any, desired, ready int64, image string) *unstructured.Unstructured {
	spec := map[string]any{"template": map[string]any{"metadata": map[string]any{"labels": labels},
		"spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": image}}}}}
	status := map[string]any{"readyReplicas": ready}
	if kind != "DaemonSet" {
		spec["replicas"] = desired
	}
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": kind,
		"metadata": map[string]any{"name": name, "namespace": ns}, "spec": spec, "status": status}}
}

func fakeInfra(t *testing.T, readyNode bool) {
	t.Helper()
	node := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "hk-1"},
		"status": map[string]any{"nodeInfo": map[string]any{"kubeletVersion": "v1.31.4+k3s1"},
			"addresses":  []any{map[string]any{"type": "InternalIP", "address": "10.0.0.2"}},
			"conditions": []any{map[string]any{"type": "Ready", "status": map[bool]string{true: "True", false: "False"}[readyNode]}}}}}
	objs := []runtime.Object{node,
		workload("Deployment", "default", "heartie-admin", map[string]any{"app": "heartie-admin"}, 2, 1, "ghcr.io/x/admin@sha256:aa"),
		workload("Deployment", "default", "heartie-api", map[string]any{"app": "heartie-api"}, 1, 1, "ghcr.io/x/api@sha256:bb"),
		workload("Deployment", "default", "blog", map[string]any{"app": "blog"}, 1, 1, "ghost:5"),
		workload("Deployment", "kube-system", "coredns", map[string]any{"k8s-app": "kube-dns"}, 1, 1, "coredns"),
	}
	lists := map[schema.GroupVersionResource]string{
		{Version: "v1", Resource: "nodes"}:                       "NodeList",
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
		{Group: "apps", Version: "v1", Resource: "daemonsets"}:   "DaemonSetList",
	}
	client := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), lists, objs...)
	oldClient, oldRead, oldHost := clusterClient, sshReadFile, hostOutput
	var gotKubeconfig string
	clusterClient = func(_ context.Context, c workspace.Connection) (dynamic.Interface, func(), error) {
		b, err := os.ReadFile(c.Kubeconfig)
		gotKubeconfig = string(b)
		if err != nil || !c.SSH.Enabled || c.SSH.Host != "203.0.113.10" {
			return nil, nil, errors.New("kubeconfig or tunnel not prepared")
		}
		return client, func() {}, nil
	}
	sshReadFile = func(_ context.Context, c workspace.Connection, path string, _ int) ([]byte, error) {
		if path != "/etc/rancher/k3s/k3s.yaml" || c.SSH.Host != "203.0.113.10" {
			return nil, errors.New("unexpected read")
		}
		return []byte("apiVersion: v1\nkind: Config\n"), nil
	}
	conf, _ := os.ReadFile(filepath.Join(example, "products/heartie/configs/admin/pro/heartie-admin.conf"))
	sum := sha256.Sum256(conf)
	hostOutput = func(_ context.Context, c workspace.Connection) ([]byte, error) {
		if c.SSH.Host == "203.0.113.20" {
			return nil, errors.New("connection refused")
		}
		return []byte("@@kernel\nLinux 5.15.0-113-generic\n@@os\nUbuntu 22.04.4 LTS\n@@cpu\n2\n@@mem\nMemTotal:        3999748 kB\n@@disk\n/dev/vda1  61865984 21000000 40865984  34% /\n" +
			"@@services\nk3s.service loaded active running Lightweight Kubernetes\nnginx.service loaded active running nginx\nssh.service loaded active running OpenBSD Secure Shell\n" +
			"@@nginx\n" + hex.EncodeToString(sum[:]) + "  /etc/nginx/conf.d/heartie-admin.conf\nabc123  /etc/nginx/conf.d/old.conf\n@@k3s\nk3s version v1.31.4+k3s1\n"), nil
	}
	t.Cleanup(func() {
		clusterClient, sshReadFile, hostOutput = oldClient, oldRead, oldHost
		if gotKubeconfig == "" {
			t.Log("cluster client was not called")
		}
	})
}

func TestScanClusterAndHosts(t *testing.T) {
	fakeInfra(t, false)
	dir := copyExample(t)
	a := load(t, dir)
	res, err := a.Scan(context.Background(), ScanInput{Kinds: []string{"cluster", "host"}, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Observations) != 3 { // hk-k3s, hk-1, sg-1 (old-1 is retired)
		t.Fatalf("observations: %d", len(res.Observations))
	}
	o, _ := a.ReadObservation("cluster:hk-k3s")
	if o == nil || o.Cluster == nil || len(o.Cluster.Workloads) != 4 || len(o.Cluster.Errors) != 0 || o.Cluster.Nodes[0].InternalIP != "10.0.0.2" {
		t.Fatalf("cluster: %+v", o)
	}
	if matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "devopspad-kube-*", "hk-k3s.yaml")); len(matches) > 0 {
		t.Fatalf("kubeconfig left on disk: %v", matches)
	}
	h, _ := a.ReadObservation("host:hk-1")
	if h.Host.CPU != 2 || h.Host.MemoryGiB != 3.8 || h.Host.DiskGiB != 59 || strings.Join(h.Host.Services, ",") != "k3s,nginx,ssh" || len(h.Host.NginxConfs) != 2 {
		t.Fatalf("host facts: %+v", h.Host)
	}
	sg, _ := a.ReadObservation("host:sg-1")
	if len(sg.Host.Errors) != 1 {
		t.Fatalf("sg-1: %+v", sg.Host)
	}

	drift, err := a.Diff([]string{"cluster:hk-k3s", "host:hk-1", "host:sg-1"}, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	has := func(ref, field, msg string) bool {
		for _, d := range drift {
			if d.Ref == ref && strings.HasPrefix(d.Field, field) && strings.Contains(d.Message, msg) {
				return true
			}
		}
		return false
	}
	for _, want := range [][3]string{
		{"service:heartie/admin", "deployment pro", "not fully ready"},
		{"service:heartie/admin", "deployment dev", "no matching workload"},
		{"cluster:hk-k3s", "default/deployment/blog", "not in the archive"},
		{"cluster:hk-k3s", "node hk-1", "not Ready"},
		{"host:hk-1", "spec.roles", "no running mysql"},
		{"host:hk-1", "/etc/nginx/conf.d/old.conf", "not archived"},
		{"host:sg-1", "", "connection refused"},
	} {
		if !has(want[0], want[1], want[2]) {
			t.Errorf("missing drift %v in %+v", want, drift)
		}
	}
	for _, d := range drift {
		if strings.Contains(d.Field, "coredns") || (d.Ref == "service:heartie/admin" && d.Field == "config nginx") || d.Ref == "service:heartie/api" {
			t.Errorf("unexpected drift: %+v", d)
		}
	}
}

type memVault map[string]string

func (m memVault) Set(k, v string) error { m[k] = v; return nil }
func (m memVault) Get(k string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}
func (m memVault) Delete(k string) error { delete(m, k); return nil }

func TestConnectSync(t *testing.T) {
	fakeInfra(t, true)
	a := load(t, copyExample(t))
	cfg := t.TempDir()
	vault := memVault{}
	store, err := workspace.NewStore(cfg, vault)
	if err != nil {
		t.Fatal(err)
	}
	kube := filepath.Join(cfg, "kube")
	plan, err := a.ConnectSync(context.Background(), store, kube, true)
	if err != nil || len(plan) != 2 || plan[0].Action != "create" || len(store.List()) != 0 {
		t.Fatalf("dry run: %+v %v (%d saved)", plan, err, len(store.List()))
	}
	items, err := a.ConnectSync(context.Background(), store, kube, false)
	if err != nil {
		t.Fatal(err)
	}
	byRef := map[string]SyncItem{}
	for _, it := range items {
		byRef[it.Ref] = it
	}
	if byRef["cluster:hk-k3s"].Action != "create" || byRef["datastore:main-mysql"].Action != "create" || !strings.Contains(byRef["datastore:main-mysql"].Reason, "saved without a secret") {
		t.Fatalf("sync: %+v", items)
	}
	conns := store.List()
	var k3s workspace.Connection
	for _, c := range conns {
		if c.Kind == "k3s" {
			k3s = c
		}
	}
	if k3s.Name != "hk-k3s" || k3s.Kubeconfig != filepath.Join(kube, "hk-k3s.yaml") || !k3s.SSH.Enabled || k3s.SSH.PrivateKeyPath == "" {
		t.Fatalf("cluster connection: %+v", k3s)
	}
	if info, err := os.Stat(k3s.Kubeconfig); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("kubeconfig: %v %v", info, err)
	}
	again, _ := a.ConnectSync(context.Background(), store, kube, false)
	for _, it := range again {
		if it.Action != "unchanged" {
			t.Fatalf("second sync not idempotent: %+v", again)
		}
	}
}
