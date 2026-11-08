package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"devopspad/internal/archive"
)

const edgeUsage = `usage: devopspadcli edge routes list --zone <zone>
       devopspadcli edge routes add --zone <zone> --pattern '<zone>/path*' [--script <worker>|none] [--yes]
       devopspadcli edge routes remove --zone <zone> --pattern '<zone>/path*' [--yes]
       devopspadcli edge dns list --zone <zone>
       devopspadcli edge dns set --zone <zone> --name <name|@> --type A|AAAA|CNAME|TXT|MX|CAA (--content <value> | --target host:<name>) [--proxied] [--ttl 1] [--yes]
  Uses the archive's provider:cloudflare token, or --token-ref; changes need Workers Routes Edit / DNS Edit.
  Changes are recorded in the archive (records/actions) with the state before and after.
  First time on this machine (no archive yet):
    devopspadcli archive init ~/opc-archive
    devopspadcli secrets set keychain:archive/providers/cloudflare/api-token
    devopspadcli archive import --cloudflare`

func cmdEdge(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[0] != "routes" && args[0] != "dns") {
		fmt.Fprintln(stderr, edgeUsage)
		return 2
	}
	group, sub := args[0], args[1]
	fs, c := newFlags("edge "+group+" "+sub, stdout, stderr)
	zone := fs.String("zone", "", "Cloudflare zone, e.g. example.com")
	tokenRef := fs.String("token-ref", "", "token reference (default: the provider's credential)")
	provider := fs.String("provider", "provider:cloudflare", "Cloudflare provider ref")
	actor := fs.String("actor", "", "human/<name> or ai/<name> (default: human/<git user.name>)")
	session := fs.String("session", "", "AI session or task ID")
	yes := fs.Bool("yes", false, "apply; without it only the plan is printed")
	pattern := fs.String("pattern", "", "routes: route pattern")
	script := fs.String("script", "none", "routes add: Worker name, or none to send traffic to the origin")
	name := fs.String("name", "", "dns: record name relative to the zone, or @")
	typ := fs.String("type", "", "dns: record type")
	content := fs.String("content", "", "dns: record content")
	target := fs.String("target", "", "dns: host:<name> for an A record to its public address")
	proxied := fs.Bool("proxied", false, "dns: proxy through Cloudflare")
	ttl := fs.Int("ttl", 1, "dns: TTL seconds, 1 = automatic")
	if pos, err := parseInterleaved(fs, args[2:]); err != nil || len(pos) != 0 || *zone == "" {
		fmt.Fprintln(stderr, edgeUsage)
		return 2
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	if *actor == "" {
		*actor = "human/" + slugName(archive.GitUser(a.Dir))
	}
	in := archive.OpInput{Zone: *zone, Provider: *provider, TokenRef: *tokenRef, Actor: *actor, Session: *session, Revision: archive.Revision(a.Dir)}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch group + " " + sub {
	case "routes list":
		routes, err := a.ListRoutes(ctx, in)
		if err != nil {
			return c.fail(err)
		}
		if c.print(routes) >= 0 {
			return 0
		}
		rows := [][]string{}
		for _, r := range routes {
			rows = append(rows, []string{r.Pattern, orNone(r.Script)})
		}
		table(stdout, []string{"PATTERN", "WORKER"}, rows)
		return 0
	case "routes add", "routes remove":
		if *pattern == "" {
			fmt.Fprintln(stderr, edgeUsage)
			return 2
		}
		s := *script
		if s == "none" || sub == "remove" {
			s = ""
		}
		plan, err := a.PlanRoute(ctx, in, sub, *pattern, s)
		if err != nil {
			return c.fail(err)
		}
		fmt.Fprintf(stderr, "Plan: %s route %s → %s on %s\n  before: %s\n  after:  %s\n", sub, plan.Pattern, orNone(plan.Script), plan.Zone, routeText(plan.Before), routeText(plan.After))
		if !*yes {
			fmt.Fprintln(stderr, "Plan only; re-run with --yes to apply")
			c.print(plan)
			return 0
		}
		rec, err := a.ApplyRoute(ctx, in, plan)
		return c.finishRecord(rec, err)
	case "dns list":
		recs, err := a.ListDNS(ctx, in)
		if err != nil {
			return c.fail(err)
		}
		if c.print(recs) >= 0 {
			return 0
		}
		rows := [][]string{}
		for _, r := range recs {
			rows = append(rows, []string{r.Name, r.Type, r.Content, fmt.Sprint(r.Proxied), fmt.Sprint(r.TTL)})
		}
		table(stdout, []string{"NAME", "TYPE", "CONTENT", "PROXIED", "TTL"}, rows)
		return 0
	case "dns set":
		if *name == "" || *typ == "" {
			fmt.Fprintln(stderr, edgeUsage)
			return 2
		}
		plan, err := a.PlanDNS(ctx, in, *name, *typ, *content, *target, *proxied, *ttl)
		if err != nil {
			return c.fail(err)
		}
		before := "(none)"
		if plan.Before != nil {
			before = fmt.Sprintf("%s %s proxied=%v ttl=%d", plan.Before.Type, plan.Before.Content, plan.Before.Proxied, plan.Before.TTL)
		}
		fmt.Fprintf(stderr, "Plan: %s\n  before: %s\n  after:  %s %s proxied=%v ttl=%d\n", plan.Name, before, plan.After.Type, plan.After.Content, plan.After.Proxied, plan.After.TTL)
		if !*yes {
			fmt.Fprintln(stderr, "Plan only; re-run with --yes to apply")
			c.print(plan)
			return 0
		}
		rec, err := a.ApplyDNS(ctx, in, plan)
		return c.finishRecord(rec, err)
	}
	fmt.Fprintln(stderr, edgeUsage)
	return 2
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func routeText(r []archive.WorkerRoute) string {
	if len(r) == 0 {
		return "(none)"
	}
	var out []string
	for _, x := range r {
		out = append(out, x.Pattern+" → "+orNone(x.Script))
	}
	return strings.Join(out, ", ")
}

// finishRecord prints the archive record and reminds to commit it.
func (c *cli) finishRecord(rec *archive.ActionRecord, err error) int {
	if rec != nil {
		if c.print(rec) < 0 {
			fmt.Fprintf(c.stdout, "%s %s (%s); commit it with devopspadcli archive commit -m \"record(%s): %s\"\n", rec.Action, rec.Status, rec.File(), rec.Targets[0], rec.Message)
		}
	}
	if err != nil {
		return c.fail(err)
	}
	return 0
}
