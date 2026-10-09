// Package module 管理内置模块的依赖、能力与生命周期，不暴露核心业务对象给扩展。
package module

import (
	"context"
	"net/http"
)

type Descriptor struct {
	ID           string   `json:"id"`
	Requires     []string `json:"requires,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// Host 只接收模块贡献的路由，具体依赖由宿主构造模块时注入。
type Host interface {
	Handle(pattern string, handler http.Handler) error
}

// Module 的 Stop 必须能清理 Register/Start 部分完成的状态，并遵守传入的停止期限。
type Module interface {
	Descriptor() Descriptor
	Register(Host) error
	Start(context.Context) error
	Stop(context.Context) error
}

type State struct {
	Descriptor
	Installed       bool `json:"installed"`
	Enabled         bool `json:"enabled"`
	Available       bool `json:"available"`
	RestartRequired bool `json:"restart_required"`
}

// Func 将现有服务接入装配器，避免先搬迁业务实现。
type Func struct {
	Spec         Descriptor
	RegisterFunc func(Host) error
	StartFunc    func(context.Context) error
	StopFunc     func(context.Context) error
}

func (f *Func) Descriptor() Descriptor { return f.Spec }
func (f *Func) Register(h Host) error {
	if f.RegisterFunc != nil {
		return f.RegisterFunc(h)
	}
	return nil
}
func (f *Func) Start(ctx context.Context) error {
	if f.StartFunc != nil {
		return f.StartFunc(ctx)
	}
	return nil
}
func (f *Func) Stop(ctx context.Context) error {
	if f.StopFunc != nil {
		return f.StopFunc(ctx)
	}
	return nil
}
