package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"devopspad/internal/workspace"

	"github.com/pmezard/go-difflib/difflib"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const maxConfigFile = 1 << 20

var (
	configMapGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	nginxPathRE  = regexp.MustCompile(`^/etc/nginx/conf\.d/[a-zA-Z0-9][a-zA-Z0-9._-]*\.conf$`)
)

// nginxSave is replaced in tests.
var nginxSave = workspace.SaveNginx

// ConfigRef names one config of a service in one environment.
type ConfigRef struct {
	Service string // service:<product>/<service>
	Env     string
	Name    string // empty when the service has a single config in that environment
}

// ParseConfigRef accepts service:p/s@env or p/s@env.
func ParseConfigRef(s, name string) (ConfigRef, error) {
	if !strings.HasPrefix(s, "service:") {
		s = "service:" + s
	}
	kind, n, _, env, err := ParseRef(s)
	if err != nil || kind != "service" || env == "" {
		return ConfigRef{}, fmt.Errorf("%q: use <product>/<service>@<environment>", strings.TrimPrefix(s, "service:"))
	}
	return ConfigRef{Service: "service:" + n, Env: env, Name: name}, nil
}

type configTarget struct {
	svc        *Service
	cfg        Config
	file       string // archive path
	deployment *ResolvedDeployment
}

func (a *Archive) configTarget(ref ConfigRef) (configTarget, error) {
	var t configTarget
	r := a.Get(ref.Service)
	if r == nil {
		return t, fmt.Errorf("%s does not exist%s", ref.Service, a.suggest("service"))
	}
	t.svc = r.Object.(*Service)
	var names []string
	var matches []Config
	for _, c := range t.svc.Spec.Configs {
		if c.Environment != ref.Env {
			continue
		}
		names = append(names, c.Name)
		if ref.Name == "" || c.Name == ref.Name {
			matches = append(matches, c)
		}
	}
	switch {
	case len(names) == 0:
		return t, fmt.Errorf("%s has no configs for environment %s", ref.Service, ref.Env)
	case len(matches) != 1:
		return t, fmt.Errorf("choose a config with --name: %s", strings.Join(names, ", "))
	}
	t.cfg = matches[0]
	t.file = "products/" + t.svc.Metadata.Product + "/" + path.Clean(t.cfg.Source)
	for _, d := range a.resolveDeployments(t.svc) {
		if d.Environment == ref.Env && d.Runtime == "k8s" {
			dd := d
			t.deployment = &dd
			break
		}
	}
	return t, nil
}

func (t configTarget) where() string {
	if t.cfg.Type == "configmap" {
		ns := ""
		if t.deployment != nil {
			ns = t.deployment.Namespace
		}
		return fmt.Sprintf("configmap %s/%s key %s", ns, t.cfg.Target.ConfigMap, t.cfg.Target.Key)
	}
	return t.cfg.Target.Host + ":" + t.cfg.Target.Path
}

type liveConfig struct {
	text     string
	revision string // sha256 for files, resourceVersion for ConfigMaps
	cm       *unstructured.Unstructured
}

// readLive fetches the live content: over SSH for host files, from the
// cluster for ConfigMaps (credentials stay local; a remote kubeconfig goes to
// a temporary directory).
func (a *Archive) readLive(ctx context.Context, t configTarget) (liveConfig, func(context.Context, *unstructured.Unstructured) error, error) {
	if t.cfg.Type != "configmap" {
		host, err := a.HostConnection(t.cfg.Target.Host)
		if err != nil {
			return liveConfig{}, nil, err
		}
		b, err := sshReadFile(ctx, host, t.cfg.Target.Path, maxConfigFile)
		if err != nil {
			return liveConfig{}, nil, fmt.Errorf("read %s: %w", t.where(), err)
		}
		sum := sha256.Sum256(b)
		return liveConfig{text: string(b), revision: hex.EncodeToString(sum[:])}, nil, nil
	}
	if t.deployment == nil || t.deployment.Cluster == "" || t.deployment.Namespace == "" {
		return liveConfig{}, nil, errors.New("configmap configs need a k8s deployment with a cluster and namespace in that environment")
	}
	tmp, err := os.MkdirTemp("", "devopspad-kube-")
	if err != nil {
		return liveConfig{}, nil, err
	}
	c, err := a.ClusterConnection(ctx, t.deployment.Cluster, tmp, true)
	if err != nil {
		os.RemoveAll(tmp)
		return liveConfig{}, nil, err
	}
	cl, closeFn, err := clusterClient(ctx, c)
	if err != nil {
		os.RemoveAll(tmp)
		return liveConfig{}, nil, err
	}
	cms := cl.Resource(configMapGVR).Namespace(t.deployment.Namespace)
	obj, err := cms.Get(ctx, t.cfg.Target.ConfigMap, metav1.GetOptions{})
	if err != nil {
		closeFn()
		os.RemoveAll(tmp)
		return liveConfig{}, nil, fmt.Errorf("read %s: %w", t.where(), err)
	}
	data, _, _ := unstructured.NestedStringMap(obj.Object, "data")
	value, ok := data[t.cfg.Target.Key]
	if !ok {
		closeFn()
		os.RemoveAll(tmp)
		return liveConfig{}, nil, fmt.Errorf("%s has no key %s", t.cfg.Target.ConfigMap, t.cfg.Target.Key)
	}
	update := func(ctx context.Context, next *unstructured.Unstructured) error {
		defer os.RemoveAll(tmp)
		defer closeFn()
		if next == nil {
			return nil
		}
		if _, err := cms.Update(ctx, next.DeepCopy(), metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
			return fmt.Errorf("ConfigMap dry-run failed: %w", err)
		}
		_, err := cms.Update(ctx, next, metav1.UpdateOptions{})
		if apierrors.IsConflict(err) {
			return errors.New("the ConfigMap changed since it was read; run config diff again")
		}
		return err
	}
	return liveConfig{text: value, revision: obj.GetResourceVersion(), cm: obj}, update, nil
}

