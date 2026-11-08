package opsrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

// ImagePlaceholder in a Job manifest is replaced by the release image.
const ImagePlaceholder = "${DEVOPSPAD_IMAGE}"

const maxJobManifest = 1 << 20

var (
	jobGVR       = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	docSeparator = regexp.MustCompile(`(?m)^---\s*$`)
)

// SavedMigrate is the rendered migration Job of a release plan.
type SavedMigrate struct {
	JobFile            string `json:"jobFile"`
	Name               string `json:"name"`
	Manifest           string `json:"manifest"` // rendered JSON
	WaitTimeoutSeconds int    `json:"waitTimeoutSeconds"`
}

// RenderJob turns a Job manifest into the object to create: the release image
// replaces ${DEVOPSPAD_IMAGE}, or the image of every container from the same
// repository; the name gets the plan's short ID so each run is unique; the
// namespace is the environment's.
func RenderJob(manifest []byte, image, namespace, planID string) (*unstructured.Unstructured, error) {
	if len(manifest) > maxJobManifest {
		return nil, errors.New("the Job manifest exceeds 1 MiB")
	}
	docs := 0
	for _, d := range docSeparator.Split(string(manifest), -1) {
		if strings.TrimSpace(d) != "" {
			docs++
		}
	}
	if docs != 1 {
		return nil, errors.New("the Job manifest must contain exactly one document")
	}
	placeholder := bytes.Contains(manifest, []byte(ImagePlaceholder))
	manifest = bytes.ReplaceAll(manifest, []byte(ImagePlaceholder), []byte(image))
	js, err := yaml.YAMLToJSON(manifest)
	if err != nil {
		return nil, fmt.Errorf("Job manifest: %w", err)
	}
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(js, &obj.Object); err != nil {
		return nil, fmt.Errorf("Job manifest: %w", err)
	}
	if obj.GetAPIVersion() != "batch/v1" || obj.GetKind() != "Job" {
		return nil, errors.New("the migration manifest must be apiVersion batch/v1, kind Job")
	}
	if ns := obj.GetNamespace(); ns != "" && ns != namespace {
		return nil, fmt.Errorf("the Job namespace %s differs from the environment's %s", ns, namespace)
	}
	repo := imageRepository(image)
	replaced := placeholder
	for _, path := range [][]string{{"spec", "template", "spec", "containers"}, {"spec", "template", "spec", "initContainers"}} {
		containers, _, _ := unstructured.NestedSlice(obj.Object, path...)
		for _, raw := range containers {
			if c, ok := raw.(map[string]any); ok {
				if img, _ := c["image"].(string); img != "" && imageRepository(img) == repo {
					c["image"] = image
					replaced = true
				}
			}
		}
		if len(containers) > 0 {
			_ = unstructured.SetNestedSlice(obj.Object, containers, path...)
		}
	}
	if !replaced {
		return nil, fmt.Errorf("no container in the Job uses %s; set image: %s", repo, ImagePlaceholder)
	}
	base := obj.GetName()
	if base == "" {
		base = obj.GetGenerateName()
	}
	base = strings.TrimSuffix(base, "-")
	if base == "" {
		base = "migrate"
	}
	short := strings.ReplaceAll(planID, "-", "")[:8]
	if len(base) > 63-9 {
		base = strings.TrimRight(base[:63-9], "-")
	}
	obj.SetName(base + "-" + short)
	obj.SetGenerateName("")
	obj.SetNamespace(namespace)
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann["devopspad.io/release"] = planID
	obj.SetAnnotations(ann)
	delete(obj.Object, "status")
	return obj, nil
}

// imageRepository strips the digest or tag from an image reference.
func imageRepository(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}

// DryRunJob validates the Job with the API server without creating it.
func DryRunJob(ctx context.Context, cl dynamic.Interface, job *unstructured.Unstructured) error {
	if _, err := cl.Resource(jobGVR).Namespace(job.GetNamespace()).Create(ctx, job.DeepCopy(), metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
		return fmt.Errorf("migration Job dry-run failed (needs create jobs): %w", err)
	}
	return nil
}

// RunJob creates the Job and waits for it to complete or fail. Records get the
// name, duration and outcome; logs stay with the executor.
func RunJob(ctx context.Context, cl dynamic.Interface, job *unstructured.Unstructured, wait time.Duration, step func(string) error) error {
	jobs := cl.Resource(jobGVR).Namespace(job.GetNamespace())
	start := time.Now()
	if _, err := jobs.Create(ctx, job.DeepCopy(), metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create Job %s: %w", job.GetName(), err)
	}
	if err := step("created Job " + job.GetName() + "; waiting for completion"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		obj, err := jobs.Get(ctx, job.GetName(), metav1.GetOptions{})
		if err == nil {
			conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
			for _, raw := range conditions {
				c, ok := raw.(map[string]any)
				if !ok || c["status"] != "True" {
					continue
				}
				took := time.Since(start).Round(time.Second)
				switch c["type"] {
				case "Complete":
					return step(fmt.Sprintf("Job %s completed in %s", job.GetName(), took))
				case "Failed":
					reason, _ := c["reason"].(string)
					return fmt.Errorf("Job %s failed after %s (%s); see its pod logs", job.GetName(), took, reason)
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Job %s did not complete within %s; it was left running for inspection", job.GetName(), wait)
		case <-ticker.C:
		}
	}
}
