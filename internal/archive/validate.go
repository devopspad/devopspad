package archive

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/labels"
)

var (
	labelNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	fqdnRE      = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
	labelKeyRE  = regexp.MustCompile(`^([a-z0-9.-]{1,253}/)?[a-z0-9]([a-z0-9._-]{0,61}[a-z0-9])?$`)
	labelValRE  = regexp.MustCompile(`^([A-Za-z0-9]([A-Za-z0-9._-]{0,61}[A-Za-z0-9])?)?$`)
	refRE       = regexp.MustCompile(`^([a-z]+):([^#@\s]+)(#[^#@\s]+)?(@[a-z0-9-]+)?$`)
)

// schemaTag is the parsed `schema:"..."` struct tag. pattern= and desc= consume
// the rest of the tag, so they come last.
type schemaTag struct {
	Required  bool
	Enum      []string
	Pattern   string
	Refs      []string
	SecretRef bool
	Format    string
	Min, Max  *float64
	Const     string
	Desc      string
}

func parseTag(tag string) schemaTag {
	var t schemaTag
	for tag != "" {
		var part string
		if strings.HasPrefix(tag, "pattern=") || strings.HasPrefix(tag, "desc=") {
			part, tag = tag, ""
		} else if i := strings.Index(tag, ","); i >= 0 {
			part, tag = tag[:i], tag[i+1:]
		} else {
			part, tag = tag, ""
		}
		k, v, _ := strings.Cut(part, "=")
		switch k {
		case "required":
			t.Required = true
		case "secretref":
			t.SecretRef = true
		case "enum":
			t.Enum = strings.Split(v, "|")
		case "ref":
			t.Refs = strings.Split(v, "|")
		case "pattern":
			t.Pattern = v
		case "format":
			t.Format = v
		case "const":
			t.Const = v
		case "desc":
			t.Desc = v
		case "min", "max":
			var f float64
			fmt.Sscan(v, &f)
			if k == "min" {
				t.Min = &f
			} else {
				t.Max = &f
			}
		}
	}
	return t
}

func yamlName(f reflect.StructField) (string, bool) {
	tag := f.Tag.Get("yaml")
	name, opts, _ := strings.Cut(tag, ",")
	if strings.Contains(opts, "inline") {
		return "", true
	}
	if name == "" {
		name = strings.ToLower(f.Name[:1]) + f.Name[1:]
	}
	return name, false
}

func specOf(obj Object) any { return reflect.ValueOf(obj).Elem().FieldByName("Spec").Interface() }

type pendingRef struct {
	use     RefUse
	allowed []string
	line    int
}

// walkStruct applies the schema tags of v. from is the owning resource ref
// (empty for the root file); p is the YAML path of v.
func (a *Archive) walkStruct(file string, node *yaml.Node, from, p string, v any) {
	a.walkValue(file, node, from, p, reflect.ValueOf(v), schemaTag{})
}

func (a *Archive) walkValue(file string, node *yaml.Node, from, p string, v reflect.Value, t schemaTag) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			a.walkValue(file, node, from, p, v.Elem(), t)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			name, inline := yamlName(f)
			fp := p
			if !inline {
				fp = joinPath(p, name)
			}
			ft := parseTag(f.Tag.Get("schema"))
			fv := v.Field(i)
			if ft.Required && fv.IsZero() {
				a.errorf(file, lineOf(node, p), fp, "field.required", "%s is required", name)
				continue
			}
			a.walkValue(file, node, from, fp, fv, ft)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			a.walkValue(file, node, from, fmt.Sprintf("%s[%d]", p, i), v.Index(i), t)
		}
	case reflect.String:
		a.checkString(file, node, from, p, v.String(), t)
	case reflect.Int, reflect.Float64:
		if v.IsZero() {
			return // unset optional number; required ones are checked by the caller
		}
		n := v.Convert(reflect.TypeOf(float64(0))).Float()
		if (t.Min != nil && n < *t.Min) || (t.Max != nil && n > *t.Max) {
			a.errorf(file, lineOf(node, p), p, "field.range", "value %v is out of range [%v, %v]", n, deref(t.Min), deref(t.Max))
		}
	}
}

