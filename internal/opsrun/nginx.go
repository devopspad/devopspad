package opsrun

import (
	"context"
	"errors"
	"fmt"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/workspace"

	"github.com/google/uuid"
)

// SaveNginx replaces an application's host Nginx file through the remote
// helper (lock, revision check, atomic replace, nginx -t, reload, restore on
// failure) and records the outcome without the file contents. summary
// describes the change (line counts, hashes) for the record.
func SaveNginx(ctx context.Context, t Target, update workspace.NginxUpdate, summary string) (ops.Record, workspace.NginxConfig, error) {
	env, app := t.Environment, t.Application
	if app.NginxPath == "" {
		return ops.Record{}, workspace.NginxConfig{}, errors.New("This application has no nginxPath configured")
	}
	steps := []string{"Read current config and check revision"}
	if summary != "" {
		steps = append(steps, summary)
	}
	r := ops.Record{ID: uuid.NewString(), Action: "nginx", Application: app.Name, Environment: env.Name, Connection: env.Connection, Namespace: env.Namespace,
		Workload: app.Name, Version: "Nginx", Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Steps: steps}
	if err := t.Store.WriteRecord(r); err != nil {
		return r, workspace.NginxConfig{}, err
	}
	result, err := workspace.SaveNginx(ctx, t.Connection, app.NginxPath, update)
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Status, r.Message = "succeeded", result.Message
	if err != nil {
		r.Status, r.Message = "failed", err.Error()
	} else {
		r.Steps = append(r.Steps, result.Message)
	}
	if saveErr := t.Store.WriteRecord(r); saveErr != nil {
		return r, result, fmt.Errorf("Nginx operation %s, but saving the record failed; refresh to check", r.Status)
	}
	return r, result, err
}
