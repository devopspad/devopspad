package ops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const validProject = `apiVersion: devopspad.project/v1
name: Test
environments:
  - name: pro
    connection: local-k3s
    namespace: default
applications:
  - name: svc-user
    selector: app=svc-user
`

func TestProjectConflictAndPortableArtifacts(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Save(validProject, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	other, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Save(validProject+"# stale\n", initial.Revision); err == nil {
		t.Fatal("stale revision accepted")
	}
	artifact := Artifact{Application: "svc-user", Version: "1.0.0", Image: "registry.local/app@sha256:" + strings.Repeat("a", 64)}
	result, err := s.AddArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].ID == "" {
		t.Fatal(result.Artifacts)
	}
	if result.Revision != saved.Revision {
		t.Fatal("artifact registration changed config revision")
	}
	if _, err = other.Read(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(root, "artifacts", result.Artifacts[0].ID+".yaml"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("artifact permissions")
	}
}
func TestProjectRejectsAmbiguityAndUnsafeFiles(t *testing.T) {
	for _, text := range []string{validProject + "password: secret\n", validProject + "---\nname: second\n", strings.Replace(validProject, "app=svc-user", "app in (", 1), strings.Replace(validProject, "namespace: default", "namespace: '*'", 1)} {
		if _, err := Validate(text); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.Root, "artifacts", "bad.yaml"), []byte("image: wrong\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(); err == nil {
		t.Fatal("corrupt artifact hidden")
	}
}
func TestProjectSaveDetectsOtherWriter(t *testing.T) {
	s, _ := New(t.TempDir())
	snapshot, _ := s.Read()
	if err := os.WriteFile(filepath.Join(s.Root, ".devopspad-write.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(validProject, snapshot.Revision); err == nil {
		t.Fatal("concurrent writer lock ignored")
	}
}
func TestGitRefusesDirtyPullAndParentRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	if err := exec.Command("git", "init", root).Run(); err != nil {
		t.Fatal(err)
	}
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Git(context.Background(), "pull"); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("dirty pull: %v", err)
	}
	nested, err := New(filepath.Join(root, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = nested.Git(context.Background(), "push"); err == nil || !strings.Contains(err.Error(), "repository root") {
		t.Fatalf("nested git: %v", err)
	}
}

func TestGitPushAndFastForwardShareProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	first := filepath.Join(base, "first")
	second := filepath.Join(base, "second")
	command := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
	command("init", "--bare", remote)
	command("clone", remote, first)
	command("-C", first, "config", "user.email", "test@example.invalid")
	command("-C", first, "config", "user.name", "Test")
	if err := os.MkdirAll(filepath.Join(first, "artifacts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "artifacts", "unrelated.txt"), []byte("not an artifact"), 0600); err != nil {
		t.Fatal(err)
	}
	command("-C", first, "add", "artifacts/unrelated.txt")
	s, err := New(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Git(context.Background(), "push"); err != nil {
		t.Fatal(err)
	}
	command("clone", remote, second)
	if _, err := os.Stat(filepath.Join(second, "artifacts", "unrelated.txt")); !os.IsNotExist(err) {
		t.Fatal("unrelated staged file was included in project commit")
	}
	other, err := New(second)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Read()
	if _, err = s.Save(validProject, snapshot.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Git(context.Background(), "push"); err != nil {
		t.Fatal(err)
	}
	if _, err = other.Git(context.Background(), "pull"); err != nil {
		t.Fatal(err)
	}
	e := startEvent("build")
	e.Start.Application = "svc-user"
	recordEvent(t, s, e)
	recordEvent(t, s, stepEvent(e.RunID, "build", "succeeded"))
	recordEvent(t, s, stepEvent(e.RunID, "push", "succeeded"))
	recordEvent(t, s, finishEvent(e.RunID, "succeeded", "registry.local/app@sha256:"+strings.Repeat("a", 64)))
	if _, err = s.Git(context.Background(), "push"); err != nil {
		t.Fatal(err)
	}
	if _, err = other.Git(context.Background(), "pull"); err != nil {
		t.Fatal(err)
	}
	synced, err := other.Read()
	if err != nil || synced.Project.Name != "Test" || len(synced.Runs) != 1 || len(synced.Artifacts) != 1 || synced.Artifacts[0].RunID != e.RunID {
		t.Fatalf("not synced: %+v %v", synced, err)
	}
}