// placeholders resolves ${secret:...} lines of archive text. It returns the
// rendered text and, per rendered line, the archive line it came from.
func placeholders(text string) (string, map[string]string, []string) {
	lines := strings.Split(text, "\n")
	back := map[string]string{}
	var missing []string
	for i, line := range lines {
		if !placeholderRE.MatchString(line) {
			continue
		}
		ok := true
		rendered := placeholderRE.ReplaceAllStringFunc(line, func(m string) string {
			ref := placeholderRE.FindStringSubmatch(m)[1]
			v, err := ResolveSecret(ref)
			if err != nil {
				ok = false
				missing = append(missing, ref)
				return m
			}
			return v
		})
		if ok {
			back[rendered] = line
			lines[i] = rendered
		}
	}
	return strings.Join(lines, "\n"), back, missing
}

// mask replaces live lines that equal a rendered placeholder line with the
// archive line, so secret values never reach a diff or the archive.
func mask(live string, back map[string]string) string {
	if len(back) == 0 {
		return live
	}
	lines := strings.Split(live, "\n")
	for i, l := range lines {
		if orig, ok := back[l]; ok {
			lines[i] = orig
		}
	}
	return strings.Join(lines, "\n")
}

func unifiedDiff(name, a, b string) (string, int, int) {
	al, bl := difflib.SplitLines(a), difflib.SplitLines(b)
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: al, B: bl, FromFile: "archive/" + name, ToFile: "live/" + name, Context: 3})
	added, removed := 0, 0
	for _, op := range difflib.NewMatcher(al, bl).GetOpCodes() {
		if op.Tag == 'r' || op.Tag == 'd' {
			removed += op.I2 - op.I1
		}
		if op.Tag == 'r' || op.Tag == 'i' {
			added += op.J2 - op.J1
		}
	}
	return text, added, removed
}

