package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"infradex/internal/ops"
	"infradex/internal/workspace"
	"time"
)

func (a *App) ReadOpsNginx(environment, application string) (workspace.NginxConfig, error) {
	_, _, _, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return workspace.NginxConfig{}, err
	}
	if app.NginxPath == "" {
		return workspace.NginxConfig{}, errors.New("此应用尚未配置 nginxPath")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return workspace.ReadNginx(ctx, c, app.NginxPath)
}

func (a *App) SaveOpsNginx(environment, application string, update workspace.NginxUpdate) (workspace.NginxConfig, error) {
	store, _, env, app, c, err := a.opsTarget(environment, application)
	if err != nil {
		return workspace.NginxConfig{}, err
	}
	if app.NginxPath == "" {
		return workspace.NginxConfig{}, errors.New("此应用尚未配置 nginxPath")
	}
	record := ops.Record{ID: uuid.NewString(), Action: "nginx", Application: app.Name, Environment: env.Name, Connection: env.Connection, Namespace: env.Namespace, Workload: app.Name, Version: "Nginx", Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Steps: []string{"读取当前配置并校验版本"}}
	if err = store.WriteRecord(record); err != nil {
		return workspace.NginxConfig{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	result, err := workspace.SaveNginx(ctx, c, app.NginxPath, update)
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	record.Status = "succeeded"
	record.Message = result.Message
	if err != nil {
		record.Status = "failed"
		record.Message = err.Error()
	} else {
		record.Steps = append(record.Steps, result.Message)
	}
	if saveErr := store.WriteRecord(record); saveErr != nil {
		return result, fmt.Errorf("Nginx 操作状态 %s，但记录保存失败，请刷新核对", record.Status)
	}
	return result, err
}
