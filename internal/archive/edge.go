package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EdgeInput selects a Cloudflare zone and who acts on it.
type EdgeInput struct {
	Zone     string
	Provider string // default provider:cloudflare
	TokenRef string // a token with Workers Routes Edit / DNS Edit for changes
	Actor    string // human/<name> or ai/<name>
	Session  string
	Revision string
}

func (a *Archive) edgeZone(ctx context.Context, in EdgeInput) (cloudflare, cfZone, error) {
	token, _, err := a.cloudflareToken(defaultProvider(in.Provider), in.TokenRef)
	if err != nil {
		return cloudflare{}, cfZone{}, err
	}
	cf := cloudflare{token: token}
	zones, err := cf.zones(ctx)
	if err != nil {
		return cf, cfZone{}, err
	}
	zone := strings.ToLower(strings.TrimSuffix(in.Zone, "."))
	for _, z := range zones {
		if strings.EqualFold(z.Name, zone) {
			return cf, z, nil
		}
	}
	return cf, cfZone{}, fmt.Errorf("zone %s is not in this Cloudflare account", zone)
}

// ListRoutes returns a zone's Workers routes; an empty script is "no Worker".
func (a *Archive) ListRoutes(ctx context.Context, in EdgeInput) ([]WorkerRoute, error) {
	cf, z, err := a.edgeZone(ctx, in)
	if err != nil {
		return nil, err
	}
	routes, err := cf.workerRoutes(ctx, z.ID)
	if err != nil {
		return nil, fmt.Errorf("%w (the token needs Workers Routes Read)", err)
	}
	sortRoutes(routes)
	return routes, nil
}

func sortRoutes(r []WorkerRoute) {
	sort.Slice(r, func(i, j int) bool { return r[i].Pattern < r[j].Pattern })
}

// RoutePlan adds or removes one Workers route.
type RoutePlan struct {
	Zone    string        `json:"zone"`
	Action  string        `json:"action"` // add | remove
	Pattern string        `json:"pattern"`
	Script  string        `json:"script,omitempty"`
	Before  []WorkerRoute `json:"before"`
	After   []WorkerRoute `json:"after"`
	zoneID  string
	routeID string
}

func (a *Archive) PlanRoute(ctx context.Context, in EdgeInput, action, pattern, script string) (RoutePlan, error) {
	p := RoutePlan{Zone: strings.ToLower(in.Zone), Action: action, Pattern: pattern, Script: script}
	if action != "add" && action != "remove" {
		return p, errors.New("action must be add or remove")
	}
	host, _, _ := strings.Cut(strings.TrimPrefix(pattern, "*."), "/")
	host = strings.TrimPrefix(host, "*")
	if pattern == "" || !(host == p.Zone || strings.HasSuffix(host, "."+p.Zone)) {
		return p, fmt.Errorf("pattern %q must be on zone %s, e.g. %s/api*", pattern, p.Zone, p.Zone)
	}
	cf, z, err := a.edgeZone(ctx, in)
	if err != nil {
		return p, err
	}
	p.zoneID = z.ID
	if p.Before, err = cf.workerRoutes(ctx, z.ID); err != nil {
		return p, fmt.Errorf("%w (the token needs Workers Routes Read)", err)
	}
	sortRoutes(p.Before)
	for _, r := range p.Before {
		if r.Pattern == pattern {
			p.routeID = r.ID
			if action == "add" {
				return p, fmt.Errorf("route %s already exists (worker %q); remove it first", pattern, orNone(r.Script))
			}
		} else {
			p.After = append(p.After, r)
		}
	}
	if action == "remove" && p.routeID == "" {
		return p, fmt.Errorf("route %s does not exist", pattern)
	}
	if action == "add" {
		p.After = append(p.After, WorkerRoute{Pattern: pattern, Script: script})
		sortRoutes(p.After)
	}
	if p.After == nil {
		p.After = []WorkerRoute{}
	}
	return p, nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func routeLines(r []WorkerRoute) string {
	if len(r) == 0 {
		return "(none)"
	}
	var out []string
	for _, x := range r {
		out = append(out, x.Pattern+" → "+orNone(x.Script))
	}
	return strings.Join(out, ", ")
}

func sameRoutes(a, b []WorkerRoute) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Pattern != b[i].Pattern || a[i].Script != b[i].Script {
			return false
		}
	}
	return true
}

