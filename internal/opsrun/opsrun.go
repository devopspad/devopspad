// Package opsrun resolves project targets and applies Deployment plans with
// local operation records. The desktop and devopspadcli share it so both
// paths enforce the same checks and leave the same records.
package opsrun

import (
	"context"
	"errors"
	"fmt"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/release"
	"devopspad/internal/workspace"
)

// Connections is the part of the local connection store a target needs.
type Connections interface {
	List() []workspace.Connection
	Get(id string) (workspace.Connection, error)
}

// Target is a project application in one environment, resolved against local
// credentials. The connection is pinned so a plan cannot drift to another endpoint.
type Target struct {
	Store       *ops.Store
	Snapshot    ops.Snapshot
	Environment ops.Environment
	Application ops.Application
	Connection  workspace.Connection
}

// Resolve reads the shared project and finds the local cluster connection named
// by the environment. No credential enters a shared record.
func Resolve(s *ops.Store, conns Connections, environment, application string) (Target, error) {
	t := Target{Store: s}
	snapshot, err := s.Read()
	if err != nil {
		return t, err
	}
	t.Snapshot = snapshot
	for _, e := range snapshot.Project.Environments {
		if e.Name == environment {
			t.Environment = e
		}
	}
	for _, v := range snapshot.Project.Applications {
		if v.Name == application {
			t.Application = v
		}
	}
	if t.Environment.Name == "" || t.Application.Name == "" {
		return t, errors.New("Environment or application not found; reload the project")
	}
	id := ""
	for _, conn := range conns.List() {
		if conn.Name == t.Environment.Connection && (conn.Kind == "k8s" || conn.Kind == "k3s") {
			if id != "" {
				return t, errors.New("Duplicate connection name; give each cluster connection a unique name")
			}
			id = conn.ID
		}
	}
	if id == "" {
		return t, errors.New("Add a cluster connection named after this environment in Cluster resources first")
	}
	c, err := conns.Get(id)
	if err == nil {
		c, err = workspace.PinClusterConnection(c)
	}
	t.Connection = c
	return t, err
}

// PlanRecord describes a prepared plan before it runs.
func PlanRecord(t Target, plan release.DesktopPlan, version, runID string) ops.Record {
	env, app := t.Environment, t.Application
	record := ops.Record{RunID: runID, ID: plan.ID, Action: plan.Action, Application: app.Name, Environment: env.Name, Connection: env.Connection, Namespace: env.Namespace, Workload: plan.Workload, Container: plan.Container, Image: plan.Image, BeforeImage: plan.BeforeImage, Version: version, Steps: []string{}}
	if plan.Action == "scale" || plan.Action == "restart" {
		record.BeforeReplicas = &plan.BeforeReplicas
		record.Replicas = &plan.Replicas
	}
	return record
}

// Apply runs a plan and keeps the record current after every step. The record
// is written as running first, so an interrupted client leaves a visible trace.
// onStep, when set, observes each step after it is recorded.
func Apply(ctx context.Context, store *ops.Store, c workspace.Connection, plan release.DesktopPlan, r ops.Record, onStep func(string)) (ops.Record, error) {
	if r.StartedAt == "" {
		r.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	r.Status = "running"
	if err := store.WriteRecord(r); err != nil {
		return r, err
	}
	cl, close, err := workspace.OpenCluster(ctx, c)
	if err == nil {
		defer close()
		err = release.ApplyDesktop(ctx, cl, plan, func(step string) error {
			r.Steps = append(r.Steps, step)
			if err := store.WriteRecord(r); err != nil {
				return err
			}
			if onStep != nil {
				onStep(step)
			}
			return nil
		})
	}
	return Finish(store, r, err, "Deployment is ready")
}

// Finish stores the final status of a record.
func Finish(store *ops.Store, r ops.Record, err error, success string) (ops.Record, error) {
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		r.Status = "failed"
		r.Message = err.Error()
	} else {
		r.Status = "succeeded"
		r.Message = success
	}
	if saveErr := store.WriteRecord(r); saveErr != nil {
		return r, fmt.Errorf("Operation %s, but saving the record failed: %w", r.Status, saveErr)
	}
	return r, nil
}
