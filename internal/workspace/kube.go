package workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
)

type KubeContext struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	ExecAuth  bool   `json:"execAuth"`
}
type KubeconfigInfo struct {
	CurrentContext string        `json:"currentContext"`
	Contexts       []KubeContext `json:"contexts"`
}

// Read metadata only. Never run auth plugins or return credentials to the webview.
func ReadKubeconfig(path string) (KubeconfigInfo, error) {
	out := KubeconfigInfo{Contexts: []KubeContext{}}
	cfg, e := clientcmd.LoadFromFile(path)
	if e != nil {
		return out, errors.New("Cannot read kubeconfig file")
	}
	out.CurrentContext = cfg.CurrentContext
	for name, c := range cfg.Contexts {
		if c == nil {
			continue
		}
		ns := c.Namespace
		if ns == "" {
			ns = "default"
		}
		auth := cfg.AuthInfos[c.AuthInfo]
		out.Contexts = append(out.Contexts, KubeContext{Name: name, Cluster: c.Cluster, Namespace: ns, ExecAuth: auth != nil && auth.Exec != nil})
	}
	sort.Slice(out.Contexts, func(i, j int) bool { return out.Contexts[i].Name < out.Contexts[j].Name })
	return out, nil
}
func listNamespaces(ctx context.Context, c Connection) ([]string, error) {
	cl, e := kubeClient(c)
	if e != nil {
		return nil, e
	}
	names := []string{"*"}
	cursor := ""
	for {
		r, e := cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).List(ctx, metav1.ListOptions{Limit: 500, Continue: cursor})
		if apierrors.IsForbidden(e) {
			return nil, fmt.Errorf("Namespace discovery is not permitted. You can still use the namespace saved in this connection. Server response: %w", e)
		}
		if e != nil {
			return nil, e
		}
		for _, item := range r.Items {
			names = append(names, item.GetName())
		}
		cursor = r.GetContinue()
		if cursor == "" {
			break
		}
		if len(names) > 10000 {
			return nil, errors.New("Namespace list exceeds 10000 entries; enter a namespace in the connection settings")
		}
	}
	sort.Strings(names)
	return names, nil
}
