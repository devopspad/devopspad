package archive

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ObservedDir holds scan results. It is under .devopspad/, which is never
// committed: observations are local, the archive keeps only desired state.
const ObservedDir = ".devopspad/observed"

type Observation struct {
	APIVersion string        `json:"apiVersion"`
	Kind       string        `json:"kind"`
	Ref        string        `json:"ref"`
	ObservedAt string        `json:"observedAt"`
	Collector  string        `json:"collector"`
	Domain     *DomainFacts  `json:"domain,omitempty"`
	Cluster    *ClusterFacts `json:"cluster,omitempty"`
	Host       *HostFacts    `json:"host,omitempty"`
}

type DomainFacts struct {
	ExpiresAt    string           `json:"expiresAt,omitempty"`
	ExpirySource string           `json:"expirySource,omitempty"` // cloudflare-registrar | rdap
	AutoRenew    *bool            `json:"autoRenew,omitempty"`
	Registrar    string           `json:"registrar,omitempty"` // provider ref when the registrar API knows the domain
	DNS          string           `json:"dns,omitempty"`       // provider ref when the zone is on Cloudflare
	ZoneStatus   string           `json:"zoneStatus,omitempty"`
	NameServers  []string         `json:"nameServers,omitempty"`
	Routes       []WorkerRoute    `json:"workerRoutes,omitempty"` // empty script: no Worker (origin)
	Records      []ObservedRecord `json:"records"`
	Certificates []ObservedCert   `json:"certificates"`
	Errors       []string         `json:"errors"`
}

type ObservedRecord struct {
	Name    string `json:"name"` // relative to the zone; @ for the apex
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied,omitempty"`
}

