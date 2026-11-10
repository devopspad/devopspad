package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"devopspad/internal/ops"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
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
	// UID and ResourceVersion identify the Deployment the plan was made against.
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
	// Changes describes a patch plan in words (field names only, no values).
	Changes []string `json:"changes,omitempty"`
	// Warnings are preflight findings to review before confirming.
	Warnings  []string `json:"warnings,omitempty"`
	candidate *unstructured.Unstructured
}

// Rebind gives a freshly prepared plan an earlier plan's ID, so a plan saved
// by one process and re-prepared by another keeps one ID in its record and in
// the Pod template annotation.
func (p *DesktopPlan) Rebind(id string) {
	p.ID = id
	if p.candidate == nil || p.Action == "scale" {
		return
	}
	annotations, _, _ := unstructured.NestedStringMap(p.candidate.Object, "spec", "template", "metadata", "annotations")
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["devopspad.io/release"] = id
	_ = unstructured.SetNestedStringMap(p.candidate.Object, annotations, "spec", "template", "metadata", "annotations")
}

func PrepareDesktop(ctx context.Context, cl dynamic.Interface, namespace, workload, container, image, selector string) (DesktopPlan, error) {
	if !ops.ValidName(container) || !ops.ValidImage(image) {
		return DesktopPlan{}, errors.New("Invalid release target or image digest")
	}
	return prepareDesktop(ctx, cl, namespace, workload, selector, "release", func(obj *unstructured.Unstructured, p *DesktopPlan) error {
		p.Container, p.Image = container, image
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
			return errors.New("Target container not found")
		}
		if p.BeforeImage == image {
			return errors.New("The target container already runs this artifact")
		}
		return unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
	})
}

// PrepareDesktopAction uses the same optimistic concurrency and lease as image releases.
func PrepareDesktopAction(ctx context.Context, cl dynamic.Interface, namespace, workload, selector, action string, replicas int64) (DesktopPlan, error) {
	switch action {
	case "restart":
		return prepareDesktop(ctx, cl, namespace, workload, selector, action, nil)
	case "scale":
		if replicas < 0 || replicas > 1000 {
			return DesktopPlan{}, errors.New("Replicas must be between 0 and 1000")
		}
		return prepareDesktop(ctx, cl, namespace, workload, selector, action, func(obj *unstructured.Unstructured, p *DesktopPlan) error {
			if replicas == p.BeforeReplicas {
				return errors.New("The replica count is unchanged")
			}
			p.Replicas = replicas
			return unstructured.SetNestedField(obj.Object, replicas, "spec", "replicas")
		})
	}
	return DesktopPlan{}, errors.New("Unsupported Deployment operation")
}

