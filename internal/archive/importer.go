package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ImportInput selects the existing DevOpsPad files to convert. Nothing is
// read from live infrastructure and source files are never modified.
type ImportInput struct {
	ConfigDir  string // contains workspace.json and fleet.json
	Workspace  bool
	Fleet      bool
	ProjectDir string // contains project.yaml
	Product    string // product name for the project; defaults to the project name
	Cloudflare bool   // import every Cloudflare zone as a Domain (read-only API calls)
	Provider   string // Cloudflare provider ref (default provider:cloudflare)
	TokenRef   string // token reference when the provider does not exist yet
	DryRun     bool
}

// DefaultCloudflareTokenRef is used when neither the provider nor --token-ref names a token.
const DefaultCloudflareTokenRef = "keychain:archive/providers/cloudflare/api-token"

type ImportFile struct {
	Path   string `json:"path"`
	Action string `json:"action"` // create | unchanged | exists
}

type ImportResult struct {
	Files []ImportFile `json:"files"`
	Todos []string     `json:"todos"`
}

// The subset of the desktop files the importer reads.
type wsConnection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Database    string `json:"database"`
	Environment string `json:"environment"`
	Kubeconfig  string `json:"kubeconfig"`
	Context     string `json:"context"`
	HasPassword bool   `json:"hasPassword"`
	TLS         bool   `json:"tls"`
	SSH         struct {
		Enabled            bool   `json:"enabled"`
		Host               string `json:"host"`
		Port               int    `json:"port"`
		Username           string `json:"username"`
		AuthMethod         string `json:"authMethod"`
		PrivateKeyPath     string `json:"privateKeyPath"`
		KnownHostsPath     string `json:"knownHostsPath"`
		HostKeyFingerprint string `json:"hostKeyFingerprint"`
		HasPassword        bool   `json:"hasPassword"`
		HasPassphrase      bool   `json:"hasPassphrase"`
	} `json:"ssh"`
}
type wsFleetFile struct {
	Fleets []struct {
		Name     string `json:"name"`
		Machines []struct {
			Name         string   `json:"name"`
			ConnectionID string   `json:"connectionId"`
			Roles        []string `json:"roles"`
		} `json:"machines"`
	} `json:"fleets"`
}
type wsProject struct {
	Name         string `yaml:"name"`
	Environments []struct {
		Name       string `yaml:"name"`
		Connection string `yaml:"connection"`
		Namespace  string `yaml:"namespace"`
	} `yaml:"environments"`
	Applications []struct {
		Name      string `yaml:"name"`
		Selector  string `yaml:"selector"`
		NginxPath string `yaml:"nginxPath"`
	} `yaml:"applications"`
}

