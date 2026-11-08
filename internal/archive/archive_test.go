package archive

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const example = "../../examples/archive"

var fixedNow = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	nowUTC = func() time.Time { return fixedNow }
	keychainGet = func(string) (string, error) { return "", os.ErrNotExist }
	os.Exit(m.Run())
}

// copyExample copies the example archive into a temporary directory.
func copyExample(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(example, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(example, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func load(t *testing.T, dir string) *Archive {
	t.Helper()
	a, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func codes(issues []Issue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

func TestExampleArchiveIsValid(t *testing.T) {
	a := load(t, example)
	if os.Getenv("UPDATE_EXAMPLE") == "1" {
		if err := WriteGenerated(a); err != nil {
			t.Fatal(err)
		}
		a = load(t, example)
	}
	if len(a.Errors) > 0 || len(a.Warnings) > 0 {
		t.Fatalf("example archive must be clean (UPDATE_EXAMPLE=1 regenerates INDEX.md):\nerrors: %v\nwarnings: %v", a.Errors, a.Warnings)
	}
	if len(a.Resources) != 20 || len(a.Notes) != 2 {
		t.Fatalf("resources=%d notes=%d", len(a.Resources), len(a.Notes))
	}
}

func TestValidationCodes(t *testing.T) {
	svc := "products/heartie/services/admin.yaml"
	cases := []struct {
		name, file, old, new, code string
	}{
		{"unknown field", svc, "  type: web\n", "  type: web\n  owner: me\n", "schema.unknown_field"},
		{"enum", svc, "type: web", "type: website", "field.enum"},
		{"required", "inventory/clusters/hk-k3s.yaml", "  distribution: k3s\n", "", "field.required"},
		{"date", "inventory/hosts/hk-1.yaml", "2027-01-10", "2027-13-10", "format.date"},
		{"unresolved", svc, "[service:heartie/api]", "[service:heartie/nope]", "ref.unresolved"},
		{"ref kind", svc, "[service:heartie/api]", "[host:hk-1]", "ref.kind"},
		{"ref part", "products/heartie/services/api.yaml", "main-mysql#heartie", "main-mysql#nope", "ref.part"},
		{"ref syntax", svc, "[service:heartie/api]", "[heartie-api]", "ref.syntax"},
		{"environment", svc, "- environment: dev", "- environment: staging", "ref.environment"},
		{"name mismatch", "inventory/hosts/sg-1.yaml", "name: sg-1", "name: sg-2", "name.file_mismatch"},
		{"product mismatch", svc, "product: heartie", "product: thumbsnap", "name.product_mismatch"},
		{"literal password in ssh", "inventory/hosts/hk-1.yaml", "    keyRef: file:~/.ssh/id_ed25519\n", "    keyRef: file:~/.ssh/id_ed25519\n    password: hunter2hunter2\n", "secret.literal"},
		{"literal password in annotations", "inventory/hosts/hk-1.yaml", "  labels:", "  annotations: { password: hunter2hunter2 }\n  labels:", "secret.literal"},
		{"secret pattern in note", "notes/2026-11-02-hk-1-disk.md", "Follow-up", "token gh" + "p_abcdefghijklmnopqrstuvwxyz0123456789", "secret.pattern"},
		{"private key", "products/heartie/notes/release-admin.md", "# Releasing", "-----BEGIN OPENSSH PRIVATE KEY-----\n# Releasing", "secret.pattern"},
		{"config literal", "products/heartie/configs/admin/pro/config.toml", `password = "${secret:keychain:archive/heartie/api/db-password}"`, `DB_PASSWORD = "hunter2hunter2"`, "secret.literal"},
		{"dsn", "products/heartie/configs/admin/pro/config.toml", `host = "10.0.0.2"`, `dsn = "mysql://root:s3cretpass@10.0.0.2/heartie"`, "secret.pattern"},
		{"secret ref", "inventory/providers/stripe.yaml", "keychain:archive/providers/stripe/live-secret-key", "op://vault/stripe", "secret.ref"},
		{"missing config", svc, "configs/admin/pro/config.toml", "configs/admin/pro/missing.toml", "file.missing"},
		{"config path escape", svc, "configs/admin/pro/config.toml", "../thumbsnap/product.yaml", "file.path"},
		{"cycle", "products/heartie/services/api.yaml", "dependsOn: [", "dependsOn: [service:heartie/admin, ", "service.cycle"},
		{"k8s cluster", "products/thumbsnap/services/worker.yaml", "runtime: host, unit: /opt/thumbsnap/docker-compose.yml", "runtime: k8s, selector: app=x", "deployment.k8s"},
		{"kubeconfig", "inventory/clusters/hk-k3s.yaml", "    host: host:hk-1\n", "", "kubeconfig.remote"},
		{"domain record", "inventory/domains/heartie.example.yaml", "type: CNAME, value: heartie.example", "type: CNAME", "domain.record"},
		{"duplicate env", "products/heartie/product.yaml", "    - name: dev", "    - name: pro", "field.duplicate"},
		{"multiple documents", "inventory/hosts/sg-1.yaml", "  roles: [docker]\n", "  roles: [docker]\n---\nfoo: bar\n", "yaml.multiple_documents"},
		{"note front matter", "notes/2026-11-02-hk-1-disk.md", "tags: [incident]", "owner: me", "note.front_matter"},
		{"note ref", "notes/2026-11-02-hk-1-disk.md", "cluster:hk-k3s", "cluster:nope", "ref.unresolved"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := copyExample(t)
			p := filepath.Join(dir, c.file)
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), c.old) {
				t.Fatalf("%s does not contain %q", c.file, c.old)
			}
			if err := os.WriteFile(p, []byte(strings.Replace(string(b), c.old, c.new, 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			a := load(t, dir)
			if !contains(codes(a.Errors), c.code) {
				t.Fatalf("want %s, got %v", c.code, a.Errors)
			}
		})
	}
}

func TestSecretsInInvalidFilesAreReported(t *testing.T) {
	dir := copyExample(t)
	p := filepath.Join(dir, "inventory/hosts/sg-1.yaml")
	if err := os.WriteFile(p, []byte("apiVersion: devopspad.io/v1alpha1\nkind: Host\nnope: [\napi: sk_" + "live_abcdefghijklmnopqrstuvwxyz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := load(t, dir)
	if !contains(codes(a.Errors), "yaml.syntax") || !contains(codes(a.Errors), "secret.pattern") {
		t.Fatalf("got %v", a.Errors)
	}
	for _, e := range a.Errors {
		if strings.Contains(e.Message, "sk_live") {
			t.Fatalf("issue echoes the secret: %v", e)
		}
	}
}

func TestAllowMarkerAndPlaceholders(t *testing.T) {
	dir := copyExample(t)
	p := filepath.Join(dir, "products/heartie/configs/admin/pro/config.toml")
	body := "token = \"${secret:env:API_TOKEN}\"\nexample_password = \"not-a-real-one\" # devopspad:allow-secret\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a := load(t, dir)
	if len(a.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", a.Errors)
	}
	found := false
	for _, s := range a.Secrets {
		found = found || s.Ref == "env:API_TOKEN"
	}
	if !found {
		t.Fatal("placeholder reference not collected")
	}
}

func TestLayoutRules(t *testing.T) {
	dir := copyExample(t)
	if err := os.MkdirAll(filepath.Join(dir, "inventory/servers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", filepath.Join(dir, "products/heartie/configs/admin/pro/hosts")); err != nil {
		t.Fatal(err)
	}
	a := load(t, dir)
	for _, want := range []string{"layout.unknown", "layout.symlink"} {
		if !contains(codes(a.Errors), want) {
			t.Fatalf("want %s, got %v", want, a.Errors)
		}
	}
}

func TestReports(t *testing.T) {
	a := load(t, example)
	m, _ := a.Matrix("", "")
	if len(m) != 2 || m[0].Product != "heartie" || strings.Join(m[1].Hosts, ",") != "hk-1,sg-1" {
		t.Fatalf("matrix: %+v", m)
	}
	r := a.Renewals(fixedNow, 30)
	if len(r) != 1 || r[0].Ref != "provider:resend" || r[0].Days != 24 {
		t.Fatalf("renewals: %+v", r)
	}
	if all := a.Renewals(fixedNow, -1); len(all) != 5 {
		t.Fatalf("all renewals: %+v", all)
	}
	cost := a.Cost()
	if cost.Totals["USD"] != "26.04" || cost.Totals["CNY"] != "39.00" {
		t.Fatalf("totals: %+v", cost.Totals)
	}
	if cost.Products[0].Exclusive["USD"] != "21.17" || strings.Join(cost.Products[0].Shared, ",") != "host:hk-1,provider:github" {
		t.Fatalf("heartie cost: %+v", cost.Products[0])
	}
	if got := strings.Join(a.UsedBy("datastore:main-mysql"), ","); got != "product:heartie,product:thumbsnap" {
		t.Fatalf("used by: %s", got)
	}
	if o := a.Orphans(); len(o) != 0 {
		t.Fatalf("orphans: %v", o)
	}
	in := a.Incoming("host:hk-1")
	if len(in) != 7 {
		t.Fatalf("incoming: %+v", in)
	}
}

func TestOrphansAndExpired(t *testing.T) {
	dir := copyExample(t)
	nowUTC = func() time.Time { return time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC) }
	defer func() { nowUTC = func() time.Time { return fixedNow } }()
	if err := os.WriteFile(filepath.Join(dir, "inventory/hosts/spare.yaml"), []byte("apiVersion: devopspad.io/v1alpha1\nkind: Host\nmetadata:\n  name: spare\nspec:\n  state: standby\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := load(t, dir)
	if o := a.Orphans(); strings.Join(o, ",") != "host:spare" {
		t.Fatalf("orphans: %v", o)
	}
	if !contains(codes(a.Warnings), "renewal.expired") || len(a.Errors) > 0 {
		t.Fatalf("errors %v warnings %v", a.Errors, a.Warnings)
	}
}

func TestDescribeResolvesEnvironmentDefaults(t *testing.T) {
	a := load(t, example)
	d, err := a.Describe("service:heartie/admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Deployments) != 2 || d.Deployments[1].Cluster != "cluster:hk-k3s" || d.Deployments[1].Namespace != "heartie-dev" {
		t.Fatalf("deployments: %+v", d.Deployments)
	}
	if d.ReleaseNote != "products/heartie/notes/release-admin.md" || len(d.Notes) != 1 {
		t.Fatalf("notes: %q %v", d.ReleaseNote, d.Notes)
	}
	w, _ := a.Describe("service:thumbsnap/worker")
	if strings.Join(w.Deployments[0].Hosts, ",") != "host:sg-1" {
		t.Fatalf("host deployment: %+v", w.Deployments)
	}
	if _, err := a.Describe("host:nope"); err == nil || !strings.Contains(err.Error(), "host:hk-1") {
		t.Fatalf("want suggestion, got %v", err)
	}
}

func TestSkeletonsValidate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RootFile), []byte("apiVersion: devopspad.io/v1alpha1\nkind: Archive\nmetadata:\n  name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !contains(codes(load(t, dir).Warnings), "layout.gitignore") {
		t.Fatal("missing .devopspad/ in .gitignore not reported")
	}
	for _, k := range []string{"provider", "host", "cluster", "datastore", "registry", "domain", "product"} {
		name := "x-" + k
		if k == "domain" {
			name = "x.example"
		}
		if _, err := load(t, dir).NewResource(k, name, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := load(t, dir).NewResource("service", "web", "x-product"); err != nil {
		t.Fatal(err)
	}
	a := load(t, dir)
	if len(a.Errors) > 0 || len(a.Resources) != 8 {
		t.Fatalf("skeletons must validate: %v (%d resources)", a.Errors, len(a.Resources))
	}
	if _, err := a.NewResource("service", "api", "missing"); err == nil {
		t.Fatal("service of a missing product accepted")
	}
}

func TestSchema(t *testing.T) {
	for _, k := range SchemaKinds() {
		s, err := Schema(k)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := json.Marshal(s); err != nil || s["additionalProperties"] != false {
			t.Fatalf("%s: %v %v", k, err, s["additionalProperties"])
		}
	}
	s, _ := Schema("service")
	spec := s["properties"].(map[string]any)["spec"].(map[string]any)
	if !contains(spec["required"].([]string), "type") {
		t.Fatalf("service spec required: %v", spec["required"])
	}
}

func TestULID(t *testing.T) {
	a := NewULID(fixedNow)
	b := NewULID(fixedNow.Add(time.Millisecond))
	if !ulidRE.MatchString(a) || a[:10] >= b[:10] {
		t.Fatalf("%s %s", a, b)
	}
	if a[:3] != "01M" {
		t.Fatalf("timestamp prefix %s", a)
	}
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func TestGitWorkflow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	dir := filepath.Join(t.TempDir(), "opc")
	res, err := Init(dir, "")
	if err != nil || !res.GitInit || res.Hook == "" {
		t.Fatalf("init: %+v %v", res, err)
	}
	// The installed hook would run this test binary; commits are validated by Commit itself.
	gitT(t, dir, "config", "core.hooksPath", "/dev/null")
	if _, err := Commit(dir, CommitInput{Message: "init", Actor: "human/test"}); err == nil {
		t.Fatal("message without type accepted")
	}
	if _, err := Commit(dir, CommitInput{Message: "archive(archive): init", Actor: "human/test"}); err != nil {
		t.Fatal(err)
	}
	a := load(t, dir)
	if _, err := a.NewResource("host", "hk-1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(dir, CommitInput{Message: "archive(host:hk-1): add", Actor: "ai/claude-code", Session: "s1"}); err != nil {
		t.Fatal(err)
	}
	if msg := gitT(t, dir, "log", "-1", "--format=%B"); !strings.Contains(msg, "DevOpsPad-Actor: ai/claude-code") || !strings.Contains(msg, "DevOpsPad-Session: s1") {
		t.Fatalf("trailers missing: %q", msg)
	}
	a = load(t, dir)
	if _, err := a.AddNote(NoteInput{Targets: []string{"host:hk-1"}, Message: "rebooted", Actor: "human/test", Now: fixedNow}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddNote(NoteInput{Targets: []string{"host:hk-1"}, Message: "key sk_" + "live_abcdefghijklmnopqrstuvwxyz", Actor: "human/test", Now: fixedNow}); err == nil {
		t.Fatal("note containing a secret accepted")
	}
	if _, err := Commit(dir, CommitInput{Message: "record(host:hk-1): reboot", Actor: "human/test"}); err != nil {
		t.Fatal(err)
	}
	rec := gitT(t, dir, "ls-files", "records/actions/*.json")
	if !strings.HasPrefix(rec, "records/actions/2026/11/") {
		t.Fatalf("record path %q", rec)
	}
	p := filepath.Join(dir, rec)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), "rebooted", "edited", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ = Check(dir, false)
	if !contains(codes(a.Errors), "record.immutable") {
		t.Fatalf("edit of finished record not detected: %v", a.Errors)
	}
	gitT(t, dir, "checkout", "--", rec)
	gitT(t, dir, "rm", "-q", rec)
	a, _ = Check(dir, true)
	if !contains(codes(a.Errors), "record.immutable") {
		t.Fatalf("staged delete not detected: %v", a.Errors)
	}
	gitT(t, dir, "reset", "-q", "HEAD", "--", rec)
	gitT(t, dir, "checkout", "--", rec)

	// The staged check sees the index, not the working tree.
	if err := os.WriteFile(filepath.Join(dir, "notes", "leak.md"), []byte("gh" + "p_abcdefghijklmnopqrstuvwxyz0123456789\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if a, _ = Check(dir, true); len(a.Errors) > 0 {
		t.Fatalf("unstaged file affected the staged check: %v", a.Errors)
	}
	gitT(t, dir, "add", "notes/leak.md")
	if a, _ = Check(dir, true); !contains(codes(a.Errors), "secret.pattern") {
		t.Fatalf("staged secret not detected: %v", a.Errors)
	}
	if _, err := Commit(dir, CommitInput{Message: "note(host:hk-1): leak", Actor: "human/test"}); err == nil {
		t.Fatal("commit with a secret accepted")
	}
	s, err := GitStatus(dir, false)
	if err != nil || s.Branch == "" || len(s.Changes) == 0 {
		t.Fatalf("status: %+v %v", s, err)
	}
	// init on an existing archive only reinstalls the hook, as after a clone.
	if res, err := Init(dir, ""); err != nil || !res.Existing {
		t.Fatalf("re-init: %+v %v", res, err)
	}
}

func TestImport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RootFile), []byte("apiVersion: devopspad.io/v1alpha1\nkind: Archive\nmetadata:\n  name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, proj := t.TempDir(), t.TempDir()
	ws := `{"version":3,"connections":[
 {"id":"c1","name":"heartie-pro","kind":"k3s","kubeconfig":"/k/hk.yaml","environment":"heartie-pro","ssh":{"enabled":true,"host":"203.0.113.10","port":22,"username":"root","authMethod":"key","privateKeyPath":"/k/id","hasPassphrase":true}},
 {"id":"c2","name":"HK MySQL","kind":"mysql","host":"127.0.0.1","port":3306,"username":"root","database":"heartie","hasPassword":true,"ssh":{"enabled":true,"host":"203.0.113.10","port":22,"username":"root","authMethod":"key","privateKeyPath":"/k/id"}},
 {"id":"c3","name":"cache","kind":"redis","host":"10.0.0.9","port":6379}]}`
	files := map[string]string{
		filepath.Join(cfg, "workspace.json"): ws,
		filepath.Join(cfg, "fleet.json"):     `{"fleets":[{"name":"hk","machines":[{"name":"hk-main","connectionId":"c1","roles":["cluster"]}]}]}`,
		filepath.Join(proj, "project.yaml"):  "apiVersion: devopspad.project/v1\nname: Heartie\nenvironments:\n  - {name: pro, connection: heartie-pro, namespace: default}\napplications:\n  - {name: heartie-admin, selector: app=heartie-admin, nginxPath: /etc/nginx/conf.d/a.conf}\n",
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in := ImportInput{ConfigDir: cfg, Workspace: true, Fleet: true, ProjectDir: proj}
	dry := in
	dry.DryRun = true
	res, err := Import(dir, dry)
	if err != nil || len(res.Files) != 6 {
		t.Fatalf("dry run: %+v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inventory/hosts/hk-main.yaml")); err == nil {
		t.Fatal("dry run wrote files")
	}
	if _, err = Import(dir, in); err != nil {
		t.Fatal(err)
	}
	a := load(t, dir)
	if len(a.Errors) > 0 {
		t.Fatalf("imported archive invalid: %v", a.Errors)
	}
	h := a.Get("host:hk-main").Object.(*Host)
	if h.Spec.SSH.PassphraseRef != "keychain:c1:ssh-passphrase" || h.Metadata.Labels["fleet"] != "hk" {
		t.Fatalf("host: %+v", h)
	}
	ds := a.Get("datastore:hk-mysql").Object.(*Datastore)
	if ds.Spec.Runtime.Host != "host:hk-main" || ds.Spec.Credentials[0].Ref != "keychain:c2" || ds.Spec.Username != "root" || ds.Metadata.Annotations[ConnectionAnnotation] != "c2" {
		t.Fatalf("datastore: %+v", ds.Spec)
	}
	if a.Get("service:heartie/admin") == nil || a.Get("product:heartie").Object.(*Product).Spec.Environments[0].Cluster != "cluster:heartie-pro" {
		t.Fatal("project not imported")
	}
	again, err := Import(dir, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range again.Files {
		if f.Action != "unchanged" {
			t.Fatalf("repeated import not idempotent: %+v", f)
		}
	}
}
