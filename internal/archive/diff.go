package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// Drift is one difference between the archive and the last scan.
type Drift struct {
	Ref      string `json:"ref"`
	Severity string `json:"severity"` // error | warn | info
	Field    string `json:"field,omitempty"`
	Archive  string `json:"archive,omitempty"`
	Observed string `json:"observed,omitempty"`
	Message  string `json:"message"`
}

const certWarningDays = 21

// Diff compares domains with their last scan. Empty refs compares every
// domain and also lists Cloudflare zones missing from the archive.
func (a *Archive) Diff(refs []string, provider string, now time.Time) ([]Drift, error) {
	provider = defaultProvider(provider)
	targets, err := a.selectTargets(refs, nil)
	if err != nil {
		return nil, err
	}
	out := []Drift{}
	add := func(d Drift) { out = append(out, d) }
	today := now.UTC().Format("2006-01-02")
	observed := func(r *Resource) *Observation {
		o, err := a.ReadObservation(r.Ref)
		if err == nil && o != nil && (o.Domain != nil || o.Cluster != nil || o.Host != nil) {
			if t, err := time.Parse(time.RFC3339, o.ObservedAt); err == nil && now.Sub(t) > 7*24*time.Hour {
				add(Drift{Ref: r.Ref, Severity: "info", Message: "last scan is older than 7 days (" + o.ObservedAt[:10] + ")"})
			}
			return o
		}
		msg := "not scanned yet; run devopspadcli scan " + r.Ref
		if err != nil {
			msg = err.Error()
		}
		add(Drift{Ref: r.Ref, Severity: "info", Message: msg})
		return nil
	}
	for _, r := range targets["Cluster"] {
		if o := observed(r); o != nil && o.Cluster != nil {
			a.diffCluster(r, o.Cluster, add)
		}
	}
	for _, r := range targets["Host"] {
		if o := observed(r); o != nil && o.Host != nil {
			a.diffHost(r, o.Host, add)
		}
	}
	for _, r := range targets["Domain"] {
		o := observed(r)
		if o == nil || o.Domain == nil {
			continue
		}
		spec := r.Object.(*Domain).Spec
		f := o.Domain
		for _, e := range f.Errors {
			add(Drift{Ref: r.Ref, Severity: "warn", Message: "scan: " + e})
		}
		// Registration
		if f.ExpiresAt != "" {
			switch {
			case f.ExpiresAt < today:
				add(Drift{Ref: r.Ref, Severity: "error", Field: "spec.renewal.expiresAt", Observed: f.ExpiresAt, Message: "the domain registration has expired"})
			case spec.Renewal == nil:
				add(Drift{Ref: r.Ref, Severity: "warn", Field: "spec.renewal", Observed: f.ExpiresAt, Message: "expiry not recorded; add renewal: { expiresAt: " + f.ExpiresAt + " }"})
			case spec.Renewal.ExpiresAt != f.ExpiresAt:
				add(Drift{Ref: r.Ref, Severity: "warn", Field: "spec.renewal.expiresAt", Archive: spec.Renewal.ExpiresAt, Observed: f.ExpiresAt, Message: "expiry differs (" + f.ExpirySource + "); update the archive"})
			}
		}
		if f.AutoRenew != nil && spec.Renewal != nil && spec.Renewal.AutoRenew != *f.AutoRenew {
			add(Drift{Ref: r.Ref, Severity: "warn", Field: "spec.renewal.autoRenew", Archive: fmt.Sprint(spec.Renewal.AutoRenew), Observed: fmt.Sprint(*f.AutoRenew), Message: "auto-renew setting differs"})
		}
		if f.Registrar != "" && spec.Registrar != f.Registrar {
			add(Drift{Ref: r.Ref, Severity: "warn", Field: "spec.registrar", Archive: spec.Registrar, Observed: f.Registrar, Message: "registered at " + f.Registrar})
		}
		if f.DNS != "" && spec.DNS != f.DNS {
			add(Drift{Ref: r.Ref, Severity: "warn", Field: "spec.dns", Archive: spec.DNS, Observed: f.DNS, Message: "the zone is on " + f.DNS})
		}
		if f.ZoneStatus != "" && f.ZoneStatus != "active" {
			add(Drift{Ref: r.Ref, Severity: "warn", Field: "zone", Observed: f.ZoneStatus, Message: "Cloudflare zone is not active"})
		}
		// Key DNS records, only when DNS data was read.
		if f.DNS != "" {
			for i, rec := range spec.Records {
				a.diffRecord(r.Ref, fmt.Sprintf("spec.records[%d]", i), rec, f.Records, add)
			}
		}
		// Certificates
		for _, c := range f.Certificates {
			switch {
			case c.NotAfter != "" && c.NotAfter < today:
				add(Drift{Ref: r.Ref, Severity: "error", Field: "certificate " + c.Host, Observed: c.NotAfter, Message: "TLS certificate has expired"})
			case !c.Valid && c.Error != "":
				add(Drift{Ref: r.Ref, Severity: "error", Field: "certificate " + c.Host, Message: "TLS check failed: " + c.Error})
			case c.NotAfter != "" && daysBetween(today, c.NotAfter) <= certWarningDays:
				add(Drift{Ref: r.Ref, Severity: "warn", Field: "certificate " + c.Host, Observed: c.NotAfter, Message: fmt.Sprintf("TLS certificate expires in %d days; check automatic renewal", daysBetween(today, c.NotAfter))})
			}
		}
		for i, cert := range spec.Certificates {
			if cert.ExpiresAt == "" {
				continue
			}
			for _, c := range f.Certificates {
				if c.NotAfter != "" && c.NotAfter != cert.ExpiresAt && coversHost(cert.Names, c.Host) {
					add(Drift{Ref: r.Ref, Severity: "info", Field: fmt.Sprintf("spec.certificates[%d].expiresAt", i), Archive: cert.ExpiresAt, Observed: c.NotAfter, Message: "served certificate for " + c.Host + " has a different expiry"})
					break
				}
			}
		}
	}
	if len(refs) == 0 {
		if po := a.readProviderObservation(provider); po != nil {
			for _, z := range po.Zones {
				if a.Get("domain:"+z) == nil {
					add(Drift{Ref: "domain:" + z, Severity: "info", Message: "zone exists in Cloudflare but not in the archive; devopspadcli archive import --cloudflare adds it"})
				}
			}
		}
	}
	rank := map[string]int{"error": 0, "warn": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] < rank[out[j].Severity]
		}
		return out[i].Ref < out[j].Ref
	})
	return out, nil
}

