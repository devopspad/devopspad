package workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type OpsPod struct {
	Name       string          `json:"name"`
	Namespace  string          `json:"namespace"`
	UID        string          `json:"uid"`
	Workload   string          `json:"workload"`
	IP         string          `json:"ip"`
	Node       string          `json:"node"`
	NodeIP     string          `json:"nodeIP"`
	Phase      string          `json:"phase"`
	Ready      int             `json:"ready"`
	Total      int             `json:"total"`
	Restarts   int64           `json:"restarts"`
	CreatedAt  string          `json:"createdAt"`
	Ports      []string        `json:"ports"`
	Containers []ContainerInfo `json:"containers"`
}
type OpsCluster struct {
	Workloads []Workload `json:"workloads"`
	Pods      []OpsPod   `json:"pods"`
	FetchedAt string     `json:"fetchedAt"`
	Truncated bool       `json:"truncated"`
	Warnings  []string   `json:"warnings"`
}
type OpsMetric struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	CPU       float64 `json:"cpu"`
	Memory    float64 `json:"memory"`
	Timestamp string  `json:"timestamp"`
}

func OpenCluster(ctx context.Context, c Connection) (dynamic.Interface, func(), error) {
	_, cl, close, err := openClusterConnection(ctx, c)
	return cl, close, err
}

func openClusterConnection(ctx context.Context, c Connection) (Connection, dynamic.Interface, func(), error) {
	if c.Kind != "k8s" && c.Kind != "k3s" {
		return c, nil, func() {}, errors.New("Choose a Kubernetes or K3s connection")
	}
	c, close, err := prepareTransport(ctx, c)
	if err != nil {
		return c, nil, func() {}, err
	}
	cl, err := kubeClient(c)
	if err != nil {
		close()
		return c, nil, func() {}, err
	}
	return c, cl, close, nil
}
func opsScope(namespace, selector string) error {
	if namespace == "" || namespace == "*" {
		return errors.New("Choose a specific namespace")
	}
	if _, err := labels.Parse(selector); err != nil {
		return err
	}
	return nil
}
func ReadOpsCluster(ctx context.Context, c Connection, namespace, selector string) (OpsCluster, error) {
	return readOpsCluster(ctx, c, namespace, selector, true)
}

// ReadOpsWorkloads does not wait for Pod listings or require Pod read permission.
func ReadOpsWorkloads(ctx context.Context, c Connection, namespace, selector string) (OpsCluster, error) {
	return readOpsCluster(ctx, c, namespace, selector, false)
}

