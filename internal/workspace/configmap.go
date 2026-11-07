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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
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
