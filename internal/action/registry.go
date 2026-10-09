// Package action 在 GUI、CLI 和 MCP 之间共享参数、权限、取消和审计边界。
package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

var ErrForbidden = errors.New("action permission denied")
var ErrUnavailable = errors.New("action unavailable")

const MaxJSONBytes = spec.MaxJSONBytes

// Schema 与扩展声明使用同一协议，保持现有调用接口。
type Schema = spec.Schema

type Descriptor struct {
	ID         string `json:"id"`
	Version    int    `json:"version"`
	Owner      string `json:"owner"`
	Title      string `json:"title"`
	Permission string `json:"permission"`
	Effect     string `json:"effect"`
	TimeoutMS  int    `json:"timeout_ms"`
	Input      Schema `json:"input_schema"`
	Output     Schema `json:"output_schema"`
}
type Principal struct {
	Subject string
	Role    string
	Scopes  []string
}

func (p Principal) Allows(permission string) bool {
	if p.Role != "admin" {
		return false
	}
	if p.Scopes == nil {
		return true
	}
	for _, s := range p.Scopes {
		if s == permission {
			return true
		}
	}
	return false
}

type Audit struct {
	ID         string    `json:"id"`
	Owner      string    `json:"owner"`
	Subject    string    `json:"subject"`
	Effect     string    `json:"effect"`
	Started    time.Time `json:"started"`
	DurationMS int64     `json:"duration_ms"`
	Outcome    string    `json:"outcome"`
}
type Handler func(context.Context, Principal, json.RawMessage) (any, error)
type entry struct {
	descriptor Descriptor
	handler    Handler
	disabled   bool
	calls      map[uint64]context.CancelFunc
	done       chan struct{}
}
type Registry struct {
	mu         sync.Mutex
	entries    map[string]*entry
	serial     uint64
	generation uint64
	audit      func(Audit) error
}

func New(audit func(Audit) error) *Registry {
	return &Registry{entries: map[string]*entry{}, audit: audit}
}
func (r *Registry) Register(d Descriptor, h Handler) error {
	return r.RegisterBatch([]Descriptor{d}, []Handler{h})
}
func (r *Registry) RegisterBatch(ds []Descriptor, hs []Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(ds) != len(hs) {
		return errors.New("handler count mismatch")
	}
	seen := map[string]bool{}
	for i, d := range ds {
		if !spec.ValidID(d.ID) || !spec.ValidID(d.Owner) || !strings.HasPrefix(d.ID, d.Owner+".") || d.Version != 1 || hs[i] == nil || seen[d.ID] || r.entries[d.ID] != nil || !spec.ValidID(d.Permission) || d.TimeoutMS < 1 || d.TimeoutMS > 300000 || (d.Effect != "read" && d.Effect != "write" && d.Effect != "execute") {
			return fmt.Errorf("invalid action %s", d.ID)
		}
		if err := d.Input.Check(); err != nil {
			return err
		}
		if err := d.Output.Check(); err != nil {
			return err
		}
		seen[d.ID] = true
	}
	for i, d := range ds {
		r.entries[d.ID] = &entry{descriptor: d, handler: hs[i], calls: map[uint64]context.CancelFunc{}, done: make(chan struct{})}
	}
	r.generation++
	return nil
}
func (r *Registry) List(p Principal) []Descriptor {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Descriptor{}
	for _, e := range r.entries {
		if !e.disabled && p.Allows(e.descriptor.Permission) {
			out = append(out, e.descriptor)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (r *Registry) Generation() uint64 { r.mu.Lock(); defer r.mu.Unlock(); return r.generation }
func (r *Registry) Invoke(ctx context.Context, p Principal, id string, input json.RawMessage) (result any, err error) {
	r.mu.Lock()
	e := r.entries[id]
	if e == nil || e.disabled {
		r.mu.Unlock()
		return nil, ErrUnavailable
	}
	d := e.descriptor
	r.mu.Unlock()
	audit := Audit{ID: id, Owner: d.Owner, Subject: p.Subject, Effect: d.Effect, Started: time.Now().UTC()}
	defer func() {
		audit.DurationMS = time.Since(audit.Started).Milliseconds()
		audit.Outcome = "ok"
		if err != nil {
			audit.Outcome = "failed"
		}
		if r.audit != nil {
			err = errors.Join(err, r.audit(audit))
		}
	}()
	if !p.Allows(d.Permission) {
		return nil, ErrForbidden
	}
	if err = d.Input.Validate(input); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(d.TimeoutMS)*time.Millisecond)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.entries[id] != e || e.disabled {
		r.mu.Unlock()
		return nil, ErrUnavailable
	}
	r.serial++
	serial := r.serial
	e.calls[serial] = cancel
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(e.calls, serial)
		if e.disabled && len(e.calls) == 0 {
			select {
			case <-e.done:
			default:
				close(e.done)
			}
		}
		r.mu.Unlock()
	}()
	result, err = e.handler(ctx, p, input)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if err = d.Output.Validate(raw); err != nil {
		return nil, fmt.Errorf("invalid action result: %w", err)
	}
	return result, nil
}

// RemoveOwner 先隐藏入口并取消，再等待处理器释放资源；超时保持停用且允许重试清理。
func (r *Registry) RemoveOwner(ctx context.Context, owner string) error {
	r.mu.Lock()
	var removed []*entry
	for _, e := range r.entries {
		if e.descriptor.Owner == owner {
			e.disabled = true
			for _, cancel := range e.calls {
				cancel()
			}
			if len(e.calls) == 0 {
				select {
				case <-e.done:
				default:
					close(e.done)
				}
			}
			removed = append(removed, e)
		}
	}
	r.generation++
	r.mu.Unlock()
	for _, e := range removed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.done:
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range removed {
		if r.entries[e.descriptor.ID] == e {
			delete(r.entries, e.descriptor.ID)
		}
	}
	return nil
}
