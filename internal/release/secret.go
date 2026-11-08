package release

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var secretGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

// SecretKeyChange describes one key by name only.
type SecretKeyChange struct {
	Key    string `json:"key"`
	Change string `json:"change"` // added | changed | removed | unchanged
}

// SecretPlan creates or replaces a Secret's data. It holds the values in
// memory only; it is never saved and never printed.
type SecretPlan struct {
	ID              string            `json:"id"`
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Create          bool              `json:"create"`
	Keys            []SecretKeyChange `json:"keys"`
	ContentSHA256   string            `json:"contentSha256"` // first 12 hex of the hash over all keys and values
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	candidate       *unstructured.Unstructured
}

// SecretTypes maps CLI names to Kubernetes Secret types.
var SecretTypes = map[string]string{"generic": "Opaque", "docker-registry": "kubernetes.io/dockerconfigjson"}

// DockerConfigJSON builds a .dockerconfigjson value for one registry.
func DockerConfigJSON(server, username, password string) ([]byte, error) {
	if server == "" || username == "" || password == "" {
		return nil, errors.New("docker-registry secrets need a server, username and password")
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return json.Marshal(map[string]any{"auths": map[string]any{server: map[string]string{"username": username, "password": password, "auth": auth}}})
}

// PrepareSecret computes key changes against the live Secret and validates the
// new object with a server dry-run.
func PrepareSecret(ctx context.Context, cl dynamic.Interface, namespace, name, kind string, data map[string][]byte) (SecretPlan, error) {
	p := SecretPlan{ID: uuid.NewString(), Namespace: namespace, Name: name}
	typ, ok := SecretTypes[kind]
	if !ok {
		return p, errors.New("--type must be generic or docker-registry")
	}
	p.Type = typ
	if !k8sNameRE.MatchString(name) || len(name) > 253 {
		return p, fmt.Errorf("%q is not a valid Secret name", name)
	}
	if len(data) == 0 {
		return p, errors.New("a Secret needs at least one key")
	}
	total := 0
	for k, v := range data {
		if !keyRE.MatchString(k) {
			return p, fmt.Errorf("%q is not a valid Secret key", k)
		}
		total += len(v)
	}
	if total > 900<<10 {
		return p, errors.New("Secret data exceeds 900 KiB")
	}
	if typ == "kubernetes.io/dockerconfigjson" {
		var cfg struct {
			Auths map[string]any `json:"auths"`
		}
		if len(data) != 1 || json.Unmarshal(data[".dockerconfigjson"], &cfg) != nil || len(cfg.Auths) == 0 {
			return p, errors.New("a docker-registry Secret has exactly one key, .dockerconfigjson, with an auths object")
		}
	}
	secrets := cl.Resource(secretGVR).Namespace(namespace)
	existing, err := secrets.Get(ctx, name, metav1.GetOptions{})
	old := map[string]string{}
	switch {
	case apierrors.IsNotFound(err):
		p.Create = true
		existing = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": name, "namespace": namespace}}}
	case err != nil:
		return p, fmt.Errorf("read Secret (needs get secrets): %w", err)
	default:
		if t, _, _ := unstructured.NestedString(existing.Object, "type"); t != "" && t != typ {
			return p, fmt.Errorf("Secret %s has type %s; the type cannot change, use another name", name, t)
		}
		if immutable, _, _ := unstructured.NestedBool(existing.Object, "immutable"); immutable {
			return p, fmt.Errorf("Secret %s is immutable", name)
		}
		old, _, _ = unstructured.NestedStringMap(existing.Object, "data")
		p.ResourceVersion = existing.GetResourceVersion()
	}
	encoded := map[string]any{}
	keys := make([]string, 0, len(data))
	for k, v := range data {
		encoded[k] = base64.StdEncoding.EncodeToString(v)
		keys = append(keys, k)
	}
	for k := range old {
		if _, ok := data[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	h := sha256.New()
	changed := false
	for _, k := range keys {
		before, had := old[k]
		after, has := encoded[k]
		c := SecretKeyChange{Key: k}
		switch {
		case !had:
			c.Change = "added"
		case !has:
			c.Change = "removed"
		case before != after:
			c.Change = "changed"
		default:
			c.Change = "unchanged"
		}
		if c.Change != "unchanged" {
			changed = true
		}
		if has {
			h.Write([]byte(k + "\x00"))
			h.Write(data[k])
			h.Write([]byte{0})
		}
		p.Keys = append(p.Keys, c)
	}
	if !changed && !p.Create {
		return p, errors.New("the Secret already has this content")
	}
	p.ContentSHA256 = hex.EncodeToString(h.Sum(nil))[:12]
	obj := existing.DeepCopy()
	obj.Object["type"] = typ
	obj.Object["data"] = encoded
	delete(obj.Object, "stringData")
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann["devopspad.io/updated-at"] = time.Now().UTC().Format(time.RFC3339)
	obj.SetAnnotations(ann)
	obj.SetManagedFields(nil)
	dry := []string{metav1.DryRunAll}
	if p.Create {
		_, err = secrets.Create(ctx, obj, metav1.CreateOptions{DryRun: dry})
	} else {
		_, err = secrets.Update(ctx, obj, metav1.UpdateOptions{DryRun: dry})
	}
	if err != nil {
		return p, fmt.Errorf("Secret dry-run failed (needs create/update secrets): %w", err)
	}
	p.candidate = obj
	return p, nil
}

// ApplySecret creates the Secret (failing if it appeared meanwhile) or updates
// it with the resourceVersion read by the plan.
func ApplySecret(ctx context.Context, cl dynamic.Interface, p SecretPlan) error {
	if p.candidate == nil {
		return errors.New("the plan has no Secret; create a new one")
	}
	secrets := cl.Resource(secretGVR).Namespace(p.Namespace)
	var err error
	if p.Create {
		_, err = secrets.Create(ctx, p.candidate.DeepCopy(), metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			return errors.New("the Secret was created by someone else meanwhile; plan again")
		}
	} else {
		_, err = secrets.Update(ctx, p.candidate.DeepCopy(), metav1.UpdateOptions{})
		if apierrors.IsConflict(err) {
			return errors.New("the Secret changed since the plan; plan again")
		}
	}
	return err
}

// SecretInfo lists a Secret without its values.
type SecretInfo struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Keys            []string `json:"keys"`
	ResourceVersion string   `json:"resourceVersion"`
	UpdatedAt       string   `json:"updatedAt,omitempty"`
}

// DescribeSecret returns key names and the last change time only.
func DescribeSecret(ctx context.Context, cl dynamic.Interface, namespace, name string) (SecretInfo, error) {
	obj, err := cl.Resource(secretGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return SecretInfo{}, err
	}
	info := SecretInfo{Name: name, ResourceVersion: obj.GetResourceVersion(), Keys: []string{}}
	info.Type, _, _ = unstructured.NestedString(obj.Object, "type")
	data, _, _ := unstructured.NestedMap(obj.Object, "data")
	for k := range data {
		info.Keys = append(info.Keys, k)
	}
	sort.Strings(info.Keys)
	info.UpdatedAt = obj.GetAnnotations()["devopspad.io/updated-at"]
	for _, mf := range obj.GetManagedFields() {
		if mf.Time != nil && mf.Time.UTC().Format(time.RFC3339) > info.UpdatedAt {
			info.UpdatedAt = mf.Time.UTC().Format(time.RFC3339)
		}
	}
	if info.UpdatedAt == "" {
		info.UpdatedAt = obj.GetCreationTimestamp().UTC().Format(time.RFC3339)
	}
	return info, nil
}
