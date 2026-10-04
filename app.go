package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gotomicro/ego/core/elog"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"infradex/internal/workspace"
)

type App struct {
	ctx     context.Context
	store   *workspace.Store
	initErr error
	mu      sync.Mutex
	queries map[string]context.CancelFunc
}

func NewApp() *App { return &App{queries: map[string]context.CancelFunc{}} }
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dir, e := os.UserConfigDir()
	if e != nil {
		a.initErr = e
		return
	}
	a.store, a.initErr = workspace.NewStore(filepath.Join(dir, "Infradex"), workspace.Keychain{})
	// A blocked Store remains available for health checks and explicit recovery.
	if a.store != nil {
		a.initErr = nil
	}
	elog.Info("Infradex desktop started")
}
func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cancel := range a.queries {
		cancel()
	}
}
func (a *App) ready() error {
	if a.initErr != nil {
		return a.initErr
	}
	if a.store == nil {
		return errors.New("Workspace is not initialized")
	}
	return a.store.Ready()
}
func (a *App) ListConnections() ([]workspace.Connection, error) {
	if e := a.ready(); e != nil {
		return nil, e
	}
	connections := a.store.List()
	for i, c := range connections {
		if c.Kind == "k8s" || c.Kind == "k3s" {
			if c.Database == "" {
				connections[i].Database = "*"
			}
			if c.Context == "" {
				if info, e := workspace.ReadKubeconfig(c.Kubeconfig); e == nil {
					connections[i].Context = info.CurrentContext
				}
			}
		}
	}
	return connections, nil
}
func (a *App) SaveConnection(c workspace.Connection) (workspace.Connection, error) {
	if e := a.ready(); e != nil {
		return c, e
	}
	if c.Kind == "k8s" || c.Kind == "k3s" {
		info, e := workspace.ReadKubeconfig(c.Kubeconfig)
		if e != nil {
			return workspace.Connection{}, e
		}
		if c.Context == "" {
			c.Context = info.CurrentContext
		}
		found := false
		for _, item := range info.Contexts {
			if item.Name == c.Context {
				found = true
				break
			}
		}
		if !found {
			return workspace.Connection{}, errors.New("Kubeconfig context was not found")
		}
		if c.Database == "" {
			c.Database = "*"
		}
	}
	return a.store.Save(c)
}
func (a *App) DeleteConnection(id string) error {
	if e := a.ready(); e != nil {
		return e
	}
	return a.store.Delete(id)
}
func (a *App) TestConnection(c workspace.Connection) (int64, error) {
	if e := a.ready(); e != nil {
		return 0, e
	}
	c, e := a.store.Resolve(c)
	if e != nil {
		return 0, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	start := time.Now()
	e = workspace.Test(ctx, c)
	return time.Since(start).Milliseconds(), e
}
func (a *App) Browse(id string) ([]workspace.Resource, error) {
	if e := a.ready(); e != nil {
		return nil, e
	}
	c, e := a.store.Get(id)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.Browse(ctx, c)
}
func (a *App) Execute(id, query, requestID string) (workspace.Result, error) {
	return a.execute(id, "", "", query, requestID)
}
func (a *App) execute(id, database, kubeContext, query, requestID string) (workspace.Result, error) {
	if e := a.ready(); e != nil {
		return workspace.Result{}, e
	}
	c, e := a.store.Get(id)
	if e != nil {
		return workspace.Result{}, e
	}
	if database != "" {
		c.Database = database
	}
	if kubeContext != "" {
		c.Context = kubeContext
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	a.mu.Lock()
	if _, exists := a.queries[requestID]; exists {
		a.mu.Unlock()
		return workspace.Result{}, errors.New("Query ID already in use")
	}
	a.queries[requestID] = cancel
	a.mu.Unlock()
	defer func() { a.mu.Lock(); delete(a.queries, requestID); a.mu.Unlock() }()
	return workspace.Execute(ctx, c, query)
}
func (a *App) CancelQuery(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cancel := a.queries[id]; cancel != nil {
		cancel()
	}
}
func (a *App) GetAISettings() (workspace.AISettings, error) {
	if e := a.ready(); e != nil {
		return workspace.AISettings{}, e
	}
	return a.store.AI(), nil
}
func (a *App) SaveAISettings(c workspace.AISettings) error {
	if e := a.ready(); e != nil {
		return e
	}
	return a.store.SaveAI(c)
}
func (a *App) AskAI(r workspace.AIRequest) (string, error) {
	if e := a.ready(); e != nil {
		return "", e
	}
	return a.store.Ask(a.ctx, r)
}
func (a *App) SelectKubeconfig() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose kubeconfig", ShowHiddenFiles: true})
}
func (a *App) ExportCSV(content string) (bool, error) {
	if len(content) > 20<<20 {
		return false, errors.New("Export exceeds 20 MB")
	}
	path, e := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Export results", DefaultFilename: "infradex-results.csv", Filters: []runtime.FileFilter{{DisplayName: "CSV files", Pattern: "*.csv"}}})
	if e != nil || path == "" {
		return false, e
	}
	return true, os.WriteFile(path, []byte(content), 0600)
}

func (a *App) ListDatabases(id string) ([]string, error) {
	if e := a.ready(); e != nil {
		return nil, e
	}
	c, e := a.store.Get(id)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.ListDatabases(ctx, c)
}
func (a *App) BrowseResources(id string, request workspace.BrowseRequest) (workspace.ResourcePage, error) {
	if e := a.ready(); e != nil {
		return workspace.ResourcePage{}, e
	}
	c, e := a.store.Get(id)
	if e != nil {
		return workspace.ResourcePage{}, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.BrowsePage(ctx, c, request)
}
func (a *App) InspectResource(id, database, name string) (workspace.ResourceDetails, error) {
	if e := a.ready(); e != nil {
		return workspace.ResourceDetails{}, e
	}
	c, e := a.store.Get(id)
	if e != nil {
		return workspace.ResourceDetails{}, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.InspectResource(ctx, c, database, name)
}

// Scope belongs to a query tab; selecting another database never rewrites the profile.
func (a *App) ExecuteScoped(id, database, query, requestID string) (workspace.Result, error) {
	return a.execute(id, database, "", query, requestID)
}

func (a *App) ReadKubeconfig(path string) (workspace.KubeconfigInfo, error) {
	return workspace.ReadKubeconfig(path)
}
func (a *App) ExecuteInContext(id, database, kubeContext, query, requestID string) (workspace.Result, error) {
	return a.execute(id, database, kubeContext, query, requestID)
}

func (a *App) clusterConnection(id string) (workspace.Connection, error) {
	if e := a.ready(); e != nil {
		return workspace.Connection{}, e
	}
	return a.store.Get(id)
}
func (a *App) InspectClusterResource(id string, ref workspace.ClusterRef) (workspace.ClusterDetails, error) {
	c, e := a.clusterConnection(id)
	if e != nil {
		return workspace.ClusterDetails{}, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.InspectClusterResource(ctx, c, ref)
}
func (a *App) ClusterEvents(id string, ref workspace.ClusterRef) (workspace.Result, error) {
	c, e := a.clusterConnection(id)
	if e != nil {
		return workspace.Result{}, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.ClusterEvents(ctx, c, ref)
}
func (a *App) ReadPodLogs(id string, request workspace.PodLogRequest) (workspace.PodLogs, error) {
	c, e := a.clusterConnection(id)
	if e != nil {
		return workspace.PodLogs{}, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return workspace.ReadPodLogs(ctx, c, request)
}

func (a *App) SelectSSHFile(kind string) (string, error) {
	title := "Choose SSH private key"
	if kind == "knownHosts" {
		title = "Choose SSH known_hosts file"
	} else if kind != "privateKey" {
		return "", errors.New("Unsupported SSH file type")
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title, ShowHiddenFiles: true})
}
