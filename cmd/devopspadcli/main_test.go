package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	okPath := filepath.Join("..", "..", "internal", "release", "testdata", "ok.release.yaml")
	code, out, errOut := run([]string{"release", "validate", "-f", okPath})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "service=heartie-admin") {
		t.Fatalf("stdout = %q", out)
	}
	if !regexp.MustCompile(`[0-9a-f]{64}`).MatchString(out) {
		t.Fatalf("stdout missing sha256: %q", out)
	}

	dir := t.TempDir()
	latest := filepath.Join(dir, "latest.release.yaml")
	body := "apiVersion: devopspad.release/v1\nservice: heartie-admin\nversion: 1.4.2\nfleet: hk\nimages:\n  - {workload: app, kind: Deployment, namespace: default, container: app, image: app:latest}\n"
	if err := os.WriteFile(latest, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = run([]string{"release", "validate", "-f", latest})
	if code != 2 || !strings.Contains(errOut, "latest") {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
}

func TestFleetList(t *testing.T) {
	t.Setenv("DEVOPSPAD_CONFIG_DIR", t.TempDir())
	code, out, errOut := run([]string{"fleet", "list"})
	if code != 0 || !strings.Contains(out, "fleets: 0") {
		t.Fatalf("code=%d stdout=%q stderr=%s", code, out, errOut)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errOut := run([]string{"deploy"})
	if code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
}
