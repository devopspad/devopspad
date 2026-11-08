package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"devopspad/internal/archive"
	"devopspad/internal/workspace"
	"devopspad/skills"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// version is set at build time with -ldflags "-X main.version=<version>".
var version = "dev"

// stdin is replaced in tests.
var stdin io.Reader = os.Stdin

// parseInterleaved parses flags that may appear before or after positional arguments.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

type cli struct {
	stdout, stderr io.Writer
	archiveDir     *string
	output         *string
}

func newFlags(name string, stdout, stderr io.Writer) (*flag.FlagSet, *cli) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	c := &cli{stdout: stdout, stderr: stderr}
	c.archiveDir = fs.String("archive", "", "archive directory (default: DEVOPSPAD_ARCHIVE, the enclosing archive, or the last one used)")
	c.output = fs.String("o", "text", "output format: text, json or yaml")
	return fs, c
}

func (c *cli) json() bool { return *c.output == "json" }

func (c *cli) fail(err error) int {
	if c.json() {
		_ = json.NewEncoder(c.stderr).Encode(map[string]string{"error": err.Error()})
	} else {
		fmt.Fprintln(c.stderr, "error:", err)
	}
	return 1
}

func (c *cli) usage(text string) int {
	fmt.Fprintln(c.stderr, "usage: devopspadcli "+text)
	return 2
}

func (c *cli) print(v any) int {
	switch *c.output {
	case "json":
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			return c.fail(err)
		}
	case "yaml":
		enc := yaml.NewEncoder(c.stdout)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			return c.fail(err)
		}
	default:
		return -1 // caller prints text
	}
	return 0
}

func (c *cli) dir() (string, error) {
	cfg, _ := configDir()
	return archive.Find(*c.archiveDir, cfg)
}

// load opens the archive; commands still work with validation errors, but say so.
func (c *cli) load() (*archive.Archive, error) {
	dir, err := c.dir()
	if err != nil {
		return nil, err
	}
	a, err := archive.Load(dir)
	if err == nil && len(a.Errors) > 0 && !c.json() {
		fmt.Fprintf(c.stderr, "warning: the archive has %d validation errors; run devopspadcli archive validate\n", len(a.Errors))
	}
	return a, err
}

