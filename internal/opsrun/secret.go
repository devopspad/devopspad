package opsrun

import (
	"context"
	"strings"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/release"
)

// ApplySecret applies a prepared Secret plan and records key names, change
// kinds and the content hash prefix only.
func ApplySecret(ctx context.Context, t Target, plan release.SecretPlan) (ops.Record, error) {
	var keys []string
	for _, k := range plan.Keys {
		keys = append(keys, k.Key+" "+k.Change)
	}
	verb := "update"
	if plan.Create {
		verb = "create"
	}
	r := ops.Record{ID: plan.ID, Action: "secret", Application: t.Application.Name, Environment: t.Environment.Name, Connection: t.Environment.Connection,
		Namespace: t.Environment.Namespace, Workload: plan.Name, Version: "Secret", Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Steps: []string{verb + " " + plan.Type + ": " + strings.Join(keys, ", "), "content sha256 " + plan.ContentSHA256}}
	if err := t.Store.WriteRecord(r); err != nil {
		return r, err
	}
	cl, closeFn, err := openCluster(ctx, t.Connection)
	if err == nil {
		defer closeFn()
		err = release.ApplySecret(ctx, cl, plan)
	}
	return Finish(t.Store, r, err, "Secret "+verb+"d")
}
