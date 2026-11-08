package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devopspad/internal/archive"
)

const exampleArchive = "../../examples/archive"

func runArchive(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("DEVOPSPAD_CONFIG_DIR", t.TempDir())
	t.Setenv("DEVOPSPAD_ARCHIVE", exampleArchive)
	return run(args)
}

func TestArchiveReadCommands(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"version"}, "protocol devopspad.io/v1alpha1"},
		{[]string{"get", "hosts", "-l", "region=hk"}, "hk-1"},
		{[]string{"get", "host", "sg-1"}, "# inventory/hosts/sg-1.yaml"},
		{[]string{"describe", "service:heartie/admin"}, "Release notes (read before releasing): products/heartie/notes/release-admin.md"},
		{[]string{"refs", "host:hk-1"}, "domain:heartie.example"},
		{[]string{"report", "matrix"}, "thumbsnap"},
		{[]string{"report", "matrix", "--stage", "live"}, "1 products"},
		{[]string{"report", "matrix", "-l", "line=consumer"}, "heartie"},
		{[]string{"report", "renewals", "--all"}, "domain:heartie.example"},
		{[]string{"report", "cost"}, "USD 26.04"},
		{[]string{"report", "orphans"}, "Every inventory resource is used by a product."},
		{[]string{"schema"}, "service"},
		{[]string{"diff"}, "not scanned yet"},
		{[]string{"report", "renewals", "--observed", "--all"}, "provider:resend"},
		{[]string{"skill", "print"}, "name: devopspad"},
		{[]string{"skill", "print", "workflows/locate.md"}, "describe service"},
	}
	for _, c := range cases {
		code, out, errOut := runArchive(t, c.args...)
		if code != 0 || !strings.Contains(out, c.want) {
			t.Errorf("%v: code=%d stderr=%q stdout:\n%s", c.args, code, errOut, out)
		}
	}
}

func TestArchiveJSONOutput(t *testing.T) {
	code, out, _ := runArchive(t, "archive", "validate", "-o", "json")
	var v struct {
		OK     bool            `json:"ok"`
		Errors []archive.Issue `json:"errors"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &v) != nil || !v.OK {
		t.Fatalf("validate: code=%d %s", code, out)
	}
	// Flags may follow positional arguments.
	code, out, _ = runArchive(t, "describe", "service:heartie/api", "-o", "json")
	var d struct {
		Ref         string                       `json:"ref"`
		Deployments []archive.ResolvedDeployment `json:"deployments"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &d) != nil || d.Ref != "service:heartie/api" || len(d.Deployments) != 1 || d.Deployments[0].Namespace != "default" {
		t.Fatalf("describe: code=%d %s", code, out)
	}
	code, out, _ = runArchive(t, "schema", "host")
	var s map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &s) != nil || s["$id"] == nil {
		t.Fatalf("schema: code=%d %s", code, out)
	}
}

func TestArchiveErrors(t *testing.T) {
	if code, _, errOut := runArchive(t, "get", "servers"); code != 1 || !strings.Contains(errOut, "unknown kind") {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if code, _, errOut := runArchive(t, "describe", "host:nope", "-o", "json"); code != 1 || !strings.Contains(errOut, `"error"`) {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if code, _, _ := runArchive(t, "describe"); code != 2 {
		t.Fatalf("missing argument: code=%d", code)
	}
	t.Setenv("DEVOPSPAD_ARCHIVE", "")
	t.Setenv("DEVOPSPAD_CONFIG_DIR", t.TempDir())
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(t.TempDir())
	if code, _, errOut := run([]string{"report", "matrix"}); code != 1 || !strings.Contains(errOut, "no archive found") {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
}

func TestSecretsSet(t *testing.T) {
	var gotAccount, gotValue string
	old := archive.KeychainSet
	archive.KeychainSet = func(account, value string) error { gotAccount, gotValue = account, value; return nil }
	defer func() { archive.KeychainSet = old }()
	stdin = strings.NewReader("s3cret-value\n")
	defer func() { stdin = os.Stdin }()
	code, out, errOut := runArchive(t, "secrets", "set", "keychain:archive/providers/x/token")
	if code != 0 || gotAccount != "archive/providers/x/token" || gotValue != "s3cret-value" {
		t.Fatalf("code=%d stderr=%s account=%q", code, errOut, gotAccount)
	}
	if strings.Contains(out+errOut, "s3cret-value") {
		t.Fatal("secret value printed")
	}
	if code, _, _ := runArchive(t, "secrets", "set", "env:TOKEN"); code != 2 {
		t.Fatalf("non-keychain ref: code=%d", code)
	}
}

func TestSkillInstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skill")
	code, out, errOut := runArchive(t, "skill", "install", "--dir", dir)
	if code != 0 || !strings.Contains(out, "Installed") {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	for _, f := range []string{"SKILL.md", "reference/protocol.md", "workflows/import.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestArchiveInitAndUse(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("DEVOPSPAD_CONFIG_DIR", cfg)
	t.Setenv("DEVOPSPAD_ARCHIVE", "")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	dir := filepath.Join(t.TempDir(), "opc")
	if code, _, errOut := run([]string{"archive", "init", dir, "--name", "my-opc"}); code != 0 {
		t.Fatalf("init: code=%d stderr=%s", code, errOut)
	}
	b, _ := os.ReadFile(filepath.Join(cfg, "archive-path"))
	if want, _ := filepath.EvalSymlinks(dir); !strings.Contains(string(b), filepath.Base(want)) {
		t.Fatalf("archive-path = %q", b)
	}
	// The remembered archive is found from any directory.
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(t.TempDir())
	if code, out, errOut := run([]string{"new", "host", "hk-1"}); code != 0 || !strings.Contains(out, "inventory/hosts/hk-1.yaml") {
		t.Fatalf("new: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if code, out, _ := run([]string{"archive", "validate"}); code != 0 || !strings.Contains(out, "ok: 1 resources") {
		t.Fatalf("validate: code=%d %s", code, out)
	}
}
