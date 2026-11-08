package main

import (
	"context"
	"errors"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/opsrun"
	"devopspad/internal/release"
	"devopspad/internal/workspace"
)

type pendingSecret struct {
	plan    release.SecretPlan
	target  opsrun.Target
	expires time.Time
}

// OpsSecretRequest carries Secret values from the desktop form. They stay in
// this process and are never returned or written to disk.
type OpsSecretRequest struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Data     map[string]string `json:"data"`
	Server   string            `json:"server"`
	Username string            `json:"username"`
	Password string            `json:"password"`
}

func (a *App) opsResolve(environment, application string) (opsrun.Target, error) {
	if err := a.ready(); err != nil {
		return opsrun.Target{}, err
	}
	s, err := a.opsStore()
	if err != nil {
		return opsrun.Target{}, err
	}
	return opsrun.Resolve(s, a.store, environment, application)
}

// DescribeOpsSecret lists a Secret's key names and last change, never values.
func (a *App) DescribeOpsSecret(environment, application, name string) (release.SecretInfo, error) {
	t, err := a.opsResolve(environment, application)
	if err != nil {
		return release.SecretInfo{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, t.Connection)
	if err != nil {
		return release.SecretInfo{}, err
	}
	defer closeFn()
	return release.DescribeSecret(ctx, cl, t.Environment.Namespace, name)
}

// PlanOpsSecret dry-runs a Secret change and keeps it in memory for ApplyOpsSecret.
func (a *App) PlanOpsSecret(environment, application string, req OpsSecretRequest) (release.SecretPlan, error) {
	t, err := a.opsResolve(environment, application)
	if err != nil {
		return release.SecretPlan{}, err
	}
	data := map[string][]byte{}
	if req.Type == "docker-registry" {
		cfg, err := release.DockerConfigJSON(req.Server, req.Username, req.Password)
		if err != nil {
			return release.SecretPlan{}, err
		}
		data[".dockerconfigjson"] = cfg
	} else {
		for k, v := range req.Data {
			data[k] = []byte(v)
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, t.Connection)
	if err != nil {
		return release.SecretPlan{}, err
	}
	defer closeFn()
	plan, err := release.PrepareSecret(ctx, cl, t.Environment.Namespace, req.Name, req.Type, data)
	if err != nil {
		return plan, err
	}
	a.opsMu.Lock()
	defer a.opsMu.Unlock()
	if a.secretPlans == nil {
		a.secretPlans = map[string]pendingSecret{}
	}
	for id, p := range a.secretPlans {
		if time.Now().After(p.expires) {
			delete(a.secretPlans, id)
		}
	}
	a.secretPlans[plan.ID] = pendingSecret{plan: plan, target: t, expires: time.Now().Add(10 * time.Minute)}
	return plan, nil
}

func (a *App) ApplyOpsSecret(planID string) (ops.Record, error) {
	a.opsMu.Lock()
	pending, ok := a.secretPlans[planID]
	delete(a.secretPlans, planID)
	a.opsMu.Unlock()
	if !ok || time.Now().After(pending.expires) {
		return ops.Record{}, errors.New("The plan does not exist, expired or was already used; plan again")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return opsrun.ApplySecret(ctx, pending.target, pending.plan)
}

// PlanOpsPatch plans pull-secret and Secret-env changes; apply it with ApplyOpsRelease.
func (a *App) PlanOpsPatch(environment, application, workload string, patch release.WorkloadPatch) (release.DesktopPlan, error) {
	s, _, env, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, c)
	if err != nil {
		return release.DesktopPlan{}, err
	}
	defer closeFn()
	plan, err := release.PrepareDesktopPatch(ctx, cl, env.Namespace, workload, app.Selector, patch)
	if err != nil {
		return plan, err
	}
	return a.queueOpsPlan(s, c, env, app, plan, "", "")
}
