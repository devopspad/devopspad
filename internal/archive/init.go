package archive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// nowUTC is replaced in tests.
var nowUTC = func() time.Time { return time.Now().UTC() }

func (a *Archive) checkExpired() {
	for _, r := range a.Renewals(nowUTC(), 0) {
		if r.Days < 0 {
			a.warnf(a.byRef[r.Ref].File, 0, "", "renewal.expired", "%s of %s expired on %s; renew it or update the date", r.What, r.Ref, r.Due)
		}
	}
}

// InitResult lists what Init created.
type InitResult struct {
	Dir      string   `json:"dir"`
	Created  []string `json:"created"`
	GitInit  bool     `json:"gitInit"`
	Existing bool     `json:"existing"`
	Hook     string   `json:"hook,omitempty"`
	HookErr  string   `json:"hookError,omitempty"`
}

// Init creates an archive skeleton in dir (a new or existing empty Git
// repository root) and installs the pre-commit validation hook.
func Init(dir, name string) (InitResult, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return InitResult{}, err
	}
	res := InitResult{Dir: abs, Created: []string{}}
	if name == "" {
		name = strings.ToLower(filepath.Base(abs))
	}
	if !labelNameRE.MatchString(name) {
		return res, fmt.Errorf("archive name %q must be lowercase letters, digits and hyphens; pass --name", name)
	}
	if _, err := os.Stat(filepath.Join(abs, RootFile)); err == nil {
		// An existing archive, e.g. a fresh clone: Git does not clone hooks.
		if err := repoRoot(abs); err != nil {
			return res, err
		}
		res.Existing = true
		if hook, err := InstallHook(abs); err != nil {
			res.HookErr = err.Error()
		} else {
			res.Hook = hook
		}
		return res, nil
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return res, err
	}
	if _, err := git(abs, "rev-parse", "--show-toplevel"); err != nil {
		if _, err := git(abs, "init", "-q"); err != nil {
			return res, err
		}
		res.GitInit = true
	} else if err := repoRoot(abs); err != nil {
		return res, err
	}
	files := map[string]string{
		RootFile:     "apiVersion: " + APIVersion + "\nkind: Archive\nmetadata:\n  name: " + name + "\nspec:\n  defaults:\n    renewalWarningDays: 30\n",
		".gitignore": ".devopspad/\n.DS_Store\n",
	}
	for _, d := range []string{"inventory/providers", "inventory/hosts", "inventory/clusters", "inventory/datastores", "inventory/registries", "inventory/domains", "products", "notes", "records/actions"} {
		files[d+"/.gitkeep"] = ""
	}
	for rel, content := range files {
		p := filepath.Join(abs, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return res, err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return res, err
		}
		res.Created = append(res.Created, rel)
	}
	a, err := Load(abs)
	if err != nil {
		return res, err
	}
	if err := WriteGenerated(a); err != nil {
		return res, err
	}
	res.Created = append(res.Created, "INDEX.md", "AGENTS.md")
	if hook, err := InstallHook(abs); err != nil {
		res.HookErr = err.Error()
	} else {
		res.Hook = hook
	}
	return res, nil
}

// NewResource writes a commented skeleton for a new resource and returns its path.
func (a *Archive) NewResource(kind, name, product string) (string, error) {
	k, ok := kindByRef(kind)
	if !ok {
		return "", fmt.Errorf("unknown kind %q; kinds: provider, host, cluster, datastore, registry, domain, product, service", kind)
	}
	if k.Kind == "Domain" {
		if !fqdnRE.MatchString(name) {
			return "", fmt.Errorf("%q is not a lowercase domain name", name)
		}
	} else if !labelNameRE.MatchString(name) {
		return "", fmt.Errorf("%q must be lowercase letters, digits and hyphens", name)
	}
	var rel string
	switch k.Kind {
	case "Product":
		rel = "products/" + name + "/product.yaml"
	case "Service":
		if product == "" {
			return "", errors.New("--product is required for a service")
		}
		if a.byRef["product:"+product] == nil {
			return "", fmt.Errorf("product:%s does not exist%s", product, a.suggest("product"))
		}
		rel = "products/" + product + "/services/" + name + ".yaml"
	default:
		rel = k.Dir + "/" + name + ".yaml"
	}
	p := filepath.Join(a.Dir, filepath.FromSlash(rel))
	if _, err := os.Stat(p); err == nil {
		return "", fmt.Errorf("%s already exists", rel)
	}
	body := strings.NewReplacer("{{name}}", name, "{{product}}", product).Replace(templates[k.Kind])
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return rel, os.WriteFile(p, []byte(body), 0o644)
}

