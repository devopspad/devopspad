package opsrun

import (
	"errors"
	"strings"
	"testing"

	"devopspad/internal/ops"
	"devopspad/internal/release"
	"devopspad/internal/workspace"
	"github.com/google/uuid"
)

type fakeConnections []workspace.Connection

func (f fakeConnections) List() []workspace.Connection { return f }
func (f fakeConnections) Get(id string) (workspace.Connection, error) {
	for _, c := range f {
		if c.ID == id {
			return c, nil
		}
	}
	return workspace.Connection{}, errors.New("not found")
}

func project(t *testing.T) *ops.Store {
	t.Helper()
	s, err := ops.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Attach(ops.Environment{Name: "heartie-pro", Connection: "heartie-pro k3s", Namespace: "default"}, ops.Application{Name: "thumbspark", Selector: "app=thumbspark"}, snap.Revision); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResolveRejectsUnknownTargetsAndAmbiguousConnections(t *testing.T) {
	s := project(t)
	cases := []struct {
		name, env, app string
		conns          fakeConnections
		want           string
	}{
		{"unknown environment", "dev", "thumbspark", nil, "not found"},
		{"unknown application", "heartie-pro", "other", nil, "not found"},
		{"no connection", "heartie-pro", "thumbspark", fakeConnections{{ID: "1", Name: "heartie-pro k3s", Kind: "mysql"}}, "Add a cluster connection"},
		{"duplicate connection", "heartie-pro", "thumbspark", fakeConnections{{ID: "1", Name: "heartie-pro k3s", Kind: "k3s"}, {ID: "2", Name: "heartie-pro k3s", Kind: "k8s"}}, "Duplicate connection name"},
	}
	for _, c := range cases {
		if _, err := Resolve(s, c.conns, c.env, c.app); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want error containing %q", c.name, err, c.want)
		}
	}
}

func TestPlanRecordAndFinish(t *testing.T) {
	s := project(t)
	target := Target{Environment: ops.Environment{Name: "heartie-pro", Connection: "heartie-pro k3s", Namespace: "default"}, Application: ops.Application{Name: "thumbspark"}}
	plan := release.DesktopPlan{ID: uuid.NewString(), Action: "restart", Workload: "thumbspark", BeforeReplicas: 1, Replicas: 1}
	r := PlanRecord(target, plan, "", "")
	if r.ID != plan.ID || r.Namespace != "default" || r.Connection != "heartie-pro k3s" || r.Replicas == nil || *r.Replicas != 1 {
		t.Fatalf("unexpected record: %+v", r)
	}
	done, err := Finish(s, r, errors.New("rollout stalled"), "ok")
	if err != nil || done.Status != "failed" || done.Message != "rollout stalled" || done.FinishedAt == "" {
		t.Fatalf("failed finish: %+v %v", done, err)
	}
	snap, err := s.Read()
	if err != nil || len(snap.Releases) != 1 || snap.Releases[0].Status != "failed" {
		t.Fatalf("record not stored: %+v %v", snap.Releases, err)
	}
}
