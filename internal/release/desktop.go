package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"devopspad/internal/ops"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var deploymentGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
var leaseGVR = schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}

type DesktopPlan struct {
	ID             string `json:"id"`
	Action         string `json:"action"`
	BeforeReplicas int64  `json:"beforeReplicas"`
	Replicas       int64  `json:"replicas"`
	Namespace      string `json:"namespace"`
	Workload       string `json:"workload"`
	Container      string `json:"container"`
	BeforeImage    string `json:"beforeImage"`
	Image          string `json:"image"`
	ExpiresAt      string `json:"expiresAt"`
	candidate      *unstructured.Unstructured
}

func PrepareDesktop(ctx context.Context, cl dynamic.Interface, namespace, workload, container, image, selector string) (DesktopPlan, error) {
	return prepareDesktop(ctx, cl, namespace, workload, container, image, selector, "release", 0)
}

// PrepareDesktopAction uses the same optimistic concurrency and lease as image releases.
func PrepareDesktopAction(ctx context.Context, cl dynamic.Interface, namespace, workload, selector, action string, replicas int64) (DesktopPlan, error) {
	if action != "scale" && action != "restart" {
		return DesktopPlan{}, errors.New("不支持的 Deployment 操作")
	}
	return prepareDesktop(ctx, cl, namespace, workload, "", "", selector, action, replicas)
}

func prepareDesktop(ctx context.Context, cl dynamic.Interface, namespace, workload, container, image, selector, action string, replicas int64) (DesktopPlan, error) {
	p := DesktopPlan{ID: uuid.NewString(), Action: action, Namespace: namespace, Workload: workload, Container: container, Image: image, ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)}
	if !ops.ValidName(namespace) || !ops.ValidName(workload) {
		return p, errors.New("无效 Deployment 目标")
	}
	if action == "release" && (!ops.ValidName(container) || !ops.ValidImage(image)) {
		return p, errors.New("无效发布目标或镜像摘要")
	}
	if action == "scale" && (replicas < 0 || replicas > 1000) {
		return p, errors.New("副本数必须在 0 到 1000 之间")
	}
	obj, err := cl.Resource(deploymentGVR).Namespace(namespace).Get(ctx, workload, metav1.GetOptions{})
	if err != nil {
		return p, err
	}
	sel, err := labels.Parse(selector)
	if err != nil || selector == "" {
		return p, errors.New("应用选择器无效")
	}
	template, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
	if !sel.Matches(labels.Set(template)) {
		return p, errors.New("Deployment 不属于当前应用")
	}
	paused, _, _ := unstructured.NestedBool(obj.Object, "spec", "paused")
	if paused {
		return p, errors.New("Deployment 已暂停，请先恢复后发布")
	}
	if obj.GetResourceVersion() == "" || obj.GetUID() == "" {
		return p, errors.New("Deployment 缺少版本或 UID")
	}
	p.BeforeReplicas, _, _ = unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if _, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); !found {
		p.BeforeReplicas = 1
	}
	p.Replicas = p.BeforeReplicas
	if action == "scale" {
		if replicas == p.BeforeReplicas {
			return p, errors.New("副本数没有变化")
		}
		p.Replicas = replicas
		if err = unstructured.SetNestedField(obj.Object, replicas, "spec", "replicas"); err != nil {
			return p, err
		}
	} else if action == "release" {
		containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
		found := false
		for _, raw := range containers {
			c, ok := raw.(map[string]any)
			if ok && c["name"] == container {
				found = true
				p.BeforeImage, _ = c["image"].(string)
				c["image"] = image
			}
		}
		if !found {
			return p, errors.New("目标容器不存在")
		}
		if p.BeforeImage == image {
			return p, errors.New("目标容器已使用此制品，无需发布")
		}
		if err = unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers"); err != nil {
			return p, err
		}
	}
	// Scaling must not change the Pod template: that would cause an unintended rollout.
	if action != "scale" {
		// A release id in the Pod template makes each rollout traceable.
		annotations, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations")
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["devopspad.io/release"] = p.ID
		if err = unstructured.SetNestedStringMap(obj.Object, annotations, "spec", "template", "metadata", "annotations"); err != nil {
			return p, err
		}
	}
	obj.SetManagedFields(nil)
	delete(obj.Object, "status")
	if _, err = cl.Resource(deploymentGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
		return p, fmt.Errorf("发布预检查失败: %w", err)
	}
	p.candidate = obj
	return p, nil
}

