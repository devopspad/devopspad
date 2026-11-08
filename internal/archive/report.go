package archive

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/labels"
)

// List returns resources of one kind (empty = all), optionally filtered by a
// label selector, ordered by reference.
func (a *Archive) List(kind, selector string) ([]*Resource, error) {
	var sel labels.Selector = labels.Everything()
	if selector != "" {
		s, err := labels.Parse(selector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}
		sel = s
	}
	out := []*Resource{}
	for _, r := range a.Resources {
		if (kind == "" || r.Kind == kind) && sel.Matches(labels.Set(r.Header().Metadata.Labels)) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

// Incoming lists references pointing at ref.
func (a *Archive) Incoming(ref string) []RefUse {
	out := []RefUse{}
	for _, u := range a.Uses {
		if u.To == ref {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].Field < out[j].Field
	})
	return out
}

// Outgoing lists references made by ref.
func (a *Archive) Outgoing(ref string) []RefUse {
	out := []RefUse{}
	for _, u := range a.Uses {
		if u.From == ref {
			out = append(out, u)
		}
	}
	return out
}

// Closure returns everything a product depends on: the product, its services
// and every resource reachable through their references.
func (a *Archive) Closure(product string) map[string]bool {
	if c, ok := a.closures()[product]; ok {
		return c
	}
	return map[string]bool{product: true}
}

// closures computes every product's closure once; the archive is immutable
// after Load, so the result is cached.
func (a *Archive) closures() map[string]map[string]bool {
	if a.closureCache != nil {
		return a.closureCache
	}
	next := map[string][]string{}
	for _, u := range a.Uses {
		next[u.From] = append(next[u.From], u.To)
	}
	services := map[string][]string{}
	for _, r := range a.Resources {
		if r.Kind == "Service" {
			p := "product:" + r.Header().Metadata.Product
			services[p] = append(services[p], r.Ref)
		}
	}
	out := map[string]map[string]bool{}
	for _, r := range a.Resources {
		if r.Kind != "Product" {
			continue
		}
		seen := map[string]bool{r.Ref: true}
		queue := append([]string{r.Ref}, services[r.Ref]...)
		for _, s := range services[r.Ref] {
			seen[s] = true
		}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, to := range next[cur] {
				if !seen[to] {
					seen[to] = true
					queue = append(queue, to)
				}
			}
		}
		out[r.Ref] = seen
	}
	a.closureCache = out
	return out
}

// UsedBy returns the products whose closure contains ref.
func (a *Archive) UsedBy(ref string) []string {
	return usedBy(a.closures(), ref)
}

