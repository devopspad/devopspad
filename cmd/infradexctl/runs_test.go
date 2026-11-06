package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"infradex/internal/ops"
)

func TestRunCommandReadsAndWritesSharedProject(t *testing.T) {
	s, err := ops.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Read()
	_, err = s.Attach(ops.Environment{Name: "pro", Connection: "k3s", Namespace: "default"}, ops.Application{Name: "app", Selector: "app=app"}, snap.Revision)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	event := ops.RunEvent{APIVersion: ops.RunEventVersion, ID: uuid.NewString(), RunID: id, Type: "start", Start: &ops.RunStart{Kind: "build", Application: "app", Version: "1.0", Actor: ops.RunActor{Kind: "ai", Name: "test-agent", SessionID: "task-123"}, Source: ops.RunSource{Repository: "file:///local/repo", Commit: strings.Repeat("a", 40)}, Recipe: "test"}}
	b, _ := json.Marshal(event)
	p := filepath.Join(t.TempDir(), "event.json")
	_ = os.WriteFile(p, b, 0600)
	for i := 0; i < 2; i++ {
		code, out, errOut := run([]string{"runs", "record", "--project", s.Root, "-f", p})
		if code != 0 || !strings.Contains(out, id) {
			t.Fatal(code, out, errOut)
		}
	}
	t.Setenv("INFRADEX_PROJECT_DIR", s.Root)
	code, out, errOut := run([]string{"runs", "get", "--id", id})
	if code != 0 {
		t.Fatal(code, errOut)
	}
	var result ops.Run
	if err = json.Unmarshal([]byte(out), &result); err != nil || len(result.Events) != 1 {
		t.Fatal("record roundtrip failed", err)
	}
	code, out, errOut = run([]string{"runs", "list"})
	if code != 0 || !strings.Contains(out, "test-agent") {
		t.Fatal(code, out, errOut)
	}
	_ = os.WriteFile(p, []byte(`{"apiVersion":"wrong"}`), 0600)
	code, _, errOut = run([]string{"runs", "record", "-f", p})
	if code != 1 || !strings.Contains(errOut, "error") {
		t.Fatal("invalid record accepted", code, errOut)
	}
}

func TestDocumentedEventsUseTheRealContract(t *testing.T) {
	s, err := ops.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Read()
	_, err = s.Attach(ops.Environment{Name: "pro", Connection: "k3s", Namespace: "default"}, ops.Application{Name: "svc-api", Selector: "app=svc-api"}, snap.Revision)
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("../../examples/ai-integration/*.json")
	if err != nil || len(files) != 4 {
		t.Fatal("missing event examples", err)
	}
	for _, file := range files {
		code, out, errOut := run([]string{"runs", "record", "--project", s.Root, "-f", file})
		if code != 0 {
			t.Fatal(code, out, errOut)
		}
	}
	snapshot, err := s.Read()
	if err != nil || len(snapshot.Artifacts) != 1 || len(snapshot.Runs) != 1 || snapshot.Runs[0].Status != "succeeded" {
		t.Fatal("example lifecycle failed", err)
	}
}