// ApplyRoute re-reads the routes, refuses if they changed since the plan,
// applies the change and records the route list before and after.
func (a *Archive) ApplyRoute(ctx context.Context, in EdgeInput, p RoutePlan) (*ActionRecord, error) {
	cf, z, err := a.edgeZone(ctx, in)
	if err != nil {
		return nil, err
	}
	now, err := cf.workerRoutes(ctx, z.ID)
	if err != nil {
		return nil, err
	}
	sortRoutes(now)
	if !sameRoutes(now, p.Before) {
		return nil, errors.New("the zone's routes changed since the plan; plan again")
	}
	start := time.Now()
	if p.Action == "add" {
		err = cf.createRoute(ctx, z.ID, p.Pattern, p.Script)
	} else {
		err = cf.deleteRoute(ctx, z.ID, p.routeID)
	}
	if err != nil {
		err = fmt.Errorf("%w (the token needs Workers Routes Edit)", err)
	}
	after, listErr := cf.workerRoutes(ctx, z.ID)
	sortRoutes(after)
	steps := []Step{{Name: "before", Status: "succeeded", Message: routeLines(p.Before)}}
	action := Step{Name: p.Action + " " + p.Pattern + " → " + orNone(p.Script), Status: "succeeded"}
	if err != nil {
		action.Status, action.Message = "failed", err.Error()
	}
	steps = append(steps, action)
	if listErr == nil {
		steps = append(steps, Step{Name: "after", Status: "succeeded", Message: routeLines(after)})
	}
	rec, recErr := a.appendRecord(in, "edge.route."+p.Action, a.zoneTargets(p.Zone), statusOf(err), fmt.Sprintf("%s route %s", p.Action, p.Pattern), steps, start)
	if recErr != nil {
		return rec, errors.Join(err, recErr)
	}
	return rec, err
}

func statusOf(err error) string {
	if err != nil {
		return "failed"
	}
	return "succeeded"
}

func (a *Archive) zoneTargets(zone string) []string {
	if a.Get("domain:"+zone) != nil {
		return []string{"domain:" + zone}
	}
	return []string{"provider:cloudflare"}
}

// ListDNS returns every record of a zone.
func (a *Archive) ListDNS(ctx context.Context, in EdgeInput) ([]CloudflareRecord, error) {
	cf, z, err := a.edgeZone(ctx, in)
	if err != nil {
		return nil, err
	}
	recs, err := cf.recordsFull(ctx, z.ID)
	sort.Slice(recs, func(i, j int) bool { return recs[i].Name+recs[i].Type < recs[j].Name+recs[j].Type })
	return recs, err
}

// DNSPlan creates or updates one DNS record.
type DNSPlan struct {
	Zone     string            `json:"zone"`
	Name     string            `json:"name"` // fully qualified
	Before   *CloudflareRecord `json:"before,omitempty"`
	After    CloudflareRecord  `json:"after"`
	zoneID   string
	recordID string
}

var dnsTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true, "TXT": true, "MX": true, "CAA": true}