func (a *Archive) diffRecord(ref, field string, rec DNSRecord, observed []ObservedRecord, add func(Drift)) {
	want := strings.TrimSuffix(strings.ToLower(rec.Value), ".")
	if rec.Target != "" {
		want = ""
		if t := a.Get(rec.Target); t != nil && rec.Type == "A" {
			if h, ok := t.Object.(*Host); ok {
				want = h.Spec.Addresses.Public
			}
		}
	}
	var same []string
	for _, o := range observed {
		if o.Name == rec.Name && o.Type == rec.Type {
			if want == "" || strings.EqualFold(o.Content, want) {
				return
			}
			same = append(same, o.Content)
		}
	}
	label := rec.Name + " " + rec.Type
	if len(same) == 0 {
		add(Drift{Ref: ref, Severity: "warn", Field: field, Archive: label + " " + want, Message: "record not found in DNS"})
		return
	}
	add(Drift{Ref: ref, Severity: "warn", Field: field, Archive: want, Observed: strings.Join(same, ","), Message: label + " points elsewhere"})
}

func coversHost(names []string, host string) bool {
	for _, n := range names {
		if ok, _ := path.Match(strings.ToLower(n), host); ok && strings.Count(n, ".") == strings.Count(host, ".") {
			return true
		}
	}
	return false
}