// prepareDesktop reads the Deployment, checks it belongs to the application,
// applies mutate to a copy and validates the result with a server dry-run.
// Every action except scale also stamps the Pod template with the plan ID,
// which rolls the Pods and makes the rollout traceable.
func prepareDesktop(ctx context.Context, cl dynamic.Interface, namespace, workload, selector, action string, mutate func(*unstructured.Unstructured, *DesktopPlan) error) (DesktopPlan, error) {
	p := DesktopPlan{ID: uuid.NewString(), Action: action, Namespace: namespace, Workload: workload, ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)}
	if !ops.ValidName(namespace) || !ops.ValidName(workload) {
		return p, errors.New("Invalid Deployment target")
	}
	obj, err := cl.Resource(deploymentGVR).Namespace(namespace).Get(ctx, workload, metav1.GetOptions{})
	if err != nil {
		return p, err
	}
	sel, err := labels.Parse(selector)
	if err != nil || selector == "" {
		return p, errors.New("Invalid application selector")
	}
	template, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
	if !sel.Matches(labels.Set(template)) {
		return p, errors.New("The Deployment does not belong to this application")
	}
	paused, _, _ := unstructured.NestedBool(obj.Object, "spec", "paused")
	if paused {
		return p, errors.New("The Deployment is paused; resume it before releasing")
	}
	if obj.GetResourceVersion() == "" || obj.GetUID() == "" {
		return p, errors.New("The Deployment has no resource version or UID")
	}
	p.UID, p.ResourceVersion = string(obj.GetUID()), obj.GetResourceVersion()
	p.BeforeReplicas, _, _ = unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if _, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); !found {
		p.BeforeReplicas = 1
	}
	p.Replicas = p.BeforeReplicas
	if mutate != nil {
		if err := mutate(obj, &p); err != nil {
			return p, err
		}
	}
	// Scaling must not change the Pod template: that would cause an unintended rollout.
	if action != "scale" {
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
		return p, fmt.Errorf("Release dry-run failed: %w", err)
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
		return errors.New("The plan has expired; create a new one")
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	unlock, err := acquireDesktopLease(ctx, cl, p)
	if err != nil {
		return err
	}
	defer unlock()
	if err = progress("Acquired the release lock"); err != nil {
		return err
	}
	current, err := cl.Resource(deploymentGVR).Namespace(p.Namespace).Get(ctx, p.Workload, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if current.GetUID() != p.candidate.GetUID() || current.GetResourceVersion() != p.candidate.GetResourceVersion() {
		return errors.New("The Deployment changed; create a new plan")
	}
	if err = progress("Dry-run passed; updating the Deployment"); err != nil {
		return err
	}
	updated, err := cl.Resource(deploymentGVR).Namespace(p.Namespace).Update(ctx, p.candidate.DeepCopy(), metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("The update was not confirmed; refresh the cluster to check the actual state: %w", err)
	}
	if err = progress("Deployment updated; waiting for rollout"); err != nil {
		return fmt.Errorf("The change was applied, but writing the record failed: %w", err)
	}
	pods := podSelector(updated)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		obj, err := cl.Resource(deploymentGVR).Namespace(p.Namespace).Get(ctx, p.Workload, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("The change was applied, but checking status failed: %w", err)
		}
		if obj.GetUID() != updated.GetUID() || obj.GetGeneration() != updated.GetGeneration() {
			return errors.New("The Deployment was modified by another operation during the release; check the actual state")
		}
		if desktopRolloutReady(obj) {
			if err := progress("Deployment is ready"); err != nil {
				return err
			}
			if p.Action != "release" {
				return nil
			}
			desired, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
			if !found {
				desired = 1
			}
			evidence, err := verifyReleasedDigest(ctx, cl, p, pods, desired)
			if err != nil {
				return err
			}
			return progress(evidence)
		}
		observed, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		for _, raw := range conditions {
			if c, ok := raw.(map[string]any); ok && observed >= obj.GetGeneration() && c["type"] == "Progressing" && c["status"] == "False" && c["reason"] == "ProgressDeadlineExceeded" {
				return withWaiting(errors.New("The Deployment exceeded its progress deadline; the change was applied and not rolled back"), cl, p, pods)
			}
		}
		select {
		case <-ctx.Done():
			return withWaiting(errors.New("Timed out or cancelled while waiting for the rollout; the change may have been applied, refresh the cluster to check"), cl, p, pods)
		case <-ticker.C:
		}
	}
}

var podGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

// podSelector selects the Deployment's pods. Every target of a multi-target
// release carries the same release annotation, so pods must also be narrowed
// to the target workload. A Deployment without a usable selector falls back to
// the annotation and container checks alone.
func podSelector(deployment *unstructured.Unstructured) labels.Selector {
	raw, found, _ := unstructured.NestedMap(deployment.Object, "spec", "selector")
	if !found {
		return labels.Everything()
	}
	var ls metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &ls); err != nil {
		return labels.Everything()
	}
	sel, err := metav1.LabelSelectorAsSelector(&ls)
	if err != nil {
		return labels.Everything()
	}
	return sel
}

// withWaiting adds why the release's pods are stuck, when they are.
func withWaiting(err error, cl dynamic.Interface, p DesktopPlan, selector labels.Selector) error {
	release := ReleasePods(p.ID)
	match := func(pod *unstructured.Unstructured) bool {
		return release(pod) && selector.Matches(labels.Set(pod.GetLabels()))
	}
	if s := PodWaitingSummary(cl, p.Namespace, match); s != "" {
		return fmt.Errorf("%w (%s)", err, s)
	}
	return err
}

