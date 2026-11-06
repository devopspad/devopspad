package workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var workloadKinds = [...]struct{ resource, kind string }{
	{"deployments", "Deployment"},
	{"statefulsets", "StatefulSet"},
	{"daemonsets", "DaemonSet"},
}

type namespaceListRequest struct {
	resource schema.GroupVersionResource
	options  metav1.ListOptions
}

type namespaceListResult struct {
	list *unstructured.UnstructuredList
	err  error
}

// Independent lists share one authenticated transport and run concurrently.
// Preserve request order so partial errors and row ordering remain deterministic.
func namespaceLists(ctx context.Context, cl dynamic.Interface, namespace string, requests []namespaceListRequest) []namespaceListResult {
	results := make([]namespaceListResult, len(requests))
	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Add(1)
		go func(i int, request namespaceListRequest) {
			defer wg.Done()
			results[i].list, results[i].err = cl.Resource(request.resource).Namespace(namespace).List(ctx, request.options)
		}(i, request)
	}
	wg.Wait()
	return results
}

func workloadListRequests() []namespaceListRequest {
	requests := make([]namespaceListRequest, 0, len(workloadKinds))
	for _, kind := range workloadKinds {
		requests = append(requests, namespaceListRequest{
			resource: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: kind.resource},
			options:  metav1.ListOptions{Limit: 500},
		})
	}
	return requests
}

// Workload is the maintenance row for a Deployment, StatefulSet, or DaemonSet.
type Workload struct {
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	UID         string `json:"uid"`
	Resource    string `json:"resource"`
	Desired     int32  `json:"desired"`
	Ready       int32  `json:"ready"`
	Image       string `json:"image"`
	ExtraImages int    `json:"extraImages"`
	UpdatedAt   string `json:"updatedAt"`
	Selector    string `json:"selector"`
}

func ListWorkloads(ctx context.Context, c Connection, namespace string) ([]Workload, error) {
	if c.Kind != "k8s" && c.Kind != "k3s" {
		return nil, errors.New("Select a Kubernetes or K3s connection")
	}
	c, closeTransport, err := prepareTransport(ctx, c)
	if err != nil {
		return nil, err
	}
	defer closeTransport()
	cl, err := kubeClient(c)
	if err != nil {
		return nil, err
	}
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = c.Database
	}
	if ns == "*" {
		ns = ""
	}
	lists := namespaceLists(ctx, cl, ns, workloadListRequests())
	out := make([]Workload, 0)
	for i, kind := range workloadKinds {
		list, err := lists[i].list, lists[i].err
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", kind.resource, err)
		}
		for _, item := range list.Items {
			out = append(out, workloadFrom(kind.kind, kind.resource, &item))
			if len(out) >= 500 {
				sortWorkloads(out)
				return out, nil
			}
		}
	}
	sortWorkloads(out)
	return out, nil
}

func workloadFrom(kind, resource string, item *unstructured.Unstructured) Workload {
	obj := item.Object
	images := containerImages(obj)
	image := ""
	extra := 0
	if len(images) > 0 {
		image = images[0]
		extra = len(images) - 1
	}
	desired, ready := replicaCounts(kind, obj)
	return Workload{
		Kind:        kind,
		Namespace:   item.GetNamespace(),
		Name:        item.GetName(),
		UID:         string(item.GetUID()),
		Resource:    resource,
		Desired:     desired,
		Ready:       ready,
		Image:       image,
		ExtraImages: extra,
		UpdatedAt:   workloadUpdated(item),
		Selector:    matchLabels(obj),
	}
}

func containerImages(obj map[string]any) []string {
	raw, _, _ := unstructured.NestedSlice(obj, "spec", "template", "spec", "containers")
	images := make([]string, 0, len(raw))
	for _, item := range raw {
		container, ok := item.(map[string]any)
		if !ok {
			continue
		}
		image, _, _ := unstructured.NestedString(container, "image")
		if image != "" {
			images = append(images, image)
		}
	}
	return images
}

func replicaCounts(kind string, obj map[string]any) (int32, int32) {
	if kind == "DaemonSet" {
		desired, _, _ := unstructured.NestedInt64(obj, "status", "desiredNumberScheduled")
		ready, _, _ := unstructured.NestedInt64(obj, "status", "numberReady")
		return int32(desired), int32(ready)
	}
	desired, found, _ := unstructured.NestedInt64(obj, "spec", "replicas")
	if !found {
		desired = 1
	}
	ready, _, _ := unstructured.NestedInt64(obj, "status", "readyReplicas")
	return int32(desired), int32(ready)
}

func matchLabels(obj map[string]any) string {
	raw, found, err := unstructured.NestedMap(obj, "spec", "selector")
	if err != nil || !found {
		return ""
	}
	var spec metav1.LabelSelector
	if runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec) != nil {
		return ""
	}
	selector, err := metav1.LabelSelectorAsSelector(&spec)
	if err != nil {
		return ""
	}
	return selector.String()
}

func workloadUpdated(item *unstructured.Unstructured) string {
	latest := item.GetCreationTimestamp().Time
	conditions, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			continue
		}
		raw, _, _ := unstructured.NestedString(condition, "lastUpdateTime")
		if raw == "" {
			raw, _, _ = unstructured.NestedString(condition, "lastTransitionTime")
		}
		parsed, err := time.Parse(time.RFC3339, raw)
		if err == nil && parsed.After(latest) {
			latest = parsed
		}
	}
	if latest.IsZero() {
		return ""
	}
	return latest.UTC().Format(time.RFC3339)
}

func sortWorkloads(items []Workload) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})
}
