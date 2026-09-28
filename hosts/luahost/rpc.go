// rpc.go — ClawPlugin 契约分派：Handshake/Chat/ListModels/Login/Refresh/GetProfile → main.lua 约定函数。
// 约定函数命名与 Go 插件同构：handshake/chat/models/login/refresh/profile。
package main

import (
	"context"
	"fmt"
	"os"

	lua "github.com/yuin/gopher-lua"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// call1 取一个 VM 调约定函数（NRet=1），返回 (结果表, 是否存在该函数, 错误)。
// 顶层 recover 兜底：任何 Go 侧 panic（含 VM 构建）转成错误而非崩溃 host 进程；panic 后 VM 状态可能损坏，不回收。
func (h *luahost) call1(name string, build func(L *lua.LState) []lua.LValue) (result *lua.LTable, present bool, err error) {
	if h.pool == nil {
		return nil, false, nil
	}
	var vm *luaVM
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[luahost] panic in %s(): %v\n", name, r)
			result, present, err = nil, true, fmt.Errorf("panic in %s(): %v", name, r)
			if vm != nil {
				vm.L.Close()
				vm = nil
			}
		}
		if vm != nil {
			h.pool.put(vm)
		}
	}()
	v, gerr := h.pool.get()
	if gerr != nil {
		return nil, false, gerr
	}
	vm = v
	f := vm.fn(name)
	if f == lua.LNil {
		return nil, false, nil
	}
	L := vm.L
	if e := L.CallByParam(lua.P{Fn: f, NRet: 1, Protect: true}, build(L)...); e != nil {
		return nil, true, e
	}
	ret := L.Get(-1)
	L.Pop(1)
	t, _ := ret.(*lua.LTable)
	return t, true, nil
}

// Handshake 优先调 plugin.handshake(req)（与 Go 插件在运行时声明能力同构）；缺失回退 manifest.json。
func (h *luahost) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	t, present, err := h.call1("handshake", func(L *lua.LState) []lua.LValue {
		r := L.NewTable()
		r.RawSetString("protocol_version", lua.LNumber(req.ProtocolVersion))
		r.RawSetString("core_version", lua.LString(req.CoreVersion))
		return []lua.LValue{r}
	})
	if err != nil {
		return &pb.HandshakeResponse{Error: &pb.Error{Code: 1, Message: err.Error()}}, nil
	}
	if present && t != nil {
		if e := errorFromField(t); e != nil {
			return &pb.HandshakeResponse{Error: e}, nil
		}
		if mt := tblField(t, "manifest"); mt != nil {
			return &pb.HandshakeResponse{Manifest: manifestFromTable(mt)}, nil
		}
	}
	mf, ferr := loadManifest(h.dir)
	if ferr != nil {
		return &pb.HandshakeResponse{Error: &pb.Error{Code: 1, Message: fmt.Sprintf("no handshake() and read manifest failed: %v", ferr)}}, nil
	}
	pbm := mf.toPB()
	pbm.ProtocolVersion = sdk.ProtocolVersion
	return &pb.HandshakeResponse{Manifest: pbm}, nil
}

// Chat 传入 stream 对象（typed 方法 → srv.Send），调 plugin.chat(req, stream)。
// 顶层 recover 兜底：chat 内任何 Go 侧 panic 转成 task_failed，不崩溃 host 进程。
func (h *luahost) Chat(req *pb.ChatRequest, srv pb.ClawPlugin_ChatServer) (err error) {
	var vm *luaVM
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[luahost] panic in chat(): %v\n", r)
			if vm != nil {
				vm.L.Close()
				vm = nil
			}
			err = srv.Send(failed(500, fmt.Sprintf("panic in chat(): %v", r)))
		}
		if vm != nil {
			h.pool.put(vm)
		}
	}()
	v, gerr := h.pool.get()
	if gerr != nil {
		return srv.Send(failed(500, gerr.Error()))
	}
	vm = v
	f := vm.fn("chat")
	if f == lua.LNil {
		return srv.Send(failed(501, "script has no chat()"))
	}
	L := vm.L
	stream := newStreamTable(L, srv)
	if e := L.CallByParam(lua.P{Fn: f, NRet: 0, Protect: true}, chatReqToTable(L, req), stream); e != nil {
		return srv.Send(failed(502, e.Error()))
	}
	return nil
}

// ListModels 调 plugin.models(cred) → {models={...}}。
func (h *luahost) ListModels(ctx context.Context, cred *pb.CredentialBlob) (*pb.ModelList, error) {
	t, present, err := h.call1("models", func(L *lua.LState) []lua.LValue { return []lua.LValue{credArg(L, cred)} })
	if err != nil {
		return &pb.ModelList{Error: &pb.Error{Code: 502, Message: err.Error()}}, nil
	}
	if !present || t == nil {
		return &pb.ModelList{}, nil
	}
	return modelListFromTable(t), nil
}