// ConfigResult describes a config comparison or change. Diff never contains
// secret values that the archive keeps as placeholders.
type ConfigResult struct {
	Service        string   `json:"service"`
	Environment    string   `json:"environment"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	File           string   `json:"file"`
	Target         string   `json:"target"`
	InSync         bool     `json:"inSync"`
	Diff           string   `json:"diff,omitempty"`
	Added          int      `json:"added"`
	Removed        int      `json:"removed"`
	MissingSecrets []string `json:"missingSecrets,omitempty"`
	Record         string   `json:"record,omitempty"`
}

func (a *Archive) compare(ctx context.Context, ref ConfigRef) (configTarget, liveConfig, func(context.Context, *unstructured.Unstructured) error, ConfigResult, error) {
	t, err := a.configTarget(ref)
	if err != nil {
		return t, liveConfig{}, nil, ConfigResult{}, err
	}
	res := ConfigResult{Service: ref.Service, Environment: ref.Env, Name: t.cfg.Name, Type: t.cfg.Type, File: t.file, Target: t.where()}
	live, update, err := a.readLive(ctx, t)
	if err != nil {
		return t, live, nil, res, err
	}
	archived := string(a.textFiles[t.file])
	_, back, missing := placeholders(archived)
	res.MissingSecrets = missing
	masked := mask(live.text, back)
	res.InSync = masked == archived
	if !res.InSync {
		// Live secrets that do not match the local value are redacted, never shown.
		res.Diff, res.Added, res.Removed = unifiedDiff(path.Base(t.file), archived, redact(masked))
	}
	return t, live, update, res, nil
}

// ConfigDiff compares the archive copy with the live config.
func (a *Archive) ConfigDiff(ctx context.Context, ref ConfigRef) (ConfigResult, error) {
	_, _, update, res, err := a.compare(ctx, ref)
	if update != nil {
		_ = update(ctx, nil)
	}
	return res, err
}

// ConfigPull writes the live config into the archive. Lines matching a
// rendered placeholder keep the placeholder; any other secret is refused.
func (a *Archive) ConfigPull(ctx context.Context, ref ConfigRef) (ConfigResult, error) {
	t, live, update, res, err := a.compare(ctx, ref)
	if update != nil {
		_ = update(ctx, nil)
	}
	if err != nil || res.InSync {
		return res, err
	}
	_, back, _ := placeholders(string(a.textFiles[t.file]))
	text := mask(live.text, back)
	if lines := secretLines(text); len(lines) > 0 {
		return res, fmt.Errorf("the live config has secret values on lines %v; replace them with ${secret:keychain:<account>} in the archive copy first (store values with devopspadcli secrets set), then pull again", lines)
	}
	full := filepath.Join(a.Dir, filepath.FromSlash(t.file))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return res, err
	}
	return res, writeAtomic(full, []byte(text), 0o644)
}

// secretLines returns line numbers that look like secrets.
func secretLines(text string) []int {
	var out []int
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, allowMarker) {
			continue
		}
		hit := assignRE.MatchString(placeholderRE.ReplaceAllString(line, ""))
		for _, p := range secretPatterns {
			hit = hit || p.re.MatchString(line)
		}
		if hit {
			out = append(out, i+1)
		}
	}
	return out
}

// ConfigPush writes the archive copy (with placeholders rendered locally) to
// the live config: host Nginx files through the remote helper (revision check,
// nginx -t, reload, restore on failure), ConfigMap keys with a dry-run and a
// resourceVersion check. apply=false only compares. Changes are recorded in
// the archive without contents.
func (a *Archive) ConfigPush(ctx context.Context, ref ConfigRef, in OpInput, apply bool) (ConfigResult, *ActionRecord, error) {
	t, live, update, res, err := a.compare(ctx, ref)
	finish := func() {
		if update != nil {
			_ = update(ctx, nil)
		}
	}
	if err != nil {
		finish()
		return res, nil, err
	}
	if t.cfg.Type != "nginx" && t.cfg.Type != "configmap" {
		finish()
		return res, nil, fmt.Errorf("push supports nginx and configmap configs; %s configs are pull and diff only", t.cfg.Type)
	}
	if t.cfg.Type == "nginx" && !nginxPathRE.MatchString(t.cfg.Target.Path) {
		finish()
		return res, nil, errors.New("nginx push only writes .conf files under /etc/nginx/conf.d/")
	}
	if len(res.MissingSecrets) > 0 {
		finish()
		return res, nil, fmt.Errorf("these secret references do not resolve on this machine: %s", strings.Join(res.MissingSecrets, ", "))
	}
	if res.InSync {
		finish()
		return res, nil, nil
	}
	if !apply {
		finish()
		return res, nil, nil
	}
	rendered, _, _ := placeholders(string(a.textFiles[t.file]))
	start := time.Now()
	var applyErr error
	switch t.cfg.Type {
	case "nginx":
		finish()
		host, err := a.HostConnection(t.cfg.Target.Host)
		if err != nil {
			return res, nil, err
		}
		_, applyErr = nginxSave(ctx, host, t.cfg.Target.Path, workspace.NginxUpdate{Revision: live.revision, Text: rendered})
	case "configmap":
		next := live.cm.DeepCopy()
		data, _, _ := unstructured.NestedStringMap(next.Object, "data")
		data[t.cfg.Target.Key] = rendered
		_ = unstructured.SetNestedStringMap(next.Object, data, "data")
		next.SetManagedFields(nil)
		applyErr = update(ctx, next)
	}
	sum := sha256.Sum256([]byte(rendered))
	steps := []Step{
		{Name: "compare", Status: "succeeded", Message: fmt.Sprintf("%d line(s) added, %d removed (archive → live)", res.Removed, res.Added)},
		{Name: "write " + t.where(), Status: statusOf(applyErr), Message: "sha256 " + hex.EncodeToString(sum[:])[:12]},
	}
	if applyErr != nil {
		steps[1].Message = applyErr.Error()
	}
	rec, recErr := a.appendRecord(in, "config.push", []string{ref.Service + "@" + ref.Env}, statusOf(applyErr), "push config "+t.cfg.Name+" to "+t.where(), steps, start)
	if rec != nil {
		res.Record = rec.File()
	}
	if recErr != nil {
		return res, rec, errors.Join(applyErr, recErr)
	}
	return res, rec, applyErr
}

// redact hides values on lines that look like secrets.
func redact(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.Contains(line, allowMarker) || placeholderRE.MatchString(line) {
			continue
		}
		if m := assignRE.FindStringSubmatchIndex(line); m != nil {
			line = line[:m[8]] + "***" + line[m[9]:]
		}
		for _, p := range secretPatterns {
			line = p.re.ReplaceAllString(line, "***")
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
