package release

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const dockerConfigType = "kubernetes.io/dockerconfigjson"

// getSecret reads a Secret for reference checks. Callers never print values.
func getSecret(ctx context.Context, cl dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	obj, err := cl.Resource(secretGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		return nil, fmt.Errorf("Secret %s not found in %s; create it first with devopspadcli secret set --name %s ... (or the desktop Secrets page)", name, namespace, name)
	case apierrors.IsForbidden(err):
		return nil, fmt.Errorf("cannot check Secret %s (needs get secrets): %w", name, err)
	case err != nil:
		return nil, fmt.Errorf("check Secret %s: %w", name, err)
	}
	return obj, nil
}

// checkPullSecret requires an existing kubernetes.io/dockerconfigjson Secret
// and returns the registry hosts it has credentials for (never the values).
func checkPullSecret(ctx context.Context, cl dynamic.Interface, namespace, name string) ([]string, error) {
	obj, err := getSecret(ctx, cl, namespace, name)
	if err != nil {
		return nil, err
	}
	if t, _, _ := unstructured.NestedString(obj.Object, "type"); t != dockerConfigType {
		return nil, fmt.Errorf("Secret %s has type %s; image pull secrets must be %s (devopspadcli secret set --type docker-registry)", name, orEmpty(t), dockerConfigType)
	}
	auths, err := dockerAuths(obj)
	if err != nil {
		return nil, fmt.Errorf("Secret %s: %w", name, err)
	}
	hosts := make([]string, 0, len(auths))
	for h := range auths {
		hosts = append(hosts, h)
	}
	return hosts, nil
}

func orEmpty(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

type dockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

// dockerAuths decodes .dockerconfigjson, keyed by normalized registry host.
func dockerAuths(obj *unstructured.Unstructured) (map[string]dockerAuth, error) {
	data, _, _ := unstructured.NestedStringMap(obj.Object, "data")
	raw, err := base64.StdEncoding.DecodeString(data[".dockerconfigjson"])
	if err != nil {
		return nil, fmt.Errorf("invalid .dockerconfigjson")
	}
	var cfg struct {
		Auths map[string]dockerAuth `json:"auths"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid .dockerconfigjson")
	}
	out := map[string]dockerAuth{}
	for host, a := range cfg.Auths {
		if a.Username == "" && a.Auth != "" {
			if b, err := base64.StdEncoding.DecodeString(a.Auth); err == nil {
				a.Username, a.Password, _ = strings.Cut(string(b), ":")
			}
		}
		out[normalizeRegistry(host)] = a
	}
	return out, nil
}

// normalizeRegistry turns auth keys such as https://index.docker.io/v1/ into hosts.
func normalizeRegistry(host string) string {
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	host, _, _ = strings.Cut(host, "/")
	switch host {
	case "index.docker.io", "registry-1.docker.io", "registry.hub.docker.com":
		return "docker.io"
	}
	return strings.ToLower(host)
}

// checkSecretKey requires an existing Secret with the key.
func checkSecretKey(ctx context.Context, cl dynamic.Interface, namespace, name, key string) error {
	obj, err := getSecret(ctx, cl, namespace, name)
	if err != nil {
		return err
	}
	data, _, _ := unstructured.NestedMap(obj.Object, "data")
	if _, ok := data[key]; !ok {
		return fmt.Errorf("Secret %s has no key %s", name, key)
	}
	return nil
}