func table(w io.Writer, header []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

func dash(list []string) string {
	if len(list) == 0 {
		return "-"
	}
	return strings.Join(list, ",")
}

func cmdVersion(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("version", stdout, stderr)
	if _, err := parseInterleaved(fs, args); err != nil {
		return 2
	}
	info := map[string]string{"version": version, "protocol": archive.APIVersion}
	if c.print(info) >= 0 {
		return 0
	}
	fmt.Fprintf(stdout, "devopspadcli %s\nprotocol %s\n", version, archive.APIVersion)
	return 0
}

func cmdArchive(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	sub, args := args[0], args[1:]
	fs, c := newFlags("archive "+sub, stdout, stderr)
	switch sub {
	case "init":
		name := fs.String("name", "", "archive name (default: directory name)")
		pos, err := parseInterleaved(fs, args)
		if err != nil || len(pos) > 1 {
			return c.usage("archive init [dir] [--name <name>]")
		}
		dir := "."
		if len(pos) == 1 {
			dir = pos[0]
		}
		res, err := archive.Init(dir, *name)
		if err != nil {
			return c.fail(err)
		}
		_ = rememberArchive(res.Dir)
		if c.print(res) >= 0 {
			return 0
		}
		if res.Existing {
			fmt.Fprintf(stdout, "%s is already an archive\n", res.Dir)
		} else {
			fmt.Fprintf(stdout, "Created archive %s\n", res.Dir)
		}
		if res.GitInit {
			fmt.Fprintln(stdout, "Initialized a Git repository; add a private remote with git remote add origin <url>")
		}
		if res.HookErr != "" {
			fmt.Fprintln(stdout, "Hook not installed:", res.HookErr)
		} else {
			fmt.Fprintln(stdout, "Installed pre-commit validation hook")
		}
		fmt.Fprintln(stdout, "This is now the default archive for devopspadcli (archive use <dir> changes it).")
		return 0
	case "use":
		pos, err := parseInterleaved(fs, args)
		if err != nil || len(pos) != 1 {
			return c.usage("archive use <dir>")
		}
		dir, err := archive.Find(pos[0], "")
		if err != nil {
			return c.fail(err)
		}
		if err := rememberArchive(dir); err != nil {
			return c.fail(err)
		}
		fmt.Fprintln(stdout, "Default archive:", dir)
		return 0
	case "validate":
		staged := fs.Bool("staged", false, "validate the Git index (what the next commit contains)")
		if pos, err := parseInterleaved(fs, args); err != nil || len(pos) != 0 {
			return c.usage("archive validate [--staged] [-o json]")
		}
		dir, err := c.dir()
		if err != nil {
			return c.fail(err)
		}
		a, err := archive.Check(dir, *staged)
		if err != nil {
			return c.fail(err)
		}
		code := 0
		if len(a.Errors) > 0 {
			code = 1
		}
		report := map[string]any{"ok": code == 0, "errors": a.Errors, "warnings": a.Warnings, "resources": len(a.Resources)}
		if c.print(report) >= 0 {
			return code
		}
		for _, i := range a.Errors {
			fmt.Fprintln(stdout, "error  ", i)
		}
		for _, i := range a.Warnings {
			fmt.Fprintln(stdout, "warning", i)
		}
		if code == 0 {
			fmt.Fprintf(stdout, "ok: %d resources, %d notes, %d records, %d warnings\n", len(a.Resources), len(a.Notes), len(a.Records), len(a.Warnings))
		} else {
			fmt.Fprintf(stdout, "%d errors, %d warnings\n", len(a.Errors), len(a.Warnings))
		}
		return code
	case "index":
		if pos, err := parseInterleaved(fs, args); err != nil || len(pos) != 0 {
			return c.usage("archive index")
		}
		a, err := c.load()
		if err != nil {
			return c.fail(err)
		}
		if err := archive.WriteGenerated(a); err != nil {
			return c.fail(err)
		}
		fmt.Fprintln(stdout, "Updated INDEX.md and AGENTS.md")
		return 0
	case "status":
		fetch := fs.Bool("fetch", false, "fetch from the remote first to report commits behind")
		if pos, err := parseInterleaved(fs, args); err != nil || len(pos) != 0 {
			return c.usage("archive status [--fetch]")
		}
		dir, err := c.dir()
		if err != nil {
			return c.fail(err)
		}
		s, err := archive.GitStatus(dir, *fetch)
		if err != nil {
			return c.fail(err)
		}
		if a, err := archive.Check(dir, false); err == nil {
			s.Errors, s.Warnings = len(a.Errors), len(a.Warnings)
		}
		if c.print(s) >= 0 {
			return 0
		}
		up := s.Upstream
		if up == "" {
			up = "(no upstream)"
		}
		fmt.Fprintf(stdout, "archive  %s\nbranch   %s → %s, ahead %d, behind %d", s.Dir, s.Branch, up, s.Ahead, s.Behind)
		if !s.Fetched {
			fmt.Fprint(stdout, " (not fetched; use --fetch)")
		}
		fmt.Fprintf(stdout, "\nchanges  %d\nvalidate %d errors, %d warnings\n", len(s.Changes), s.Errors, s.Warnings)
		for _, ch := range s.Changes {
			fmt.Fprintln(stdout, "  "+ch)
		}
		return 0
	case "commit":
		msg := fs.String("m", "", "message: <type>(<scope>): <summary>")
		actor := fs.String("actor", "", "human/<name> or ai/<name> (default: human/<git user.name>)")
		session := fs.String("session", "", "AI session or task ID")
		if pos, err := parseInterleaved(fs, args); err != nil || len(pos) != 0 || *msg == "" {
			return c.usage(`archive commit -m "<type>(<scope>): <summary>" [--actor ai/<name>] [--session <id>]`)
		}
		dir, err := c.dir()
		if err != nil {
			return c.fail(err)
		}
		if *actor == "" {
			*actor = "human/" + slugName(archive.GitUser(dir))
		}
		rev, err := archive.Commit(dir, archive.CommitInput{Message: *msg, Actor: *actor, Session: *session})
		if err != nil {
			return c.fail(err)
		}
		if c.print(map[string]string{"commit": rev}) >= 0 {
			return 0
		}
		fmt.Fprintf(stdout, "Committed %s (not pushed; run devopspadcli archive push)\n", rev)
		return 0
	case "pull", "push":
		if pos, err := parseInterleaved(fs, args); err != nil || len(pos) != 0 {
			return c.usage("archive " + sub)
		}
		dir, err := c.dir()
		if err != nil {
			return c.fail(err)
		}
		if sub == "push" {
			if err := archive.Push(dir); err != nil {
				return c.fail(err)
			}
			fmt.Fprintln(stdout, "Pushed")
			return 0
		}
		a, err := archive.Pull(dir)
		if err != nil {
			return c.fail(err)
		}
		fmt.Fprintf(stdout, "Pulled; %d validation errors, %d warnings\n", len(a.Errors), len(a.Warnings))
		if len(a.Errors) > 0 {
			return 1
		}
		return 0
	case "project":
		write := fs.Bool("write", false, "replace project.yaml of the desktop project directory")
		root := fs.String("project", "", "desktop project directory (default: the desktop's)")
		if pos, err := parseInterleaved(fs, args); err != nil || len(pos) != 0 {
			return c.usage("archive project [--project <dir>] [--write]")
		}
		a, err := c.load()
		if err != nil {
			return c.fail(err)
		}
		p, notes := a.ProjectFromArchive(a.Root.Metadata.Name)
		text, err := archive.ProjectYAML(p, a.Root.Metadata.Name)
		if err != nil {
			return c.fail(err)
		}
		for _, n := range notes {
			fmt.Fprintln(stderr, "note:", n)
		}
		if !*write {
			fmt.Fprint(stdout, text)
			fmt.Fprintln(stderr, "Preview only; --write replaces project.yaml of the desktop project")
			return 0
		}
		store, err := projectStore(*root)
		if err != nil {
			return c.fail(err)
		}
		snap, err := store.Read()
		if err != nil {
			return c.fail(err)
		}
		if _, err := store.Save(text, snap.Revision); err != nil {
			return c.fail(err)
		}
		fmt.Fprintf(stdout, "Wrote %d environments and %d applications to %s/project.yaml\n", len(p.Environments), len(p.Applications), store.Root)
		return 0
	case "import":
		ws := fs.Bool("workspace", false, "import desktop connections (hosts, clusters, datastores)")
		fleet := fs.Bool("fleet", false, "import fleet.json machines as hosts")
		records := fs.String("records", "", "mirror the finished release records of a desktop project directory")
		cloudflare := fs.Bool("cloudflare", false, "import every Cloudflare zone as a domain (read-only API calls)")
		tokenRef := fs.String("token-ref", "", "Cloudflare token reference (default: the provider's credential, or "+archive.DefaultCloudflareTokenRef+")")
		cfProvider := fs.String("provider", "provider:cloudflare", "Cloudflare provider ref")
		project := fs.String("project", "", "import a desktop project directory (project.yaml)")
		product := fs.String("product", "", "product name for --project (default: the project name)")
		dry := fs.Bool("dry-run", false, "only list the files that would be written")
		if pos, err := parseInterleaved(fs, args); err != nil || len(pos) != 0 {
			return c.usage("archive import [--workspace] [--fleet] [--project <dir> [--product <name>]] [--cloudflare [--token-ref <ref>]] [--records <project dir>] [--dry-run]")
		}
		dir, err := c.dir()
		if err != nil {
			return c.fail(err)
		}
		cfg, err := configDir()
		if err != nil {
			return c.fail(err)
		}
		if *records != "" {
			store, err := projectStore(*records)
			if err != nil {
				return c.fail(err)
			}
			store.OnFinished = nil
			a, err := archive.Load(dir)
			if err != nil {
				return c.fail(err)
			}
			actor := os.Getenv("DEVOPSPAD_ACTOR")
			if actor == "" {
				actor = "human/" + slugName(archive.GitUser(dir))
			}
			n, skipped, err := a.ImportRecords(store, actor)
			if err != nil {
				return c.fail(err)
			}
			fmt.Fprintf(stdout, "Mirrored %d records into records/actions (%d without a matching archive service)\n", n, skipped)
			if !*ws && !*fleet && *project == "" && !*cloudflare {
				return 0
			}
		}
		res, err := archive.Import(dir, archive.ImportInput{ConfigDir: cfg, Workspace: *ws, Fleet: *fleet, ProjectDir: *project, Product: *product, Cloudflare: *cloudflare, TokenRef: *tokenRef, Provider: *cfProvider, DryRun: *dry})
		if err != nil {
			return c.fail(err)
		}
		if c.print(res) >= 0 {
			return 0
		}
		for _, f := range res.Files {
			verb := map[string]string{"create": "create   ", "unchanged": "unchanged", "exists": "skip     "}[f.Action]
			if *dry && f.Action == "create" {
				verb = "would create"
			}
			fmt.Fprintln(stdout, verb, f.Path)
		}
		if len(res.Todos) > 0 {
			fmt.Fprintln(stdout, "\nTo complete by hand:")
			for _, t := range res.Todos {
				fmt.Fprintln(stdout, "  -", t)
			}
		}
		if !*dry {
			fmt.Fprintln(stdout, "\nNext: edit the files, run devopspadcli archive validate, then archive commit.")
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown archive command: %s\n\n%s", sub, usage)
		return 2
	}
}

func rememberArchive(dir string) error {
	cfg, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg, "archive-path"), []byte(dir+"\n"), 0o600)
}

