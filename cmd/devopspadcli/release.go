package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/opsrun"
	"devopspad/internal/release"
	"devopspad/internal/workspace"

	"github.com/google/uuid"
	"k8s.io/client-go/dynamic"
)

// targetList collects repeated --target application:workload:container flags.
type targetList []opsrun.SavedTarget

func (l *targetList) String() string { return fmt.Sprint(*l) }
func (l *targetList) Set(v string) error {
	parts := strings.Split(v, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return errors.New("use application:workload:container")
	}
	*l = append(*l, opsrun.SavedTarget{Application: parts[0], Workload: parts[1], Container: parts[2]})
	return nil
}

type releaseFlags struct {
	environment, application, workload, container string
	artifact, image, version, root                string
	migrate                                       string
	targets                                       targetList
	yes, ackRegistryChange, verifyPull            bool
	placeholder                                   string
	timeout, migrateWait                          time.Duration
}

func addReleaseFlags(fs *flag.FlagSet, f *releaseFlags) {
	fs.StringVar(&f.environment, "environment", "", "project environment")
	fs.StringVar(&f.application, "application", "", "project application (single target)")
	fs.StringVar(&f.workload, "workload", "", "Deployment (default: the application name)")
	fs.StringVar(&f.container, "container", "", "container to update (single target)")
	fs.Var(&f.targets, "target", "application:workload:container; repeat for workloads sharing the image")
	fs.StringVar(&f.artifact, "artifact", "", "artifact id or build run id")
	fs.StringVar(&f.image, "image", "", "image pinned by digest (instead of --artifact)")
	fs.StringVar(&f.version, "version", "", "version label for the record (default: the artifact's)")
	fs.StringVar(&f.migrate, "migrate", "", "Job manifest to run with the new image before switching (see docs)")
	fs.DurationVar(&f.migrateWait, "migrate-timeout", 180*time.Second, "how long to wait for the migration Job (max 600s)")
	fs.StringVar(&f.placeholder, "migrate-placeholder", opsrun.ImagePlaceholder, "text in the Job manifest replaced by the release image (e.g. IMAGE)")
	fs.BoolVar(&f.ackRegistryChange, "ack-registry-change", false, "allow moving to another registry or owner without a pull secret for it (public image)")
	fs.BoolVar(&f.verifyPull, "verify-pull", false, "request the image manifest from the registry with the pull secrets (needs network access to the registry)")
	fs.StringVar(&f.root, "project", "", "local project directory")
	fs.DurationVar(&f.timeout, "timeout", maxRolloutTimeout, "rollout timeout per workload (max 180s)")
}

func jsonFail(stderr io.Writer, err error) int {
	_ = json.NewEncoder(stderr).Encode(map[string]string{"error": err.Error()})
	return 1
}