func usedBy(closures map[string]map[string]bool, ref string) []string {
	out := []string{}
	for p, c := range closures {
		if c[ref] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

type MatrixRow struct {
	Product      string   `json:"product"`
	Title        string   `json:"title,omitempty"`
	Stage        string   `json:"stage"`
	Services     []string `json:"services"`
	Domains      []string `json:"domains"`
	Environments []string `json:"environments"`
	Hosts        []string `json:"hosts"`
}

// Matrix summarizes products, optionally filtered by a label selector and stage.
func (a *Archive) Matrix(selector, stage string) ([]MatrixRow, error) {
	products, err := a.List("Product", selector)
	if err != nil {
		return nil, err
	}
	rows := []MatrixRow{}
	for _, r := range products {
		p := r.Object.(*Product)
		if stage != "" && p.Spec.Stage != stage {
			continue
		}
		row := MatrixRow{Product: r.Name, Title: p.Metadata.Title, Stage: p.Spec.Stage, Services: []string{}, Domains: []string{}, Environments: []string{}, Hosts: []string{}}
		for _, d := range p.Spec.Domains {
			row.Domains = append(row.Domains, strings.TrimPrefix(d, "domain:"))
		}
		for _, e := range p.Spec.Environments {
			where := strings.TrimPrefix(e.Cluster, "cluster:")
			if e.Namespace != "" {
				where += "/" + e.Namespace
			}
			if where == "" {
				where = "-"
			}
			row.Environments = append(row.Environments, e.Name+"→"+where)
		}
		for ref := range a.Closure(r.Ref) {
			switch {
			case strings.HasPrefix(ref, "host:"):
				row.Hosts = append(row.Hosts, strings.TrimPrefix(ref, "host:"))
			case strings.HasPrefix(ref, "service:"+r.Name+"/"):
				row.Services = append(row.Services, strings.TrimPrefix(ref, "service:"+r.Name+"/"))
			}
		}
		sort.Strings(row.Hosts)
		sort.Strings(row.Services)
		rows = append(rows, row)
	}
	return rows, nil
}

type RenewalRow struct {
	Due       string   `json:"due"`
	Days      int      `json:"days"`
	What      string   `json:"what"`
	Ref       string   `json:"ref"`
	AutoRenew *bool    `json:"autoRenew,omitempty"`
	Cost      string   `json:"cost,omitempty"`
	UsedBy    []string `json:"usedBy"`
}

// Renewals lists everything that expires. within < 0 returns all items.
func (a *Archive) Renewals(now time.Time, within int) []RenewalRow {
	closures := a.closures()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	rows := []RenewalRow{}
	add := func(what, ref, due string, auto *bool, cost *Cost) {
		t, err := time.Parse("2006-01-02", due)
		if err != nil {
			return
		}
		days := int(t.Sub(today).Hours() / 24)
		if within >= 0 && days > within {
			return
		}
		rows = append(rows, RenewalRow{Due: due, Days: days, What: what, Ref: ref, AutoRenew: auto, Cost: cost.String(), UsedBy: usedBy(closures, ref)})
	}
	for _, r := range a.Resources {
		switch o := r.Object.(type) {
		case *Provider:
			if rn := o.Spec.Renewal; rn != nil {
				add("subscription", r.Ref, rn.ExpiresAt, &rn.AutoRenew, o.Spec.Cost)
			}
		case *Host:
			if rn := o.Spec.Renewal; rn != nil && o.Spec.State != "retired" {
				add("host", r.Ref, rn.ExpiresAt, &rn.AutoRenew, o.Spec.Cost)
			}
		case *Domain:
			if rn := o.Spec.Renewal; rn != nil {
				add("domain", r.Ref, rn.ExpiresAt, &rn.AutoRenew, o.Spec.Cost)
			}
			for _, c := range o.Spec.Certificates {
				if c.ExpiresAt != "" {
					add("certificate "+strings.Join(c.Names, ","), r.Ref, c.ExpiresAt, nil, nil)
				}
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Due != rows[j].Due {
			return rows[i].Due < rows[j].Due
		}
		return rows[i].Ref < rows[j].Ref
	})
	return rows
}

func (c *Cost) String() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("%s %s/%s", c.Currency, c.Amount, c.Period)
}

// monthly converts a cost to a monthly amount; one-off costs return nil.
func (c *Cost) monthly() *big.Rat {
	if c == nil || c.Period == "once" {
		return nil
	}
	r, ok := new(big.Rat).SetString(c.Amount)
	if !ok {
		return nil
	}
	if c.Period == "year" {
		r.Quo(r, big.NewRat(12, 1))
	}
	return r
}

type CostRow struct {
	Ref     string   `json:"ref"`
	Cost    string   `json:"cost"`
	Monthly string   `json:"monthly"`
	UsedBy  []string `json:"usedBy"`
}

type ProductCost struct {
	Product   string            `json:"product"`
	Exclusive map[string]string `json:"exclusive"` // currency → monthly amount used only by this product
	Shared    []string          `json:"shared"`    // costed resources shared with other products
}

type CostReport struct {
	Items      []CostRow         `json:"items"`
	Totals     map[string]string `json:"totals"` // currency → monthly total
	Products   []ProductCost     `json:"products"`
	Unassigned []string          `json:"unassigned"` // costed resources no product uses
}

// Cost reports monthly running costs. Amounts are not converted between currencies.
func (a *Archive) Cost() CostReport {
	closures := a.closures()
	rep := CostReport{Items: []CostRow{}, Totals: map[string]string{}, Products: []ProductCost{}, Unassigned: []string{}}
	totals := map[string]*big.Rat{}
	exclusive := map[string]map[string]*big.Rat{}
	shared := map[string][]string{}
	for _, r := range a.Resources {
		var c *Cost
		switch o := r.Object.(type) {
		case *Provider:
			c = o.Spec.Cost
		case *Host:
			if o.Spec.State != "retired" {
				c = o.Spec.Cost
			}
		case *Domain:
			c = o.Spec.Cost
		case *Datastore:
			c = o.Spec.Cost
		}
		m := c.monthly()
		if c == nil {
			continue
		}
		users := usedBy(closures, r.Ref)
		row := CostRow{Ref: r.Ref, Cost: c.String(), UsedBy: users}
		if m != nil {
			row.Monthly = c.Currency + " " + m.FloatString(2)
			if totals[c.Currency] == nil {
				totals[c.Currency] = new(big.Rat)
			}
			totals[c.Currency].Add(totals[c.Currency], m)
		}
		rep.Items = append(rep.Items, row)
		switch {
		case len(users) == 0:
			rep.Unassigned = append(rep.Unassigned, r.Ref)
		case len(users) == 1 && m != nil:
			if exclusive[users[0]] == nil {
				exclusive[users[0]] = map[string]*big.Rat{}
			}
			if exclusive[users[0]][c.Currency] == nil {
				exclusive[users[0]][c.Currency] = new(big.Rat)
			}
			exclusive[users[0]][c.Currency].Add(exclusive[users[0]][c.Currency], m)
		case len(users) > 1:
			for _, u := range users {
				shared[u] = append(shared[u], r.Ref)
			}
		}
	}
	for cur, t := range totals {
		rep.Totals[cur] = t.FloatString(2)
	}
	for p := range closures {
		pc := ProductCost{Product: p, Exclusive: map[string]string{}, Shared: shared[p]}
		for cur, t := range exclusive[p] {
			pc.Exclusive[cur] = t.FloatString(2)
		}
		if pc.Shared == nil {
			pc.Shared = []string{}
		}
		rep.Products = append(rep.Products, pc)
	}
	sort.Slice(rep.Items, func(i, j int) bool { return rep.Items[i].Ref < rep.Items[j].Ref })
	sort.Slice(rep.Products, func(i, j int) bool { return rep.Products[i].Product < rep.Products[j].Product })
	return rep
}

// Orphans lists inventory resources no product uses (retired hosts excluded).
func (a *Archive) Orphans() []string {
	closures := a.closures()
	out := []string{}
	for _, r := range a.Resources {
		if r.Kind == "Product" || r.Kind == "Service" {
			continue
		}
		if h, ok := r.Object.(*Host); ok && h.Spec.State == "retired" {
			continue
		}
		if len(usedBy(closures, r.Ref)) == 0 {
			out = append(out, r.Ref)
		}
	}
	sort.Strings(out)
	return out
}

type ResolvedDeployment struct {
	Environment string    `json:"environment"`
	Runtime     string    `json:"runtime"`
	Cluster     string    `json:"cluster,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	Selector    string    `json:"selector,omitempty"`
	Workload    *Workload `json:"workload,omitempty"`
	Hosts       []string  `json:"hosts,omitempty"`
	Unit        string    `json:"unit,omitempty"`
	Provider    string    `json:"provider,omitempty"`
}

type Description struct {
	Ref         string               `json:"ref"`
	Kind        string               `json:"kind"`
	File        string               `json:"file"`
	Object      Object               `json:"object"`
	Deployments []ResolvedDeployment `json:"deployments,omitempty"`
	ReleaseNote string               `json:"releaseNotes,omitempty"`
	Outgoing    []RefUse             `json:"references"`
	Incoming    []RefUse             `json:"referencedBy"`
	UsedBy      []string             `json:"usedByProducts"`
	Secrets     []SecretUse          `json:"secrets"`
	Notes       []string             `json:"notes"`
	Records     []*ActionRecord      `json:"recentRecords"`
}

// Describe gathers a resource with its relations, notes and latest records.
func (a *Archive) Describe(ref string) (*Description, error) {
	kind, name, _, _, err := ParseRef(ref)
	if err != nil {
		return nil, err
	}
	ref = kind + ":" + name
	r := a.byRef[ref]
	if r == nil {
		return nil, fmt.Errorf("%s does not exist%s", ref, a.suggest(kind))
	}
	d := &Description{Ref: ref, Kind: r.Kind, File: r.File, Object: r.Object, Outgoing: a.Outgoing(ref), Incoming: a.Incoming(ref), UsedBy: a.UsedBy(ref), Secrets: []SecretUse{}, Notes: []string{}, Records: []*ActionRecord{}}
	for _, s := range a.Secrets {
		if s.From == ref {
			d.Secrets = append(d.Secrets, s)
		}
	}
	if s, ok := r.Object.(*Service); ok {
		d.Deployments = a.resolveDeployments(s)
		if s.Spec.Release != nil && s.Spec.Release.Notes != "" {
			d.ReleaseNote = "products/" + s.Metadata.Product + "/" + s.Spec.Release.Notes
		}
	}
	for _, n := range a.Notes {
		if contains(n.Meta.Refs, ref) {
			d.Notes = append(d.Notes, n.File)
		}
	}
	for _, rec := range a.Records {
		for _, t := range rec.Targets {
			if t == ref || strings.HasPrefix(t, ref+"@") {
				d.Records = append(d.Records, rec)
				break
			}
		}
	}
	sort.Slice(d.Records, func(i, j int) bool { return d.Records[i].ID > d.Records[j].ID })
	if len(d.Records) > 10 {
		d.Records = d.Records[:10]
	}
	return d, nil
}

func (a *Archive) resolveDeployments(s *Service) []ResolvedDeployment {
	out := []ResolvedDeployment{}
	for _, dep := range s.Spec.Deployments {
		rd := ResolvedDeployment{Environment: dep.Environment, Runtime: dep.Runtime, Cluster: dep.Cluster, Namespace: dep.Namespace, Selector: dep.Selector, Workload: dep.Workload, Hosts: dep.Hosts, Unit: dep.Unit, Provider: dep.Provider}
		if env, _ := a.environment(s.Metadata.Product, dep.Environment); env != nil {
			if rd.Cluster == "" {
				rd.Cluster = env.Cluster
			}
			if rd.Namespace == "" {
				rd.Namespace = env.Namespace
			}
			if len(rd.Hosts) == 0 && dep.Runtime == "host" {
				rd.Hosts = env.Hosts
			}
		}
		out = append(out, rd)
	}
	return out
}

// Summary returns table columns for a resource listing.
func Summary(r *Resource) (header []string, row []string) {
	j := func(s []string) string {
		if len(s) == 0 {
			return "-"
		}
		return strings.Join(s, ",")
	}
	or := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	trim := func(list []string, prefix string) []string {
		out := make([]string, len(list))
		for i, s := range list {
			out[i] = strings.TrimPrefix(s, prefix)
		}
		return out
	}
	switch o := r.Object.(type) {
	case *Provider:
		return []string{"NAME", "CATEGORIES", "ACCOUNT", "PLAN", "COST"}, []string{r.Name, j(o.Spec.Categories), or(o.Spec.Account), or(o.Spec.Plan), or(o.Spec.Cost.String())}
	case *Host:
		return []string{"NAME", "STATE", "PUBLIC", "ROLES", "PROVIDER"}, []string{r.Name, or(o.Spec.State), or(o.Spec.Addresses.Public), j(o.Spec.Roles), or(strings.TrimPrefix(o.Spec.Provider, "provider:"))}
	case *Cluster:
		return []string{"NAME", "DISTRIBUTION", "HOSTS", "KUBECONFIG"}, []string{r.Name, o.Spec.Distribution, j(trim(o.Spec.Hosts, "host:")), o.Spec.Kubeconfig.Source}
	case *Datastore:
		where := or(strings.TrimPrefix(o.Spec.Runtime.Host+o.Spec.Runtime.Cluster+o.Spec.Runtime.Provider, ""))
		return []string{"NAME", "ENGINE", "RUNTIME", "ENDPOINT"}, []string{r.Name, o.Spec.Engine, where, or(o.Spec.Endpoint)}
	case *Registry:
		return []string{"NAME", "URL"}, []string{r.Name, o.Spec.URL}
	case *Domain:
		exp := "-"
		if o.Spec.Renewal != nil {
			exp = o.Spec.Renewal.ExpiresAt
		}
		return []string{"NAME", "REGISTRAR", "DNS", "EXPIRES"}, []string{r.Name, or(strings.TrimPrefix(o.Spec.Registrar, "provider:")), or(strings.TrimPrefix(o.Spec.DNS, "provider:")), exp}
	case *Product:
		var envs []string
		for _, e := range o.Spec.Environments {
			envs = append(envs, e.Name)
		}
		return []string{"NAME", "STAGE", "TITLE", "ENVIRONMENTS"}, []string{r.Name, o.Spec.Stage, or(o.Metadata.Title), j(envs)}
	case *Service:
		var envs []string
		for _, d := range o.Spec.Deployments {
			envs = append(envs, d.Environment+":"+d.Runtime)
		}
		return []string{"REF", "TYPE", "DEPLOYMENTS"}, []string{r.Ref, o.Spec.Type, j(envs)}
	}
	return []string{"REF"}, []string{r.Ref}
}