func readOpsCluster(ctx context.Context, c Connection, namespace, selector string, includePods bool) (OpsCluster, error) {
	out := OpsCluster{Workloads: []Workload{}, Pods: []OpsPod{}, Warnings: []string{}, FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := opsScope(namespace, selector); err != nil {
		return out, err
	}
	c, cl, close, err := openClusterConnection(ctx, c)
	if err != nil {
		return out, err
	}
	defer close()
	requests := workloadListRequests()
	if includePods {
		requests = append(requests, namespaceListRequest{
			resource: schema.GroupVersionResource{Version: "v1", Resource: "pods"},
			options:  metav1.ListOptions{LabelSelector: selector, Limit: 500},
		})
	}
	var lists []namespaceListResult
	if !includePods && selector != "" {
		lists, err = scopedWorkloadLists(ctx, c, namespace, selector)
		if err != nil {
			return out, err
		}
	} else {
		lists = namespaceLists(ctx, cl, namespace, requests)
	}
	selected, _ := labels.Parse(selector)
	selectors := map[string]labels.Selector{}
	for i, kind := range workloadKinds {
		list, err := lists[i].list, lists[i].err
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %v", kind.kind, err))
			continue
		}
		out.Truncated = out.Truncated || list.GetContinue() != ""
		for _, obj := range list.Items {
			template, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
			if !selected.Matches(labels.Set(obj.GetLabels())) && !selected.Matches(labels.Set(template)) {
				continue
			}
			w := workloadFrom(kind.kind, kind.resource, &obj)
			out.Workloads = append(out.Workloads, w)
			raw, _, _ := unstructured.NestedMap(obj.Object, "spec", "selector")
			var spec metav1.LabelSelector
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err == nil {
				if sel, e := metav1.LabelSelectorAsSelector(&spec); e == nil {
					selectors[w.Name] = sel
				}
			}
		}
	}
	sortWorkloads(out.Workloads)
	if !includePods {
		return out, nil
	}
	pods, err := lists[len(workloadKinds)].list, lists[len(workloadKinds)].err
	if err != nil {
		return out, err
	}
	out.Truncated = out.Truncated || pods.GetContinue() != ""
	for _, obj := range pods.Items {
		containers := podContainers(&obj)
		p := OpsPod{Name: obj.GetName(), Namespace: obj.GetNamespace(), UID: string(obj.GetUID()), IP: objectString(obj.Object, "status", "podIP"), Node: objectString(obj.Object, "spec", "nodeName"), NodeIP: objectString(obj.Object, "status", "hostIP"), Phase: objectString(obj.Object, "status", "phase"), CreatedAt: obj.GetCreationTimestamp().UTC().Format(time.RFC3339), Containers: containers, Ports: []string{}}
		if obj.GetDeletionTimestamp() != nil {
			p.Phase = "Terminating"
		}
		for _, container := range containers {
			p.Restarts += container.Restarts
			if container.Kind == "Application" {
				p.Total++
				if container.Ready {
					p.Ready++
				}
			}
		}
		for _, workload := range out.Workloads {
			if sel, ok := selectors[workload.Name]; ok && sel.Matches(labels.Set(obj.GetLabels())) {
				p.Workload = workload.Name
				break
			}
		}
		raw, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ports, _, _ := unstructured.NestedSlice(m, "ports")
			for _, port := range ports {
				if m, ok := port.(map[string]any); ok {
					v, _, _ := unstructured.NestedInt64(m, "containerPort")
					p.Ports = append(p.Ports, fmt.Sprint(v))
				}
			}
		}
		out.Pods = append(out.Pods, p)
	}
	sort.Slice(out.Pods, func(i, j int) bool { return out.Pods[i].Name < out.Pods[j].Name })
	return out, nil
}
func ReadOpsMetrics(ctx context.Context, c Connection, namespace, selector string) ([]OpsMetric, error) {
	if err := opsScope(namespace, selector); err != nil {
		return nil, err
	}
	cl, close, err := OpenCluster(ctx, c)
	if err != nil {
		return nil, err
	}
	defer close()
	pods, err := cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector, Limit: 500})
	if err != nil {
		return nil, err
	}
	if pods.GetContinue() != "" {
		return nil, errors.New("More than 500 Pods; narrow the application selector")
	}
	allowed := map[string]bool{}
	for _, p := range pods.Items {
		allowed[p.GetName()] = true
	}
	list, err := cl.Resource(schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Cannot read CPU / memory metrics; metrics-server and read access to metrics.k8s.io are required: %w", err)
	}
	out := []OpsMetric{}
	for _, obj := range list.Items {
		if !allowed[obj.GetName()] {
			continue
		}
		m := OpsMetric{Pod: obj.GetName(), Namespace: namespace, Timestamp: objectString(obj.Object, "timestamp")}
		cs, _, _ := unstructured.NestedSlice(obj.Object, "containers")
		for _, raw := range cs {
			c, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			cpu, e1 := resource.ParseQuantity(objectString(c, "usage", "cpu"))
			mem, e2 := resource.ParseQuantity(objectString(c, "usage", "memory"))
			if e1 != nil || e2 != nil {
				return nil, errors.New("The metrics contain invalid resource usage")
			}
			m.CPU += float64(cpu.MilliValue())
			m.Memory += float64(mem.Value()) / (1024 * 1024)
		}
		out = append(out, m)
	}
	return out, nil
}

// PinClusterConnection freezes the API endpoint/context at plan time, so a
// kubeconfig file edit cannot silently redirect a later apply to another cluster.
func PinClusterConnection(c Connection) (Connection, error) {
	cfg, err := kubeConfig(c)
	if err != nil {
		return c, err
	}
	c.clusterConfig = cfg
	return c, nil
}