func cmdReleaseOps(sub string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("release "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f releaseFlags
	planID := ""
	switch sub {
	case "plan", "image":
		addReleaseFlags(fs, &f)
		if sub == "image" {
			fs.BoolVar(&f.yes, "yes", false, "apply immediately; without it only the plan is saved and printed")
		}
	case "apply":
		fs.StringVar(&planID, "plan", "", "plan id printed by release plan")
		fs.StringVar(&f.root, "project", "", "local project directory")
		fs.DurationVar(&f.timeout, "timeout", maxRolloutTimeout, "rollout timeout per workload (max 180s)")
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if f.timeout <= 0 || f.timeout > maxRolloutTimeout {
		fmt.Fprintln(stderr, "--timeout must be between 1s and 180s; the release lock lasts 300s")
		return 2
	}
	if f.migrate != "" && (f.migrateWait <= 0 || f.migrateWait > maxJobWait) {
		fmt.Fprintln(stderr, "--migrate-timeout must be between 1s and 600s")
		return 2
	}
	if sub == "apply" {
		if planID == "" {
			fmt.Fprintln(stderr, "usage: devopspadcli release apply --plan <plan id> [--timeout 180s]")
			return 2
		}
		r, err := applySavedPlan(f.root, planID, f.timeout, stderr)
		return printRecord(r, err, stdout, stderr)
	}
	if f.application != "" {
		if f.workload == "" {
			f.workload = f.application
		}
		if f.container == "" {
			fmt.Fprintln(stderr, "--container is required with --application")
			return 2
		}
		f.targets = append(targetList{{Application: f.application, Workload: f.workload, Container: f.container}}, f.targets...)
	}
	if f.environment == "" || len(f.targets) == 0 || (f.artifact == "") == (f.image == "") {
		fmt.Fprintln(stderr, "usage: devopspadcli release "+sub+" --environment <env> (--application <app> --container <c> [--workload <w>] | --target app:workload:container ...) (--artifact <id> | --image <ref@sha256:...>)")
		return 2
	}
	saved, err := makeReleasePlan(f, stderr)
	if err != nil {
		return jsonFail(stderr, err)
	}
	if sub == "plan" || !f.yes {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(saved)
		fmt.Fprintf(stderr, "Plan %s saved until %s; apply it with devopspadcli release apply --plan %s\n", saved.ID, saved.ExpiresAt, saved.ID)
		return 0
	}
	r, err := applySavedPlan(f.root, saved.ID, f.timeout, stderr)
	return printRecord(r, err, stdout, stderr)
}

func connectionStore() (*workspace.Store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	conns, err := workspace.NewStore(dir, workspace.Keychain{})
	if err != nil {
		return nil, err
	}
	return conns, conns.Ready()
}

// resolveTargets resolves every target application in one environment; they
// must share the environment's cluster connection.
func resolveTargets(root, environment string, targets []opsrun.SavedTarget) ([]opsrun.Target, error) {
	store, err := projectStore(root)
	if err != nil {
		return nil, err
	}
	conns, err := connectionStore()
	if err != nil {
		return nil, err
	}
	var out []opsrun.Target
	for _, st := range targets {
		t, err := opsrun.Resolve(store, conns, environment, st.Application)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", st.Application, err)
		}
		out = append(out, t)
	}
	return out, nil
}

func makeReleasePlan(f releaseFlags, stderr io.Writer) (opsrun.SavedPlan, error) {
	ts, err := resolveTargets(f.root, f.environment, f.targets)
	if err != nil {
		return opsrun.SavedPlan{}, err
	}
	now := time.Now().UTC()
	saved := opsrun.SavedPlan{ID: uuid.NewString(), Environment: f.environment, Image: f.image, Version: f.version,
		CreatedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(opsrun.PlanTTL).Format(time.RFC3339)}
	if f.artifact != "" {
		var art ops.Artifact
		for _, a := range ts[0].Snapshot.Artifacts {
			if a.ID == f.artifact || (a.RunID != "" && a.RunID == f.artifact) {
				art = a
			}
		}
		if art.ID == "" {
			return saved, fmt.Errorf("artifact %s not found in the project; list them with devopspadcli runs list", f.artifact)
		}
		saved.Image, saved.RunID = art.Image, art.RunID
		if saved.Version == "" {
			saved.Version = art.Version
		}
	}
	if !ops.ValidImage(saved.Image) {
		return saved, errors.New("the image must be pinned by digest: registry/name@sha256:<64 hex>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, ts[0].Connection)
	if err != nil {
		return saved, err
	}
	defer closeFn()
	var pulls []release.PullTarget
	for i, st := range f.targets {
		p, err := release.PrepareDesktop(ctx, cl, ts[i].Environment.Namespace, st.Workload, st.Container, saved.Image, ts[i].Application.Selector)
		if err != nil {
			return saved, fmt.Errorf("%s: %w", st.Workload, err)
		}
		pulls = append(pulls, release.PullTargetOf(p))
		st.BeforeImage, st.UID, st.ResourceVersion = p.BeforeImage, p.UID, p.ResourceVersion
		saved.Targets = append(saved.Targets, st)
		fmt.Fprintf(stderr, "Plan: %s/%s container %s: %s -> %s (server dry-run passed)\n", ts[i].Environment.Namespace, st.Workload, st.Container, p.BeforeImage, saved.Image)
	}
	if f.migrate != "" {
		m, pull, err := planMigration(ctx, cl, f.migrate, ts[0].Environment.Namespace, saved, f.migrateWait, f.placeholder)
		if err != nil {
			return saved, err
		}
		saved.Migrate = m
		pulls = append(pulls, pull)
		fmt.Fprintf(stderr, "Plan: run migration Job %s from %s first (server dry-run passed)\n", m.Name, f.migrate)
	}
	// A server dry-run accepts missing pull secrets and never pulls; check before applying.
	notes, err := release.PreflightPull(ctx, cl, pulls, release.PreflightOptions{AckRegistryChange: f.ackRegistryChange, Verify: f.verifyPull})
	if err != nil {
		return saved, err
	}
	saved.Notes = notes
	for _, n := range notes {
		fmt.Fprintln(stderr, "Plan: "+n)
	}
	dir, err := configDir()
	if err != nil {
		return saved, err
	}
	return saved, opsrun.SavePlan(dir, saved)
}

func applySavedPlan(root, id string, timeout time.Duration, stderr io.Writer) (ops.Record, error) {
	dir, err := configDir()
	if err != nil {
		return ops.Record{}, err
	}
	saved, err := opsrun.TakePlan(dir, id)
	if err != nil {
		return ops.Record{}, err
	}
	ts, err := resolveTargets(root, saved.Environment, saved.Targets)
	if err != nil {
		return ops.Record{}, err
	}
	budget := timeout * time.Duration(len(saved.Targets))
	if saved.Migrate != nil {
		budget += time.Duration(saved.Migrate.WaitTimeoutSeconds+30) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget+30*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, ts[0].Connection)
	if err != nil {
		return ops.Record{}, err
	}
	var plans []release.DesktopPlan
	for i, st := range saved.Targets {
		p, err := opsrun.Reprepare(ctx, cl, ts[i], saved, st)
		if err != nil {
			closeFn()
			return ops.Record{}, err
		}
		plans = append(plans, p)
	}
	closeFn()
	var pre func(context.Context, dynamic.Interface, func(string) error) error
	if saved.Migrate != nil {
		pre = migrationStep(ts[0], saved)
	}
	return opsrun.ApplyBatch(ctx, ts[0].Store, ts[0].Connection, plans, opsrun.BatchRecord(ts[0].Environment, saved), pre, func(step string) { fmt.Fprintln(stderr, step) })
}
