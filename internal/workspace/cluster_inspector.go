package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	typedcore "k8s.io/client-go/kubernetes/typed/core/v1"
)

const LogByteLimit = 256 << 10

type ClusterRef struct {
	Context   string `json:"context"`
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}
type ContainerInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Image    string `json:"image"`
	State    string `json:"state"`
	Ready    bool   `json:"ready"`
	Restarts int64  `json:"restarts"`
	// ImageID is the resolved image the kubelet runs, e.g. registry/app@sha256:…
	ImageID string `json:"imageId,omitempty"`
}
type ClusterDetails struct {
	Ref        ClusterRef        `json:"ref"`
	Properties []Property        `json:"properties"`
	Labels     map[string]string `json:"labels"`
	Containers []ContainerInfo   `json:"containers"`
	Definition string            `json:"definition"`
}
type PodLogRequest struct {
	Ref       ClusterRef `json:"ref"`
	Container string     `json:"container"`
	TailLines int64      `json:"tailLines"`
	Previous  bool       `json:"previous"`
}
type PodLogs struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

func clusterTarget(c Connection, ref ClusterRef) (Connection, schema.GroupVersionResource, error) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: ref.Resource}
	if c.Kind != "k8s" && c.Kind != "k3s" {
		return c, gvr, errors.New("Select a Kubernetes or K3s connection")
	}
	switch ref.Resource {
	case "deployments", "statefulsets", "daemonsets":
		gvr.Group = "apps"
	case "pods", "services", "configmaps", "events":
	case "nodes", "namespaces":
		if ref.Namespace != "" {
			return c, gvr, errors.New("Cluster-scoped resources must not have a namespace")
		}
	default:
		return c, gvr, errors.New("This resource type is not supported")
	}
	valid := func(s string) bool {
		return s != "" && s != "." && s != ".." && len(s) <= 253 && !strings.ContainsAny(s, "/\\%?# \t\r\n\x00*")
	}
	if !valid(ref.Name) {
		return c, gvr, errors.New("Invalid resource name")
	}
	if ref.Resource != "nodes" && ref.Resource != "namespaces" && !valid(ref.Namespace) {
		return c, gvr, errors.New("Select a resource in a specific namespace")
	}
	if ref.Context != "" {
		c.Context = ref.Context
	}
	return c, gvr, nil
}
func clusterReadError(err error, operation string) error {
	if apierrors.IsForbidden(err) {
		return fmt.Errorf("Access denied: %s. Ask your cluster administrator for read permission on this resource. Server response: %w", operation, err)
	}
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("This resource is no longer available. Refresh the resource list. Server response: %w", err)
	}
	return err
}
func clusterObject(ctx context.Context, c Connection, ref ClusterRef) (dynamic.Interface, *unstructured.Unstructured, error) {
	c, gvr, e := clusterTarget(c, ref)
	if e != nil {
		return nil, nil, e
	}
	cl, e := kubeClient(c)
	if e != nil {
		return nil, nil, e
	}
	obj, e := cl.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if e != nil {
		return nil, nil, clusterReadError(e, "get "+ref.Resource)
	}
	if ref.UID != "" && string(obj.GetUID()) != ref.UID {
		return nil, nil, errors.New("This resource was replaced. Close the inspector and refresh the resource list")
	}
	return cl, obj, nil
}
func objectString(obj map[string]any, path ...string) string {
	v, _, _ := unstructured.NestedString(obj, path...)
	return v
}
func podContainers(obj *unstructured.Unstructured) []ContainerInfo {
	out := []ContainerInfo{}
	for _, group := range []struct{ spec, status, kind string }{{"containers", "containerStatuses", "Application"}, {"initContainers", "initContainerStatuses", "Init"}, {"ephemeralContainers", "ephemeralContainerStatuses", "Ephemeral"}} {
		containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", group.spec)
		statuses, _, _ := unstructured.NestedSlice(obj.Object, "status", group.status)
		for _, raw := range containers {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			item := ContainerInfo{Name: objectString(m, "name"), Image: objectString(m, "image"), Kind: group.kind, State: "Unknown"}
			for _, s := range statuses {
				status, ok := s.(map[string]any)
				if !ok || objectString(status, "name") != item.Name {
					continue
				}
				item.Ready, _, _ = unstructured.NestedBool(status, "ready")
				item.Restarts, _, _ = unstructured.NestedInt64(status, "restartCount")
				item.ImageID = objectString(status, "imageID")
				for _, state := range []string{"running", "waiting", "terminated"} {
					if _, exists, _ := unstructured.NestedMap(status, "state", state); exists {
						item.State = state
						if reason := objectString(status, "state", state, "reason"); reason != "" {
							item.State += " · " + reason
						}
					}
				}
			}
			out = append(out, item)
		}
	}
	return out
}
func InspectClusterResource(ctx context.Context, c Connection, ref ClusterRef) (ClusterDetails, error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return ClusterDetails{}, transportErr
	}
	defer closeTransport()

	d := ClusterDetails{Ref: ref, Properties: []Property{}, Labels: map[string]string{}, Containers: []ContainerInfo{}}
	_, obj, e := clusterObject(ctx, c, ref)
	if e != nil {
		return d, e
	}
	d.Ref.UID = string(obj.GetUID())
	d.Labels = obj.GetLabels()
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	d.Properties = []Property{{Name: "Name", Value: obj.GetName()}, {Name: "Namespace", Value: obj.GetNamespace()}, {Name: "UID", Value: string(obj.GetUID())}, {Name: "Created", Value: obj.GetCreationTimestamp().UTC().Format("2006-01-02 15:04:05 UTC")}}
	for _, item := range []struct {
		name string
		path []string
	}{{"Phase", []string{"status", "phase"}}, {"Node", []string{"spec", "nodeName"}}, {"Pod IP", []string{"status", "podIP"}}, {"Service type", []string{"spec", "type"}}} {
		if v := objectString(obj.Object, item.path...); v != "" {
			d.Properties = append(d.Properties, Property{Name: item.name, Value: v})
		}
	}
	if ref.Resource == "pods" {
		d.Containers = podContainers(obj)
	}
	// Managed fields can dominate the document without helping inspection.
	obj = obj.DeepCopy()
	obj.SetManagedFields(nil)
	raw, e := json.MarshalIndent(obj.Object, "", "  ")
	if e != nil {
		return d, e
	}
	if len(raw) > 2<<20 {
		return d, errors.New("Resource definition exceeds 2 MiB")
	}
	d.Definition = string(raw)
	return d, nil
}
func ClusterEvents(ctx context.Context, c Connection, ref ClusterRef) (Result, error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return Result{}, transportErr
	}
	defer closeTransport()

	result := emptyResult()
	cl, obj, e := clusterObject(ctx, c, ref)
	if e != nil {
		return result, e
	}
	if obj.GetUID() == "" {
		return result, errors.New("Resource has no UID; cannot safely match events")
	}
	r, e := cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "events"}).Namespace(ref.Namespace).List(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("involvedObject.uid", string(obj.GetUID())).String(), Limit: 200})
	if e != nil {
		return result, clusterReadError(e, "list events")
	}
	result.Columns = []string{"last_seen", "type", "reason", "message", "count"}
	size := 0
	for _, event := range r.Items {
		if len(result.Rows) >= 200 {
			result.Truncated = true
			break
		}
		last := objectString(event.Object, "lastTimestamp")
		if last == "" {
			last = objectString(event.Object, "eventTime")
		}
		if last == "" {
			last = objectString(event.Object, "metadata", "creationTimestamp")
		}
		count, _, _ := unstructured.NestedInt64(event.Object, "count")
		row := []any{last, objectString(event.Object, "type"), objectString(event.Object, "reason"), objectString(event.Object, "message"), count}
		raw, _ := json.Marshal(row)
		size += len(raw)
		if size > 1<<20 {
			if len(result.Rows) == 0 {
				return result, errors.New("An event exceeds the 1 MiB preview budget")
			}
			result.Truncated = true
			break
		}
		result.Rows = append(result.Rows, row)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool { return fmt.Sprint(result.Rows[i][0]) > fmt.Sprint(result.Rows[j][0]) })
	result.Truncated = result.Truncated || r.GetContinue() != ""
	result.Message = "Up to 200 events / 1 MiB for this resource UID; events may expire according to cluster retention."
	return result, nil
}
func ReadPodLogs(ctx context.Context, c Connection, request PodLogRequest) (PodLogs, error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return PodLogs{}, transportErr
	}
	defer closeTransport()

	out := PodLogs{}
	if request.Ref.Resource != "pods" {
		return out, errors.New("Logs are available for Pods only")
	}
	if request.TailLines == 0 {
		request.TailLines = 200
	}
	if request.TailLines < 1 || request.TailLines > 2000 {
		return out, errors.New("Log tail must be between 1 and 2000 lines")
	}
	_, obj, e := clusterObject(ctx, c, request.Ref)
	if e != nil {
		return out, e
	}
	found := false
	for _, container := range podContainers(obj) {
		if container.Name == request.Container {
			found = true
			break
		}
	}
	if !found {
		return out, errors.New("Select an application, init, or ephemeral container in this Pod")
	}
	if request.Ref.Context != "" {
		c.Context = request.Ref.Context
	}
	cfg, e := kubeConfig(c)
	if e != nil {
		return out, e
	}
	cl, e := typedcore.NewForConfig(cfg)
	if e != nil {
		return out, e
	}
	limit := int64(LogByteLimit + 1)
	stream, e := cl.Pods(request.Ref.Namespace).GetLogs(request.Ref.Name, &corev1.PodLogOptions{Container: request.Container, TailLines: &request.TailLines, Previous: request.Previous, Timestamps: true, LimitBytes: &limit, Follow: false}).Stream(ctx)
	if e != nil {
		return out, clusterReadError(e, "get pods/log")
	}
	defer stream.Close()
	data, e := io.ReadAll(io.LimitReader(stream, LogByteLimit+1))
	if e != nil {
		return out, e
	}
	out.Truncated = len(data) > LogByteLimit
	if out.Truncated {
		data = data[:LogByteLimit]
	}
	out.Text = strings.ToValidUTF8(string(data), "�")
	if len(out.Text) > LogByteLimit {
		out.Truncated = true
		out.Text = out.Text[:LogByteLimit]
		for !utf8.ValidString(out.Text) {
			out.Text = out.Text[:len(out.Text)-1]
		}
	}
	return out, nil
}
