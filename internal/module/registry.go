package module

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var validID = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)

type entry struct {
	impl    Module
	spec    Descriptor
	running bool
}

// Registry 只启动一次；失败或停止后需创建新实例，不将内置模块伪装成可热插拔扩展。
type Registry struct {
	op       sync.Mutex
	mu       sync.RWMutex
	ordered  []*entry
	prepared []*entry
	used     bool
	cancel   context.CancelFunc
}

func New(modules ...Module) (*Registry, error) {
	byID := map[string]*entry{}
	capabilities := map[string]string{}
	for _, impl := range modules {
		if impl == nil {
			return nil, fmt.Errorf("nil module")
		}
		spec := clone(impl.Descriptor())
		if !validID.MatchString(spec.ID) {
			return nil, fmt.Errorf("invalid module id %q", spec.ID)
		}
		if _, exists := byID[spec.ID]; exists {
			return nil, fmt.Errorf("duplicate module %s", spec.ID)
		}
		for _, capability := range spec.Capabilities {
			if !validID.MatchString(capability) {
				return nil, fmt.Errorf("invalid capability %q", capability)
			}
			if owner, exists := capabilities[capability]; exists {
				return nil, fmt.Errorf("capability %s provided by both %s and %s", capability, owner, spec.ID)
			}
			capabilities[capability] = spec.ID
		}
		byID[spec.ID] = &entry{impl: impl, spec: spec}
	}
	var ids []string
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	visited := map[string]int{}
	registry := &Registry{}
	var visit func(string, []string) error
	visit = func(id string, path []string) error {
		e, ok := byID[id]
		if !ok {
			return fmt.Errorf("missing module dependency: %s", strings.Join(append(path, id), " -> "))
		}
		if visited[id] == 1 {
			return fmt.Errorf("module dependency cycle: %s", strings.Join(append(path, id), " -> "))
		}
		if visited[id] == 2 {
			return nil
		}
		visited[id] = 1
		for _, dependency := range e.spec.Requires {
			if err := visit(dependency, append(path, id)); err != nil {
				return err
			}
		}
		visited[id] = 2
		registry.ordered = append(registry.ordered, e)
		return nil
	}
	for _, id := range ids {
		if err := visit(id, nil); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func clone(spec Descriptor) Descriptor {
	spec.Requires = append([]string(nil), spec.Requires...)
	spec.Capabilities = append([]string{}, spec.Capabilities...)
	return spec
}

func (r *Registry) States() []State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]State, 0, len(r.ordered))
	for _, e := range r.ordered {
		out = append(out, State{Descriptor: clone(e.spec), Installed: true, Enabled: true, Available: e.running, RestartRequired: true})
	}
	return out
}

func (r *Registry) Start(ctx context.Context, host Host) (err error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.used {
		return fmt.Errorf("module registry cannot be restarted")
	}
	r.used = true
	ctx, r.cancel = context.WithCancel(ctx)
	defer func() {
		if err != nil {
			r.cancel()
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			err = errors.Join(err, r.stop(cleanupCtx))
		}
	}()
	for _, e := range r.ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.prepared = append(r.prepared, e)
		if err := e.impl.Register(host); err != nil {
			return fmt.Errorf("register module %s: %w", e.spec.ID, err)
		}
	}
	for _, e := range r.ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.impl.Start(ctx); err != nil {
			return fmt.Errorf("start module %s: %w", e.spec.ID, err)
		}
		r.mu.Lock()
		e.running = true
		r.mu.Unlock()
	}
	return nil
}

func (r *Registry) Stop(ctx context.Context) error {
	r.op.Lock()
	defer r.op.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	return r.stop(ctx)
}

func (r *Registry) stop(ctx context.Context) error {
	var errs []error
	for i := len(r.prepared) - 1; i >= 0; i-- {
		e := r.prepared[i]
		r.mu.Lock()
		e.running = false
		r.mu.Unlock()
		if err := e.impl.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop module %s: %w", e.spec.ID, err))
		}
	}
	r.prepared = nil
	return errors.Join(errs...)
}
