package archive

import (
	"fmt"
	"sort"
	"strings"

	"devopspad/internal/ops"

	"gopkg.in/yaml.v3"
)

// ProjectFromArchive derives the desktop project mapping (environments and
// applications used by Infra, releases and the CLI) from the archive, so the
// archive is the single source. Environment names are <product>-<env> and
// application names <product>-<service>; connections are cluster names, as
// connect sync creates them.
func (a *Archive) ProjectFromArchive(name string) (ops.Project, []string) {
	p := ops.Project{APIVersion: "devopspad.project/v1", Name: name, Environments: []ops.Environment{}, Applications: []ops.Application{}}
	var notes []string
	envs := map[string]bool{}
	for _, r := range a.Resources {
		s, ok := r.Object.(*Service)
		if !ok {
			continue
		}
		product := s.Metadata.Product
		app := ops.Application{Name: product + "-" + s.Metadata.Name}
		for _, d := range a.resolveDeployments(s) {
			if d.Runtime != "k8s" || d.Cluster == "" || d.Namespace == "" {
				continue
			}
			env := ops.Environment{Name: product + "-" + d.Environment, Connection: strings.TrimPrefix(d.Cluster, "cluster:"), Namespace: d.Namespace}
			if !ops.ValidName(env.Name) {
				notes = append(notes, fmt.Sprintf("%s: environment name %s is too long or invalid; skipped", r.Ref, env.Name))
				continue
			}
			if !envs[env.Name] {
				envs[env.Name] = true
				p.Environments = append(p.Environments, env)
			}
			selector := d.Selector
			if selector == "" {
				notes = append(notes, fmt.Sprintf("%s@%s has no selector; add spec.deployments[].selector to use it in Infra", r.Ref, d.Environment))
				continue
			}
			if app.Selector == "" {
				app.Selector = selector
			} else if app.Selector != selector {
				notes = append(notes, fmt.Sprintf("%s uses different selectors per environment; Infra uses %s", r.Ref, app.Selector))
			}
		}
		for _, c := range s.Spec.Configs {
			if c.Type == "nginx" && nginxPathRE.MatchString(c.Target.Path) && app.NginxPath == "" {
				app.NginxPath = c.Target.Path
			}
		}
		if app.Selector != "" {
			if !ops.ValidName(app.Name) {
				notes = append(notes, fmt.Sprintf("%s: application name %s is too long or invalid; skipped", r.Ref, app.Name))
				continue
			}
			p.Applications = append(p.Applications, app)
		}
	}
	sort.Slice(p.Environments, func(i, j int) bool { return p.Environments[i].Name < p.Environments[j].Name })
	sort.Slice(p.Applications, func(i, j int) bool { return p.Applications[i].Name < p.Applications[j].Name })
	return p, notes
}

// ProjectYAML renders a project as project.yaml text.
func ProjectYAML(p ops.Project, source string) (string, error) {
	var b strings.Builder
	b.WriteString("# Generated from the DevOpsPad archive " + source + " by devopspadcli archive project --write.\n# Edit the archive (products/*/services/*.yaml) and regenerate instead of editing this file.\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(p); err != nil {
		return "", err
	}
	return b.String(), nil
}
