package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
)

type ConfigMapUpdate struct {
	Ref             ClusterRef        `json:"ref"`
	ResourceVersion string            `json:"resourceVersion"`
	Data            map[string]string `json:"data"`
}

// SaveConfigMap updates only text data, preserving metadata and binary entries.
// Both the read and the atomic Kubernetes update enforce the editor's version.
func SaveConfigMap(ctx context.Context, c Connection, request ConfigMapUpdate) (string, error) {
	if request.Ref.Resource != "configmaps" || request.Ref.UID == "" || request.ResourceVersion == "" || request.Data == nil {
		return "", errors.New("Missing ConfigMap identity, version or data; refresh the config and try again")
	}
	if _, _, err := clusterTarget(c, request.Ref); err != nil {
		return "", err
	}
	size := 0
	for key, value := range request.Data {
		if len(validation.IsConfigMapKey(key)) != 0 || !utf8.ValidString(value) {
			return "", errors.New("The config contains an invalid file name or non-UTF-8 content")
		}
		size += len(key) + len(value)
	}
	if size > 1<<20 {
		return "", errors.New("ConfigMap text data exceeds 1 MiB")
	}
	c, closeTransport, err := prepareTransport(ctx, c)
	if err != nil {
		return "", err
	}
	defer closeTransport()
	cl, obj, err := clusterObject(ctx, c, request.Ref)
	if err != nil {
		return "", err
	}
	if obj.GetResourceVersion() != request.ResourceVersion {
		return "", errors.New("The cluster config changed; keep the edits you need, cancel editing, refresh and try again")
	}
	if immutable, _, _ := unstructured.NestedBool(obj.Object, "immutable"); immutable {
		return "", errors.New("This ConfigMap is immutable")
	}
	if err = unstructured.SetNestedStringMap(obj.Object, request.Data, "data"); err != nil {
		return "", err
	}
	updated, err := cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(request.Ref.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if apierrors.IsConflict(err) {
		return "", fmt.Errorf("The cluster config changed; keep the edits you need, cancel editing, refresh and try again: %w", err)
	}
	if apierrors.IsForbidden(err) {
		return "", fmt.Errorf("No permission to update this ConfigMap: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("The save was not confirmed; keep your edits and refresh to check the cluster state: %w", err)
	}
	updated.SetManagedFields(nil)
	raw, err := json.Marshal(updated.Object)
	return string(raw), err
}

// ApplicationConfigMap is a ConfigMap referenced by an application's workloads.
type ApplicationConfigMap struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	UID             string            `json:"uid"`
	ResourceVersion string            `json:"resourceVersion"`
	Immutable       bool              `json:"immutable"`
	Data            map[string]string `json:"-"`
}

// ReadApplicationConfigMap reads a ConfigMap only when a workload of the
// application references it, matching what the desktop config page offers.
func ReadApplicationConfigMap(ctx context.Context, c Connection, namespace, selector, name string) (ApplicationConfigMap, error) {
	if err := opsScope(namespace, selector); err != nil {
		return ApplicationConfigMap{}, err
	}
	_, cl, close, err := openClusterConnection(ctx, c)
	if err != nil {
		return ApplicationConfigMap{}, err
	}
	defer close()
	return readApplicationConfigMap(ctx, cl, namespace, selector, name)
}

func readApplicationConfigMap(ctx context.Context, cl dynamic.Interface, namespace, selector, name string) (ApplicationConfigMap, error) {
	out := ApplicationConfigMap{Name: name, Namespace: namespace}
	selected, err := labels.Parse(selector)
	if err != nil || selector == "" {
		return out, errors.New("Invalid application selector")
	}
	referenced := false
	for i, result := range namespaceLists(ctx, cl, namespace, workloadListRequests()) {
		if result.err != nil {
			return out, clusterReadError(result.err, "list "+workloadKinds[i].resource)
		}
		if result.list.GetContinue() != "" {
			return out, fmt.Errorf("More than 500 %s in namespace %s; cannot confirm the ConfigMap belongs to the application", workloadKinds[i].resource, namespace)
		}
		for _, obj := range result.list.Items {
			template, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
			if !selected.Matches(labels.Set(obj.GetLabels())) && !selected.Matches(labels.Set(template)) {
				continue
			}
			for _, ref := range configMapRefs(&obj) {
				referenced = referenced || ref == name
			}
		}
	}
	if !referenced {
		return out, fmt.Errorf("ConfigMap %s is not used by the application's workloads in namespace %s", name, namespace)
	}
	obj, err := cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return out, clusterReadError(err, "get configmaps")
	}
	out.UID = string(obj.GetUID())
	out.ResourceVersion = obj.GetResourceVersion()
	out.Immutable, _, _ = unstructured.NestedBool(obj.Object, "immutable")
	out.Data, _, _ = unstructured.NestedStringMap(obj.Object, "data")
	if out.Data == nil {
		out.Data = map[string]string{}
	}
	return out, nil
}

// configMapRefs lists ConfigMaps a Pod template uses through env, envFrom and
// mounted configMap or projected volumes, in init and app containers.
func configMapRefs(obj *unstructured.Unstructured) []string {
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
	var refs []string
	add := func(m map[string]any, fields ...string) {
		if name, _, _ := unstructured.NestedString(m, fields...); name != "" {
			refs = append(refs, name)
		}
	}
	volumes := map[string]map[string]any{}
	raw, _, _ := unstructured.NestedSlice(spec, "volumes")
	for _, item := range raw {
		if v, ok := item.(map[string]any); ok {
			name, _, _ := unstructured.NestedString(v, "name")
			volumes[name] = v
		}
	}
	for _, field := range []string{"initContainers", "containers"} {
		containers, _, _ := unstructured.NestedSlice(spec, field)
		for _, item := range containers {
			container, ok := item.(map[string]any)
			if !ok {
				continue
			}
			env, _, _ := unstructured.NestedSlice(container, "env")
			for _, e := range env {
				if m, ok := e.(map[string]any); ok {
					add(m, "valueFrom", "configMapKeyRef", "name")
				}
			}
			envFrom, _, _ := unstructured.NestedSlice(container, "envFrom")
			for _, e := range envFrom {
				if m, ok := e.(map[string]any); ok {
					add(m, "configMapRef", "name")
				}
			}
			mounts, _, _ := unstructured.NestedSlice(container, "volumeMounts")
			for _, mount := range mounts {
				m, ok := mount.(map[string]any)
				if !ok {
					continue
				}
				name, _, _ := unstructured.NestedString(m, "name")
				volume, ok := volumes[name]
				if !ok {
					continue
				}
				add(volume, "configMap", "name")
				sources, _, _ := unstructured.NestedSlice(volume, "projected", "sources")
				for _, source := range sources {
					if s, ok := source.(map[string]any); ok {
						add(s, "configMap", "name")
					}
				}
			}
		}
	}
	return refs
}
