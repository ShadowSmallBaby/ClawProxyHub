// proto.go — 入向 proto message → Lua table，与出向 table → ModelList。
package main

import (
	lua "github.com/yuin/gopher-lua"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// chatReqToTable 把 ChatRequest 信封转成脚本入参 table（字段名与 cph.proto snake_case 对齐）。
func chatReqToTable(L *lua.LState, req *pb.ChatRequest) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("model", lua.LString(req.Model))
	t.RawSetString("stream", lua.LBool(req.Stream))
	t.RawSetString("temperature", lua.LNumber(req.Temperature))
	t.RawSetString("max_tokens", lua.LNumber(req.MaxTokens))
	t.RawSetString("source", lua.LString(req.Source))
	if len(req.Extra) > 0 {
		e := L.NewTable()
		for k, v := range req.Extra {
			e.RawSetString(k, lua.LString(v))
		}
		t.RawSetString("extra", e)
	}
	msgs := L.NewTable()
	for _, m := range req.Messages {
		msgs.Append(messageToTable(L, m))
	}
	t.RawSetString("messages", msgs)
	if len(req.Tools) > 0 {
		tools := L.NewTable()
		for _, td := range req.Tools {
			tt := L.NewTable()
			tt.RawSetString("name", lua.LString(td.Name))
			tt.RawSetString("description", lua.LString(td.Description))
			tt.RawSetString("parameters_schema", lua.LString(td.ParametersSchema))
			tools.Append(tt)
		}
		t.RawSetString("tools", tools)
	}
	if tc := req.ToolChoice; tc != nil {
		ct := L.NewTable()
		ct.RawSetString("type", lua.LString(tc.Type))
		ct.RawSetString("tool_name", lua.LString(tc.ToolName))
		t.RawSetString("tool_choice", ct)
	}
	if req.Credential != nil {
		t.RawSetString("credential", credToTable(L, req.Credential))
	}
	return t
}

// messageToTable 转一条 EnvelopeMessage：text + 可选 tool_calls / parts。
func messageToTable(L *lua.LState, m *pb.EnvelopeMessage) *lua.LTable {
	mt := L.NewTable()
	mt.RawSetString("role", lua.LString(m.Role))
	mt.RawSetString("text", lua.LString(m.Text))
	if m.ToolCallId != "" {
		mt.RawSetString("tool_call_id", lua.LString(m.ToolCallId))
	}
	if m.ToolError {
		mt.RawSetString("tool_error", lua.LBool(true))
	}
	if len(m.ToolCalls) > 0 {
		tcs := L.NewTable()
		for _, tc := range m.ToolCalls {
			e := L.NewTable()
			e.RawSetString("id", lua.LString(tc.Id))
			e.RawSetString("name", lua.LString(tc.Name))
			e.RawSetString("arguments", lua.LString(tc.Arguments))
			tcs.Append(e)
		}
		mt.RawSetString("tool_calls", tcs)
	}
	if len(m.Parts) > 0 {
		parts := L.NewTable()
		for _, p := range m.Parts {
			pt := L.NewTable()
			pt.RawSetString("type", lua.LString(p.Type))
			pt.RawSetString("text", lua.LString(p.Text))
			pt.RawSetString("media_type", lua.LString(p.MediaType))
			pt.RawSetString("data", lua.LString(p.Data))
			pt.RawSetString("url", lua.LString(p.Url))
			pt.RawSetString("signature", lua.LString(p.Signature))
			parts.Append(pt)
		}
		mt.RawSetString("parts", parts)
	}
	return mt
}

// loginReqToTable 把 LoginRequest 转脚本入参 table。
func loginReqToTable(L *lua.LState, req *pb.LoginRequest) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("method_id", lua.LString(req.MethodId))
	if len(req.Form) > 0 {
		f := L.NewTable()
		for k, v := range req.Form {
			f.RawSetString(k, lua.LString(v))
		}
		t.RawSetString("form", f)
	}
	if len(req.State) > 0 {
		t.RawSetString("state", lua.LString(string(req.State)))
	}
	t.RawSetString("instance_id", lua.LNumber(req.InstanceId))
	return t
}

// credToTable 把凭据信封转成脚本可读 table（blob 作为普通 string）。
func credToTable(L *lua.LState, c *pb.CredentialBlob) *lua.LTable {
	ct := L.NewTable()
	ct.RawSetString("account_id", lua.LString(c.AccountId))
	ct.RawSetString("blob", lua.LString(string(c.Blob)))
	ct.RawSetString("instance_id", lua.LNumber(c.InstanceId))
	ct.RawSetString("updated_at", lua.LNumber(c.UpdatedAt))
	return ct
}

// modelListFromTable 把 list_models 返回的 {models={...}} 转成 ModelList。
func modelListFromTable(ret *lua.LTable) *pb.ModelList {
	out := &pb.ModelList{}
	models, ok := ret.RawGetString("models").(*lua.LTable)
	if !ok {
		return out
	}
	models.ForEach(func(_, v lua.LValue) {
		m, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		mi := &pb.ModelInfo{
			Id:             strField(m, "id"),
			ContextWindow:  int32(numField(m, "context_window")),
			SupportsTools:  boolField(m, "supports_tools"),
			SupportsStream: boolField(m, "supports_stream"),
		}
		if lbl, ok := m.RawGetString("label").(*lua.LTable); ok {
			mi.Label = map[string]string{}
			lbl.ForEach(func(k, val lua.LValue) { mi.Label[k.String()] = val.String() })
		}
		out.Models = append(out.Models, mi)
	})
	return out
}
