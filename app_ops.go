package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devopspad/internal/archive"
	"devopspad/internal/ops"
	"devopspad/internal/opsrun"
	"devopspad/internal/release"
	"devopspad/internal/workspace"
	"github.com/gotomicro/ego/core/elog"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type pendingOpsRelease struct {
	plan       release.DesktopPlan
	connection workspace.Connection
	store      *ops.Store
	record     ops.Record
}

func (a *App) opsStore() (*ops.Store, error) {
	a.opsMu.Lock()
	defer a.opsMu.Unlock()
	if a.ops != nil {
		return a.ops, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(dir, "DevOpsPad", "applications")
	if path, err := os.ReadFile(filepath.Join(dir, "DevOpsPad", "project-path")); err == nil && len(path) > 0 {
		root = string(path)
	}
	a.ops, err = ops.New(root)
	if err == nil {
		a.ops.OnFinished = a.mirrorToArchive
	}
	return a.ops, err
}

// mirrorToArchive copies finished operation records into the selected archive.
func (a *App) mirrorToArchive(r ops.Record, app ops.Application) {
	ar, err := a.archive()
	if err != nil {
		return
	}
	actor := "human/" + strings.ReplaceAll(strings.TrimSpace(archive.GitUser(ar.Dir)), " ", "-")
	if actor == "human/" {
		actor = "human/desktop"
	}
	if _, err := ar.MirrorRecord(r, app, actor); err != nil {
		elog.Warn("archive mirror failed: " + err.Error())
	}
}
func (a *App) GetOpsProject() (ops.Snapshot, error) {
	s, err := a.opsStore()
	if err != nil {
		return ops.Snapshot{}, err
	}
	return s.Read()
}
func (a *App) SaveOpsProject(text, revision string) (ops.Snapshot, error) {
	s, err := a.opsStore()
	if err != nil {
		return ops.Snapshot{}, err
	}
	return s.Save(text, revision)
}
func (a *App) SelectOpsProject() (ops.Snapshot, error) {
	root, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose a local project / Git repository directory"})
	if err != nil {
		return ops.Snapshot{}, err
	}
	if root == "" {
		return a.GetOpsProject()
	}
	s, err := ops.New(root)
	if err != nil {
		return ops.Snapshot{}, err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ops.Snapshot{}, err
	}
	if err = os.MkdirAll(filepath.Join(dir, "DevOpsPad"), 0700); err != nil {
		return ops.Snapshot{}, err
	}
	if err = os.WriteFile(filepath.Join(dir, "DevOpsPad", "project-path"), []byte(root), 0600); err != nil {
		return ops.Snapshot{}, err
	}
	s.OnFinished = a.mirrorToArchive
	a.opsMu.Lock()
	a.ops = s
	a.opsMu.Unlock()
	return s.Read()
}
func (a *App) AddOpsArtifact(artifact ops.Artifact) (ops.Snapshot, error) {
	s, err := a.opsStore()
	if err != nil {
		return ops.Snapshot{}, err
	}
	return s.AddArtifact(artifact)
}
func (a *App) OpsGit(action string) (string, error) {
	s, err := a.opsStore()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
	defer cancel()
	return s.Git(ctx, action)
}
func (a *App) ReadOpsCluster(id, namespace, selector string) (workspace.OpsCluster, error) {
	c, err := a.clusterConnection(id)
	if err != nil {
		return workspace.OpsCluster{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.ReadOpsCluster(ctx, c, namespace, selector)
}
func (a *App) ReadOpsMetrics(id, namespace, selector string) ([]workspace.OpsMetric, error) {
	c, err := a.clusterConnection(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.ReadOpsMetrics(ctx, c, namespace, selector)
}

func (a *App) ReadOpsWorkloads(id, namespace, selector string) (workspace.OpsCluster, error) {
	c, err := a.clusterConnection(id)
	if err != nil {
		return workspace.OpsCluster{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.ReadOpsWorkloads(ctx, c, namespace, selector)
}

func (a *App) SaveConfigMap(id string, request workspace.ConfigMapUpdate) (string, error) {
	c, err := a.clusterConnection(id)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.SaveConfigMap(ctx, c, request)
}

// opsTarget resolves the shared project against local credentials and pins the
// endpoint before planning. No credential enters a shared record.
func (a *App) opsTarget(environment, application string) (*ops.Store, ops.Snapshot, ops.Environment, ops.Application, workspace.Connection, error) {
	if err := a.ready(); err != nil {
		return nil, ops.Snapshot{}, ops.Environment{}, ops.Application{}, workspace.Connection{}, err
	}
	s, err := a.opsStore()
	if err != nil {
		return nil, ops.Snapshot{}, ops.Environment{}, ops.Application{}, workspace.Connection{}, err
	}
	t, err := opsrun.Resolve(s, a.store, environment, application)
	return s, t.Snapshot, t.Environment, t.Application, t.Connection, err
}

func (a *App) queueOpsPlan(s *ops.Store, c workspace.Connection, env ops.Environment, app ops.Application, plan release.DesktopPlan, version, runID string) (release.DesktopPlan, error) {
	record := opsrun.PlanRecord(opsrun.Target{Environment: env, Application: app}, plan, version, runID)
	a.opsMu.Lock()
	defer a.opsMu.Unlock()
	if a.opsPlans == nil {
		a.opsPlans = map[string]pendingOpsRelease{}
	}
	for id, p := range a.opsPlans {
		at, _ := time.Parse(time.RFC3339, p.plan.ExpiresAt)
		if time.Now().After(at) {
			delete(a.opsPlans, id)
		}
	}
	if len(a.opsPlans) >= 100 {
		return plan, errors.New("Too many pending plans; wait for them to expire and try again")
	}
	a.opsPlans[plan.ID] = pendingOpsRelease{plan: plan, connection: c, store: s, record: record}
	return plan, nil
}

func (a *App) PlanOpsRelease(environment, application, workload, container, artifactID string) (release.DesktopPlan, error) {
	s, snapshot, env, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	var artifact ops.Artifact
	for _, v := range snapshot.Artifacts {
		if v.ID == artifactID && v.Application == application {
			artifact = v
		}
	}
	if artifact.ID == "" {
		return release.DesktopPlan{}, errors.New("Artifact not found; reload the project")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	cl, close, err := workspace.OpenCluster(ctx, c)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	defer close()
	plan, err := release.PrepareDesktop(ctx, cl, env.Namespace, workload, container, artifact.Image, app.Selector)
	if err != nil {
		return plan, err
	}
	return a.queueOpsPlan(s, c, env, app, plan, artifact.Version, artifact.RunID)
}

func (a *App) PlanOpsWorkloadAction(environment, application, workload, action string, replicas int64) (release.DesktopPlan, error) {
	s, _, env, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	cl, close, err := workspace.OpenCluster(ctx, c)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	defer close()
	plan, err := release.PrepareDesktopAction(ctx, cl, env.Namespace, workload, app.Selector, action, replicas)
	if err != nil {
		return plan, err
	}
	return a.queueOpsPlan(s, c, env, app, plan, "", "")
}

func (a *App) PlanOpsRollback(environment, application, recordID string) (release.DesktopPlan, error) {
	s, snapshot, env, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	var record ops.Record
	for _, r := range snapshot.Releases {
		if r.ID == recordID && len(r.Targets) > 1 {
			return release.DesktopPlan{}, errors.New("This release updated several workloads; roll each back with devopspadcli release image --image <previous digest>")
		}
		if r.ID == recordID && r.Application == application && r.Environment == environment && r.Namespace == env.Namespace && r.Connection == env.Connection && r.Status == "succeeded" && ops.ValidImage(r.Image) {
			record = r
		}
	}
	if record.ID == "" {
		return release.DesktopPlan{}, errors.New("No successful image release found for this environment")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	cl, close, err := workspace.OpenCluster(ctx, c)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	defer close()
	plan, err := release.PrepareDesktop(ctx, cl, env.Namespace, record.Workload, record.Container, record.Image, app.Selector)
	if err != nil {
		return plan, err
	}
	plan.Action = "rollback"
	return a.queueOpsPlan(s, c, env, app, plan, record.Version, record.RunID)
}

func (a *App) ApplyOpsRelease(planID string) (ops.Record, error) {
	a.opsMu.Lock()
	pending, ok := a.opsPlans[planID]
	delete(a.opsPlans, planID)
	a.opsMu.Unlock()
	if !ok {
		return ops.Record{}, errors.New("The plan does not exist or was already used; create a new one")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 180*time.Second)
	defer cancel()
	return opsrun.Apply(ctx, pending.store, pending.connection, pending.plan, pending.record, nil)
}

// AttachOpsWorkload discovers selectors from the live resource, never from
// caller-supplied YAML, and writes only the local project mapping.
type OpsAttachRequest struct {
	ConnectionID string `json:"connectionId"`
	Namespace    string `json:"namespace"`
	Workload     string `json:"workload"`
	UID          string `json:"uid"`
	Resource     string `json:"resource"`
	Environment  string `json:"environment"`
	Application  string `json:"application"`
	Revision     string `json:"revision"`
}

func (a *App) AttachOpsWorkload(request OpsAttachRequest) (ops.Snapshot, error) {
	s, err := a.opsStore()
	if err != nil {
		return ops.Snapshot{}, err
	}
	c, err := a.clusterConnection(request.ConnectionID)
	if err != nil {
		return ops.Snapshot{}, err
	}
	for _, other := range a.store.List() {
		if other.ID != c.ID && other.Name == c.Name && (other.Kind == "k3s" || other.Kind == "k8s") {
			return ops.Snapshot{}, errors.New("Duplicate cluster connection name; rename it first")
		}
	}
	if request.Namespace == "" || request.Namespace == "*" || request.UID == "" {
		return ops.Snapshot{}, errors.New("Choose a specific namespace and workload")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
	defer cancel()
	cluster, err := workspace.ReadOpsCluster(ctx, c, request.Namespace, "")
	if err != nil {
		return ops.Snapshot{}, err
	}
	for _, w := range cluster.Workloads {
		if w.Name == request.Workload && w.UID == request.UID && w.Resource == request.Resource {
			if w.Selector == "" {
				return ops.Snapshot{}, errors.New("The workload has no usable selector; set one in the application config")
			}
			return s.Attach(ops.Environment{Name: request.Environment, Connection: c.Name, Namespace: request.Namespace}, ops.Application{Name: request.Application, Selector: w.Selector}, request.Revision)
		}
	}
	return ops.Snapshot{}, errors.New("The workload changed or is out of range; refresh the list and try again")
}