// Skeletons: required fields filled, optional fields commented out.
var templates = map[string]string{
	"Provider": `apiVersion: devopspad.io/v1alpha1
kind: Provider
metadata:
  name: {{name}}
spec:
  categories: [other]   # cloud registrar dns cdn storage payment email appstore analytics ai ci registry monitoring saas other
  # console: https://console.example.com
  # account: me@example.com           # login identifier, not a secret
  # mfa: phone authenticator          # where the second factor lives
  # plan: free
  # cost: { amount: "0", currency: USD, period: month }   # month | year | once
  # renewal: { expiresAt: 2027-01-01, autoRenew: true }
  # credentials:
  #   - { name: api-token, ref: keychain:archive/providers/{{name}}/api-token, scope: DNS edit }
`,
	"Host": `apiVersion: devopspad.io/v1alpha1
kind: Host
metadata:
  name: {{name}}
  # labels: { region: hk, env: pro }
spec:
  state: active          # active | standby | retired
  # provider: provider:<name>
  # addresses: { public: 203.0.113.10, private: 10.0.0.2 }
  # ssh:
  #   host: 203.0.113.10
  #   port: 22
  #   user: root
  #   auth: key          # key | agent | password
  #   keyRef: file:~/.ssh/id_ed25519
  #   hostKey: SHA256:<fingerprint>
  # os: ubuntu-22.04
  # size: { cpu: 2, memoryGiB: 4, diskGiB: 60 }
  # roles: [k3s-server, nginx]
  # cost: { amount: "34.00", currency: CNY, period: month }
  # renewal: { expiresAt: 2027-01-10, autoRenew: false }
`,
	"Cluster": `apiVersion: devopspad.io/v1alpha1
kind: Cluster
metadata:
  name: {{name}}
spec:
  distribution: k3s      # k3s | k8s
  # hosts: [host:<name>]
  kubeconfig:
    source: local        # local: a file on this machine | remote: read over SSH from a host
    pathRef: file:~/.kube/{{name}}.yaml
    # source: remote
    # host: host:<name>
    # path: /etc/rancher/k3s/k3s.yaml
    # context: default
`,
	"Datastore": `apiVersion: devopspad.io/v1alpha1
kind: Datastore
metadata:
  name: {{name}}
spec:
  engine: mysql          # mysql postgres redis mongodb clickhouse kafka s3 other
  # runtime: { host: host:<name> }      # or { cluster: cluster:<name>, namespace: db } or { provider: provider:<name>, managed: rds }
  # endpoint: 10.0.0.2:3306
  # parts: [app]                        # databases / buckets: datastore:{{name}}#app
  # credentials:
  #   - { name: admin, ref: keychain:archive/datastores/{{name}}/admin }
  # backup: { method: mysqldump cron, schedule: "0 3 * * *", target: provider:<name>, retentionDays: 14 }
`,
	"Registry": `apiVersion: devopspad.io/v1alpha1
kind: Registry
metadata:
  name: {{name}}
spec:
  url: registry.example.com/namespace
  # provider: provider:<name>
  # credentials:
  #   - { name: push, ref: keychain:archive/registries/{{name}}/push }
`,
	"Domain": `apiVersion: devopspad.io/v1alpha1
kind: Domain
metadata:
  name: {{name}}
spec:
  # registrar: provider:<name>
  # dns: provider:<name>
  # cost: { amount: "12.00", currency: USD, period: year }
  # renewal: { expiresAt: 2027-01-01, autoRenew: true }
  # records:
  #   - { name: "@", type: A, target: host:<name>, proxied: true }
  #   - { name: www, type: CNAME, value: {{name}} }
  # certificates:
  #   - { names: [{{name}}, "*.{{name}}"], issuer: letsencrypt, managedBy: host:<name>, method: acme.sh dns-01 }
`,
	"Product": `apiVersion: devopspad.io/v1alpha1
kind: Product
metadata:
  name: {{name}}
  # title: Display Name
spec:
  stage: building        # idea | building | live | paused | sunset
  # launchedAt: 2026-01-01
  # links: { site: https://example.com, repo: https://github.com/me/{{name}} }
  # domains: [domain:<name>]
  # providers:
  #   - { ref: provider:<name>, purpose: payments }
  # environments:
  #   - { name: pro, cluster: cluster:<name>, namespace: default, hosts: [host:<name>] }
`,
	"Service": `apiVersion: devopspad.io/v1alpha1
kind: Service
metadata:
  name: {{name}}
  product: {{product}}
spec:
  type: web              # web | api | worker | cron | static | other
  # repo: { url: git@github.com:me/{{product}}.git, path: {{name}}, defaultBranch: main }
  # build: { method: ci, ci: { provider: provider:github, workflow: .github/workflows/{{name}}.yml }, image: registry:<name>#{{product}}-{{name}} }
  # release: { method: devopspad, notes: notes/release-{{name}}.md }   # devopspad | devopspad-spec | ci | manual
  # deployments:
  #   - environment: pro             # an environment of product:{{product}}
  #     runtime: k8s                 # k8s | host | static
  #     selector: app={{product}}-{{name}}
  #     workload: { kind: Deployment, name: {{product}}-{{name}}, container: app }
  # endpoints:
  #   - { url: https://example.com, health: /healthz }
  # configs:
  #   - name: nginx
  #     type: nginx                  # nginx | configmap | env-file | systemd | file
  #     environment: pro
  #     source: configs/{{name}}/pro/{{name}}.conf
  #     target: { host: host:<name>, path: /etc/nginx/conf.d/{{name}}.conf }
  # dependsOn: [datastore:<name>#<part>]
  # secrets:
  #   - { name: DB_PASSWORD, ref: keychain:archive/{{product}}/{{name}}/db-password }
`,
}