// PlanDNS prepares a record. target may be a host reference (A record to its
// public address) instead of content.
func (a *Archive) PlanDNS(ctx context.Context, in EdgeInput, name, typ, content, target string, proxied bool, ttl int) (DNSPlan, error) {
	zone := strings.ToLower(in.Zone)
	p := DNSPlan{Zone: zone}
	typ = strings.ToUpper(typ)
	if !dnsTypes[typ] {
		return p, fmt.Errorf("--type must be one of A, AAAA, CNAME, TXT, MX, CAA")
	}
	if target != "" {
		var h *Host
		if r := a.Get(target); r != nil {
			h, _ = r.Object.(*Host)
		}
		if h == nil || h.Spec.Addresses.Public == "" || typ != "A" {
			return p, fmt.Errorf("--target must be a host with addresses.public, for an A record")
		}
		content = h.Spec.Addresses.Public
	}
	if content == "" {
		return p, errors.New("--content or --target is required")
	}
	fqdn := strings.ToLower(strings.TrimSuffix(name, "."))
	if fqdn == "@" || fqdn == "" {
		fqdn = zone
	} else if !strings.HasSuffix(fqdn, "."+zone) && fqdn != zone {
		fqdn += "." + zone
	}
	if ttl == 0 {
		ttl = 1 // automatic
	}
	p.Name = fqdn
	p.After = CloudflareRecord{Name: fqdn, Type: typ, Content: content, Proxied: proxied, TTL: ttl}
	cf, z, err := a.edgeZone(ctx, in)
	if err != nil {
		return p, err
	}
	p.zoneID = z.ID
	records, err := cf.recordsFull(ctx, z.ID)
	if err != nil {
		return p, err
	}
	for i, r := range records {
		if strings.EqualFold(strings.TrimSuffix(r.Name, "."), fqdn) && r.Type == typ {
			if p.Before != nil {
				return p, fmt.Errorf("%s has several %s records; edit them in Cloudflare", fqdn, typ)
			}
			p.Before = &records[i]
			p.recordID = r.ID
		}
	}
	if p.Before != nil && p.Before.Content == content && p.Before.Proxied == proxied && p.Before.TTL == ttl {
		return p, errors.New("the record already has this value")
	}
	return p, nil
}

func recordLine(r *CloudflareRecord) string {
	if r == nil {
		return "(none)"
	}
	return fmt.Sprintf("%s %s %s proxied=%v ttl=%d", r.Name, r.Type, r.Content, r.Proxied, r.TTL)
}

func (a *Archive) ApplyDNS(ctx context.Context, in EdgeInput, p DNSPlan) (*ActionRecord, error) {
	cf, z, err := a.edgeZone(ctx, in)
	if err != nil {
		return nil, err
	}
	records, err := cf.recordsFull(ctx, z.ID)
	if err != nil {
		return nil, err
	}
	var now *CloudflareRecord
	for i, r := range records {
		if r.ID == p.recordID && p.recordID != "" {
			now = &records[i]
		}
		if p.recordID == "" && strings.EqualFold(strings.TrimSuffix(r.Name, "."), p.Name) && r.Type == p.After.Type {
			return nil, errors.New("the record was created by someone else since the plan; plan again")
		}
	}
	if p.Before != nil && (now == nil || now.Content != p.Before.Content || now.Proxied != p.Before.Proxied || now.TTL != p.Before.TTL) {
		return nil, errors.New("the record changed since the plan; plan again")
	}
	start := time.Now()
	err = cf.upsertRecord(ctx, z.ID, p.recordID, p.After)
	if err != nil {
		err = fmt.Errorf("%w (the token needs DNS Edit)", err)
	}
	set := Step{Name: "set " + recordLine(&p.After), Status: "succeeded"}
	if err != nil {
		set.Status, set.Message = "failed", err.Error()
	}
	steps := []Step{{Name: "before", Status: "succeeded", Message: recordLine(p.Before)}, set}
	rec, recErr := a.appendRecord(in, "edge.dns.set", a.zoneTargets(p.Zone), statusOf(err), "set DNS "+p.After.Type+" "+p.Name, steps, start)
	if recErr != nil {
		return rec, errors.Join(err, recErr)
	}
	return rec, err
}

// appendRecord writes one finished ActionRecord to records/actions.
func (a *Archive) appendRecord(in EdgeInput, action string, targets []string, status, message string, steps []Step, start time.Time) (*ActionRecord, error) {
	if !actorRE.MatchString(in.Actor) {
		return nil, errors.New("actor must be human/<name> or ai/<name>")
	}
	typ, name, _ := strings.Cut(in.Actor, "/")
	machine, _ := os.Hostname()
	rec := &ActionRecord{APIVersion: APIVersion, Kind: "ActionRecord", ID: NewULID(start), Action: action, Targets: targets,
		Actor: Actor{Type: typ, Name: name, Session: in.Session, Machine: machine}, ArchiveRevision: in.Revision, Status: status, Message: message,
		Steps: steps, StartedAt: start.Format(time.RFC3339), FinishedAt: time.Now().Format(time.RFC3339)}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return nil, err
	}
	rel, _ := recordPath(rec.ID, rec.StartedAt)
	full := filepath.Join(a.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}
	rec.file = rel
	return rec, writeAtomic(full, append(b, '\n'), 0o644)
}
