package release

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestImageParts(t *testing.T) {
	for ref, want := range map[string][3]string{
		"nginx":                          {"docker.io", "library/nginx", "latest"},
		"ghcr.io/org/app:1.2":            {"ghcr.io", "org/app", "1.2"},
		"localhost:5000/app@sha256:abc":  {"localhost:5000", "app", "sha256:abc"},
		"registry.lan:5000/team/svc:dev": {"registry.lan:5000", "team/svc", "dev"},
		"bitnami/redis":                  {"docker.io", "bitnami/redis", "latest"},
	} {
		h, r, x := imageParts(ref)
		if [3]string{h, r, x} != want {
			t.Errorf("%s: got %s %s %s", ref, h, r, x)
		}
	}
}

func preflightClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClient(runtime.NewScheme(), objs...)
}

func serviceAccount(name string, pull ...string) *unstructured.Unstructured {
	var list []any
	for _, p := range pull {
		list = append(list, map[string]any{"name": p})
	}
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ServiceAccount",
		"metadata": map[string]any{"name": name, "namespace": "default"}, "imagePullSecrets": list}}
}

func TestPreflightPull(t *testing.T) {
	ctx := context.Background()
	newImage := "ghcr.io/org/app@sha256:" + strings.Repeat("a", 64)
	oldImage := "ghcr.io/olduser/app@sha256:" + strings.Repeat("b", 64)
	cl := preflightClient(patchSecrets()...)
	target := PullTarget{Label: "deployment api", Namespace: "default", Image: newImage, BeforeImage: oldImage, ServiceAccount: "default"}

	missing := target
	missing.PullSecrets = []string{"nope"}
	if _, err := PreflightPull(ctx, cl, []PullTarget{missing}, PreflightOptions{}); err == nil || !strings.Contains(err.Error(), "Secret nope not found") {
		t.Fatalf("missing secret: %v", err)
	}
	opaque := target
	opaque.PullSecrets = []string{"app-keys"}
	if _, err := PreflightPull(ctx, cl, []PullTarget{opaque}, PreflightOptions{}); err == nil || !strings.Contains(err.Error(), "must be kubernetes.io/dockerconfigjson") {
		t.Fatalf("opaque secret: %v", err)
	}
	// Owner change on the same host with a ghcr.io secret: a note pointing to --verify-pull.
	covered := target
	covered.PullSecrets = []string{"ghcr-pull"}
	notes, err := PreflightPull(ctx, cl, []PullTarget{covered}, PreflightOptions{})
	if err != nil || len(notes) != 1 || !strings.Contains(notes[0], "moves from ghcr.io/olduser to ghcr.io/org") || !strings.Contains(notes[0], "--verify-pull") {
		t.Fatalf("covered move: %v %v", notes, err)
	}
	// Moving to another registry without credentials needs an acknowledgement.
	moved := target
	moved.Image = "registry.example.com/org/app@sha256:" + strings.Repeat("a", 64)
	moved.PullSecrets = []string{"ghcr-pull"}
	if _, err := PreflightPull(ctx, cl, []PullTarget{moved}, PreflightOptions{}); err == nil || !strings.Contains(err.Error(), "--ack-registry-change") {
		t.Fatalf("uncovered move: %v", err)
	}
	if notes, err := PreflightPull(ctx, cl, []PullTarget{moved}, PreflightOptions{AckRegistryChange: true}); err != nil || !strings.Contains(notes[0], "acknowledged") {
		t.Fatalf("acknowledged move: %v %v", notes, err)
	}
	// The ServiceAccount's pull secrets count too.
	cl = preflightClient(append(patchSecrets(), serviceAccount("default", "ghcr-pull"))...)
	if notes, err := PreflightPull(ctx, cl, []PullTarget{target}, PreflightOptions{}); err != nil || !strings.Contains(notes[0], "ghcr-pull has credentials") {
		t.Fatalf("service account secret: %v %v", notes, err)
	}
	// Same image and secrets as before: nothing to report.
	same := covered
	same.BeforeImage = "ghcr.io/org/app@sha256:" + strings.Repeat("c", 64)
	if notes, err := PreflightPull(ctx, cl, []PullTarget{same}, PreflightOptions{}); err != nil || len(notes) != 0 {
		t.Fatalf("unchanged registry: %v %v", notes, err)
	}
	job := PullTarget{Label: "migration Job m", Namespace: "default", Image: newImage, PullSecrets: []string{"new-secret"}, ServiceAccount: "default"}
	if _, err := PreflightPull(ctx, cl, []PullTarget{job}, PreflightOptions{}); err == nil || !strings.Contains(err.Error(), "migration Job m: Secret new-secret not found") {
		t.Fatalf("job secret: %v", err)
	}
}

// fakeRegistry serves org/app to user bot with password pat through a Bearer token.
func fakeRegistry(t *testing.T) (host string) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			user, pass, ok := r.BasicAuth()
			if !ok || user != "bot" || pass != "pat" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			scope := r.URL.Query().Get("scope")
			json.NewEncoder(w).Encode(map[string]string{"token": "t:" + scope})
		case strings.HasPrefix(r.URL.Path, "/v2/"):
			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("Www-Authenticate", `Bearer realm="`+srv.URL+`/token",service="fake"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if auth == "Bearer t:repository:org/app:pull" && r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/v2/org/app/manifests/") {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(srv.Close)
	old := registryScheme
	registryScheme = "http"
	t.Cleanup(func() { registryScheme = old })
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestPreflightVerifyPull(t *testing.T) {
	ctx := context.Background()
	host := fakeRegistry(t)
	good := testSecret("reg-pull", dockerConfigType, map[string]string{".dockerconfigjson": `{"auths":{"` + host + `":{"username":"bot","password":"pat"}}}`})
	bad := testSecret("old-pull", dockerConfigType, map[string]string{".dockerconfigjson": `{"auths":{"` + host + `":{"username":"bot","password":"expired"}}}`})
	cl := preflightClient(good, bad)
	image := host + "/org/app@sha256:" + strings.Repeat("a", 64)
	target := PullTarget{Label: "deployment api", Namespace: "default", Image: image, PullSecrets: []string{"reg-pull"}, ServiceAccount: "default"}
	notes, err := PreflightPull(ctx, cl, []PullTarget{target}, PreflightOptions{Verify: true})
	if err != nil || len(notes) != 1 || !strings.Contains(notes[0], "readable with pull secret reg-pull") {
		t.Fatalf("verify ok: %v %v", notes, err)
	}
	target.PullSecrets = []string{"old-pull"}
	if _, err := PreflightPull(ctx, cl, []PullTarget{target}, PreflightOptions{Verify: true}); err == nil || !strings.Contains(err.Error(), "old-pull → HTTP 401") || strings.Contains(err.Error(), "expired") {
		t.Fatalf("bad credentials: %v", err)
	}
	other := target
	other.Image = host + "/someone/else:1"
	other.PullSecrets = []string{"reg-pull"}
	if _, err := PreflightPull(ctx, cl, []PullTarget{other}, PreflightOptions{Verify: true}); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("no access: %v", err)
	}
	public := target
	public.PullSecrets = nil
	if _, err := PreflightPull(ctx, cl, []PullTarget{public}, PreflightOptions{Verify: true}); err == nil || !strings.Contains(err.Error(), "without credentials") {
		t.Fatalf("anonymous: %v", err)
	}
}