func deref(f *float64) any {
	if f == nil {
		return "-"
	}
	return *f
}

func joinPath(p, name string) string {
	if p == "" {
		return name
	}
	return p + "." + name
}

func (a *Archive) checkString(file string, node *yaml.Node, from, p, s string, t schemaTag) {
	if s == "" {
		return
	}
	line := lineOf(node, p)
	if t.Const != "" && s != t.Const {
		a.errorf(file, line, p, "field.const", "must be %s", t.Const)
	}
	if len(t.Enum) > 0 && !contains(t.Enum, s) {
		a.errorf(file, line, p, "field.enum", "%q is not allowed; use one of: %s", s, strings.Join(t.Enum, ", "))
	}
	if t.Pattern != "" && !regexp.MustCompile(t.Pattern).MatchString(s) {
		a.errorf(file, line, p, "field.pattern", "%q does not match %s", s, t.Pattern)
	}
	switch t.Format {
	case "date":
		if !validDate(s) {
			a.errorf(file, line, p, "format.date", "%q is not a date; use YYYY-MM-DD", s)
		}
	case "url":
		if u, err := url.Parse(s); err != nil || u.Scheme == "" || u.Host == "" {
			a.errorf(file, line, p, "format.url", "%q is not an absolute URL", s)
		}
	}
	if t.SecretRef {
		if err := ParseSecretRef(s); err != nil {
			a.errorf(file, line, p, "secret.ref", "%v", err)
		} else if from != "" {
			a.Secrets = append(a.Secrets, SecretUse{From: from, Field: p, Ref: s, File: file})
		}
	}
	if len(t.Refs) > 0 && from != "" {
		kind, name, part, env, err := ParseRef(s)
		switch {
		case err != nil:
			a.errorf(file, line, p, "ref.syntax", "%v", err)
		case env != "":
			a.errorf(file, line, p, "ref.syntax", "@environment is only used in records and commands, not in resource files")
		default:
			a.pending = append(a.pending, pendingRef{use: RefUse{From: from, Field: p, To: kind + ":" + name, Part: part, File: file}, allowed: t.Refs, line: line})
		}
	}
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ParseRef splits kind:name[#part][@env]. Service names are product/service.
func ParseRef(s string) (kind, name, part, env string, err error) {
	m := refRE.FindStringSubmatch(s)
	if m == nil {
		return "", "", "", "", fmt.Errorf("%q is not a reference; use <kind>:<name>, e.g. host:hk-1 or service:heartie/admin", s)
	}
	kind, name, part, env = m[1], m[2], strings.TrimPrefix(m[3], "#"), strings.TrimPrefix(m[4], "@")
	k, ok := kindByRef(kind)
	if !ok || k.Ref != kind {
		return "", "", "", "", fmt.Errorf("%q has unknown kind %q; kinds: provider, host, cluster, datastore, registry, domain, product, service", s, kind)
	}
	switch kind {
	case "service":
		prod, svc, ok := strings.Cut(name, "/")
		if !ok || !labelNameRE.MatchString(prod) || !labelNameRE.MatchString(svc) {
			return "", "", "", "", fmt.Errorf("%q: service references are service:<product>/<service>", s)
		}
	case "domain":
		if !fqdnRE.MatchString(name) {
			return "", "", "", "", fmt.Errorf("%q: domain references use the lowercase domain name", s)
		}
	default:
		if !labelNameRE.MatchString(name) {
			return "", "", "", "", fmt.Errorf("%q: names are lowercase letters, digits and hyphens", s)
		}
	}
	if part != "" && kind != "datastore" && kind != "registry" {
		return "", "", "", "", fmt.Errorf("%q: #part is only used with datastore and registry references", s)
	}
	return kind, name, part, env, nil
}

func (a *Archive) checkName(file, name string, domain bool) {
	if domain {
		if !fqdnRE.MatchString(name) {
			a.errorf(file, 0, "metadata.name", "name.format", "domain names are lowercase FQDNs such as example.com")
		}
		return
	}
	if !labelNameRE.MatchString(name) {
		a.errorf(file, 0, "metadata.name", "name.format", "%q must be lowercase letters, digits and hyphens (max 63)", name)
	}
}

func (a *Archive) checkLabels(file string, node *yaml.Node, l map[string]string) {
	for k, v := range l {
		if !labelKeyRE.MatchString(k) || !labelValRE.MatchString(v) {
			a.errorf(file, lineOf(node, "metadata.labels"), "metadata.labels."+k, "label.format", "label %s=%s is invalid; keys and values are short lowercase identifiers", k, v)
		}
	}
}

func (a *Archive) line(r *Resource, p string) int { return lineOf(r.node, p) }

// checkKinds applies the rules that depend on several fields of one resource.
func (a *Archive) checkKinds() {
	for _, r := range a.Resources {
		switch o := r.Object.(type) {
		case *Host:
			if s := o.Spec.SSH; s != nil {
				if s.Auth == "key" && s.KeyRef == "" {
					a.errorf(r.File, a.line(r, "spec.ssh"), "spec.ssh.keyRef", "field.required", "auth: key needs keyRef, e.g. file:~/.ssh/id_ed25519")
				}
				if s.Auth == "password" && s.PasswordRef == "" {
					a.errorf(r.File, a.line(r, "spec.ssh"), "spec.ssh.passwordRef", "field.required", "auth: password needs passwordRef, e.g. keychain:archive/hosts/%s/ssh", r.Name)
				}
			}
		case *Cluster:
			k := o.Spec.Kubeconfig
			if k.Source == "remote" && (k.Host == "" || !strings.HasPrefix(k.Path, "/")) {
				a.errorf(r.File, a.line(r, "spec.kubeconfig"), "spec.kubeconfig", "kubeconfig.remote", "source: remote needs host and an absolute path, e.g. /etc/rancher/k3s/k3s.yaml")
			}
			if k.Source == "local" && !strings.HasPrefix(k.PathRef, "file:") {
				a.errorf(r.File, a.line(r, "spec.kubeconfig"), "spec.kubeconfig.pathRef", "kubeconfig.local", "source: local needs pathRef: file:<path>")
			}
		case *Datastore:
			rt := o.Spec.Runtime
			n := 0
			for _, s := range []string{rt.Host, rt.Cluster, rt.Provider} {
				if s != "" {
					n++
				}
			}
			if n > 1 {
				a.errorf(r.File, a.line(r, "spec.runtime"), "spec.runtime", "datastore.runtime", "set only one of runtime.host, runtime.cluster or runtime.provider")
			}
			if rt.Namespace != "" && rt.Cluster == "" {
				a.errorf(r.File, a.line(r, "spec.runtime.namespace"), "spec.runtime.namespace", "datastore.runtime", "runtime.namespace needs runtime.cluster")
			}
			a.checkUnique(r, "spec.parts", o.Spec.Parts)
		case *Domain:
			for i, rec := range o.Spec.Records {
				if (rec.Value == "") == (rec.Target == "") {
					p := fmt.Sprintf("spec.records[%d]", i)
					a.errorf(r.File, a.line(r, p), p, "domain.record", "set exactly one of value or target")
				}
			}
		case *Product:
			names := make([]string, 0, len(o.Spec.Environments))
			for i, e := range o.Spec.Environments {
				p := fmt.Sprintf("spec.environments[%d]", i)
				if !labelNameRE.MatchString(e.Name) {
					a.errorf(r.File, a.line(r, p+".name"), p+".name", "name.format", "environment names are lowercase letters, digits and hyphens")
				}
				if e.Namespace != "" && !labelNameRE.MatchString(e.Namespace) {
					a.errorf(r.File, a.line(r, p+".namespace"), p+".namespace", "name.format", "namespace %q is not a valid Kubernetes namespace", e.Namespace)
				}
				names = append(names, e.Name)
			}
			a.checkUnique(r, "spec.environments", names)
		case *Service:
			a.checkService(r, o)
		}
	}
}

func (a *Archive) checkUnique(r *Resource, p string, list []string) {
	seen := map[string]bool{}
	for _, s := range list {
		if seen[s] {
			a.errorf(r.File, a.line(r, p), p, "field.duplicate", "%q appears more than once", s)
		}
		seen[s] = true
	}
}

func (a *Archive) environment(product, name string) (*Environment, []string) {
	pr, ok := a.byRef["product:"+product]
	if !ok {
		return nil, nil
	}
	var names []string
	for i, e := range pr.Object.(*Product).Spec.Environments {
		if e.Name == name {
			return &pr.Object.(*Product).Spec.Environments[i], nil
		}
		names = append(names, e.Name)
	}
	return nil, names
}

func (a *Archive) checkService(r *Resource, s *Service) {
	product := s.Metadata.Product
	envErr := func(p, env string) bool {
		e, names := a.environment(product, env)
		if e == nil && env != "" {
			a.errorf(r.File, a.line(r, p), p, "ref.environment", "environment %q is not defined in product:%s; defined: %s", env, product, listOrNone(names))
		}
		return e != nil
	}
	for i, d := range s.Spec.Deployments {
		p := fmt.Sprintf("spec.deployments[%d]", i)
		if !envErr(p+".environment", d.Environment) {
			continue
		}
		env, _ := a.environment(product, d.Environment)
		switch d.Runtime {
		case "k8s":
			if d.Cluster == "" && env.Cluster == "" {
				a.errorf(r.File, a.line(r, p), p+".cluster", "deployment.k8s", "runtime k8s needs a cluster on the deployment or on environment %s", d.Environment)
			}
			if d.Namespace == "" && env.Namespace == "" {
				a.errorf(r.File, a.line(r, p), p+".namespace", "deployment.k8s", "runtime k8s needs a namespace on the deployment or on environment %s", d.Environment)
			}
			if d.Selector == "" && d.Workload == nil {
				a.errorf(r.File, a.line(r, p), p, "deployment.k8s", "runtime k8s needs a selector or a workload")
			}
			if d.Selector != "" {
				if _, err := labels.Parse(d.Selector); err != nil {
					a.errorf(r.File, a.line(r, p+".selector"), p+".selector", "deployment.selector", "invalid label selector: %v", err)
				}
			}
		case "host":
			if len(d.Hosts) == 0 && len(env.Hosts) == 0 {
				a.errorf(r.File, a.line(r, p), p+".hosts", "deployment.host", "runtime host needs hosts on the deployment or on environment %s", d.Environment)
			}
		}
	}
	seen := map[string]bool{}
	for i, c := range s.Spec.Configs {
		p := fmt.Sprintf("spec.configs[%d]", i)
		envErr(p+".environment", c.Environment)
		key := c.Environment + "/" + c.Name
		if seen[key] {
			a.errorf(r.File, a.line(r, p+".name"), p+".name", "field.duplicate", "config %s is defined twice for environment %s", c.Name, c.Environment)
		}
		seen[key] = true
		a.checkProductFile(r, product, p+".source", c.Source)
		switch c.Type {
		case "configmap":
			if c.Target.ConfigMap == "" || c.Target.Key == "" {
				a.errorf(r.File, a.line(r, p), p+".target", "config.target", "configmap configs need target.configMap and target.key")
			}
		default:
			if c.Target.Host == "" || !strings.HasPrefix(c.Target.Path, "/") {
				a.errorf(r.File, a.line(r, p), p+".target", "config.target", "%s configs need target.host and an absolute target.path", c.Type)
			}
		}
	}
	if s.Spec.Release != nil {
		if s.Spec.Release.Notes != "" {
			a.checkProductFile(r, product, "spec.release.notes", s.Spec.Release.Notes)
		}
		if s.Spec.Release.Method == "devopspad-spec" && s.Spec.Release.Spec == "" {
			a.errorf(r.File, a.line(r, "spec.release"), "spec.release.spec", "field.required", "method devopspad-spec needs spec: <path of service.release.yaml in the code repository>")
		}
	}
	for i, dep := range s.Spec.DependsOn {
		if dep == r.Ref {
			p := fmt.Sprintf("spec.dependsOn[%d]", i)
			a.errorf(r.File, a.line(r, p), p, "service.cycle", "a service cannot depend on itself")
		}
	}
}

// checkProductFile requires rel to name an existing file inside the product directory.
func (a *Archive) checkProductFile(r *Resource, product, p, rel string) {
	clean := path.Clean(rel)
	if rel == "" || path.IsAbs(rel) || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(rel) {
		a.errorf(r.File, a.line(r, p), p, "file.path", "%q must be a path relative to products/%s/", rel, product)
		return
	}
	full := "products/" + product + "/" + clean
	if _, ok := a.textFiles[full]; !ok {
		a.errorf(r.File, a.line(r, p), p, "file.missing", "%s does not exist (configs/ and notes/ files only)", full)
	}
}

func listOrNone(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

func (a *Archive) resolveRefs() {
	for _, n := range a.pending {
		u := n.use
		kind, _, _ := strings.Cut(u.To, ":")
		target := a.byRef[u.To]
		switch {
		case !contains(n.allowed, kind):
			a.errorf(u.File, n.line, u.Field, "ref.kind", "%s is a %s reference; this field accepts: %s", u.To, kind, strings.Join(n.allowed, ", "))
			continue
		case target == nil:
			a.errorf(u.File, n.line, u.Field, "ref.unresolved", "%s does not exist%s", u.To, a.suggest(kind))
			continue
		case u.Part != "" && kind == "datastore" && !contains(target.Object.(*Datastore).Spec.Parts, u.Part):
			a.errorf(u.File, n.line, u.Field, "ref.part", "%s has no part %q; parts: %s", u.To, u.Part, listOrNone(target.Object.(*Datastore).Spec.Parts))
			continue
		}
		a.Uses = append(a.Uses, u)
	}
	a.pending = nil
	for _, note := range a.Notes {
		for i, ref := range note.Meta.Refs {
			kind, name, _, _, err := ParseRef(ref)
			if err != nil {
				a.errorf(note.File, 0, fmt.Sprintf("refs[%d]", i), "ref.syntax", "%v", err)
			} else if a.byRef[kind+":"+name] == nil {
				a.errorf(note.File, 0, fmt.Sprintf("refs[%d]", i), "ref.unresolved", "%s does not exist%s", kind+":"+name, a.suggest(kind))
			}
		}
	}
	for _, rec := range a.Records {
		for _, ref := range rec.Targets {
			kind, name, _, _, err := ParseRef(ref)
			if err == nil && a.byRef[kind+":"+name] == nil {
				a.warnf(rec.file, 0, "targets", "record.target_unresolved", "%s no longer exists", ref)
			}
		}
	}
}

func (a *Archive) suggest(kind string) string {
	var names []string
	for _, r := range a.Resources {
		if strings.HasPrefix(r.Ref, kind+":") {
			names = append(names, r.Ref)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("; no %s resources exist yet (devopspadcli new %s <name>)", kind, kind)
	}
	sort.Strings(names)
	if len(names) > 10 {
		names = append(names[:10], "…")
	}
	return "; existing: " + strings.Join(names, ", ")
}

func (a *Archive) checkServiceCycles() {
	deps := map[string][]string{}
	for _, u := range a.Uses {
		if strings.HasPrefix(u.From, "service:") && strings.HasPrefix(u.To, "service:") && strings.HasPrefix(u.Field, "spec.dependsOn") {
			deps[u.From] = append(deps[u.From], u.To)
		}
	}
	state := map[string]int{}
	var visit func(string, []string) bool
	visit = func(s string, stack []string) bool {
		switch state[s] {
		case 1:
			r := a.byRef[stack[0]]
			a.errorf(r.File, a.line(r, "spec.dependsOn"), "spec.dependsOn", "service.cycle", "dependency cycle: %s", strings.Join(append(stack, s), " → "))
			return true
		case 2:
			return false
		}
		state[s] = 1
		for _, d := range deps[s] {
			if visit(d, append(stack, s)) {
				return true
			}
		}
		state[s] = 2
		return false
	}
	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if state[k] == 0 {
			visit(k, nil)
		}
	}
}