func slugName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' || r == '@' {
			return r
		}
		return '-'
	}, strings.TrimSpace(s))
	if s == "" {
		return "unknown"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func kindArg(s string) (string, error) {
	for _, k := range []string{"provider", "host", "cluster", "datastore", "registry", "domain", "product", "service"} {
		if s == k || s == k+"s" || (k == "registry" && s == "registries") {
			return strings.ToUpper(k[:1]) + k[1:], nil
		}
	}
	return "", fmt.Errorf("unknown kind %q; kinds: provider, host, cluster, datastore, registry, domain, product, service", s)
}

func cmdGet(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("get", stdout, stderr)
	selector := fs.String("l", "", "label selector, e.g. region=hk,env=pro")
	pos, err := parseInterleaved(fs, args)
	if err != nil || len(pos) < 1 || len(pos) > 2 {
		return c.usage("get <kind> [name] [-l <selector>] [-o json|yaml]")
	}
	kind, err := kindArg(pos[0])
	if err != nil {
		return c.fail(err)
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	if len(pos) == 2 {
		r := a.Get(strings.ToLower(kind) + ":" + pos[1])
		if r == nil {
			return c.fail(fmt.Errorf("%s:%s does not exist; list them with devopspadcli get %s", strings.ToLower(kind), pos[1], strings.ToLower(kind)))
		}
		if c.print(r.Object) >= 0 {
			return 0
		}
		fmt.Fprintf(stdout, "# %s\n%s", r.File, r.Raw())
		return 0
	}
	list, err := a.List(kind, *selector)
	if err != nil {
		return c.fail(err)
	}
	if *c.output != "text" {
		objs := make([]archive.Object, len(list))
		for i, r := range list {
			objs[i] = r.Object
		}
		return c.print(objs)
	}
	if len(list) == 0 {
		fmt.Fprintf(stdout, "No %s resources. Create one with devopspadcli new %s <name>\n", strings.ToLower(kind), strings.ToLower(kind))
		return 0
	}
	header, _ := archive.Summary(list[0])
	rows := make([][]string, len(list))
	for i, r := range list {
		_, rows[i] = archive.Summary(r)
	}
	table(stdout, header, rows)
	return 0
}

func cmdDescribe(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("describe", stdout, stderr)
	pos, err := parseInterleaved(fs, args)
	if err != nil || len(pos) != 1 {
		return c.usage("describe <ref> [-o json]   e.g. describe service:heartie/admin")
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	d, err := a.Describe(pos[0])
	if err != nil {
		return c.fail(err)
	}
	if c.print(d) >= 0 {
		return 0
	}
	r := a.Get(d.Ref)
	fmt.Fprintf(stdout, "# %s (%s)\n%s\n", d.Ref, d.File, strings.TrimRight(string(r.Raw()), "\n"))
	if len(d.Deployments) > 0 {
		fmt.Fprintln(stdout, "\nDeployments (environment defaults applied):")
		rows := [][]string{}
		for _, dep := range d.Deployments {
			where := dep.Cluster
			if dep.Namespace != "" {
				where += "/" + dep.Namespace
			}
			what := dep.Selector
			if dep.Workload != nil {
				what = dep.Workload.Kind + "/" + dep.Workload.Name
			}
			if dep.Runtime == "host" {
				where, what = dash(dep.Hosts), dep.Unit
			}
			if dep.Runtime == "static" {
				where = dep.Provider
			}
			rows = append(rows, []string{dep.Environment, dep.Runtime, orDash(where), orDash(what)})
		}
		table(stdout, []string{"  ENV", "RUNTIME", "WHERE", "WHAT"}, indent(rows))
	}
	if d.ReleaseNote != "" {
		fmt.Fprintf(stdout, "\nRelease notes (read before releasing): %s\n", d.ReleaseNote)
	}
	section := func(title string, uses []archive.RefUse, other func(archive.RefUse) string) {
		if len(uses) == 0 {
			return
		}
		fmt.Fprintf(stdout, "\n%s:\n", title)
		rows := [][]string{}
		for _, u := range uses {
			target := other(u)
			if u.Part != "" && target == u.To {
				target += "#" + u.Part
			}
			rows = append(rows, []string{target, u.Field})
		}
		table(stdout, []string{"  REF", "FIELD"}, indent(rows))
	}
	section("References", d.Outgoing, func(u archive.RefUse) string { return u.To })
	section("Referenced by", d.Incoming, func(u archive.RefUse) string { return u.From })
	fmt.Fprintf(stdout, "\nUsed by products: %s\n", dash(d.UsedBy))
	if len(d.Secrets) > 0 {
		fmt.Fprintln(stdout, "\nSecret references (check with devopspadcli secrets check):")
		for _, s := range d.Secrets {
			fmt.Fprintf(stdout, "  %s  %s\n", s.Field, s.Ref)
		}
	}
	if len(d.Notes) > 0 {
		fmt.Fprintf(stdout, "\nNotes:\n  %s\n", strings.Join(d.Notes, "\n  "))
	}
	if len(d.Records) > 0 {
		fmt.Fprintln(stdout, "\nRecent records:")
		rows := [][]string{}
		for _, rec := range d.Records {
			rows = append(rows, []string{rec.StartedAt, rec.Action, rec.Status, rec.Actor.Type + "/" + rec.Actor.Name, rec.Message})
		}
		table(stdout, []string{"  STARTED", "ACTION", "STATUS", "ACTOR", "MESSAGE"}, indent(rows))
	}
	return 0
}

func indent(rows [][]string) [][]string {
	for _, r := range rows {
		r[0] = "  " + r[0]
	}
	return rows
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func cmdRefs(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("refs", stdout, stderr)
	pos, err := parseInterleaved(fs, args)
	if err != nil || len(pos) != 1 {
		return c.usage("refs <ref> [-o json]   e.g. refs host:hk-1")
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	kind, name, _, _, err := archive.ParseRef(pos[0])
	if err != nil {
		return c.fail(err)
	}
	ref := kind + ":" + name
	if a.Get(ref) == nil {
		return c.fail(fmt.Errorf("%s does not exist", ref))
	}
	uses := a.Incoming(ref)
	result := map[string]any{"ref": ref, "referencedBy": uses, "usedByProducts": a.UsedBy(ref)}
	if c.print(result) >= 0 {
		return 0
	}
	if len(uses) == 0 {
		fmt.Fprintf(stdout, "Nothing references %s.\n", ref)
	} else {
		rows := [][]string{}
		for _, u := range uses {
			rows = append(rows, []string{u.From, u.Field})
		}
		table(stdout, []string{"FROM", "FIELD"}, rows)
	}
	fmt.Fprintf(stdout, "\nUsed by products: %s\n", dash(a.UsedBy(ref)))
	return 0
}

func cmdSchema(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("schema", stdout, stderr)
	pos, err := parseInterleaved(fs, args)
	if err != nil || len(pos) > 1 {
		return c.usage("schema [kind]")
	}
	if len(pos) == 0 {
		fmt.Fprintln(stdout, strings.Join(archive.SchemaKinds(), "\n"))
		return 0
	}
	s, err := archive.Schema(strings.TrimSuffix(pos[0], "s"))
	if err != nil {
		return c.fail(err)
	}
	if *c.output == "yaml" {
		return c.print(s)
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s)
	return 0
}

func cmdNew(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("new", stdout, stderr)
	product := fs.String("product", "", "product of a new service")
	pos, err := parseInterleaved(fs, args)
	if err != nil || len(pos) != 2 {
		return c.usage("new <kind> <name> [--product <product>]")
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	rel, err := a.NewResource(pos[0], pos[1], *product)
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(stdout, "Created %s; edit it, then run devopspadcli archive validate\n", rel)
	return 0
}

func cmdReport(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("report", stdout, stderr)
	within := fs.Int("within", -1, "renewals: days ahead (default: the archive's renewalWarningDays)")
	selector := fs.String("l", "", "matrix: product label selector, e.g. line=tools")
	stage := fs.String("stage", "", "matrix: only products in this stage (idea, building, live, paused, sunset)")
	all := fs.Bool("all", false, "renewals: list every expiry date")
	observed := fs.Bool("observed", false, "renewals: include certificate and domain expiries from the last scan")
	pos, err := parseInterleaved(fs, args)
	if err != nil || len(pos) != 1 {
		return c.usage("report matrix [-l <selector>] [--stage <stage>] | renewals [--within <days>|--all] | cost | orphans [-o json]")
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	switch pos[0] {
	case "matrix":
		rows, err := a.Matrix(*selector, *stage)
		if err != nil {
			return c.fail(err)
		}
		if c.print(rows) >= 0 {
			return 0
		}
		if len(rows) == 0 {
			fmt.Fprintln(stdout, "No matching products.")
			return 0
		}
		out := [][]string{}
		for _, r := range rows {
			out = append(out, []string{r.Product, r.Stage, dash(r.Services), dash(r.Domains), dash(r.Environments), dash(r.Hosts)})
		}
		table(stdout, []string{"PRODUCT", "STAGE", "SERVICES", "DOMAINS", "ENVIRONMENTS", "HOSTS"}, out)
		fmt.Fprintf(stdout, "\n%d products\n", len(rows))
	case "renewals":
		days := *within
		if days < 0 {
			days = a.Root.Spec.Defaults.RenewalWarningDays
			if days == 0 {
				days = 30
			}
		}
		if *all {
			days = -1
		}
		rows := a.Renewals(time.Now(), days)
		if *observed {
			rows = a.RenewalsWithObserved(time.Now(), days)
		}
		if c.print(rows) >= 0 {
			return 0
		}
		if len(rows) == 0 {
			fmt.Fprintf(stdout, "Nothing expires in the next %d days.\n", days)
			return 0
		}
		out := [][]string{}
		for _, r := range rows {
			auto := "-"
			if r.AutoRenew != nil {
				auto = map[bool]string{true: "yes", false: "no"}[*r.AutoRenew]
			}
			out = append(out, []string{r.Due, fmt.Sprint(r.Days), r.What, r.Ref, auto, orDash(r.Cost), dash(r.UsedBy)})
		}
		table(stdout, []string{"DUE", "DAYS", "WHAT", "REF", "AUTO", "COST", "USED BY"}, out)
	case "cost":
		rep := a.Cost()
		if c.print(rep) >= 0 {
			return 0
		}
		out := [][]string{}
		for _, r := range rep.Items {
			out = append(out, []string{r.Ref, r.Cost, orDash(r.Monthly), dash(r.UsedBy)})
		}
		table(stdout, []string{"REF", "COST", "MONTHLY", "USED BY"}, out)
		fmt.Fprintln(stdout, "\nMonthly total (one-off costs excluded, currencies not converted):")
		for _, cur := range sortedMapKeys(rep.Totals) {
			fmt.Fprintf(stdout, "  %s %s\n", cur, rep.Totals[cur])
		}
		fmt.Fprintln(stdout, "\nBy product (exclusive = used only by that product):")
		out = [][]string{}
		for _, p := range rep.Products {
			var ex []string
			for _, cur := range sortedMapKeys(p.Exclusive) {
				ex = append(ex, cur+" "+p.Exclusive[cur])
			}
			out = append(out, []string{"  " + p.Product, dash(ex), dash(p.Shared)})
		}
		table(stdout, []string{"  PRODUCT", "EXCLUSIVE/MONTH", "SHARED"}, out)
		if len(rep.Unassigned) > 0 {
			fmt.Fprintf(stdout, "\nCosted but used by no product: %s\n", strings.Join(rep.Unassigned, ", "))
		}
	case "orphans":
		list := a.Orphans()
		if c.print(list) >= 0 {
			return 0
		}
		if len(list) == 0 {
			fmt.Fprintln(stdout, "Every inventory resource is used by a product.")
			return 0
		}
		fmt.Fprintln(stdout, "Not used by any product (retire, delete, or reference them):")
		for _, r := range list {
			fmt.Fprintln(stdout, "  "+r)
		}
	default:
		return c.usage("report matrix|renewals|cost|orphans")
	}
	return 0
}

func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cmdSecrets(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: devopspadcli secrets check | secrets set keychain:<account>")
		return 2
	}
	sub, args := args[0], args[1:]
	fs, c := newFlags("secrets "+sub, stdout, stderr)
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return 2
	}
	switch sub {
	case "check":
		if len(pos) != 0 {
			return c.usage("secrets check [-o json]")
		}
		a, err := c.load()
		if err != nil {
			return c.fail(err)
		}
		list := a.SecretReport()
		missing := 0
		for _, s := range list {
			if !s.OK {
				missing++
			}
		}
		code := 0
		if missing > 0 {
			code = 1
		}
		if c.print(list) >= 0 {
			return code
		}
		rows := [][]string{}
		for _, s := range list {
			status := "ok"
			if !s.OK {
				status = "MISSING: " + s.Reason
			}
			rows = append(rows, []string{s.Ref, status, strings.Join(s.UsedBy, "; ")})
		}
		table(stdout, []string{"REF", "STATUS", "USED BY"}, rows)
		fmt.Fprintf(stdout, "\n%d references, %d missing on this machine\n", len(list), missing)
		return code
	case "set":
		if len(pos) != 1 || !strings.HasPrefix(pos[0], "keychain:") {
			return c.usage("secrets set keychain:<account>   (reads the value from stdin; never stored in the archive)")
		}
		if err := archive.ParseSecretRef(pos[0]); err != nil {
			return c.fail(err)
		}
		value, err := readSecret(pos[0], stderr)
		if err != nil {
			return c.fail(err)
		}
		if err := archive.KeychainSet(strings.TrimPrefix(pos[0], "keychain:"), value); err != nil {
			return c.fail(fmt.Errorf("could not write the OS keychain: %w", err))
		}
		fmt.Fprintf(stdout, "Stored %s in the OS keychain (service %s)\n", pos[0], archive.KeychainService)
		return 0
	default:
		return c.usage("secrets check | secrets set keychain:<account>")
	}
}

func readSecret(ref string, stderr io.Writer) (string, error) {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprintf(stderr, "Value for %s (input hidden): ", ref)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		if len(b) == 0 {
			return "", errors.New("empty value")
		}
		return string(b), nil
	}
	b, err := io.ReadAll(io.LimitReader(stdin, 64<<10))
	if err != nil {
		return "", err
	}
	v := strings.TrimRight(string(b), "\r\n")
	if v == "" {
		return "", errors.New("empty value on stdin")
	}
	return v, nil
}

func cmdNote(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "add" {
		fmt.Fprintln(stderr, `usage: devopspadcli note add --refs <ref>[,<ref>] -m "<what was done>" [--actor ai/<name>] [--session <id>] [--status succeeded|failed|unknown]`)
		return 2
	}
	fs, c := newFlags("note add", stdout, stderr)
	refs := fs.String("refs", "", "affected resources, comma separated; service:p/s@env selects an environment")
	msg := fs.String("m", "", "what was done")
	actor := fs.String("actor", "", "human/<name> or ai/<name> (default: human/<git user.name>)")
	session := fs.String("session", "", "AI session or task ID")
	status := fs.String("status", "succeeded", "succeeded, failed, cancelled or unknown")
	if pos, err := parseInterleaved(fs, args[1:]); err != nil || len(pos) != 0 {
		return 2
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	if *actor == "" {
		*actor = "human/" + slugName(archive.GitUser(a.Dir))
	}
	var targets []string
	for _, r := range strings.Split(*refs, ",") {
		if r = strings.TrimSpace(r); r != "" {
			targets = append(targets, r)
		}
	}
	rec, err := a.AddNote(archive.NoteInput{Targets: targets, Message: *msg, Actor: *actor, Session: *session, Status: *status, Revision: archive.Revision(a.Dir), Now: time.Now()})
	if err != nil {
		return c.fail(err)
	}
	if c.print(rec) >= 0 {
		return 0
	}
	fmt.Fprintf(stdout, "Recorded %s; commit it with devopspadcli archive commit -m \"record(%s): <summary>\"\n", rec.File(), targets[0])
	return 0
}

func cmdSkill(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: devopspadcli skill install [--scope user|archive] [--dir <dir>] | skill print [file]")
		return 2
	}
	sub, args := args[0], args[1:]
	fs, c := newFlags("skill "+sub, stdout, stderr)
	scope := fs.String("scope", "user", "user: ~/.claude/skills/devopspad; archive: <archive>/.claude/skills/devopspad")
	target := fs.String("dir", "", "install into this directory instead")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return 2
	}
	switch sub {
	case "print":
		name := "SKILL.md"
		if len(pos) == 1 {
			name = pos[0]
		}
		b, err := skills.FS.ReadFile("devopspad/" + name)
		if err != nil {
			return c.fail(fmt.Errorf("no skill file %s; files: %s", name, strings.Join(skillFiles(), ", ")))
		}
		stdout.Write(b)
		return 0
	case "install":
		dir := *target
		switch {
		case dir != "":
		case *scope == "user":
			home, err := os.UserHomeDir()
			if err != nil {
				return c.fail(err)
			}
			dir = filepath.Join(home, ".claude", "skills", "devopspad")
		case *scope == "archive":
			root, err := c.dir()
			if err != nil {
				return c.fail(err)
			}
			dir = filepath.Join(root, ".claude", "skills", "devopspad")
		default:
			return c.usage("skill install [--scope user|archive] [--dir <dir>]")
		}
		for _, f := range skillFiles() {
			b, _ := skills.FS.ReadFile("devopspad/" + f)
			p := filepath.Join(dir, filepath.FromSlash(f))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return c.fail(err)
			}
			if err := os.WriteFile(p, b, 0o644); err != nil {
				return c.fail(err)
			}
		}
		fmt.Fprintf(stdout, "Installed the devopspad skill (%d files) to %s\n", len(skillFiles()), dir)
		return 0
	default:
		return c.usage("skill install | skill print [file]")
	}
}

func skillFiles() []string {
	var out []string
	var walk func(string)
	walk = func(dir string) {
		entries, _ := skills.FS.ReadDir(dir)
		for _, e := range entries {
			p := dir + "/" + e.Name()
			if e.IsDir() {
				walk(p)
			} else {
				out = append(out, strings.TrimPrefix(p, "devopspad/"))
			}
		}
	}
	walk("devopspad")
	sort.Strings(out)
	return out
}

func cmdScan(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("scan", stdout, stderr)
	tokenRef := fs.String("token-ref", "", "Cloudflare token reference (default: the provider's api-token credential)")
	provider := fs.String("provider", "provider:cloudflare", "Cloudflare provider ref")
	kind := fs.String("kind", "", "only domain, cluster or host (default: all)")
	refs, err := parseInterleaved(fs, args)
	if err != nil {
		return c.usage("scan [<ref> ...] [--kind domain|cluster|host] [--token-ref <ref>] [-o json]")
	}
	var kinds []string
	if *kind != "" {
		kinds = []string{*kind}
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	res, err := a.Scan(ctx, archive.ScanInput{Refs: refs, Kinds: kinds, Provider: *provider, TokenRef: *tokenRef, Collector: "devopspadcli/" + version})
	if err != nil {
		return c.fail(err)
	}
	if c.print(res) >= 0 {
		return 0
	}
	rows := [][]string{}
	var infra [][]string
	for _, o := range res.Observations {
		switch {
		case o.Cluster != nil:
			infra = append(infra, []string{o.Ref, fmt.Sprintf("%d nodes, %d workloads", len(o.Cluster.Nodes), len(o.Cluster.Workloads)), fmt.Sprint(len(o.Cluster.Errors))})
			continue
		case o.Host != nil:
			h := o.Host
			infra = append(infra, []string{o.Ref, fmt.Sprintf("%s, %d CPU, %.1f GiB, %d services, %d nginx confs", orDash(h.OS), h.CPU, h.MemoryGiB, len(h.Services), len(h.NginxConfs)), fmt.Sprint(len(h.Errors))})
			continue
		}
		f := o.Domain
		exp := orDash(f.ExpiresAt)
		if f.ExpirySource != "" {
			exp += " (" + f.ExpirySource + ")"
		}
		certs := []string{}
		for _, cert := range f.Certificates {
			st := cert.NotAfter
			if !cert.Valid {
				st = "INVALID " + st
			}
			certs = append(certs, cert.Host+" "+strings.TrimSpace(st))
		}
		rows = append(rows, []string{strings.TrimPrefix(o.Ref, "domain:"), exp, fmt.Sprint(len(f.Records)), dash(certs), fmt.Sprint(len(f.Errors))})
	}
	if len(rows) > 0 {
		table(stdout, []string{"DOMAIN", "EXPIRES", "RECORDS", "CERTIFICATES", "ERRORS"}, rows)
	}
	if len(infra) > 0 {
		if len(rows) > 0 {
			fmt.Fprintln(stdout)
		}
		table(stdout, []string{"REF", "OBSERVED", "ERRORS"}, infra)
	}
	for _, n := range res.Notes {
		fmt.Fprintln(stdout, "note:", n)
	}
	if len(res.Unregistered) > 0 {
		fmt.Fprintf(stdout, "%d Cloudflare zones are not in the archive (devopspadcli archive import --cloudflare --dry-run)\n", len(res.Unregistered))
	}
	fmt.Fprintf(stdout, "\nScanned %d resources; results are local (%s, not committed). Next: devopspadcli diff\n", len(res.Observations), archive.ObservedDir)
	return 0
}

func cmdDiff(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("diff", stdout, stderr)
	provider := fs.String("provider", "provider:cloudflare", "Cloudflare provider ref")
	refs, err := parseInterleaved(fs, args)
	if err != nil {
		return c.usage("diff [<ref> ...] [-o json]")
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	drift, err := a.Diff(refs, *provider, time.Now())
	if err != nil {
		return c.fail(err)
	}
	code := 0
	counts := map[string]int{}
	for _, d := range drift {
		counts[d.Severity]++
	}
	if counts["error"] > 0 {
		code = 1
	}
	if c.print(drift) >= 0 {
		return code
	}
	if len(drift) == 0 {
		fmt.Fprintln(stdout, "The archive matches the last scan.")
		return 0
	}
	rows := [][]string{}
	for _, d := range drift {
		rows = append(rows, []string{d.Severity, d.Ref, orDash(d.Field), orDash(d.Archive), orDash(d.Observed), d.Message})
	}
	table(stdout, []string{"SEVERITY", "REF", "FIELD", "ARCHIVE", "OBSERVED", "MESSAGE"}, rows)
	fmt.Fprintf(stdout, "\n%d errors, %d warnings, %d info\n", counts["error"], counts["warn"], counts["info"])
	return code
}

func cmdConnect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "sync" {
		fmt.Fprintln(stderr, "usage: devopspadcli connect sync [--dry-run]")
		return 2
	}
	fs, c := newFlags("connect sync", stdout, stderr)
	dry := fs.Bool("dry-run", false, "only show what would change")
	if pos, err := parseInterleaved(fs, args[1:]); err != nil || len(pos) != 0 {
		return c.usage("connect sync [--dry-run]")
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	cfg, err := configDir()
	if err != nil {
		return c.fail(err)
	}
	store, err := workspace.NewStore(cfg, workspace.Keychain{})
	if err != nil {
		return c.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	items, err := a.ConnectSync(ctx, store, filepath.Join(cfg, "kube"), *dry)
	if err != nil {
		return c.fail(err)
	}
	if c.print(items) >= 0 {
		return 0
	}
	rows := [][]string{}
	for _, it := range items {
		act := it.Action
		if *dry && (act == "create" || act == "update") {
			act = "would " + act
		}
		rows = append(rows, []string{it.Ref, act, it.Reason})
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "The archive has no clusters or datastores.")
		return 0
	}
	table(stdout, []string{"REF", "ACTION", "NOTE"}, rows)
	if *dry {
		fmt.Fprintln(stdout, "\nClose the DevOpsPad desktop app before applying (an open app keeps its own copy and would overwrite the file), or use Sync connections on its Archive page.")
	} else {
		fmt.Fprintln(stdout, "\nDesktop connections updated. Start DevOpsPad to use them.")
	}
	return 0
}
