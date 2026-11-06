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
		return "", errors.New("缺少 ConfigMap 身份、版本或数据，请刷新配置后重试")
	}
	if _, _, err := clusterTarget(c, request.Ref); err != nil {
		return "", err
	}
	size := 0
	for key, value := range request.Data {
		if len(validation.IsConfigMapKey(key)) != 0 || !utf8.ValidString(value) {
			return "", errors.New("配置包含无效的文件名或非 UTF-8 内容")
		}
		size += len(key) + len(value)
	}
	if size > 1<<20 {
		return "", errors.New("ConfigMap 文本数据超过 1 MiB")
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
		return "", errors.New("集群配置已变化，请保留需要的修改，取消编辑并刷新后重试")
	}
	if immutable, _, _ := unstructured.NestedBool(obj.Object, "immutable"); immutable {
		return "", errors.New("此 ConfigMap 设置为不可修改（immutable）")
	}
	if err = unstructured.SetNestedStringMap(obj.Object, request.Data, "data"); err != nil {
		return "", err
	}
	updated, err := cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(request.Ref.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if apierrors.IsConflict(err) {
		return "", fmt.Errorf("集群配置已变化，请保留需要的修改，取消编辑并刷新后重试: %w", err)
	}
	if apierrors.IsForbidden(err) {
		return "", fmt.Errorf("没有更新此 ConfigMap 的权限: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("保存未确认，请保留修改并刷新核对集群实际状态: %w", err)
	}
	updated.SetManagedFields(nil)
	raw, err := json.Marshal(updated.Object)
	return string(raw), err
}
