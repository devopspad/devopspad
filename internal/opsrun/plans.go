package opsrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/release"
	"devopspad/internal/workspace"

	"k8s.io/client-go/dynamic"
)

// openCluster is replaced in tests.
var openCluster = workspace.OpenCluster

// PlanTTL matches the desktop's in-memory plans.
const PlanTTL = 10 * time.Minute

// SavedTarget is one workload of a saved release plan.
type SavedTarget struct {
	Application     string `json:"application"`
	Workload        string `json:"workload"`
	Container       string `json:"container"`
	BeforeImage     string `json:"beforeImage"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
}

// SavedPlan lets a CLI plan be applied by a later process. It stores only the
// parameters and the Deployment identities, never the objects (their env
// values may be sensitive); apply re-prepares and requires the same versions.
type SavedPlan struct {
	ID          string        `json:"id"`
	Environment string        `json:"environment"`
	Image       string        `json:"image"`
	Version     string        `json:"version,omitempty"`
	RunID       string        `json:"runId,omitempty"`
	CreatedAt   string        `json:"createdAt"`
	ExpiresAt   string        `json:"expiresAt"`
	Targets     []SavedTarget `json:"targets"`
	Migrate     *SavedMigrate `json:"migrate,omitempty"`
	Notes       []string      `json:"notes,omitempty"` // preflight findings shown with the plan
}

var planIDRE = regexp.MustCompile(`^[0-9a-f-]{36}$`)

func planPath(dir, id string) (string, error) {
	if !planIDRE.MatchString(id) {
		return "", errors.New("invalid plan id")
	}
	return filepath.Join(dir, "plans", id+".json"), nil
}

// SavePlan writes a plan under <config dir>/plans with owner-only access.
func SavePlan(dir string, p SavedPlan) error {
	path, err := planPath(dir, p.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	prunePlans(filepath.Dir(path))
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// TakePlan reads and deletes a plan: a plan can be applied once.
func TakePlan(dir, id string) (SavedPlan, error) {
	var p SavedPlan
	path, err := planPath(dir, id)
	if err != nil {
		return p, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, errors.New("the plan does not exist or was already used; create a new one")
	}
	if err != nil {
		return p, err
	}
	if err := os.Remove(path); err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("corrupt plan: %w", err)
	}
	if at, err := time.Parse(time.RFC3339, p.ExpiresAt); err != nil || time.Now().After(at) {
		return p, errors.New("the plan has expired; create a new one")
	}
	return p, nil
}

func prunePlans(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// Reprepare rebuilds a saved target's plan and refuses if the Deployment
// changed since the plan was made.
func Reprepare(ctx context.Context, cl dynamic.Interface, t Target, saved SavedPlan, st SavedTarget) (release.DesktopPlan, error) {
	p, err := release.PrepareDesktop(ctx, cl, t.Environment.Namespace, st.Workload, st.Container, saved.Image, t.Application.Selector)
	if err != nil {
		return p, fmt.Errorf("%s: %w", st.Workload, err)
	}
	if p.UID != st.UID || p.ResourceVersion != st.ResourceVersion {
		return p, fmt.Errorf("deployment %s changed since the plan was made; create a new plan", st.Workload)
	}
	p.Rebind(saved.ID)
	return p, nil
}

// BatchRecord is the single record of a release across several workloads.
func BatchRecord(env ops.Environment, saved SavedPlan) ops.Record {
	r := ops.Record{RunID: saved.RunID, ID: saved.ID, Action: "release", Environment: env.Name, Connection: env.Connection, Namespace: env.Namespace,
		Image: saved.Image, Version: saved.Version, Steps: []string{}}
	var apps, workloads, containers, before []string
	for _, t := range saved.Targets {
		apps = append(apps, t.Application)
		workloads = append(workloads, t.Workload)
		containers = append(containers, t.Container)
		before = append(before, t.BeforeImage)
		r.Targets = append(r.Targets, ops.RecordTarget{Application: t.Application, Workload: t.Workload, Container: t.Container, BeforeImage: t.BeforeImage, Status: "not-started"})
	}
	r.Application, r.Workload, r.Container, r.BeforeImage = apps[0], strings.Join(workloads, ", "), strings.Join(containers, ", "), strings.Join(unique(before), ", ")
	if len(saved.Targets) == 1 {
		r.Targets = nil
	}
	return r
}

func unique(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ApplyBatch applies prepared plans in order with one record, stopping at the
// first failure; later workloads stay not-started. The release locks of all
// targets are taken up front. pre, when set, then runs (the migration Job) and
// a failure there changes no image.
func ApplyBatch(ctx context.Context, store *ops.Store, c workspace.Connection, plans []release.DesktopPlan, r ops.Record, pre func(context.Context, dynamic.Interface, func(string) error) error, onStep func(string)) (ops.Record, error) {
	r.Targets = append([]ops.RecordTarget(nil), r.Targets...) // never alias the caller's slice
	r.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Status = "running"
	if err := store.WriteRecord(r); err != nil {
		return r, err
	}
	step := func(prefix string) func(string) error {
		return func(s string) error {
			if prefix != "" {
				s = prefix + ": " + s
			}
			r.Steps = append(r.Steps, s)
			if err := store.WriteRecord(r); err != nil {
				return err
			}
			if onStep != nil {
				onStep(s)
			}
			return nil
		}
	}
	cl, closeFn, err := openCluster(ctx, c)
	if err != nil {
		return Finish(store, r, err, "")
	}
	defer closeFn()
	// Take every target's lock first, so a release held by another client
	// stops before the migration Job runs or any workload changes.
	for _, p := range plans {
		unlock, err := release.LockDesktop(ctx, cl, p)
		if err != nil {
			return Finish(store, r, fmt.Errorf("%s: %w; nothing was changed", p.Workload, err), "")
		}
		defer unlock()
	}
	if pre != nil {
		if err := pre(ctx, cl, step("migrate")); err != nil {
			return Finish(store, r, fmt.Errorf("migration failed, no image was changed: %w", err), "")
		}
	}
	multi := len(plans) > 1
	for i, p := range plans {
		prefix := ""
		if multi {
			prefix = p.Workload
		}
		err := release.ApplyDesktop(ctx, cl, p, step(prefix))
		if len(r.Targets) > i {
			r.Targets[i].Status = map[bool]string{true: "failed", false: "succeeded"}[err != nil]
		}
		if err != nil {
			if multi {
				err = fmt.Errorf("%s: %w", p.Workload, err)
			}
			return Finish(store, r, err, "")
		}
	}
	return Finish(store, r, nil, "Release is ready")
}