func daysBetween(from, to string) int {
	a, err1 := time.Parse("2006-01-02", from)
	b, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

// RenewalsWithObserved adds what the last scans saw: served certificate
// expiries and domain expiries the archive does not record. It is for local
// reports only; INDEX.md never includes observations.
func (a *Archive) RenewalsWithObserved(now time.Time, within int) []RenewalRow {
	rows := a.Renewals(now, within)
	closures := a.closures()
	today := now.UTC().Format("2006-01-02")
	domains, _ := a.List("Domain", "")
	for _, r := range domains {
		o, err := a.ReadObservation(r.Ref)
		if err != nil || o == nil || o.Domain == nil {
			continue
		}
		add := func(what, due string) {
			days := daysBetween(today, due)
			if within >= 0 && days > within {
				return
			}
			rows = append(rows, RenewalRow{Due: due, Days: days, What: what, Ref: r.Ref, UsedBy: usedBy(closures, r.Ref)})
		}
		if spec := r.Object.(*Domain).Spec; spec.Renewal == nil && o.Domain.ExpiresAt != "" {
			add("domain (observed)", o.Domain.ExpiresAt)
		}
		for _, c := range o.Domain.Certificates {
			if c.NotAfter != "" {
				add("certificate "+c.Host+" (observed)", c.NotAfter)
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

func (a *Archive) diffCluster(r *Resource, f *ClusterFacts, add func(Drift)) {
	for _, e := range f.Errors {
		add(Drift{Ref: r.Ref, Severity: "warn", Message: "scan: " + e})
	}
	if f.Truncated {
		add(Drift{Ref: r.Ref, Severity: "info", Message: fmt.Sprintf("more than %d objects; the comparison is partial", maxClusterItems)})
	}
	for _, n := range f.Nodes {
		if !n.Ready {
			add(Drift{Ref: r.Ref, Severity: "warn", Field: "node " + n.Name, Message: "node is not Ready"})
		}
	}
	if len(f.Errors) > 0 && len(f.Workloads) == 0 {
		return // nothing was read; do not report every deployment as missing
	}
	matched := map[int]bool{}
	deps := a.clusterDeployments(r.Ref)
	services := make([]string, 0, len(deps))
	for s := range deps {
		services = append(services, s)
	}
	sort.Strings(services)
	for _, svc := range services {
		for _, d := range deps[svc] {
			found := false
			for i, w := range f.Workloads {
				if !matchesWorkload(d, w) {
					continue
				}
				found, matched[i] = true, true
				if w.Ready < w.Desired {
					add(Drift{Ref: svc, Severity: "warn", Field: "deployment " + d.Environment, Observed: fmt.Sprintf("%d/%d ready", w.Ready, w.Desired), Message: fmtWorkload(w) + " is not fully ready"})
				}
			}
			if !found {
				what := d.Selector
				if d.Workload != nil {
					what = d.Workload.Kind + "/" + d.Workload.Name
				}
				add(Drift{Ref: svc, Severity: "warn", Field: "deployment " + d.Environment, Archive: d.Namespace + " " + what, Message: "no matching workload in " + r.Ref})
			}
		}
	}
	for i, w := range f.Workloads {
		if !matched[i] && !systemNamespaces[w.Namespace] {
			add(Drift{Ref: r.Ref, Severity: "info", Field: fmtWorkload(w), Observed: strings.Join(w.Images, ","), Message: "running but not in the archive; add it to a service's deployments"})
		}
	}
}

func (a *Archive) diffHost(r *Resource, f *HostFacts, add func(Drift)) {
	for _, e := range f.Errors {
		add(Drift{Ref: r.Ref, Severity: "warn", Message: "scan: " + e})
	}
	if len(f.Errors) > 0 && f.Kernel == "" {
		return
	}
	spec := r.Object.(*Host).Spec
	if spec.OS == "" && f.OS != "" {
		add(Drift{Ref: r.Ref, Severity: "info", Field: "spec.os", Observed: f.OS, Message: "operating system not recorded"})
	}
	size := func(field string, want, got float64) {
		switch {
		case got == 0:
		case want == 0:
			add(Drift{Ref: r.Ref, Severity: "info", Field: field, Observed: fmt.Sprint(got), Message: "not recorded"})
		case got < want*0.9 || got > want*1.1:
			add(Drift{Ref: r.Ref, Severity: "info", Field: field, Archive: fmt.Sprint(want), Observed: fmt.Sprint(got), Message: "differs from the archive"})
		}
	}
	size("spec.size.cpu", float64(spec.Size.CPU), float64(f.CPU))
	size("spec.size.memoryGiB", spec.Size.MemoryGiB, f.MemoryGiB)
	size("spec.size.diskGiB", spec.Size.DiskGiB, f.DiskGiB)
	running := func(names ...string) bool {
		for _, n := range names {
			if contains(f.Services, n) {
				return true
			}
		}
		return false
	}
	for _, role := range spec.Roles {
		ok := true
		switch role {
		case "nginx":
			ok = running("nginx")
		case "k3s-server", "k3s-agent", "k3s":
			ok = running("k3s", "k3s-agent")
		case "docker":
			ok = running("docker")
		case "mysql":
			ok = running("mysql", "mysqld", "mariadb")
		}
		if !ok {
			add(Drift{Ref: r.Ref, Severity: "warn", Field: "spec.roles", Archive: role, Message: "no running " + role + " service"})
		}
	}
	archived := map[string]bool{}
	for _, res := range a.Resources {
		s, ok := res.Object.(*Service)
		if !ok {
			continue
		}
		for _, c := range s.Spec.Configs {
			if c.Type != "nginx" || c.Target.Host != r.Ref {
				continue
			}
			archived[c.Target.Path] = true
			body, ok := a.textFiles["products/"+s.Metadata.Product+"/"+path.Clean(c.Source)]
			if !ok {
				continue
			}
			sum := sha256.Sum256(body)
			want := hex.EncodeToString(sum[:])
			got := ""
			for _, nf := range f.NginxConfs {
				if nf.Path == c.Target.Path {
					got = nf.SHA256
				}
			}
			switch {
			case got == "":
				add(Drift{Ref: res.Ref, Severity: "warn", Field: "config " + c.Name, Archive: c.Target.Path, Message: "file not found on " + r.Ref})
			case got != want:
				add(Drift{Ref: res.Ref, Severity: "warn", Field: "config " + c.Name, Archive: want[:12], Observed: got[:min(12, len(got))], Message: c.Target.Path + " on " + r.Ref + " differs from the archive copy"})
			}
		}
	}
	for _, nf := range f.NginxConfs {
		if !archived[nf.Path] {
			add(Drift{Ref: r.Ref, Severity: "info", Field: nf.Path, Message: "Nginx config not archived; copy it into a service's configs/"})
		}
	}
}
