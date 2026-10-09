package extension

import (
	"context"
	"errors"
	"fmt"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type DataHooks struct {
	Prepare   func(context.Context, State) error
	Committed func([]State) error
	Clear     func(context.Context, string) error
	Obsolete  func(context.Context, string, *spec.StorageSchema, bool) ([]string, error)
	Snapshot  func(context.Context, string) error
}

func (m *Manager) SetDataHooks(hooks DataHooks) {
	m.op.Lock()
	defer m.op.Unlock()
	m.dataHooks = hooks
}

// ObsoleteTables 与安装串行，以最终提交的清单计算废弃表。
type ObsoleteData struct {
	Hash   string   `json:"hash"`
	Tables []string `json:"tables"`
}

func (m *Manager) ObsoleteTables(ctx context.Context, id string, clean bool, expectedHash string) (ObsoleteData, error) {
	m.op.Lock()
	defer m.op.Unlock()
	state, exists := m.State(id)
	if !exists {
		return ObsoleteData{}, fmt.Errorf("extension not installed")
	}
	if err := m.CheckManagement(ctx, state.Manifest); err != nil {
		return ObsoleteData{}, err
	}
	if clean && expectedHash != state.Hash {
		return ObsoleteData{}, fmt.Errorf("extension changed; review obsolete tables again")
	}
	result := ObsoleteData{Hash: state.Hash, Tables: []string{}}
	if m.dataHooks.Obsolete == nil {
		return result, nil
	}
	var err error
	result.Tables, err = m.dataHooks.Obsolete(ctx, id, state.Manifest.Storage, clean)
	return result, err
}
func (m *Manager) SnapshotData(ctx context.Context, path string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if m.dataHooks.Snapshot == nil {
		return fmt.Errorf("extension storage unavailable")
	}
	return m.dataHooks.Snapshot(ctx, path)
}
func (m *Manager) HasDataStore() bool {
	m.op.Lock()
	defer m.op.Unlock()
	return m.dataHooks.Snapshot != nil
}

// BackendExited 在生命周期锁内核对进程会话，旧进程退出不会覆盖同包重新启用后的状态。
func (m *Manager) BackendExited(ctx context.Context, id, hash string, cause error, current func() bool) error {
	m.op.Lock()
	defer m.op.Unlock()
	state, exists := m.State(id)
	if !exists || state.Hash != hash || !state.Available || !current() {
		return nil
	}
	err := m.stop(ctx, state)
	state.Available = false
	state.Status = "failed"
	state.Error = cause.Error()
	m.put(id, &state)
	return errors.Join(err, m.persist())
}
