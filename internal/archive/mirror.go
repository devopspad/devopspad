package archive

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devopspad/internal/ops"
)

// opsActions maps desktop/CLI record actions to archive actions.
var opsActions = map[string]string{
	"": "release.image", "release": "release.image", "rollback": "release.rollback", "scale": "workload.scale",
	"restart": "workload.restart", "config": "config.set", "nginx": "nginx.set", "secret": "secret.set",
	"patch": "workload.patch", "job": "job.run",
}

// ServiceFor finds the archive service a desktop/CLI record belongs to: a k8s
// deployment on the cluster named like the record's connection, in the same
// namespace, with the application's selector or the record's workload.
func (a *Archive) ServiceFor(r ops.Record, app ops.Application) (string, string, bool) {
	workload := r.Workload
	if len(r.Targets) > 0 {
		workload = r.Targets[0].Workload
	}
	var found []string
	var env string
	for _, res := range a.Resources {
		s, ok := res.Object.(*Service)
		if !ok {
			continue
		}
		for _, d := range a.resolveDeployments(s) {
			if d.Runtime != "k8s" || d.Namespace != r.Namespace || strings.TrimPrefix(d.Cluster, "cluster:") != r.Connection {
				continue
			}
			if (app.Selector != "" && d.Selector == app.Selector) || (d.Workload != nil && d.Workload.Name == workload) {
				found = append(found, res.Ref)
				env = d.Environment
				break
			}
		}
	}
	if len(found) != 1 {
		return "", "", false
	}
	return found[0], env, true
}

// stableULID derives a ULID from a time and a seed, so the same desktop
// record always maps to the same archive record (imports are idempotent).
func stableULID(t time.Time, seed string) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(t.UnixMilli())<<16)
	h := sha256.Sum256([]byte(seed))
	copy(b[6:], h[:10])
	hi, lo := binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:])
	var out [26]byte
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}

func parseAnyTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// MirrorRecord writes a finished desktop/CLI record as an archive
// ActionRecord. It returns false when no archive service matches.
func (a *Archive) MirrorRecord(r ops.Record, app ops.Application, actor string) (bool, error) {
	if r.Status == "" || r.Status == "running" {
		return false, nil
	}
	ref, env, ok := a.ServiceFor(r, app)
	if !ok {
		return false, nil
	}
	if !actorRE.MatchString(actor) {
		return false, errors.New("actor must be human/<name> or ai/<name>")
	}
	started, ok := parseAnyTime(r.StartedAt)
	if !ok {
		return false, fmt.Errorf("record %s has no start time", r.ID)
	}
	finished, ok := parseAnyTime(r.FinishedAt)
	if !ok {
		finished = started
	}
	status := r.Status
	if status != "succeeded" && status != "failed" && status != "cancelled" {
		status = "unknown"
	}
	steps := []Step{}
	if r.Image != "" && (r.Action == "" || r.Action == "release" || r.Action == "rollback") {
		steps = append(steps, Step{Name: "image " + r.BeforeImage + " -> " + r.Image, Status: "succeeded"})
	}
	for _, s := range r.Steps {
		steps = append(steps, Step{Name: s, Status: "succeeded"})
	}
	if status != "succeeded" {
		steps = append(steps, Step{Name: "result", Status: status, Message: r.Message})
	}
	typ, name, _ := strings.Cut(actor, "/")
	machine, _ := os.Hostname()
	rec := &ActionRecord{APIVersion: APIVersion, Kind: "ActionRecord", ID: stableULID(started, "ops:"+r.ID), Action: opsActions[r.Action],
		Targets: []string{ref + "@" + env}, Actor: Actor{Type: typ, Name: name, Machine: machine}, Status: status,
		Message: strings.TrimSpace(recordSubject(r) + ": " + r.Message), Steps: steps,
		StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), Links: Links{Run: r.RunID}}
	if rec.Action == "" {
		rec.Action = "ops." + r.Action
	}
	rel, _ := recordPath(rec.ID, rec.StartedAt)
	full := filepath.Join(a.Dir, filepath.FromSlash(rel))
	if _, err := os.Stat(full); err == nil {
		return true, nil // already mirrored
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return false, err
	}
	if lines := secretLines(string(b)); len(lines) > 0 {
		return false, errors.New("the record looks like it contains a secret; not mirrored")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return false, err
	}
	return true, writeAtomic(full, append(b, '\n'), 0o644)
}

// ImportRecords mirrors every finished record of a desktop project directory.
func (a *Archive) ImportRecords(store *ops.Store, actor string) (imported, skipped int, err error) {
	snap, err := store.Read()
	if err != nil {
		return 0, 0, err
	}
	apps := map[string]ops.Application{}
	for _, app := range snap.Project.Applications {
		apps[app.Name] = app
	}
	for _, r := range snap.Releases {
		ok, err := a.MirrorRecord(r, apps[r.Application], actor)
		if err != nil {
			return imported, skipped, fmt.Errorf("record %s: %w", r.ID, err)
		}
		if ok {
			imported++
		} else {
			skipped++
		}
	}
	return imported, skipped, nil
}

// recordSubject names what a record changed, for the archive message.
func recordSubject(r ops.Record) string {
	if r.ConfigMap != "" {
		s := "configmap " + r.ConfigMap + "/" + r.Key
		if r.Workload != "" {
			s += ", restarted " + r.Workload
		}
		return s
	}
	return strings.TrimSpace(r.Workload + " " + r.Version)
}
