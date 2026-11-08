package release

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// EnvSecretRef sets an environment variable from one Secret key.
type EnvSecretRef struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
	Key    string `json:"key"`
}

// WorkloadPatch is the whitelist of Deployment fields DevOpsPad changes besides
// images and replicas: pull secrets and Secret-backed environment. Literal
// environment values are never written or replaced.
type WorkloadPatch struct {
	Container              string         `json:"container,omitempty"`
	AddImagePullSecrets    []string       `json:"addImagePullSecrets,omitempty"`
	RemoveImagePullSecrets []string       `json:"removeImagePullSecrets,omitempty"`
	AddEnvFromSecrets      []string       `json:"addEnvFromSecrets,omitempty"`
	RemoveEnvFromSecrets   []string       `json:"removeEnvFromSecrets,omitempty"`
	SetEnvSecretRefs       []EnvSecretRef `json:"setEnvSecretRefs,omitempty"`
	RemoveEnv              []string       `json:"removeEnv,omitempty"`
}

var (
	k8sNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)
	envNameRE = regexp.MustCompile(`^[-._a-zA-Z][-._a-zA-Z0-9]*$`)
	keyRE     = regexp.MustCompile(`^[-._a-zA-Z0-9]{1,253}$`)
)

func (w WorkloadPatch) containerScoped() bool {
	return len(w.AddEnvFromSecrets)+len(w.RemoveEnvFromSecrets)+len(w.SetEnvSecretRefs)+len(w.RemoveEnv) > 0
}

func (w WorkloadPatch) validate() error {
	names := append(append(append([]string{}, w.AddImagePullSecrets...), w.RemoveImagePullSecrets...), append(w.AddEnvFromSecrets, w.RemoveEnvFromSecrets...)...)
	for _, n := range names {
		if !k8sNameRE.MatchString(n) {
			return fmt.Errorf("%q is not a valid Secret name", n)
		}
	}
	for _, e := range w.SetEnvSecretRefs {
		if !envNameRE.MatchString(e.Name) || !k8sNameRE.MatchString(e.Secret) || !keyRE.MatchString(e.Key) {
			return fmt.Errorf("invalid env secret reference %s=%s:%s", e.Name, e.Secret, e.Key)
		}
	}
	for _, n := range w.RemoveEnv {
		if !envNameRE.MatchString(n) {
			return fmt.Errorf("invalid env name %q", n)
		}
	}
	if w.containerScoped() && w.Container == "" {
		return errors.New("envFrom and env changes need --container")
	}
	if len(names)+len(w.SetEnvSecretRefs)+len(w.RemoveEnv) == 0 {
		return errors.New("nothing to change")
	}
	return nil
}

// PrepareDesktopPatch plans a whitelisted change with the same dry-run, lease,
// version checks and rollout wait as image releases. plan.Changes lists what
// will change in words, for review and the record.
func PrepareDesktopPatch(ctx context.Context, cl dynamic.Interface, namespace, workload, selector string, patch WorkloadPatch) (DesktopPlan, error) {
	if err := patch.validate(); err != nil {
		return DesktopPlan{}, err
	}
	return prepareDesktop(ctx, cl, namespace, workload, selector, "patch", func(obj *unstructured.Unstructured, p *DesktopPlan) error {
		p.Container = patch.Container
		spec, _, _ := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
		changes := []string{}
		// imagePullSecrets
		pulls, _ := spec["imagePullSecrets"].([]any)
		has := func(list []any, name string) int {
			for i, raw := range list {
				if m, ok := raw.(map[string]any); ok && m["name"] == name {
					return i
				}
			}
			return -1
		}
		for _, n := range patch.AddImagePullSecrets {
			if has(pulls, n) < 0 {
				pulls = append(pulls, map[string]any{"name": n})
				changes = append(changes, "add imagePullSecret "+n)
			}
		}
		for _, n := range patch.RemoveImagePullSecrets {
			if i := has(pulls, n); i >= 0 {
				pulls = append(pulls[:i], pulls[i+1:]...)
				changes = append(changes, "remove imagePullSecret "+n)
			}
		}
		if len(pulls) > 0 {
			spec["imagePullSecrets"] = pulls
		} else {
			delete(spec, "imagePullSecrets")
		}
		if patch.containerScoped() {
			containers, _ := spec["containers"].([]any)
			var c map[string]any
			for _, raw := range containers {
				if m, ok := raw.(map[string]any); ok && m["name"] == patch.Container {
					c = m
				}
			}
			if c == nil {
				return fmt.Errorf("container %s not found", patch.Container)
			}
			envFrom, _ := c["envFrom"].([]any)
			secretRef := func(i int) string {
				m, _ := envFrom[i].(map[string]any)
				ref, _ := m["secretRef"].(map[string]any)
				name, _ := ref["name"].(string)
				return name
			}
			findFrom := func(name string) int {
				for i := range envFrom {
					if secretRef(i) == name {
						return i
					}
				}
				return -1
			}
			for _, n := range patch.AddEnvFromSecrets {
				if findFrom(n) < 0 {
					envFrom = append(envFrom, map[string]any{"secretRef": map[string]any{"name": n}})
					changes = append(changes, "add envFrom secret "+n+" to "+patch.Container)
				}
			}
			for _, n := range patch.RemoveEnvFromSecrets {
				if i := findFrom(n); i >= 0 {
					envFrom = append(envFrom[:i], envFrom[i+1:]...)
					changes = append(changes, "remove envFrom secret "+n+" from "+patch.Container)
				}
			}
			if len(envFrom) > 0 {
				c["envFrom"] = envFrom
			} else {
				delete(c, "envFrom")
			}
			env, _ := c["env"].([]any)
			findEnv := func(name string) int {
				for i, raw := range env {
					if m, ok := raw.(map[string]any); ok && m["name"] == name {
						return i
					}
				}
				return -1
			}
			isSecretRef := func(i int) bool {
				m, _ := env[i].(map[string]any)
				from, _ := m["valueFrom"].(map[string]any)
				_, ok := from["secretKeyRef"]
				return ok && m["value"] == nil
			}
			for _, e := range patch.SetEnvSecretRefs {
				item := map[string]any{"name": e.Name, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": e.Secret, "key": e.Key}}}
				if i := findEnv(e.Name); i >= 0 {
					if !isSecretRef(i) {
						return fmt.Errorf("env %s in %s has a literal value or another source; DevOpsPad only replaces secretKeyRef entries", e.Name, patch.Container)
					}
					env[i] = item
				} else {
					env = append(env, item)
				}
				changes = append(changes, fmt.Sprintf("set env %s from secret %s key %s in %s", e.Name, e.Secret, e.Key, patch.Container))
			}
			for _, n := range patch.RemoveEnv {
				i := findEnv(n)
				if i < 0 {
					continue
				}
				if !isSecretRef(i) {
					return fmt.Errorf("env %s in %s is not a secretKeyRef; DevOpsPad only removes secretKeyRef entries", n, patch.Container)
				}
				env = append(env[:i], env[i+1:]...)
				changes = append(changes, "remove env "+n+" from "+patch.Container)
			}
			if len(env) > 0 {
				c["env"] = env
			} else {
				delete(c, "env")
			}
			spec["containers"] = containers
		}
		if len(changes) == 0 {
			return errors.New("the Deployment already has these settings")
		}
		p.Changes = changes
		return unstructured.SetNestedMap(obj.Object, spec, "spec", "template", "spec")
	})
}

// ChangeSummary joins plan changes for display.
func ChangeSummary(p DesktopPlan) string { return strings.Join(p.Changes, "; ") }