// Login 调 plugin.login(req) → {blob, profile, next, error}。
func (h *luahost) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResult, error) {
	t, present, err := h.call1("login", func(L *lua.LState) []lua.LValue { return []lua.LValue{loginReqToTable(L, req)} })
	if err != nil {
		return &pb.LoginResult{Error: &pb.Error{Code: 502, Message: err.Error()}}, nil
	}
	if !present || t == nil {
		return &pb.LoginResult{Error: &pb.Error{Code: 501, Message: "script has no login()"}}, nil
	}
	return loginResultFromTable(t), nil
}

// Refresh 调 plugin.refresh(cred) → {blob, profile, error}。
func (h *luahost) Refresh(ctx context.Context, cred *pb.CredentialBlob) (*pb.RefreshResult, error) {
	t, present, err := h.call1("refresh", func(L *lua.LState) []lua.LValue { return []lua.LValue{credArg(L, cred)} })
	if err != nil {
		return &pb.RefreshResult{Error: &pb.Error{Code: 502, Message: err.Error()}}, nil
	}
	if !present || t == nil {
		return &pb.RefreshResult{}, nil
	}
	return refreshResultFromTable(t), nil
}

// GetProfile 调 plugin.profile(cred) → AccountProfile。
func (h *luahost) GetProfile(ctx context.Context, cred *pb.CredentialBlob) (*pb.AccountProfile, error) {
	t, present, err := h.call1("profile", func(L *lua.LState) []lua.LValue { return []lua.LValue{credArg(L, cred)} })
	if err != nil || !present {
		return &pb.AccountProfile{}, nil
	}
	return profileFromTable(t), nil
}

// ListTaskCapabilities 调 plugin.tasks() → {capabilities={{id,label,kind,per_account,default_schedule},...}}；
// 脚本未声明 tasks() 回空（未声明任务能力，与 Go 插件 Unimplemented 兜底同构）。
func (h *luahost) ListTaskCapabilities(ctx context.Context, req *pb.TaskCapabilitiesRequest) (*pb.TaskCapabilities, error) {
	t, present, err := h.call1("tasks", func(L *lua.LState) []lua.LValue { return nil })
	if err != nil {
		return nil, err
	}
	if !present || t == nil {
		return &pb.TaskCapabilities{}, nil
	}
	out := &pb.TaskCapabilities{}
	if caps, ok := t.RawGetString("capabilities").(*lua.LTable); ok {
		caps.ForEach(func(_, v lua.LValue) {
			c, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			out.Capabilities = append(out.Capabilities, &pb.TaskCapability{
				Id:              strField(c, "id"),
				Label:           strMapField(c, "label"),
				Kind:            strField(c, "kind"),
				PerAccount:      boolField(c, "per_account"),
				DefaultSchedule: strField(c, "default_schedule"),
			})
		})
	}
	return out, nil
}

// RunTask 调 plugin.task(req) → {summary, changed, blob, detail_json, notification, error}。
// credential_id 从凭据信封带入 req.context（脚本侧一般只读 blob）。脚本未实现回 501。
func (h *luahost) RunTask(ctx context.Context, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	t, present, err := h.call1("task", func(L *lua.LState) []lua.LValue {
		r := L.NewTable()
		r.RawSetString("capability_id", lua.LString(req.CapabilityId))
		if req.Credential != nil {
			r.RawSetString("credential", credToTable(L, req.Credential))
		}
		if len(req.Context) > 0 {
			c := L.NewTable()
			for k, v := range req.Context {
				c.RawSetString(k, lua.LString(v))
			}
			r.RawSetString("context", c)
		}
		return []lua.LValue{r}
	})
	if err != nil {
		return &pb.RunTaskResponse{Error: &pb.Error{Code: 502, Message: err.Error()}}, nil
	}
	if !present || t == nil {
		return &pb.RunTaskResponse{Error: &pb.Error{Code: 501, Message: "script has no task()"}}, nil
	}
	out := &pb.RunTaskResponse{
		Summary:    strField(t, "summary"),
		Changed:    boolField(t, "changed"),
		DetailJson: strField(t, "detail_json"),
	}
	if b := strField(t, "blob"); b != "" {
		out.Blob = []byte(b)
	}
	if n := tblField(t, "notification"); n != nil {
		out.Notification = &pb.TaskNotification{
			Title: strField(n, "title"), Content: strField(n, "content"), Level: strField(n, "level"),
		}
	}
	if e := errorFromField(t); e != nil {
		out.Error = e
	}
	return out, nil
}

// credArg 凭据可空：nil 传空 table，脚本自行判空。
func credArg(L *lua.LState, c *pb.CredentialBlob) *lua.LTable {
	if c == nil {
		return L.NewTable()
	}
	return credToTable(L, c)
}

// failed 构造 task_failed 事件（Chat 出错回吐）。
func failed(code int32, msg string) *pb.StreamEvent {
	return &pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{TaskFailed: &pb.TaskFailed{
		Error: &pb.Error{Code: code, Message: msg},
	}}}
}
