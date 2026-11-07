package workspace

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func appWorkload(kind, name, app string, podSpec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": kind,
		"metadata": map[string]any{"name": name, "namespace": "default"},
		"spec": map[string]any{"template": map[string]any{
			"metadata": map[string]any{"labels": map[string]any{"app": app}},
			"spec":     podSpec,
		}},
	}}
}

func configMapObject(name string, data map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": name, "namespace": "default", "uid": "uid-" + name, "resourceVersion": "41"},
		"data":     data,
	}}
}

func TestConfigMapRefsFollowsEnvAndMountedVolumes(t *testing.T) {
	obj := appWorkload("Deployment", "web", "web", map[string]any{
		"initContainers": []any{map[string]any{"name": "init", "envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "init-env"}}}}},
		"containers": []any{map[string]any{
			"name":         "web",
			"env":          []any{map[string]any{"name": "A", "valueFrom": map[string]any{"configMapKeyRef": map[string]any{"name": "key-ref", "key": "a"}}}, map[string]any{"name": "B", "value": "plain"}},
			"volumeMounts": []any{map[string]any{"name": "conf", "mountPath": "/etc/web"}, map[string]any{"name": "bundle", "mountPath": "/etc/bundle"}},
		}},
		"volumes": []any{
			map[string]any{"name": "conf", "configMap": map[string]any{"name": "web-config"}},
			map[string]any{"name": "bundle", "projected": map[string]any{"sources": []any{map[string]any{"configMap": map[string]any{"name": "projected"}}, map[string]any{"secret": map[string]any{"name": "token"}}}}},
			map[string]any{"name": "unmounted", "configMap": map[string]any{"name": "unused"}},
		},
	})
	refs := configMapRefs(obj)
	sort.Strings(refs)
	if want := []string{"init-env", "key-ref", "projected", "web-config"}; !reflect.DeepEqual(refs, want) {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
}

func TestReadApplicationConfigMapOnlyAllowsReferencedMaps(t *testing.T) {
	mount := func(name string) map[string]any {
		return map[string]any{
			"containers": []any{map[string]any{"name": "app", "volumeMounts": []any{map[string]any{"name": "conf", "mountPath": "/conf"}}}},
			"volumes":    []any{map[string]any{"name": "conf", "configMap": map[string]any{"name": name}}},
		}
	}
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
		{Group: "apps", Version: "v1", Resource: "daemonsets"}:   "DaemonSetList",
	}
	cl := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds,
		appWorkload("Deployment", "thumbspark", "thumbspark", mount("thumbspark-config-toml")),
		appWorkload("StatefulSet", "other", "other", mount("other-config")),
		configMapObject("thumbspark-config-toml", map[string]any{"config.toml": "a = 1\n"}),
		configMapObject("other-config", map[string]any{"x": "y"}),
	)
	cm, err := readApplicationConfigMap(context.Background(), cl, "default", "app=thumbspark", "thumbspark-config-toml")
	if err != nil {
		t.Fatal(err)
	}
	if cm.UID != "uid-thumbspark-config-toml" || cm.ResourceVersion != "41" || cm.Data["config.toml"] != "a = 1\n" {
		t.Fatalf("unexpected ConfigMap: %+v", cm)
	}
	for _, name := range []string{"other-config", "missing"} {
		if _, err := readApplicationConfigMap(context.Background(), cl, "default", "app=thumbspark", name); err == nil || !strings.Contains(err.Error(), "not used by the application") {
			t.Fatalf("%s: expected a reference error, got %v", name, err)
		}
	}
	if _, err := readApplicationConfigMap(context.Background(), cl, "default", "", "thumbspark-config-toml"); err == nil {
		t.Fatal("empty selector accepted")
	}
}
