package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCommandArguments(t *testing.T) {
	t.Setenv("DEVOPSPAD_CONFIG_DIR", t.TempDir())
	t.Setenv("DEVOPSPAD_PROJECT_DIR", t.TempDir())
	for _, args := range [][]string{
		{"release", "plan"},
		{"release", "plan", "--environment", "pro", "--application", "api"},                                  // no container
		{"release", "plan", "--environment", "pro", "--target", "api:api:app"},                               // no image or artifact
		{"release", "plan", "--environment", "pro", "--target", "api:api", "--image", "x@sha256:" + hex64()}, // bad target
		{"release", "image", "--environment", "pro", "--target", "a:b:c", "--artifact", "x", "--image", "y"}, // both
		{"release", "apply"},
		{"release", "apply", "--plan", "p", "--timeout", "10m"},
		{"release", "plan", "--environment", "pro", "--target", "a:b:c", "--artifact", "x", "--migrate", "j.yaml", "--migrate-timeout", "1h"},
		{"job", "run", "--environment", "pro"},
	} {
		if code, _, _ := run(args); code != 2 {
			t.Errorf("%v: code=%d, want 2", args, code)
		}
	}
	code, _, errOut := run([]string{"release", "apply", "--plan", "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0"})
	if code != 1 || !strings.Contains(errOut, "already used") {
		t.Fatalf("unknown plan: code=%d stderr=%s", code, errOut)
	}
	code, _, errOut = run([]string{"release", "plan", "--environment", "pro", "--target", "a:b:c", "--image", "x@sha256:" + hex64()})
	if code != 1 || !strings.Contains(errOut, `"error"`) || !strings.Contains(errOut, "project.yaml") {
		t.Fatalf("missing project: code=%d stderr=%s", code, errOut)
	}
}

func hex64() string { return strings.Repeat("a", 64) }

func TestNginxAndAppArguments(t *testing.T) {
	t.Setenv("DEVOPSPAD_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"nginx", "get", "--environment", "pro", "--application", "api"},
		{"nginx", "set", "--environment", "pro", "--application", "api", "-f", "x.conf"},
		{"app", "set", "--application", "api"},
	} {
		if code, _, _ := run(args); code != 2 {
			t.Errorf("%v: code=%d, want 2", args, code)
		}
	}
	dir := t.TempDir()
	t.Setenv("DEVOPSPAD_PROJECT_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "project.yaml"), []byte("apiVersion: devopspad.project/v1\nname: demo\nenvironments: []\napplications:\n  - {name: api, selector: app=api}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run([]string{"app", "set", "--application", "api", "--nginx-path", "/etc/nginx/conf.d/api.conf"})
	if code != 0 || !strings.Contains(out, `"nginxPath":"/etc/nginx/conf.d/api.conf"`) {
		t.Fatalf("app set: code=%d out=%s err=%s", code, out, errOut)
	}
}
