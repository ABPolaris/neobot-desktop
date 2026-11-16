package main

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"neobot-desktop/internal/launcher"
)

type App struct {
	ctx     context.Context
	manager *launcher.Manager
	initErr error
	mu      sync.RWMutex
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx
	root, err := launcher.AppData()
	if err == nil {
		a.manager, err = launcher.Open(root, func() { runtime.EventsEmit(ctx, "launcher:changed") })
	}
	a.initErr = err
}

func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	m := a.manager
	a.manager = nil
	a.mu.Unlock()
	if m != nil {
		m.Close()
	}
}

func (a *App) get() (*launcher.Manager, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.initErr != nil {
		return nil, a.initErr
	}
	if a.manager == nil {
		return nil, errors.New("启动器尚未就绪")
	}
	return a.manager, nil
}

func (a *App) GetSnapshot() (launcher.Snapshot, error) {
	m, err := a.get()
	if err != nil {
		return launcher.Snapshot{}, err
	}
	return m.Snapshot(), nil
}

func (a *App) GetLogs(revision uint64) (launcher.LogSnapshot, error) {
	m, err := a.get()
	if err != nil {
		return launcher.LogSnapshot{}, err
	}
	return m.Logs(revision), nil
}

func (a *App) ListVersions() ([]launcher.Version, error) {
	m, err := a.get()
	if err != nil {
		return nil, err
	}
	return m.Available()
}

func (a *App) SaveSources(s launcher.Sources) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.SaveSources(s)
}

func (a *App) Install(version string, official, start bool) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.Install(version, official, start)
}

func (a *App) InstallBranch(branch string, start, refresh bool) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.InstallBranch(branch, start, refresh)
}

func (a *App) ListGitHubBranches(force bool) ([]launcher.Branch, error) {
	m, err := a.get()
	if err != nil {
		return nil, err
	}
	return m.ListGitHubBranches(force)
}

func (a *App) SetVersionIsolation(enabled bool) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.SetVersionIsolation(enabled)
}

func (a *App) SelectVersion(version string) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.SelectVersion(version)
}

func (a *App) SwitchInfo() (launcher.DataSwitch, error) {
	m, err := a.get()
	if err != nil {
		return launcher.DataSwitch{}, err
	}
	return m.SwitchInfo(), nil
}

func (a *App) Start(confirmed bool) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.Start(confirmed)
}

func (a *App) Stop(force bool) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.Stop(force)
}

func (a *App) OpenDashboard() error {
	m, err := a.get()
	if err != nil {
		return err
	}
	address := m.Snapshot().DashboardURL
	if address == "" {
		return errors.New("网页面板尚未就绪")
	}
	runtime.BrowserOpenURL(a.ctx, address)
	return nil
}

func (a *App) BindQQ(revision string, refresh bool) error {
	m, err := a.get()
	if err != nil {
		return err
	}
	return m.BindQQ(revision, refresh)
}

func (a *App) SaveQQ(revision, id, secret string) (launcher.SaveResult, error) {
	m, err := a.get()
	if err != nil {
		return launcher.SaveResult{}, err
	}
	return m.SaveQQ(revision, id, secret)
}

func (a *App) SaveModel(revision, envRevision, key, url, name string) (launcher.SaveResult, error) {
	m, err := a.get()
	if err != nil {
		return launcher.SaveResult{}, err
	}
	return m.SaveModel(revision, envRevision, key, url, name)
}
