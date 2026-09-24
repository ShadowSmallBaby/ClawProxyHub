package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	lua "github.com/yuin/gopher-lua"
	"google.golang.org/grpc"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func autoclawDir() string {
	return filepath.Join("..", "..", "plugins", "plugins-lua", "autoclaw")
}

func newAutoclaw() *luahost {
	return &luahost{dir: autoclawDir(), pool: newVMPool(autoclawDir(), nil)}
}

// fakeChatServer 捕获 Chat 推送的事件（只实现 Send/Context，其余由内嵌 ServerStream 占位）。
type fakeChatServer struct {
	grpc.ServerStream
	events []*pb.StreamEvent
}

func (f *fakeChatServer) Send(ev *pb.StreamEvent) error { f.events = append(f.events, ev); return nil }
func (f *fakeChatServer) Context() context.Context      { return context.Background() }

// TestLoadAutoclaw 验证沙箱 VM 能加载 autoclaw main.lua，且约定函数齐全。
func TestLoadAutoclaw(t *testing.T) {
	vm, err := newVM(autoclawDir(), nil)
	if err != nil {
		t.Fatalf("newVM: %v", err)
	}
	defer vm.L.Close()
	for _, fn := range []string{"handshake", "chat", "models", "login", "refresh", "profile"} {
		if vm.fn(fn) == lua.LNil {
			t.Errorf("missing convention function: %s", fn)
		}
	}
}

// TestHandshakeV2 验证 autoclaw.handshake 接受 protocol_version=2 并回带 capabilities 的 manifest。
func TestHandshakeV2(t *testing.T) {
	h := newAutoclaw()
	resp, err := h.Handshake(context.Background(), &pb.HandshakeRequest{ProtocolVersion: 2})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("handshake error for v2: %s", resp.Error.Message)
	}
	if resp.Manifest == nil || resp.Manifest.Name != "autoclaw" || len(resp.Manifest.Capabilities) == 0 {
		t.Errorf("unexpected manifest: %+v", resp.Manifest)
	}
}

// TestListModelsE2E 端到端过 luahost.ListModels：autoclaw 返回内置模型目录（无需网络）。
func TestListModelsE2E(t *testing.T) {
	h := newAutoclaw()
	ml, err := h.ListModels(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if ml.Error != nil {
		t.Fatalf("models error: %s", ml.Error.Message)
	}
	if len(ml.Models) == 0 {
		t.Error("expected non-empty model list")
	}
}

// TestChatBadCredE2E 端到端过 luahost.Chat：无凭据时 autoclaw 应经 stream.failed 回 task_failed（无需网络）。
func TestChatBadCredE2E(t *testing.T) {
	h := newAutoclaw()
	fs := &fakeChatServer{}
	err := h.Chat(&pb.ChatRequest{
		Model:    "claude-opus",
		Messages: []*pb.EnvelopeMessage{{Role: "user", Text: "hi"}},
	}, fs)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(fs.events) == 0 {
		t.Fatal("no stream events captured")
	}
	last := fs.events[len(fs.events)-1]
	if last.GetTaskFailed() == nil {
		t.Errorf("expected task_failed for missing credential, got %T", last.Event)
	}
}

// TestSafeRequire 验证 require("lib.util") 能加载 lib/ 子脚本并取到其返回值（支持 main.lua 分层）。
func TestSafeRequire(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "main.lua"),
		[]byte(`local u = require("lib.util"); local M = {}; function M.answer() return u.n end; return M`), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "lib"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "lib", "util.lua"), []byte(`return { n = 42 }`), 0o644)
	vm, err := newVM(dir, nil)
	if err != nil {
		t.Fatalf("newVM: %v", err)
	}
	defer vm.L.Close()
	if err := vm.L.CallByParam(lua.P{Fn: vm.fn("answer"), NRet: 1, Protect: true}); err != nil {
		t.Fatalf("call answer(): %v", err)
	}
	if got := int(lua.LVAsNumber(vm.L.Get(-1))); got != 42 {
		t.Errorf("require(lib.util).n = %d, want 42", got)
	}
}

// TestRequireRejectsEscape 验证非 lib.* 的 require 被拒（防加载 os/io 或路径逃逸）。
func TestRequireRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "main.lua"),
		[]byte(`local ok = pcall(require, "os"); local ok2 = pcall(require, "../secret"); local M = {}; M.blocked = (not ok) and (not ok2); return M`), 0o644)
	vm, err := newVM(dir, nil)
	if err != nil {
		t.Fatalf("newVM: %v", err)
	}
	defer vm.L.Close()
	if vm.L.GetField(vm.mod, "blocked") != lua.LTrue {
		t.Error(`require("os") / 路径逃逸应被拒`)
	}
}
