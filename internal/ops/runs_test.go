package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func runTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Read()
	_, err = s.Attach(Environment{Name: "pro", Connection: "cluster", Namespace: "default"}, Application{Name: "app", Selector: "app=app"}, snap.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func startEvent(kind string) RunEvent {
	return RunEvent{APIVersion: RunEventVersion, ID: uuid.NewString(), RunID: uuid.NewString(), Type: "start", Start: &RunStart{Kind: kind, Application: "app", Version: "1.0", Actor: RunActor{Kind: "ai", Name: "Codex", SessionID: "session-1"}, Source: RunSource{Repository: "https://example.com/repo.git", Commit: strings.Repeat("a", 40), Branch: "main"}, Recipe: "docker buildx"}}
}
func stepEvent(id, name, status string) RunEvent {
	return RunEvent{APIVersion: RunEventVersion, ID: uuid.NewString(), RunID: id, Type: "step", Step: &RunStep{Name: name, Status: status, Message: "result recorded"}}
}
func finishEvent(id, status, image string) RunEvent {
	return RunEvent{APIVersion: RunEventVersion, ID: uuid.NewString(), RunID: id, Type: "finish", Finish: &RunFinish{Status: status, Image: image, Message: "execution complete"}}
}
func recordEvent(t *testing.T, s *Store, e RunEvent) Run {
	t.Helper()
	r, err := s.RecordRunEvent(e)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestRunsLifecycleAndBuildArtifact(t *testing.T) {
	s := runTestStore(t)
	e := startEvent("build")
	first := recordEvent(t, s, e)
	replay := recordEvent(t, s, e)
	if first.StartedAt != replay.StartedAt || len(replay.Events) != 1 {
		t.Fatal("start retry duplicated")
	}
	changed := e
	copyStart := *e.Start
	changed.Start = &copyStart
	changed.Start.Version = "2.0"
	if _, err := s.RecordRunEvent(changed); err == nil {
		t.Fatal("event collision accepted")
	}
	image := "registry.local/app@sha256:" + strings.Repeat("b", 64)
	done := finishEvent(e.RunID, "succeeded", image)
	if _, err := s.RecordRunEvent(done); err == nil {
		t.Fatal("success without build/push evidence accepted")
	}
	build := stepEvent(e.RunID, "build", "succeeded")
	recordEvent(t, s, build)
	recordEvent(t, s, build)
	recordEvent(t, s, stepEvent(e.RunID, "push", "succeeded"))
	result := recordEvent(t, s, done)
	retry := recordEvent(t, s, done)
	if len(result.Events) != 4 || retry.FinishedAt != result.FinishedAt {
		t.Fatal("retry duplicated history")
	}
	if _, err := s.RecordRunEvent(stepEvent(e.RunID, "new", "succeeded")); err == nil {
		t.Fatal("terminal history changed")
	}
	snap, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Runs) != 1 || len(snap.Artifacts) != 1 || snap.Artifacts[0].RunID != e.RunID || snap.Artifacts[0].Image != image {
		t.Fatalf("missing derived artifact: %+v", snap)
	}
	files, _ := os.ReadDir(filepath.Join(s.Root, "artifacts"))
	if len(files) != 0 {
		t.Fatal("derived artifact should not require second-file transaction")
	}
	// External publishing is linked to the exact source and successful build.
	release := startEvent("release")
	release.Start.Target = &RunTarget{Environment: "pro", Workload: "app", Container: "app", BuildID: e.RunID}
	r := recordEvent(t, s, release)
	if r.Connection != "cluster" || r.Namespace != "default" || r.Image != image {
		t.Fatal("release lost target or build")
	}
	wrong := finishEvent(r.ID, "succeeded", "registry.local/app@sha256:"+strings.Repeat("c", 64))
	if _, err := s.RecordRunEvent(wrong); err == nil {
		t.Fatal("wrong image accepted")
	}
	recordEvent(t, s, stepEvent(r.ID, "rollout", "succeeded"))
	recordEvent(t, s, stepEvent(r.ID, "image-check", "succeeded"))
	recordEvent(t, s, finishEvent(r.ID, "succeeded", image))
	snap, err = s.Read()
	if err != nil || len(snap.Runs) != 2 || len(snap.Artifacts) != 1 || len(snap.Releases) != 0 {
		t.Fatal("external report was confused with independently executed release")
	}
}
func TestRunsFailuresLocksAndValidation(t *testing.T) {
	s := runTestStore(t)
	e := startEvent("build")
	if _, err := s.RecordRunEvent(stepEvent(e.RunID, "build", "succeeded")); err == nil {
		t.Fatal("step before start accepted")
	}
	recordEvent(t, s, e)
	recordEvent(t, s, stepEvent(e.RunID, "build", "failed"))
	recordEvent(t, s, finishEvent(e.RunID, "failed", ""))
	snap, _ := s.Read()
	if len(snap.Artifacts) != 0 {
		t.Fatal("failed build entered artifact library")
	}
	release := startEvent("release")
	release.Start.Target = &RunTarget{Environment: "pro", Workload: "app", Container: "app", BuildID: e.RunID}
	if _, err := s.RecordRunEvent(release); err == nil {
		t.Fatal("failed build released")
	}
	e = startEvent("build")
	e.Start.Source.Repository = "https://secret:password@example.com/repo"
	if _, err := s.RecordRunEvent(e); err == nil {
		t.Fatal("credentialed URL accepted")
	}
	e = startEvent("build")
	e.Start.Source.Commit = "a123"
	if _, err := s.RecordRunEvent(e); err == nil {
		t.Fatal("short commit accepted")
	}
	e = startEvent("build")
	e.Start.Application = "missing"
	if _, err := s.RecordRunEvent(e); err == nil {
		t.Fatal("unregistered app accepted")
	}
	e = startEvent("build")
	bytes, _ := json.Marshal(e)
	bytes = append(bytes[:len(bytes)-1], []byte(`,"unknown":true}`)...)
	if _, err := DecodeRunEvent(bytes); err == nil {
		t.Fatal("unknown input field accepted")
	}
	if _, err := DecodeRunEvent([]byte(`{} {}`)); err == nil {
		t.Fatal("multiple documents accepted")
	}
	if err := os.WriteFile(filepath.Join(s.Root, ".infradex-write.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRunEvent(startEvent("build")); err == nil {
		t.Fatal("cross-process lock ignored")
	}
	_ = os.Remove(filepath.Join(s.Root, ".infradex-write.lock"))
	e = startEvent("build")
	recordEvent(t, s, e)
	step := stepEvent(e.RunID, "build", "failed")
	step.Step.LogPath = "logs/../../secrets"
	if _, err := s.RecordRunEvent(step); err == nil {
		t.Fatal("unsafe log reference accepted")
	}
}
func TestAttachPreservesCommentsAndRejectsStaleMapping(t *testing.T) {
	s := runTestStore(t)
	snap, _ := s.Read()
	snap, err := s.Save("# keep this comment\n"+snap.YAML, snap.Revision)
	if err != nil {
		t.Fatal(err)
	}
	attached, err := s.Attach(Environment{Name: "dev", Connection: "cluster", Namespace: "dev"}, Application{Name: "app-two", Selector: "app in (two,three)"}, snap.Revision)
	if err != nil || !strings.Contains(attached.YAML, "# keep this comment") {
		t.Fatal("comments lost", err)
	}
	if _, err = s.Attach(Environment{Name: "test", Connection: "cluster", Namespace: "test"}, Application{Name: "app-two", Selector: "app in (two,three)"}, snap.Revision); err == nil {
		t.Fatal("stale mapping accepted")
	}
	if _, err = s.Attach(Environment{Name: "pro", Connection: "other", Namespace: "default"}, Application{Name: "app", Selector: "app=app"}, attached.Revision); err == nil {
		t.Fatal("environment retargeted")
	}
}

func TestRunsRejectIncompleteSharedRecords(t *testing.T) {
	s := runTestStore(t)
	e := startEvent("build")
	r := recordEvent(t, s, e)
	r.Events = nil
	b, _ := json.Marshal(r)
	if err := os.WriteFile(filepath.Join(s.Root, "runs", r.ID+".json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(); err == nil {
		t.Fatal("corrupt history exposed to UI")
	}
	if _, err := s.RecordRunEvent(stepEvent(r.ID, "build", "succeeded")); err == nil {
		t.Fatal("corrupt history updated")
	}
}
