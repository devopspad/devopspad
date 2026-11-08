package main

import (
	"context"
	"errors"
	"time"

	"devopspad/internal/opsrun"
	"devopspad/internal/workspace"
)

func (a *App) ReadOpsNginx(environment, application string) (workspace.NginxConfig, error) {
	_, _, _, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return workspace.NginxConfig{}, err
	}
	if app.NginxPath == "" {
		return workspace.NginxConfig{}, errors.New("This application has no nginxPath configured")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return workspace.ReadNginx(ctx, c, app.NginxPath)
}

func (a *App) SaveOpsNginx(environment, application string, update workspace.NginxUpdate) (workspace.NginxConfig, error) {
	store, snapshot, env, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return workspace.NginxConfig{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	_, result, err := opsrun.SaveNginx(ctx, opsrun.Target{Store: store, Snapshot: snapshot, Environment: env, Application: app, Connection: c}, update, "")
	return result, err
}
