package archive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"devopspad/internal/workspace"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type ClusterFacts struct {
	Nodes     []ObservedNode     `json:"nodes"`
	Workloads []ObservedWorkload `json:"workloads"`
	Truncated bool               `json:"truncated,omitempty"`
	Errors    []string           `json:"errors"`
}

type ObservedNode struct {
	Name       string `json:"name"`
	InternalIP string `json:"internalIP,omitempty"`
	Version    string `json:"version,omitempty"`
	Ready      bool   `json:"ready"`
}

type ObservedWorkload struct {
	Kind           string            `json:"kind"`
	Namespace      string            `json:"namespace"`
	Name           string            `json:"name"`
	Labels         map[string]string `json:"labels,omitempty"`
	TemplateLabels map[string]string `json:"templateLabels,omitempty"`
	Images         []string          `json:"images"`
	Desired        int64             `json:"desired"`
	Ready          int64             `json:"ready"`
}

type HostFacts struct {
	OS         string         `json:"os,omitempty"`
	Kernel     string         `json:"kernel,omitempty"`
	CPU        int            `json:"cpu,omitempty"`
	MemoryGiB  float64        `json:"memoryGiB,omitempty"`
	DiskGiB    float64        `json:"diskGiB,omitempty"`
	Services   []string       `json:"services"`
	NginxConfs []ObservedFile `json:"nginxConfs"`
	K3s        string         `json:"k3s,omitempty"`
	Errors     []string       `json:"errors"`
}

type ObservedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

const maxClusterItems = 5000

// systemNamespaces are never reported as unregistered workloads.
var systemNamespaces = map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

// scanCluster lists nodes and workloads of every namespace. Credentials for a
// remote kubeconfig are fetched into a temporary directory removed afterwards.
func (a *Archive) scanCluster(ctx context.Context, r *Resource) *ClusterFacts {
	f := &ClusterFacts{Nodes: []ObservedNode{}, Workloads: []ObservedWorkload{}, Errors: []string{}}
	tmp, err := os.MkdirTemp("", "devopspad-kube-")
	if err != nil {
		f.Errors = append(f.Errors, err.Error())
		return f
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	c, err := a.ClusterConnection(ctx, r.Ref, tmp, true)
	if err != nil {
		f.Errors = append(f.Errors, err.Error())
		return f
	}
	cl, closeFn, err := clusterClient(ctx, c)
	if err != nil {
		f.Errors = append(f.Errors, err.Error())
		return f
	}
	defer closeFn()
	nodes, truncated, err := listAll(ctx, cl, schema.GroupVersionResource{Version: "v1", Resource: "nodes"})
	f.Truncated = f.Truncated || truncated
	if err != nil {
		f.Errors = append(f.Errors, "list nodes: "+err.Error())
	}
	for _, n := range nodes {
		node := ObservedNode{Name: n.GetName()}
		node.Version, _, _ = unstructured.NestedString(n.Object, "status", "nodeInfo", "kubeletVersion")
		addrs, _, _ := unstructured.NestedSlice(n.Object, "status", "addresses")
		for _, ad := range addrs {
			if m, ok := ad.(map[string]any); ok && m["type"] == "InternalIP" {
				node.InternalIP, _ = m["address"].(string)
			}
		}
		conds, _, _ := unstructured.NestedSlice(n.Object, "status", "conditions")
		for _, cd := range conds {
			if m, ok := cd.(map[string]any); ok && m["type"] == "Ready" {
				node.Ready = m["status"] == "True"
			}
		}
		f.Nodes = append(f.Nodes, node)
	}
	for _, res := range []struct{ resource, kind string }{{"deployments", "Deployment"}, {"statefulsets", "StatefulSet"}, {"daemonsets", "DaemonSet"}} {
		items, truncated, err := listAll(ctx, cl, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: res.resource})
		f.Truncated = f.Truncated || truncated
		if err != nil {
			f.Errors = append(f.Errors, "list "+res.resource+": "+err.Error())
			continue
		}
		for _, it := range items {
			w := ObservedWorkload{Kind: res.kind, Namespace: it.GetNamespace(), Name: it.GetName(), Labels: it.GetLabels(), Images: []string{}}
			w.TemplateLabels, _, _ = unstructured.NestedStringMap(it.Object, "spec", "template", "metadata", "labels")
			containers, _, _ := unstructured.NestedSlice(it.Object, "spec", "template", "spec", "containers")
			for _, ct := range containers {
				if m, ok := ct.(map[string]any); ok {
					if img, ok := m["image"].(string); ok {
						w.Images = append(w.Images, img)
					}
				}
			}
			if res.kind == "DaemonSet" {
				w.Desired, _, _ = unstructured.NestedInt64(it.Object, "status", "desiredNumberScheduled")
				w.Ready, _, _ = unstructured.NestedInt64(it.Object, "status", "numberReady")
			} else {
				w.Desired, _, _ = unstructured.NestedInt64(it.Object, "spec", "replicas")
				w.Ready, _, _ = unstructured.NestedInt64(it.Object, "status", "readyReplicas")
			}
			f.Workloads = append(f.Workloads, w)
		}
	}
	sort.Slice(f.Workloads, func(i, j int) bool {
		x, y := f.Workloads[i], f.Workloads[j]
		return x.Namespace+"/"+x.Kind+"/"+x.Name < y.Namespace+"/"+y.Kind+"/"+y.Name
	})
	return f
}