type ObservedCert struct {
	Host     string `json:"host"`
	NotAfter string `json:"notAfter,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
}

type providerObservation struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Ref        string   `json:"ref"`
	ObservedAt string   `json:"observedAt"`
	Zones      []string `json:"zones"`
}

type ScanInput struct {
	Refs      []string // domain, cluster or host refs; empty scans everything of Kinds
	Kinds     []string // domain, cluster, host; empty means all three
	Provider  string   // the Cloudflare provider ref (default provider:cloudflare)
	TokenRef  string   // overrides the provider's credential
	Collector string
	Now       time.Time
}

type ScanResult struct {
	Observations []*Observation `json:"observations"`
	Unregistered []string       `json:"unregisteredZones"`
	Notes        []string       `json:"notes"`
}

// certFetch is replaced in tests.
var certFetch = fetchCert

// cloudflareToken resolves the token for the Cloudflare provider.
func (a *Archive) cloudflareToken(provider, override string) (string, string, error) {
	ref := override
	if ref == "" {
		r := a.Get(provider)
		if r == nil {
			return "", "", fmt.Errorf("%s does not exist; create it (devopspadcli new provider cloudflare) or pass --token-ref", provider)
		}
		creds := r.Object.(*Provider).Spec.Credentials
		for _, c := range creds {
			if ref == "" || c.Name == "api-token" {
				ref = c.Ref
			}
		}
		if ref == "" {
			return "", "", fmt.Errorf("%s has no credentials; add { name: api-token, ref: keychain:archive/providers/cloudflare/api-token } and store the token with devopspadcli secrets set", provider)
		}
	}
	v, err := ResolveSecret(ref)
	return v, ref, err
}

func defaultProvider(p string) string {
	if p == "" {
		return "provider:cloudflare"
	}
	return p
}

// Scan reads live facts about domains: Cloudflare zone, DNS records and
// Registrar expiry (when a token is available), RDAP expiry otherwise, and the
// TLS certificate of each endpoint. Results are written to ObservedDir.
// Nothing is changed anywhere.
func (a *Archive) Scan(ctx context.Context, in ScanInput) (*ScanResult, error) {
	in.Provider = defaultProvider(in.Provider)
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	targets, err := a.selectTargets(in.Refs, in.Kinds)
	if err != nil {
		return nil, err
	}
	res := &ScanResult{Observations: []*Observation{}, Unregistered: []string{}, Notes: []string{}}
	if added, err := a.ensureIgnored(); err != nil {
		return nil, err
	} else if added {
		res.Notes = append(res.Notes, ".devopspad/ added to .gitignore so scan results are never committed; commit .gitignore")
	}
	if len(targets["Domain"]) > 0 || (len(in.Refs) == 0 && wants(in.Kinds, "domain")) {
		if err := a.scanDomains(ctx, in, targets["Domain"], res); err != nil {
			return nil, err
		}
	}
	for _, kind := range []string{"Cluster", "Host"} {
		obs := make([]*Observation, len(targets[kind]))
		parallel(len(targets[kind]), 4, func(i int) {
			r := targets[kind][i]
			o := &Observation{APIVersion: APIVersion, Kind: "Observation", Ref: r.Ref, ObservedAt: in.Now.Format(time.RFC3339), Collector: in.Collector}
			if kind == "Cluster" {
				o.Cluster = a.scanCluster(ctx, r)
			} else {
				o.Host = a.scanHost(ctx, r)
			}
			obs[i] = o
		})
		for _, o := range obs {
			if err := a.writeObserved(observedPath(o.Ref), o); err != nil {
				return nil, err
			}
			res.Observations = append(res.Observations, o)
		}
	}
	return res, nil
}

func wants(kinds []string, k string) bool { return len(kinds) == 0 || contains(kinds, k) }

// selectTargets groups the resources to scan or diff by kind.
func (a *Archive) selectTargets(refs, kinds []string) (map[string][]*Resource, error) {
	out := map[string][]*Resource{}
	for _, k := range kinds {
		if k != "domain" && k != "cluster" && k != "host" {
			return nil, fmt.Errorf("--kind must be domain, cluster or host")
		}
	}
	if len(refs) == 0 {
		for _, k := range []string{"domain", "cluster", "host"} {
			if wants(kinds, k) {
				kind, _ := kindArgName(k)
				list, _ := a.List(kind, "")
				if kind == "Host" {
					list = activeHostsWithSSH(list)
				}
				out[kind] = list
			}
		}
		return out, nil
	}
	for _, ref := range refs {
		if !strings.Contains(ref, ":") {
			ref = "domain:" + ref
		}
		r := a.Get(strings.ToLower(ref))
		if r == nil || (r.Kind != "Domain" && r.Kind != "Cluster" && r.Kind != "Host") {
			return nil, fmt.Errorf("%s is not a domain, cluster or host in the archive", ref)
		}
		out[r.Kind] = append(out[r.Kind], r)
	}
	return out, nil
}

func kindArgName(k string) (string, bool) {
	kk, ok := kindByRef(k)
	return kk.Kind, ok
}

func activeHostsWithSSH(list []*Resource) []*Resource {
	var out []*Resource
	for _, r := range list {
		if h := r.Object.(*Host); h.Spec.SSH != nil && h.Spec.State != "retired" {
			out = append(out, r)
		}
	}
	return out
}

func observedPath(ref string) string {
	kind, name, _ := strings.Cut(ref, ":")
	return kind + "s/" + name + ".json"
}

func (a *Archive) scanDomains(ctx context.Context, in ScanInput, domains []*Resource, res *ScanResult) error {
	var cf *cloudflare
	if token, _, err := a.cloudflareToken(in.Provider, in.TokenRef); err != nil {
		res.Notes = append(res.Notes, "Cloudflare skipped (DNS records and registrar data): "+err.Error())
	} else {
		cf = &cloudflare{token: token}
	}
	zones := map[string]cfZone{}
	registrar := map[string]cfRegistrarDomain{}
	if cf != nil {
		list, err := cf.zones(ctx)
		if err != nil {
			return fmt.Errorf("%w (the token needs Zone Read and DNS Read)", err)
		}
		accounts := map[string]bool{}
		for _, z := range list {
			zones[strings.ToLower(z.Name)] = z
			accounts[z.Account.ID] = true
		}
		for id := range accounts {
			if id == "" {
				continue
			}
			m, err := cf.registrar(ctx, id)
			if err != nil {
				res.Notes = append(res.Notes, "Cloudflare Registrar data unavailable, expiry dates come from RDAP: "+err.Error())
				continue
			}
			for k, v := range m {
				registrar[k] = v
			}
		}
		if len(in.Refs) == 0 {
			names := make([]string, 0, len(zones))
			for n := range zones {
				names = append(names, n)
				if a.Get("domain:"+n) == nil {
					res.Unregistered = append(res.Unregistered, n)
				}
			}
			sort.Strings(names)
			sort.Strings(res.Unregistered)
			po := providerObservation{APIVersion: APIVersion, Kind: "Observation", Ref: in.Provider, ObservedAt: in.Now.Format(time.RFC3339), Zones: names}
			if err := a.writeObserved("providers/"+strings.TrimPrefix(in.Provider, "provider:")+".json", po); err != nil {
				return err
			}
		}
	}
	obs := make([]*Observation, len(domains))
	var routesDenied atomic.Bool
	var notesMu sync.Mutex
	parallel(len(domains), 8, func(i int) {
		d := domains[i]
		facts := DomainFacts{Records: []ObservedRecord{}, Certificates: []ObservedCert{}, Errors: []string{}}
		if z, ok := zones[d.Name]; ok {
			facts.DNS, facts.ZoneStatus, facts.NameServers = in.Provider, z.Status, z.NameServers
			if !routesDenied.Load() {
				if routes, err := cf.workerRoutes(ctx, z.ID); err != nil {
					if routesDenied.CompareAndSwap(false, true) {
						notesMu.Lock()
						res.Notes = append(res.Notes, "Workers routes not read (the token needs Workers Routes Read): "+err.Error())
						notesMu.Unlock()
					}
				} else {
					sortRoutes(routes)
					facts.Routes = routes
				}
			}
			if recs, err := cf.records(ctx, z.ID); err != nil {
				facts.Errors = append(facts.Errors, err.Error())
			} else {
				for _, r := range recs {
					facts.Records = append(facts.Records, ObservedRecord{Name: relName(r.Name, d.Name), Type: r.Type, Content: strings.TrimSuffix(r.Content, "."), Proxied: r.Proxied})
				}
			}
		} else if cf != nil {
			facts.Errors = append(facts.Errors, "zone not found in Cloudflare")
		}
		if reg, ok := registrar[d.Name]; ok && dateOf(reg.ExpiresAt) != "" {
			facts.ExpiresAt, facts.ExpirySource, facts.AutoRenew, facts.Registrar = dateOf(reg.ExpiresAt), "cloudflare-registrar", reg.AutoRenew, in.Provider
		} else if exp, err := rdapExpiry(ctx, d.Name); err == nil {
			facts.ExpiresAt, facts.ExpirySource = exp, "rdap"
		} else {
			facts.Errors = append(facts.Errors, err.Error())
		}
		for _, host := range a.certHosts(d.Name) {
			facts.Certificates = append(facts.Certificates, certFetch(ctx, host))
		}
		sort.Slice(facts.Records, func(i, j int) bool {
			if facts.Records[i].Name != facts.Records[j].Name {
				return facts.Records[i].Name < facts.Records[j].Name
			}
			return facts.Records[i].Type < facts.Records[j].Type
		})
		obs[i] = &Observation{APIVersion: APIVersion, Kind: "Observation", Ref: d.Ref, ObservedAt: in.Now.Format(time.RFC3339), Collector: in.Collector, Domain: &facts}
	})
	for _, o := range obs {
		if err := a.writeObserved(observedPath(o.Ref), o); err != nil {
			return err
		}
		res.Observations = append(res.Observations, o)
	}
	return nil
}

// certHosts returns the HTTPS endpoint hosts under a domain, or the apex.
func (a *Archive) certHosts(domain string) []string {
	seen := map[string]bool{}
	var hosts []string
	for _, r := range a.Resources {
		s, ok := r.Object.(*Service)
		if !ok {
			continue
		}
		for _, e := range s.Spec.Endpoints {
			u, err := url.Parse(e.URL)
			if err != nil || u.Scheme != "https" {
				continue
			}
			h := strings.ToLower(u.Hostname())
			if (h == domain || strings.HasSuffix(h, "."+domain)) && !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
	}
	if len(hosts) == 0 {
		hosts = []string{domain}
	}
	sort.Strings(hosts)
	return hosts
}

func relName(name, zone string) string {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	if name == zone {
		return "@"
	}
	return strings.TrimSuffix(name, "."+zone)
}

// fetchCert reads the served certificate. Verification is done separately so
// expired or mismatched certificates are still reported with their dates.
func fetchCert(ctx context.Context, host string) ObservedCert {
	c := ObservedCert{Host: host}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	d := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: &tls.Config{ServerName: host, InsecureSkipVerify: true}} // #nosec G402 -- verified below
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, "443"))
	if err != nil {
		c.Error = err.Error()
		return c
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		c.Error = "no certificate presented"
		return c
	}
	leaf := state.PeerCertificates[0]
	c.NotAfter = leaf.NotAfter.UTC().Format("2006-01-02")
	c.Issuer = leaf.Issuer.CommonName
	if len(leaf.Issuer.Organization) > 0 {
		c.Issuer = leaf.Issuer.Organization[0]
	}
	pool := x509.NewCertPool()
	for _, ic := range state.PeerCertificates[1:] {
		pool.AddCert(ic)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Intermediates: pool}); err != nil {
		c.Error = err.Error()
	} else {
		c.Valid = true
	}
	return c
}

func parallel(n, workers int, fn func(int)) {
	var wg sync.WaitGroup
	ch := make(chan int)
	for w := 0; w < workers && w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		ch <- i
	}
	close(ch)
	wg.Wait()
}

func (a *Archive) writeObserved(rel string, v any) error {
	p := filepath.Join(a.Dir, ObservedDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(p, append(b, '\n'), 0o600)
}

// ReadObservation returns the last scan of a resource, or nil if never scanned.
func (a *Archive) ReadObservation(ref string) (*Observation, error) {
	b, err := os.ReadFile(filepath.Join(a.Dir, ObservedDir, filepath.FromSlash(observedPath(ref))))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var o Observation
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("%s: corrupt observation, scan again: %w", ref, err)
	}
	return &o, nil
}

func (a *Archive) readProviderObservation(provider string) *providerObservation {
	b, err := os.ReadFile(filepath.Join(a.Dir, ObservedDir, "providers", strings.TrimPrefix(provider, "provider:")+".json"))
	if err != nil {
		return nil
	}
	var o providerObservation
	if json.Unmarshal(b, &o) != nil {
		return nil
	}
	return &o
}

func gitignored(dir string) bool {
	b, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	for _, line := range strings.Split(string(b), "\n") {
		if l := strings.TrimSpace(line); l == ".devopspad" || l == ".devopspad/" || l == "/.devopspad/" || l == "/.devopspad" {
			return true
		}
	}
	return false
}

// ensureIgnored appends .devopspad/ to .gitignore when missing.
func (a *Archive) ensureIgnored() (bool, error) {
	if gitignored(a.Dir) {
		return false, nil
	}
	p := filepath.Join(a.Dir, ".gitignore")
	b, _ := os.ReadFile(p)
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		b = append(b, '\n')
	}
	return true, os.WriteFile(p, append(b, ".devopspad/\n"...), 0o644)
}
