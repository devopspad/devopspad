package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"infradex/internal/ops"
	"infradex/internal/release"
	"infradex/internal/workspace"
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
	root := filepath.Join(dir, "Infradex", "applications")
	if path, err := os.ReadFile(filepath.Join(dir, "Infradex", "project-path")); err == nil && len(path) > 0 {
		root = string(path)
	}
	a.ops, err = ops.New(root)
	return a.ops, err
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
	root, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择本地项目 / Git 仓库目录"})
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
	if err = os.MkdirAll(filepath.Join(dir, "Infradex"), 0700); err != nil {
		return ops.Snapshot{}, err
	}
	if err = os.WriteFile(filepath.Join(dir, "Infradex", "project-path"), []byte(root), 0600); err != nil {
		return ops.Snapshot{}, err
	}
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
	var snapshot ops.Snapshot
	var env ops.Environment
	var app ops.Application
	var c workspace.Connection
	if err := a.ready(); err != nil {
		return nil, snapshot, env, app, c, err
	}
	s, err := a.opsStore()
	if err != nil {
		return nil, snapshot, env, app, c, err
	}
	snapshot, err = s.Read()
	if err != nil {
		return s, snapshot, env, app, c, err
	}
	for _, e := range snapshot.Project.Environments {
		if e.Name == environment {
			env = e
		}
	}
	for _, v := range snapshot.Project.Applications {
		if v.Name == application {
			app = v
		}
	}
	if env.Name == "" || app.Name == "" {
		return s, snapshot, env, app, c, errors.New("环境或应用不存在，请刷新项目")
	}
	id := ""
	for _, conn := range a.store.List() {
		if conn.Name == env.Connection && (conn.Kind == "k8s" || conn.Kind == "k3s") {
			if id != "" {
				return s, snapshot, env, app, c, errors.New("连接名称重复，请为集群连接设置唯一名称")
			}
			id = conn.ID
		}
	}
	if id == "" {
		return s, snapshot, env, app, c, errors.New("请先在资源中心配置与环境同名的集群连接")
	}
	c, err = a.store.Get(id)
	if err == nil {
		c, err = workspace.PinClusterConnection(c)
	}
	return s, snapshot, env, app, c, err
}

func (a *App) queueOpsPlan(s *ops.Store, c workspace.Connection, env ops.Environment, app ops.Application, plan release.DesktopPlan, version, runID string) (release.DesktopPlan, error) {
	record := ops.Record{RunID: runID, ID: plan.ID, Action: plan.Action, Application: app.Name, Environment: env.Name, Connection: env.Connection, Namespace: env.Namespace, Workload: plan.Workload, Container: plan.Container, Image: plan.Image, BeforeImage: plan.BeforeImage, Version: version, Steps: []string{}}
	if plan.Action == "scale" || plan.Action == "restart" {
		record.BeforeReplicas = &plan.BeforeReplicas
		record.Replicas = &plan.Replicas
	}
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
		return plan, errors.New("待执行计划过多，请等待过期后重试")
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
		return release.DesktopPlan{}, errors.New("制品不存在，请刷新项目")
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
		if r.ID == recordID && r.Application == application && r.Environment == environment && r.Namespace == env.Namespace && r.Connection == env.Connection && r.Status == "succeeded" && ops.ValidImage(r.Image) {
			record = r
		}
	}
	if record.ID == "" {
		return release.DesktopPlan{}, errors.New("没有找到当前环境的成功镜像发布记录")
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
		return ops.Record{}, errors.New("发布计划不存在或已经使用，请重新生成")
	}
	r := pending.record
	r.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Status = "running"
	if err := pending.store.WriteRecord(r); err != nil {
		return r, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 180*time.Second)
	defer cancel()
	cl, close, err := workspace.OpenCluster(ctx, pending.connection)
	if err == nil {
		defer close()
		err = release.ApplyDesktop(ctx, cl, pending.plan, func(step string) error { r.Steps = append(r.Steps, step); return pending.store.WriteRecord(r) })
	}
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		r.Status = "failed"
		r.Message = err.Error()
	} else {
		r.Status = "succeeded"
		r.Message = "Deployment 已就绪"
	}
	if saveErr := pending.store.WriteRecord(r); saveErr != nil {
		return r, fmt.Errorf("发布结果 %s，但保存记录失败: %w", r.Status, saveErr)
	}
	return r, nil
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
			return ops.Snapshot{}, errors.New("集群连接名称重复，请先重命名")
		}
	}
	if request.Namespace == "" || request.Namespace == "*" || request.UID == "" {
		return ops.Snapshot{}, errors.New("请选择具体命名空间及负载")
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
				return ops.Snapshot{}, errors.New("负载没有可用选择器，请在应用配置中指定")
			}
			return s.Attach(ops.Environment{Name: request.Environment, Connection: c.Name, Namespace: request.Namespace}, ops.Application{Name: request.Application, Selector: w.Selector}, request.Revision)
		}
	}
	return ops.Snapshot{}, errors.New("负载已变化或不在读取范围，请刷新列表后重试")
}
