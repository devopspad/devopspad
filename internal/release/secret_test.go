package release

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

// dryRunFirst makes the first create/update of a resource a no-op, like a
// server dry-run (the fake client ignores DryRun options).
func dryRunFirst(cl *fake.FakeDynamicClient, verb, resource string) {
	calls := 0
	cl.PrependReactor(verb, resource, func(action ktesting.Action) (bool, runtime.Object, error) {
		calls++
		if calls == 1 {
			if a, ok := action.(ktesting.CreateActionImpl); ok {
				return true, a.GetObject(), nil
			}
			return true, action.(ktesting.UpdateActionImpl).GetObject(), nil
		}
		return false, nil, nil
	})
}

func patchDeployment() *unstructured.Unstructured {
	d := desktopObject()
	containers := []any{map[string]any{"name": "app", "image": "old:1", "env": []any{
		map[string]any{"name": "MODE", "value": "prod"},
		map[string]any{"name": "TOKEN", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "old", "key": "t"}}},
	}}}
	_ = unstructured.SetNestedSlice(d.Object, containers, "spec", "template", "spec", "containers")
	return d
}

func TestPrepareDesktopPatch(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme(), patchDeployment())
	cl.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, action.(ktesting.UpdateActionImpl).GetObject(), nil
	})
	patch := WorkloadPatch{Container: "app", AddImagePullSecrets: []string{"ghcr-pull"}, AddEnvFromSecrets: []string{"app-keys"},
		SetEnvSecretRefs: []EnvSecretRef{{Name: "TOKEN", Secret: "app-keys", Key: "token"}, {Name: "OWNER", Secret: "app-keys", Key: "owner"}}}
	p, err := PrepareDesktopPatch(context.Background(), cl, "default", "svc-user", "app=svc-user", patch)
	if err != nil {
		t.Fatal(err)
	}
	if p.Action != "patch" || len(p.Changes) != 4 || !strings.Contains(ChangeSummary(p), "add imagePullSecret ghcr-pull") {
		t.Fatalf("changes: %v", p.Changes)
	}
	spec, _, _ := unstructured.NestedMap(p.candidate.Object, "spec", "template", "spec")
	if spec["imagePullSecrets"].([]any)[0].(map[string]any)["name"] != "ghcr-pull" {
		t.Fatalf("pull secrets: %v", spec["imagePullSecrets"])
	}
	c := spec["containers"].([]any)[0].(map[string]any)
	env := c["env"].([]any)
	if len(env) != 3 || env[0].(map[string]any)["value"] != "prod" || c["envFrom"].([]any)[0].(map[string]any)["secretRef"].(map[string]any)["name"] != "app-keys" {
		t.Fatalf("container: %v", c)
	}
	if ann, _, _ := unstructured.NestedStringMap(p.candidate.Object, "spec", "template", "metadata", "annotations"); ann["devopspad.io/release"] != p.ID {
		t.Fatal("patch must roll the pods")
	}
	for name, bad := range map[string]WorkloadPatch{
		"literal env":    {Container: "app", SetEnvSecretRefs: []EnvSecretRef{{Name: "MODE", Secret: "s", Key: "k"}}},
		"remove literal": {Container: "app", RemoveEnv: []string{"MODE"}},
		"no container":   {AddEnvFromSecrets: []string{"s"}},
		"nothing":        {},
		"bad name":       {AddImagePullSecrets: []string{"Bad_Name"}},
		"missing":        {Container: "nope", AddEnvFromSecrets: []string{"s"}},
		"already":        {RemoveImagePullSecrets: []string{"never-added"}},
	} {
		if _, err := PrepareDesktopPatch(context.Background(), cl, "default", "svc-user", "app=svc-user", bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestSecretCreateUpdateAndDescribe(t *testing.T) {
	cl := fake.NewSimpleDynamicClient(runtime.NewScheme())
	dryRunFirst(cl, "create", "secrets")
	p, err := PrepareSecret(context.Background(), cl, "default", "app-keys", "generic", map[string][]byte{"token": []byte("s3cret"), "owner": []byte("me")})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Create || len(p.Keys) != 2 || p.Keys[0].Change != "added" || len(p.ContentSHA256) != 12 {
		t.Fatalf("create plan: %+v", p)
	}
	if _, err := cl.Resource(secretGVR).Namespace("default").Get(context.Background(), "app-keys", metav1.GetOptions{}); err == nil {
		t.Fatal("plan created the Secret")
	}
	if err := ApplySecret(context.Background(), cl, p); err != nil {
		t.Fatal(err)
	}
	cl2 := cl
	dryRunFirst(cl2, "update", "secrets")
	p, err = PrepareSecret(context.Background(), cl2, "default", "app-keys", "generic", map[string][]byte{"token": []byte("rotated"), "extra": []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]string{}
	for _, k := range p.Keys {
		changes[k.Key] = k.Change
	}
	if p.Create || changes["token"] != "changed" || changes["extra"] != "added" || changes["owner"] != "removed" {
		t.Fatalf("update plan: %+v", p.Keys)
	}
	if err := ApplySecret(context.Background(), cl2, p); err != nil {
		t.Fatal(err)
	}
	info, err := DescribeSecret(context.Background(), cl2, "default", "app-keys")
	if err != nil || strings.Join(info.Keys, ",") != "extra,token" || info.Type != "Opaque" {
		t.Fatalf("describe: %+v %v", info, err)
	}
	obj, _ := cl2.Resource(secretGVR).Namespace("default").Get(context.Background(), "app-keys", metav1.GetOptions{})
	data, _, _ := unstructured.NestedStringMap(obj.Object, "data")
	if data["token"] != base64.StdEncoding.EncodeToString([]byte("rotated")) {
		t.Fatalf("stored data: %v", data)
	}
	if _, err := PrepareSecret(context.Background(), cl2, "default", "app-keys", "generic", map[string][]byte{"token": []byte("rotated"), "extra": []byte("x")}); err == nil || !strings.Contains(err.Error(), "already has") {
		t.Fatalf("unchanged: %v", err)
	}
	if _, err := PrepareSecret(context.Background(), cl2, "default", "app-keys", "docker-registry", map[string][]byte{".dockerconfigjson": []byte(`{"auths":{"ghcr.io":{}}}`)}); err == nil || !strings.Contains(err.Error(), "type cannot change") {
		t.Fatalf("type change: %v", err)
	}
	if _, err := PrepareSecret(context.Background(), cl2, "default", "pull", "docker-registry", map[string][]byte{".dockerconfigjson": []byte(`{}`)}); err == nil {
		t.Fatal("empty docker config accepted")
	}
	cfg, err := DockerConfigJSON("ghcr.io", "bot", "pat")
	if err != nil || !strings.Contains(string(cfg), `"ghcr.io"`) {
		t.Fatalf("docker config: %s %v", cfg, err)
	}
}
