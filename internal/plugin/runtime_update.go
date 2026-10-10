package plugin

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// RuntimeUpdate 锁定插件启停，记录更新前正在使用 Lua Host 的会话。
type RuntimeUpdate struct {
	manager *Manager
	dirs    []string
	closed  sync.Once
}

// BeginLuaRuntimeUpdate 暂停运行中的 Lua 插件，不修改持久化启停状态。
func (m *Manager) BeginLuaRuntimeUpdate(ctx context.Context) (*RuntimeUpdate, error) {
	m.runtimeGate.Lock()
	u := &RuntimeUpdate{manager: m}
	if err := ctx.Err(); err != nil {
		u.Close()
		return nil, err
	}
	m.mu.RLock()
	for _, instance := range m.plugins {
		if instance.runtime == "lua" {
			u.dirs = append(u.dirs, instance.dir)
		}
	}
	m.mu.RUnlock()
	sort.Strings(u.dirs)
	if err := u.Suspend(); err != nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		err = errors.Join(err, u.Resume(recovery))
		u.Close()
		return nil, err
	}
	return u, nil
}

// Suspend 在回滚前关闭已恢复的会话；关闭失败时保留会话，阻止替换使用中的运行时。
func (u *RuntimeUpdate) Suspend() error {
	m := u.manager
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	var failures []error
	for _, dir := range u.dirs {
		failures = append(failures, m.stop(filepath.Base(filepath.Clean(dir)), false))
	}
	return errors.Join(failures...)
}

// Resume 只恢复更新前的会话，沿用当前已激活的运行时。
func (u *RuntimeUpdate) Resume(ctx context.Context) error {
	m := u.manager
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	var failures []error
	for _, dir := range u.dirs {
		_, err := m.start(ctx, dir)
		failures = append(failures, err)
	}
	if len(u.dirs) > 0 {
		m.RefreshCatalog(ctx)
	}
	return errors.Join(failures...)
}

// Close 在状态提交或回滚及会话恢复完成后允许新的启停请求。
func (u *RuntimeUpdate) Close() {
	u.closed.Do(u.manager.runtimeGate.Unlock)
}
