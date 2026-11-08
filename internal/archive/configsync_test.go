package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devopspad/internal/workspace"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

type configFakes struct {
	nginxLive string
	saved     []workspace.NginxUpdate
	client    *dynfake.FakeDynamicClient
}

func fakeConfigs(t *testing.T, cmValue string) *configFakes {
	t.Helper()
	conf, _ := os.ReadFile(filepath.Join(example, "products/heartie/configs/admin/pro/heartie-admin.conf"))
	f := &configFakes{nginxLive: string(conf)}
	cm := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "heartie-admin-config", "namespace": "default", "resourceVersion": "7"},
		"data":     map[string]any{"config.toml": cmValue, "other": "keep"}}}
	f.client = dynfake.NewSimpleDynamicClient(runtime.NewScheme(), cm)
	dryRun := true
	f.client.PrependReactor("update", "configmaps", func(action ktesting.Action) (bool, runtime.Object, error) {
		if dryRun {
			dryRun = false
			return true, action.(ktesting.UpdateActionImpl).GetObject(), nil
		}
		return false, nil, nil
	})
	oldRead, oldClient, oldSave, oldKey := sshReadFile, clusterClient, nginxSave, keychainGet
	sshReadFile = func(_ context.Context, c workspace.Connection, p string, _ int) ([]byte, error) {
		switch p {
		case "/etc/nginx/conf.d/heartie-admin.conf":
			return []byte(f.nginxLive), nil
		case "/etc/rancher/k3s/k3s.yaml":
			return []byte("apiVersion: v1\nkind: Config\n"), nil
		}
		return nil, errors.New("unexpected read " + p)
	}
	clusterClient = func(context.Context, workspace.Connection) (dynamic.Interface, func(), error) {
		return f.client, func() {}, nil
	}
	nginxSave = func(_ context.Context, _ workspace.Connection, p string, u workspace.NginxUpdate) (workspace.NginxConfig, error) {
		f.saved = append(f.saved, u)
		return workspace.NginxConfig{Path: p}, nil
	}
	keychainGet = func(account string) (string, error) {
		if account == "archive/heartie/api/db-password" {
			return "pw123", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { sshReadFile, clusterClient, nginxSave, keychainGet = oldRead, oldClient, oldSave, oldKey })
	return f
}

const tomlLive = "[server]\nlisten = \":8080\"\ntimeout = \"15s\"\n\n[database]\nhost = \"10.0.0.2\"\nname = \"heartie\"\nuser = \"heartie\"\npassword = \"pw123\"\n"

func TestConfigDiffMasksSecrets(t *testing.T) {
	fakeConfigs(t, tomlLive)
	a := load(t, copyExample(t))
	ctx := context.Background()
	ref, err := ParseConfigRef("heartie/admin@pro", "app")
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.ConfigDiff(ctx, ref)
	if err != nil || !res.InSync {
		t.Fatalf("rendered secret should match: %+v %v", res, err)
	}
	if _, err := a.ConfigDiff(ctx, ConfigRef{Service: "service:heartie/admin", Env: "pro"}); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("ambiguous config: %v", err)
	}
	// A live secret that differs from the local one is redacted in the diff.
	f := fakeConfigs(t, strings.Replace(tomlLive, "pw123", "rotated-elsewhere", 1))
	_ = f
	res, err = a.ConfigDiff(ctx, ref)
	if err != nil || res.InSync || strings.Contains(res.Diff, "rotated-elsewhere") || strings.Contains(res.Diff, "pw123") || !strings.Contains(res.Diff, "***") {
		t.Fatalf("diff leaks or misses: %+v %v", res, err)
	}
}

