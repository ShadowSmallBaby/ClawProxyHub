// openai.go — cph.openai：信封→OpenAI chat body，以及 OpenAI 兼容 SSE 解析驱动 stream 对象。
// 多数上游是 OpenAI 兼容协议，脚本无需自己拆 data: 帧、拼 tool_call 分片。
package main

import (
	"encoding/json"

	lua "github.com/yuin/gopher-lua"
)

func newOpenAIModule(L *lua.LState) *lua.LTable {
	return tableOf(L, map[string]lua.LGFunction{"chat_body": cphOpenAIChatBody})
}

// cphOpenAIChatBody(req) → OpenAI chat completions body（table，脚本可再改 model 等）。
func cphOpenAIChatBody(L *lua.LState) int {
	req := L.CheckTable(1)
	body := L.NewTable()
	body.RawSetString("model", lua.LString(strField(req, "model")))
	body.RawSetString("stream", lua.LBool(true))
	if v := numField(req, "temperature"); v != 0 {
		body.RawSetString("temperature", lua.LNumber(v))
	}
	if v := numField(req, "max_tokens"); v != 0 {
		body.RawSetString("max_tokens", lua.LNumber(v))
	}
	body.RawSetString("messages", openAIMessages(L, tblField(req, "messages")))
	if tools := tblField(req, "tools"); tools != nil && tools.Len() > 0 {
		body.RawSetString("tools", openAITools(L, tools))
	}
	L.Push(body)
	return 1
}

func openAIMessages(L *lua.LState, msgs *lua.LTable) *lua.LTable {
	out := L.NewTable()
	if msgs == nil {
		return out
	}
	msgs.ForEach(func(_, v lua.LValue) {
		m, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		om := L.NewTable()
		role := strField(m, "role")
		om.RawSetString("role", lua.LString(role))
		om.RawSetString("content", lua.LString(strField(m, "text")))
		if role == "tool" {
			om.RawSetString("tool_call_id", lua.LString(strField(m, "tool_call_id")))
		}
		if tcs := tblField(m, "tool_calls"); tcs != nil && tcs.Len() > 0 {
			arr := L.NewTable()
			tcs.ForEach(func(_, tv lua.LValue) {
				tc, ok := tv.(*lua.LTable)
				if !ok {
					return
				}
				fn := L.NewTable()
				fn.RawSetString("name", lua.LString(strField(tc, "name")))
				fn.RawSetString("arguments", lua.LString(strField(tc, "arguments")))
				e := L.NewTable()
				e.RawSetString("id", lua.LString(strField(tc, "id")))
				e.RawSetString("type", lua.LString("function"))
				e.RawSetString("function", fn)
				arr.Append(e)
			})
			om.RawSetString("tool_calls", arr)
		}
		out.Append(om)
	})
	return out
}

func openAITools(L *lua.LState, tools *lua.LTable) *lua.LTable {
	out := L.NewTable()
	tools.ForEach(func(_, v lua.LValue) {
		td, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		fn := L.NewTable()
		fn.RawSetString("name", lua.LString(strField(td, "name")))
		fn.RawSetString("description", lua.LString(strField(td, "description")))
		if sch := strField(td, "parameters_schema"); sch != "" {
			var parsed interface{}
			if json.Unmarshal([]byte(sch), &parsed) == nil {
				fn.RawSetString("parameters", goToLua(L, parsed))
			}
		}
		e := L.NewTable()
		e.RawSetString("type", lua.LString("function"))
		e.RawSetString("function", fn)
		out.Append(e)
	})
	return out
}