// clusterClient is replaced in tests.
var clusterClient = func(ctx context.Context, c workspace.Connection) (dynamic.Interface, func(), error) {
	return workspace.OpenCluster(ctx, c)
}

func listAll(ctx context.Context, cl dynamic.Interface, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, bool, error) {
	var out []unstructured.Unstructured
	opts := metav1.ListOptions{Limit: 500}
	for {
		list, err := cl.Resource(gvr).List(ctx, opts)
		if err != nil {
			return out, false, err
		}
		out = append(out, list.Items...)
		if list.GetContinue() == "" {
			return out, false, nil
		}
		if len(out) >= maxClusterItems {
			return out, true, nil
		}
		opts.Continue = list.GetContinue()
	}
}

// hostScript only reads. It is a constant: nothing from the archive is
// interpolated into the remote command.
const hostScript = `sh -c 'echo @@kernel; uname -sr
echo @@os; . /etc/os-release 2>/dev/null; echo "$PRETTY_NAME"
echo @@cpu; nproc 2>/dev/null
echo @@mem; grep MemTotal /proc/meminfo 2>/dev/null
echo @@disk; df -Pk / 2>/dev/null | tail -n 1
echo @@services; systemctl list-units --type=service --state=running --no-legend --plain 2>/dev/null
echo @@nginx; sha256sum /etc/nginx/conf.d/*.conf 2>/dev/null
echo @@k3s; k3s --version 2>/dev/null | head -n 1
true'`

// hostOutput is replaced in tests.
var hostOutput = func(ctx context.Context, c workspace.Connection) ([]byte, error) {
	return workspace.SSHOutput(ctx, c, hostScript, 1<<20)
}

func (a *Archive) scanHost(ctx context.Context, r *Resource) *HostFacts {
	f := &HostFacts{Services: []string{}, NginxConfs: []ObservedFile{}, Errors: []string{}}
	c, err := a.HostConnection(r.Ref)
	if err != nil {
		f.Errors = append(f.Errors, err.Error())
		return f
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	out, err := hostOutput(ctx, c)
	if err != nil {
		f.Errors = append(f.Errors, err.Error())
		return f
	}
	parseHostFacts(string(out), f)
	return f
}

func parseHostFacts(out string, f *HostFacts) {
	section := ""
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "@@") {
			section = line[2:]
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		switch section {
		case "kernel":
			f.Kernel = line
		case "os":
			f.OS = line
		case "cpu":
			f.CPU, _ = strconv.Atoi(line)
		case "mem":
			if len(fields) >= 2 {
				kb, _ := strconv.ParseFloat(fields[1], 64)
				f.MemoryGiB = round1(kb / (1 << 20))
			}
		case "disk":
			if len(fields) >= 2 {
				kb, _ := strconv.ParseFloat(fields[1], 64)
				f.DiskGiB = round1(kb / (1 << 20))
			}
		case "services":
			f.Services = append(f.Services, strings.TrimSuffix(fields[0], ".service"))
		case "nginx":
			if len(fields) == 2 {
				f.NginxConfs = append(f.NginxConfs, ObservedFile{Path: fields[1], SHA256: fields[0]})
			}
		case "k3s":
			f.K3s = line
		}
	}
	sort.Strings(f.Services)
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// matchesWorkload reports whether an archive deployment describes a workload.
func matchesWorkload(d ResolvedDeployment, w ObservedWorkload) bool {
	if d.Namespace != w.Namespace {
		return false
	}
	if d.Workload != nil {
		return d.Workload.Name == w.Name && d.Workload.Kind == w.Kind
	}
	sel, err := labels.Parse(d.Selector)
	if err != nil || sel.Empty() {
		return false
	}
	return sel.Matches(labels.Set(w.Labels)) || sel.Matches(labels.Set(w.TemplateLabels))
}

// clusterDeployments lists every k8s deployment the archive places on a cluster.
func (a *Archive) clusterDeployments(cluster string) map[string][]ResolvedDeployment {
	out := map[string][]ResolvedDeployment{}
	for _, r := range a.Resources {
		s, ok := r.Object.(*Service)
		if !ok {
			continue
		}
		for _, d := range a.resolveDeployments(s) {
			if d.Runtime == "k8s" && d.Cluster == cluster {
				out[r.Ref] = append(out[r.Ref], d)
			}
		}
	}
	return out
}

func fmtWorkload(w ObservedWorkload) string {
	return fmt.Sprintf("%s/%s/%s", w.Namespace, strings.ToLower(w.Kind), w.Name)
}