func TestConfigPullAndPushNginx(t *testing.T) {
	f := fakeConfigs(t, tomlLive)
	dir := copyExample(t)
	a := load(t, dir)
	ctx := context.Background()
	ref, _ := ParseConfigRef("service:heartie/admin@pro", "nginx")
	archived := f.nginxLive
	f.nginxLive = strings.Replace(archived, "    location / {", "    gzip on;\n    location / {", 1)
	res, err := a.ConfigDiff(ctx, ref)
	if err != nil || res.InSync || !strings.Contains(res.Diff, "+    gzip on;") || res.Added != 1 {
		t.Fatalf("diff: %+v %v", res, err)
	}
	// Push the archive copy back over the live file.
	res, rec, err := a.ConfigPush(ctx, ref, OpInput{Actor: "ai/claude-code"}, false)
	if err != nil || rec != nil || len(f.saved) != 0 {
		t.Fatalf("plan must not write: %v %v", rec, err)
	}
	res, rec, err = a.ConfigPush(ctx, ref, OpInput{Actor: "ai/claude-code"}, true)
	if err != nil || rec == nil || rec.Action != "config.push" || rec.Targets[0] != "service:heartie/admin@pro" {
		t.Fatalf("push: %+v %+v %v", res, rec, err)
	}
	sum := sha256.Sum256([]byte(f.nginxLive))
	if len(f.saved) != 1 || f.saved[0].Text != archived || f.saved[0].Revision != hex.EncodeToString(sum[:]) {
		t.Fatalf("nginx save: %+v", f.saved)
	}
	// Pull the live change into the archive instead.
	if _, err := a.ConfigPull(ctx, ref); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "products/heartie/configs/admin/pro/heartie-admin.conf"))
	if !strings.Contains(string(b), "gzip on;") {
		t.Fatalf("pulled file:\n%s", b)
	}
	if after := load(t, dir); len(after.Errors) > 0 {
		t.Fatalf("archive invalid after pull/push: %v", after.Errors)
	}
}

func TestConfigPullKeepsPlaceholdersAndRefusesSecrets(t *testing.T) {
	fakeConfigs(t, strings.Replace(tomlLive, "timeout = \"15s\"", "timeout = \"30s\"", 1))
	dir := copyExample(t)
	a := load(t, dir)
	ctx := context.Background()
	ref, _ := ParseConfigRef("heartie/admin@pro", "app")
	if _, err := a.ConfigPull(ctx, ref); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "products/heartie/configs/admin/pro/config.toml"))
	if !strings.Contains(string(b), `timeout = "30s"`) || !strings.Contains(string(b), "${secret:keychain:archive/heartie/api/db-password}") || strings.Contains(string(b), "pw123") {
		t.Fatalf("pulled toml:\n%s", b)
	}
	fakeConfigs(t, tomlLive+"api_token = \"abcdef0123456789\"\n")
	a = load(t, dir)
	if _, err := a.ConfigPull(ctx, ref); err == nil || !strings.Contains(err.Error(), "secret values on lines") {
		t.Fatalf("secret pulled: %v", err)
	}
}

func TestConfigPushConfigMap(t *testing.T) {
	f := fakeConfigs(t, strings.Replace(tomlLive, "timeout = \"15s\"", "timeout = \"99s\"", 1))
	a := load(t, copyExample(t))
	ctx := context.Background()
	ref, _ := ParseConfigRef("heartie/admin@pro", "app")
	_, rec, err := a.ConfigPush(ctx, ref, OpInput{Actor: "human/me"}, true)
	if err != nil || rec.Status != "succeeded" {
		t.Fatalf("push: %+v %v", rec, err)
	}
	cm, _ := f.client.Resource(configMapGVR).Namespace("default").Get(ctx, "heartie-admin-config", metav1.GetOptions{})
	data, _, _ := unstructured.NestedStringMap(cm.Object, "data")
	if data["config.toml"] != tomlLive || data["other"] != "keep" {
		t.Fatalf("configmap data: %v", data)
	}
	for _, s := range rec.Steps {
		if strings.Contains(s.Message, "pw123") {
			t.Fatal("record contains the secret")
		}
	}
	keychainGet = func(string) (string, error) { return "", os.ErrNotExist }
	if _, _, err := a.ConfigPush(ctx, ref, OpInput{Actor: "human/me"}, true); err == nil || !strings.Contains(err.Error(), "do not resolve") {
		t.Fatalf("missing secret: %v", err)
	}
}
