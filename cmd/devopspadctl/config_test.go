package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigAndWorkloadFlagsRejectedBeforeClusterAccess(t *testing.T) {
	t.Setenv("DEVOPSPAD_CONFIG_DIR", t.TempDir())
	base := []string{"--environment", "pro", "--application", "web", "--configmap", "web-config", "--key", "app.toml"}
	for _, args := range [][]string{
		{"config"},
		{"config", "edit"},
		{"config", "get", "--environment", "pro"},
		append([]string{"config", "get"}, base...),
		append(append([]string{"config", "set"}, base...), "-f", "new.toml"),
		append(append([]string{"config", "set"}, base...), "--expect-revision", "1"),
		append(append([]string{"config", "set"}, base...), "-f", "new.toml", "--expect-revision", "1", "--restart"),
		append(append([]string{"config", "set"}, base...), "-f", "new.toml", "--expect-revision", "1", "--timeout", "10m"),
		{"workload"},
		{"workload", "restart", "--environment", "pro"},
		{"workload", "restart", "--environment", "pro", "--application", "web", "--timeout", "0s"},
	} {
		code, _, errOut := run(args)
		if code != 2 {
			t.Errorf("%v: expected usage failure, got %d %s", args, code, errOut)
		}
	}
}

func TestConfigSetRestartWithoutYesWritesNothing(t *testing.T) {
	t.Setenv("DEVOPSPAD_CONFIG_DIR", t.TempDir())
	_, _, errOut := run([]string{"config", "set", "--environment", "pro", "--application", "web", "--configmap", "c", "--key", "k", "-f", "x", "--expect-revision", "1", "--restart"})
	if !strings.Contains(errOut, "requires --yes; nothing was written") {
		t.Fatalf("unexpected error: %s", errOut)
	}
}

func TestLineDiffSummaryNamesNoContent(t *testing.T) {
	diff, summary := lineDiff("config.toml", "a = 1\ntoken = [\"old\"]\n", "a = 1\ntoken = [\"old\", \"new\"]\nb = 2\n")
	if !strings.Contains(diff, "--- a/config.toml") || !strings.Contains(diff, "+b = 2") || !strings.Contains(diff, "-token = [\"old\"]") {
		t.Fatalf("unexpected diff:\n%s", diff)
	}
	if summary != "2 line(s) added, 1 removed" || strings.Contains(summary, "token") {
		t.Fatalf("unexpected summary: %s", summary)
	}
}

func TestWritePrivateTightensExistingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old contents that are longer"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(path, "secret"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v, %v", info.Mode().Perm(), err)
	}
	if b, _ := os.ReadFile(path); string(b) != "secret" {
		t.Fatalf("contents = %q", b)
	}
}

func TestReadValueRejectsBinaryAndOversizedInput(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "bin")
	big := filepath.Join(dir, "big")
	os.WriteFile(binary, []byte{0xff, 0xfe}, 0600)
	os.WriteFile(big, make([]byte, (1<<20)+1), 0600)
	for _, path := range []string{binary, big, filepath.Join(dir, "missing")} {
		if _, err := readValue(path); err == nil {
			t.Errorf("%s accepted", path)
		}
	}
}