// ApplyDesktop is deliberately bounded below the five-minute lease lifetime.
// A contender must acquire the same lease and still pass the resourceVersion
// precondition. A lost response is reported as uncertain, never auto-retried.
func ApplyDesktop(ctx context.Context, cl dynamic.Interface, p DesktopPlan, progress func(string) error) error {
	expires, err := time.Parse(time.RFC3339, p.ExpiresAt)
	if err != nil || time.Now().After(expires) || p.candidate == nil {
		return errors.New("发布计划已过期，请重新生成")
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	unlock, err := acquireDesktopLease(ctx, cl, p)
	if err != nil {
		return err
	}
	defer unlock()
	if err = progress("已取得目标发布锁"); err != nil {
		return err
	}
	current, err := cl.Resource(deploymentGVR).Namespace(p.Namespace).Get(ctx, p.Workload, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if current.GetUID() != p.candidate.GetUID() || current.GetResourceVersion() != p.candidate.GetResourceVersion() {
		return errors.New("Deployment 已变化，请重新生成发布计划")
	}
	if err = progress("预检查通过，准备更新 Deployment"); err != nil {
		return err
	}
	updated, err := cl.Resource(deploymentGVR).Namespace(p.Namespace).Update(ctx, p.candidate.DeepCopy(), metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("更新请求未确认，请刷新集群核对实际状态: %w", err)
	}
	if err = progress("Deployment 已更新，等待就绪"); err != nil {
		return fmt.Errorf("配置已更新，但操作记录写入失败: %w", err)
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		obj, err := cl.Resource(deploymentGVR).Namespace(p.Namespace).Get(ctx, p.Workload, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("配置已提交，状态核对失败: %w", err)
		}
		if obj.GetUID() != updated.GetUID() || obj.GetGeneration() != updated.GetGeneration() {
			return errors.New("发布期间 Deployment 被其他操作修改，请核对实际状态")
		}
		if desktopRolloutReady(obj) {
			return progress("Deployment 已就绪")
		}
		observed, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		for _, raw := range conditions {
			if c, ok := raw.(map[string]any); ok && observed >= obj.GetGeneration() && c["type"] == "Progressing" && c["status"] == "False" && c["reason"] == "ProgressDeadlineExceeded" {
				return errors.New("Deployment 超过就绪期限；配置已提交，未自动回滚")
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("等待就绪超时或已取消；配置可能已更新，请刷新集群核对")
		case <-ticker.C:
		}
	}
}
func desktopRolloutReady(obj *unstructured.Unstructured) bool {
	desired, ok, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if !ok {
		desired = 1
	}
	observed, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
	updated, _, _ := unstructured.NestedInt64(obj.Object, "status", "updatedReplicas")
	ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "availableReplicas")
	total, _, _ := unstructured.NestedInt64(obj.Object, "status", "replicas")
	return observed >= obj.GetGeneration() && updated == desired && ready == desired && total == desired
}
func acquireDesktopLease(ctx context.Context, cl dynamic.Interface, p DesktopPlan) (func(), error) {
	h := sha256.Sum256([]byte(p.Workload))
	name := "devopspad-release-" + hex.EncodeToString(h[:12])
	leases := cl.Resource(leaseGVR).Namespace(p.Namespace)
	obj, err := leases.Get(ctx, name, metav1.GetOptions{})
	missing := apierrors.IsNotFound(err)
	if err != nil && !missing {
		return nil, fmt.Errorf("无法获取发布锁（需要 leases 的 get/create/update 权限）: %w", err)
	}
	now := time.Now().UTC()
	if missing {
		obj = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "coordination.k8s.io/v1", "kind": "Lease", "metadata": map[string]any{"name": name, "namespace": p.Namespace}}}
	} else {
		holder, _, _ := unstructured.NestedString(obj.Object, "spec", "holderIdentity")
		renew, _, _ := unstructured.NestedString(obj.Object, "spec", "renewTime")
		duration, _, _ := unstructured.NestedInt64(obj.Object, "spec", "leaseDurationSeconds")
		at, parseErr := time.Parse(time.RFC3339Nano, renew)
		if holder != "" && (parseErr != nil || duration <= 0 || now.Before(at.Add(time.Duration(duration)*time.Second))) {
			return nil, errors.New("此 Deployment 正在由另一个客户端发布，请稍后重试")
		}
	}
	obj.Object["spec"] = map[string]any{"holderIdentity": p.ID, "leaseDurationSeconds": int64(300), "renewTime": now.Format(time.RFC3339Nano)}
	if missing {
		_, err = leases.Create(ctx, obj, metav1.CreateOptions{})
	} else {
		_, err = leases.Update(ctx, obj, metav1.UpdateOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("发布锁获取失败: %w", err)
	}
	return func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		obj, err := leases.Get(cleanup, name, metav1.GetOptions{})
		if err != nil {
			return
		}
		holder, _, _ := unstructured.NestedString(obj.Object, "spec", "holderIdentity")
		if holder != p.ID {
			return
		}
		_ = unstructured.SetNestedField(obj.Object, "", "spec", "holderIdentity")
		_, _ = leases.Update(cleanup, obj, metav1.UpdateOptions{})
	}, nil
}
