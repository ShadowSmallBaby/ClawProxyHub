package extension

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// UpdateDependents 由宿主暂停、恢复依赖方，直至运行时状态提交或回滚结束。
type UpdateDependents interface {
	Suspend() error
	Resume(context.Context) error
	Close()
}

// SetUpdateDependents 在更新或停用已有可用包前暂停依赖方；停用成功后不恢复会话。
func (m *Manager) SetUpdateDependents(prepare func(context.Context, State) (UpdateDependents, error)) {
	m.op.Lock()
	defer m.op.Unlock()
	m.prepareUpdate = prepare
}

func resumeDependents(ctx context.Context, dependents UpdateDependents) error {
	if dependents == nil {
		return nil
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	return dependents.Resume(recovery)
}

func (m *Manager) rollbackUpdate(ctx context.Context, next, old State, exists, published bool, dependents UpdateDependents) error {
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	if dependents != nil {
		if err := dependents.Suspend(); err != nil {
			next.Error = "rollback: " + err.Error()
			m.put(next.Manifest.ID, &next)
			return errors.Join(fmt.Errorf("cannot stop dependents for rollback: %w", err), m.persist(), dependents.Resume(recovery))
		}
	}
	if err := m.stop(recovery, next); err != nil {
		return fmt.Errorf("cannot stop updated package for rollback: %w", err)
	}
	if !exists {
		old.Manifest.ID = next.Manifest.ID
	}
	restoreErr := m.restore(recovery, old, exists)
	persistErr := m.persist()
	if restoreErr != nil || published && persistErr != nil {
		return errors.Join(restoreErr, persistErr)
	}
	if dependents != nil {
		return errors.Join(persistErr, dependents.Resume(recovery))
	}
	return persistErr
}
