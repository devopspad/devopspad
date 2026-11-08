package archive

import (
	"strings"
	"testing"

	"devopspad/internal/ops"

	"github.com/google/uuid"
)

func TestMirrorRecord(t *testing.T) {
	dir := copyExample(t)
	a := load(t, dir)
	r := ops.Record{ID: uuid.NewString(), Action: "release", Application: "heartie-admin", Environment: "heartie-pro", Connection: "hk-k3s", Namespace: "default",
		Workload: "heartie-admin", Container: "app", Image: "ghcr.io/x/admin@sha256:" + strings.Repeat("a", 64), BeforeImage: "ghcr.io/x/admin@sha256:" + strings.Repeat("b", 64),
		Version: "1.4.2", Status: "succeeded", StartedAt: "2026-11-07T10:00:00.123Z", FinishedAt: "2026-11-07T10:01:00Z", Steps: []string{"Deployment is ready", "image-check: a run sha256:aaa"}}
	app := ops.Application{Name: "heartie-admin", Selector: "app=heartie-admin"}
	ok, err := a.MirrorRecord(r, app, "ai/claude-code")
	if err != nil || !ok {
		t.Fatalf("mirror: %v %v", ok, err)
	}
	if ok, err := a.MirrorRecord(r, app, "ai/claude-code"); err != nil || !ok {
		t.Fatalf("second mirror: %v %v", ok, err)
	}
	b := load(t, dir)
	if len(b.Errors) > 0 || len(b.Records) != 1 {
		t.Fatalf("records: %d errors %v", len(b.Records), b.Errors)
	}
	rec := b.Records[0]
	if rec.Action != "release.image" || rec.Targets[0] != "service:heartie/admin@pro" || rec.Actor.Type != "ai" || len(rec.Steps) != 3 {
		t.Fatalf("record: %+v", rec)
	}
	d, _ := b.Describe("service:heartie/admin")
	if len(d.Records) != 1 {
		t.Fatal("record not linked to the service")
	}
	running := r
	running.ID, running.Status = uuid.NewString(), "running"
	if ok, _ := a.MirrorRecord(running, app, "human/me"); ok {
		t.Fatal("running record mirrored")
	}
	other := r
	other.ID, other.Connection = uuid.NewString(), "elsewhere"
	if ok, _ := a.MirrorRecord(other, app, "human/me"); ok {
		t.Fatal("unmatched record mirrored")
	}
}

func TestProjectFromArchive(t *testing.T) {
	a := load(t, example)
	p, notes := a.ProjectFromArchive("example-opc")
	text, err := ProjectYAML(p, "example-opc")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ops.Validate(text)
	if err != nil {
		t.Fatalf("generated project invalid: %v\n%s", err, text)
	}
	if len(parsed.Environments) != 2 || parsed.Environments[0].Name != "heartie-dev" || parsed.Environments[1].Connection != "hk-k3s" {
		t.Fatalf("environments: %+v", parsed.Environments)
	}
	if len(parsed.Applications) != 2 || parsed.Applications[0].Name != "heartie-admin" || parsed.Applications[0].NginxPath != "/etc/nginx/conf.d/heartie-admin.conf" || parsed.Applications[1].Selector != "app=heartie-api" {
		t.Fatalf("applications: %+v", parsed.Applications)
	}
	if len(notes) != 0 {
		t.Fatalf("notes: %v", notes)
	}
}

func TestStoreHookOnlyOnFinal(t *testing.T) {
	s, err := ops.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	s.OnFinished = func(r ops.Record, _ ops.Application) { seen = append(seen, r.Status) }
	r := ops.Record{ID: uuid.NewString(), Status: "running"}
	_ = s.WriteRecord(r)
	r.Status = "succeeded"
	_ = s.WriteRecord(r)
	if strings.Join(seen, ",") != "succeeded" {
		t.Fatalf("hook calls: %v", seen)
	}
}