// verifyReleasedDigest checks that every running pod of this release (the
// target Deployment's pods carrying its devopspad.io/release annotation) runs
// the released digest in the target container. Registry names may be
// normalized by the runtime, so only the sha256 digest is compared. Pods
// without an imageID yet are retried briefly; a mismatch fails the release.
func verifyReleasedDigest(ctx context.Context, cl dynamic.Interface, p DesktopPlan, selector labels.Selector, desired int64) (string, error) {
	_, digest, ok := strings.Cut(p.Image, "@")
	if !ok || !strings.HasPrefix(digest, "sha256:") {
		return "image-check skipped: the release image is not pinned by digest", nil
	}
	var last string
	for attempt := 0; attempt < 15; attempt++ {
		pods, err := cl.Resource(podGVR).Namespace(p.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String(), Limit: 500})
		if err != nil {
			return "", fmt.Errorf("The rollout is ready, but listing pods for the image check failed (needs list pods): %w", err)
		}
		var checked []string
		pending := false
		for _, pod := range pods.Items {
			if pod.GetAnnotations()["devopspad.io/release"] != p.ID || pod.GetDeletionTimestamp() != nil || !selector.Matches(labels.Set(pod.GetLabels())) || !hasContainer(&pod, p.Container) {
				continue
			}
			phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
			if phase != "Running" {
				continue
			}
			statuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
			imageID := ""
			for _, raw := range statuses {
				if st, ok := raw.(map[string]any); ok && st["name"] == p.Container {
					imageID, _ = st["imageID"].(string)
				}
			}
			switch {
			case imageID == "":
				pending, last = true, "pod "+pod.GetName()+" has not reported an imageID yet"
			case !strings.HasSuffix(imageID, "@"+digest):
				return "", fmt.Errorf("The rollout is ready, but pod %s runs %s instead of the released %s; the release is marked failed and was not rolled back", pod.GetName(), imageID, digest)
			default:
				checked = append(checked, pod.GetName())
			}
		}
		if !pending && len(checked) == 0 && desired > 0 {
			pending, last = true, "no running pod carries this release's annotation yet"
		}
		if !pending {
			if len(checked) == 0 {
				return "image-check: the Deployment has 0 replicas", nil
			}
			sort.Strings(checked)
			return fmt.Sprintf("image-check: %s run %s", strings.Join(checked, ", "), digest), nil
		}
		select {
		case <-ctx.Done():
			return "", errors.New("The rollout is ready, but the image check timed out: " + last)
		case <-time.After(2 * time.Second):
		}
	}
	return "", errors.New("The rollout is ready, but the image check could not complete: " + last)
}

func hasContainer(pod *unstructured.Unstructured, name string) bool {
	containers, _, _ := unstructured.NestedSlice(pod.Object, "spec", "containers")
	for _, raw := range containers {
		if c, ok := raw.(map[string]any); ok && c["name"] == name {
			return true
		}
	}
	return false
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

// LockDesktop takes the plan's release lock ahead of ApplyDesktop, e.g. before
// a migration Job, so a lock held by another client stops the release before
// anything changes. ApplyDesktop re-acquires it for the same plan ID.
func LockDesktop(ctx context.Context, cl dynamic.Interface, p DesktopPlan) (func(), error) {
	return acquireDesktopLease(ctx, cl, p)
}

// leaseClock is replaced in tests.
var leaseClock = time.Now

// acquireDesktopLease is re-entrant for the plan that holds the lease.
func acquireDesktopLease(ctx context.Context, cl dynamic.Interface, p DesktopPlan) (func(), error) {
	h := sha256.Sum256([]byte(p.Workload))
	name := "devopspad-release-" + hex.EncodeToString(h[:12])
	leases := cl.Resource(leaseGVR).Namespace(p.Namespace)
	obj, err := leases.Get(ctx, name, metav1.GetOptions{})
	missing := apierrors.IsNotFound(err)
	if err != nil && !missing {
		return nil, fmt.Errorf("Cannot acquire the release lock (needs get/create/update on leases): %w", err)
	}
	now := leaseClock().UTC()
	if missing {
		obj = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "coordination.k8s.io/v1", "kind": "Lease", "metadata": map[string]any{"name": name, "namespace": p.Namespace}}}
	} else {
		holder, _, _ := unstructured.NestedString(obj.Object, "spec", "holderIdentity")
		renew, _, _ := unstructured.NestedString(obj.Object, "spec", "renewTime")
		duration, _, _ := unstructured.NestedInt64(obj.Object, "spec", "leaseDurationSeconds")
		at, parseErr := time.Parse(time.RFC3339Nano, renew)
		if holder != "" && holder != p.ID && (parseErr != nil || duration <= 0 || now.Before(at.Add(time.Duration(duration)*time.Second))) {
			return nil, errors.New("Another client is releasing this Deployment; try again later")
		}
	}
	// renewTime is a MicroTime: the API server rejects anything but six fractional digits.
	obj.Object["spec"] = map[string]any{"holderIdentity": p.ID, "leaseDurationSeconds": int64(300), "renewTime": now.Format(metav1.RFC3339Micro)}
	if missing {
		_, err = leases.Create(ctx, obj, metav1.CreateOptions{})
	} else {
		_, err = leases.Update(ctx, obj, metav1.UpdateOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("Failed to acquire the release lock: %w", err)
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
