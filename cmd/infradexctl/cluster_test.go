package main

import (
	"os"
	"path/filepath"
	"testing"

	"infradex/internal/ops"
)

func TestExplicitApplicationMappingBeforeFirstDeployment(t *testing.T) {
	s, err := ops.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := ops.Environment{Name: "pro", Connection: "cluster", Namespace: "default"}
	app := ops.Application{Name: "new-app", Selector: "app=new-app"}
	for i := 0; i < 2; i++ {
		snap, err := attachConfiguredApplication(s.Root, env, app)
		if err != nil || len(snap.Project.Applications) != 1 || len(snap.Project.Environments) != 1 {
			t.Fatalf("idempotent mapping: %+v %v", snap.Project, err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(s.Root, "project.yaml"))
	for _, bad := range []ops.Application{{Name: "new-app", Selector: "app=different"}, {Name: "other", Selector: "invalid==="}} {
		if _, err := attachConfiguredApplication(s.Root, env, bad); err == nil {
			t.Fatal("invalid or conflicting mapping accepted")
		}
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "project.yaml"))
	if string(before) != string(after) {
		t.Fatal("failed mapping changed project")
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := attachConfiguredApplication(missing, env, app); err == nil {
		t.Fatal("missing project accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing project created")
	}
}

func TestExplicitMappingFlagsRejectedBeforeConnectionAccess(t *testing.T) {
	for _, args := range [][]string{
		{"inspect", "--connection", "missing", "--application", "app", "--selector", "app=app"},
		{"attach", "--connection", "missing", "--environment", "pro", "--application", "app"},
		{"attach", "--connection", "missing", "--environment", "pro", "--selector", "app=app"},
		{"attach", "--connection", "missing", "--application", "app", "--selector", "app=app"},
	} {
		code, _, errOut := run(append([]string{"cluster"}, args...))
		if code != 2 {
			t.Fatalf("expected usage failure: %v: %d %s", args, code, errOut)
		}
	}
}