type importer struct {
	out       map[string]Object // archive path → resource
	todos     []string
	names     map[string]bool // kind:name already used
	hostBySSH map[string]string
	clusterOf map[string]string // connection name → cluster ref
	conns     []wsConnection
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s, fallback string) string {
	s = strings.Trim(slugRE.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	if s == "" {
		return fallback
	}
	return s
}

func (im *importer) unique(kind, base string) string {
	name := base
	for i := 2; im.names[kind+":"+name]; i++ {
		name = fmt.Sprintf("%s-%d", strings.TrimRight(base[:min(len(base), 60)], "-"), i)
	}
	im.names[kind+":"+name] = true
	return name
}

func (im *importer) todo(format string, args ...any) {
	im.todos = append(im.todos, fmt.Sprintf(format, args...))
}

func header(kind, name string) Header {
	return Header{APIVersion: APIVersion, Kind: kind, Metadata: Metadata{Name: name}}
}

// Import converts desktop files into archive resources under dir.
func Import(dir string, in ImportInput) (ImportResult, error) {
	a, err := Load(dir)
	if err != nil {
		return ImportResult{}, err
	}
	// Names are unique within one run only, so a repeated import maps to the
	// same files and reports them as unchanged instead of creating copies.
	im := &importer{out: map[string]Object{}, names: map[string]bool{}, hostBySSH: map[string]string{}, clusterOf: map[string]string{}}
	if in.Workspace || in.Fleet {
		if err := im.workspace(in); err != nil {
			return ImportResult{}, err
		}
	}
	if in.ProjectDir != "" {
		if err := im.project(in); err != nil {
			return ImportResult{}, err
		}
	}
	if in.Cloudflare {
		if err := im.cloudflare(a, in); err != nil {
			return ImportResult{}, err
		}
	}
	if len(im.out) == 0 && len(im.todos) == 0 {
		return ImportResult{}, errors.New("nothing to import; pass --workspace, --fleet, --project <dir> and/or --cloudflare")
	}
	res := ImportResult{Files: []ImportFile{}, Todos: im.todos}
	paths := make([]string, 0, len(im.out))
	for p := range im.out {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(im.out[p]); err != nil {
			return res, err
		}
		b := append([]byte("# Imported by devopspadcli archive import; review and complete it.\n"), buf.Bytes()...)
		full := filepath.Join(dir, filepath.FromSlash(p))
		f := ImportFile{Path: p, Action: "create"}
		if old, err := os.ReadFile(full); err == nil {
			f.Action = "exists"
			if bytes.Equal(old, b) {
				f.Action = "unchanged"
			} else {
				res.Todos = append(res.Todos, p+" already exists and was not overwritten; merge by hand if needed")
			}
		}
		res.Files = append(res.Files, f)
		if f.Action != "create" || in.DryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return res, err
		}
		if err := writeAtomic(full, b, 0o644); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (im *importer) workspace(in ImportInput) error {
	b, err := os.ReadFile(filepath.Join(in.ConfigDir, "workspace.json"))
	if err != nil {
		return fmt.Errorf("read workspace.json: %w", err)
	}
	var ws struct {
		Connections []wsConnection `json:"connections"`
	}
	if err := json.Unmarshal(b, &ws); err != nil {
		return fmt.Errorf("read workspace.json: %w", err)
	}
	im.conns = ws.Connections
	type machine struct {
		name, fleet string
		roles       []string
	}
	machines := map[string]machine{}
	if in.Fleet {
		if fb, err := os.ReadFile(filepath.Join(in.ConfigDir, "fleet.json")); err == nil {
			var ff wsFleetFile
			if err := json.Unmarshal(fb, &ff); err != nil {
				return fmt.Errorf("read fleet.json: %w", err)
			}
			for _, f := range ff.Fleets {
				for _, m := range f.Machines {
					machines[m.ConnectionID] = machine{m.Name, f.Name, m.Roles}
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, c := range ws.Connections {
		if !in.Workspace && machines[c.ID].name == "" {
			continue
		}
		tunnel := ""
		if c.SSH.Enabled {
			tunnel = "host:" + im.host(c, machines[c.ID].name, machines[c.ID].fleet, machines[c.ID].roles)
		} else if m := machines[c.ID]; m.name != "" {
			im.todo("fleet machine %s uses connection %q without SSH; add inventory/hosts/%s.yaml by hand", m.name, c.Name, slug(m.name, "host"))
		}
		if !in.Workspace {
			continue
		}
		switch c.Kind {
		case "k8s", "k3s":
			name := im.unique("cluster", slug(c.Name, "cluster"))
			cl := &Cluster{Header: header("Cluster", name), Spec: ClusterSpec{Distribution: c.Kind, Tunnel: tunnel,
				Kubeconfig: Kubeconfig{Source: "local", PathRef: fileRef(c.Kubeconfig), Context: c.Context}}}
			setEnvLabel(&cl.Metadata, c.Environment)
			bindConnection(&cl.Metadata, c.ID)
			im.out["inventory/clusters/"+name+".yaml"] = cl
			im.clusterOf[c.Name] = "cluster:" + name
			im.todo("cluster:%s — list the hosts it runs on (spec.hosts); switch to kubeconfig source: remote if you read it from a host", name)
		default:
			name := im.unique("datastore", slug(c.Name, "datastore"))
			engine := map[string]string{"mongo": "mongodb"}[c.Kind]
			if engine == "" {
				engine = c.Kind
			}
			ds := &Datastore{Header: header("Datastore", name), Spec: DatastoreSpec{Engine: engine, Tunnel: tunnel, Endpoint: net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), Username: c.Username, TLS: c.TLS}}
			bindConnection(&ds.Metadata, c.ID)
			if c.Database != "" {
				ds.Spec.Parts = []string{c.Database}
			}
			if tunnel != "" && (c.Host == "localhost" || c.Host == "127.0.0.1" || c.Host == "::1") {
				ds.Spec.Runtime.Host = tunnel
			}
			if c.HasPassword {
				ds.Spec.Credentials = []Credential{{Name: "password", Ref: "keychain:" + c.ID}}
			}
			setEnvLabel(&ds.Metadata, c.Environment)
			im.out["inventory/datastores/"+name+".yaml"] = ds
			if ds.Spec.Runtime.Host == "" {
				im.todo("datastore:%s — set spec.runtime (the host, cluster or managed provider it runs on)", name)
			}
		}
	}
	return nil
}

// host returns the host name for a connection's SSH endpoint, creating it once.
func (im *importer) host(c wsConnection, machineName, fleet string, roles []string) string {
	port := c.SSH.Port
	if port == 0 {
		port = 22
	}
	key := fmt.Sprintf("%s@%s:%d", c.SSH.Username, c.SSH.Host, port)
	if name, ok := im.hostBySSH[key]; ok {
		return name
	}
	name := im.unique("host", slug(machineName, slug(c.SSH.Host, "host")))
	ssh := &SSH{Host: c.SSH.Host, Port: port, User: c.SSH.Username, Auth: c.SSH.AuthMethod, HostKey: c.SSH.HostKeyFingerprint, KnownHosts: c.SSH.KnownHostsPath}
	switch ssh.Auth {
	case "key":
		ssh.KeyRef = fileRef(c.SSH.PrivateKeyPath)
		if c.SSH.HasPassphrase {
			ssh.PassphraseRef = "keychain:" + c.ID + ":ssh-passphrase"
		}
	case "password":
		if c.SSH.HasPassword {
			ssh.PasswordRef = "keychain:" + c.ID + ":ssh-password"
		}
	}
	h := &Host{Header: header("Host", name), Spec: HostSpec{State: "active", SSH: ssh, Roles: roles}}
	if net.ParseIP(c.SSH.Host) != nil {
		h.Spec.Addresses.Public = c.SSH.Host
	}
	if fleet != "" {
		h.Metadata.Labels = map[string]string{"fleet": fleet}
	}
	im.out["inventory/hosts/"+name+".yaml"] = h
	im.hostBySSH[key] = name
	im.todo("host:%s — add provider, size, cost and renewal", name)
	return name
}

func fileRef(p string) string {
	if p == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		p = "~/" + strings.TrimPrefix(p, home+"/")
	}
	return "file:" + p
}

// ConnectionAnnotation binds a resource to the desktop connection it was
// imported from, so connect sync updates that connection instead of adding one.
const ConnectionAnnotation = "devopspad.io/connection-id"

func bindConnection(m *Metadata, id string) {
	if id != "" {
		m.Annotations = map[string]string{ConnectionAnnotation: id}
	}
}

func setEnvLabel(m *Metadata, env string) {
	if v := slug(env, ""); v != "" {
		m.Labels = map[string]string{"env": v}
	}
}

func (im *importer) project(in ImportInput) error {
	b, err := os.ReadFile(filepath.Join(in.ProjectDir, "project.yaml"))
	if err != nil {
		return fmt.Errorf("read project.yaml: %w", err)
	}
	var p wsProject
	if err := yaml.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("read project.yaml: %w", err)
	}
	product := in.Product
	if product == "" {
		product = slug(p.Name, "product")
	}
	if !labelNameRE.MatchString(product) {
		return fmt.Errorf("product name %q must be lowercase letters, digits and hyphens; pass --product", product)
	}
	pr := &Product{Header: header("Product", product), Spec: ProductSpec{Stage: "live"}}
	if p.Name != product {
		pr.Metadata.Title = p.Name
	}
	prefixes := map[string]bool{}
	for _, e := range p.Environments {
		env := Environment{Name: slug(e.Name, "env"), Namespace: e.Namespace, Cluster: im.clusterOf[e.Connection]}
		if env.Cluster == "" {
			im.todo("product:%s environment %s — set cluster (connection %q was not imported; add --workspace)", product, env.Name, e.Connection)
		}
		if i := strings.LastIndex(env.Name, "-"); i > 0 {
			prefixes[env.Name[:i]] = true
		}
		pr.Spec.Environments = append(pr.Spec.Environments, env)
	}
	if len(prefixes) > 1 {
		im.todo("product:%s — environment names suggest several products (%s); split them into separate products", product, strings.Join(sortedKeys(prefixes), ", "))
	}
	im.out["products/"+product+"/product.yaml"] = pr
	im.todo("product:%s — check stage, add domains and providers", product)
	for _, app := range p.Applications {
		name := slug(strings.TrimPrefix(app.Name, product+"-"), "service")
		if im.names["service:"+product+"/"+name] {
			continue
		}
		im.names["service:"+product+"/"+name] = true
		svc := &Service{Header: header("Service", name), Spec: ServiceSpec{Type: "web"}}
		svc.Metadata.Product = product
		for _, env := range pr.Spec.Environments {
			svc.Spec.Deployments = append(svc.Spec.Deployments, Deployment{Environment: env.Name, Runtime: "k8s", Selector: app.Selector})
		}
		im.out["products/"+product+"/services/"+name+".yaml"] = svc
		if app.NginxPath != "" {
			im.todo("service:%s/%s — copy %s from the host into products/%s/configs/%s/<env>/ and add it to spec.configs", product, name, app.NginxPath, product, name)
		}
	}
	if len(p.Applications) > 0 {
		im.todo("services of product:%s — set type, repo, build and release; remove deployments to environments they do not run in", product)
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// cloudflare imports every zone of the token's accounts as a Domain, with the
// expiry from Cloudflare Registrar or, for other registrars, public RDAP.
func (im *importer) cloudflare(a *Archive, in ImportInput) error {
	provider := defaultProvider(in.Provider)
	tokenRef := in.TokenRef
	if tokenRef == "" && a.Get(provider) == nil {
		tokenRef = DefaultCloudflareTokenRef
	}
	token, ref, err := a.cloudflareToken(provider, tokenRef)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cf := cloudflare{token: token}
	zones, err := cf.zones(ctx)
	if err != nil {
		return fmt.Errorf("%w (the token needs Zone Read and DNS Read)", err)
	}
	name := strings.TrimPrefix(provider, "provider:")
	if a.Get(provider) == nil {
		p := &Provider{Header: header("Provider", name), Spec: ProviderSpec{Categories: []string{"registrar", "dns", "cdn"}, Console: "https://dash.cloudflare.com",
			Credentials: []Credential{{Name: "api-token", Ref: ref, Scope: "Zone Read, DNS Read"}}}}
		im.out["inventory/providers/"+name+".yaml"] = p
	}
	registrar := map[string]cfRegistrarDomain{}
	accounts := map[string]bool{}
	for _, z := range zones {
		accounts[z.Account.ID] = true
	}
	for id := range accounts {
		if id == "" {
			continue
		}
		m, err := cf.registrar(ctx, id)
		if err != nil {
			im.todo("Cloudflare Registrar data unavailable (%v); expiry dates came from RDAP and spec.registrar is unset", err)
			continue
		}
		for k, v := range m {
			registrar[k] = v
		}
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })
	domains := make([]*Domain, len(zones))
	var unknownRegistrar, unknownAuto, noExpiry []string
	parallel(len(zones), 8, func(i int) {
		z := strings.ToLower(zones[i].Name)
		if !fqdnRE.MatchString(z) {
			return
		}
		d := &Domain{Header: header("Domain", z), Spec: DomainSpec{DNS: provider}}
		if reg, ok := registrar[z]; ok && dateOf(reg.ExpiresAt) != "" {
			d.Spec.Registrar = provider
			d.Spec.Renewal = &Renewal{ExpiresAt: dateOf(reg.ExpiresAt)}
			if reg.AutoRenew != nil {
				d.Spec.Renewal.AutoRenew = *reg.AutoRenew
			}
		} else if exp, err := rdapExpiry(ctx, z); err == nil {
			d.Spec.Renewal = &Renewal{ExpiresAt: exp}
		}
		domains[i] = d
	})
	for i, d := range domains {
		if d == nil {
			im.todo("zone %q skipped: not a valid domain name", zones[i].Name)
			continue
		}
		n := d.Metadata.Name
		if d.Spec.Registrar == "" {
			unknownRegistrar = append(unknownRegistrar, n)
		}
		if d.Spec.Renewal == nil {
			noExpiry = append(noExpiry, n)
		} else if reg, ok := registrar[n]; !ok || reg.AutoRenew == nil {
			unknownAuto = append(unknownAuto, n)
		}
		im.out["inventory/domains/"+d.Metadata.Name+".yaml"] = d
	}
	summarize := func(list []string, msg string) {
		if len(list) == 0 {
			return
		}
		shown := list
		if len(shown) > 8 {
			shown = append(append([]string{}, shown[:8]...), fmt.Sprintf("… %d more", len(list)-8))
		}
		im.todo("%d domains: %s (%s)", len(list), msg, strings.Join(shown, ", "))
	}
	summarize(unknownRegistrar, "not in Cloudflare Registrar data; set spec.registrar")
	summarize(unknownAuto, "auto-renew unknown, recorded as false; check Cloudflare and set renewal.autoRenew")
	summarize(noExpiry, "expiry date not found; set renewal.expiresAt")
	im.todo("link the %d domains to products (Product spec.domains); devopspadcli report orphans lists unlinked ones", len(zones))
	return nil
}
