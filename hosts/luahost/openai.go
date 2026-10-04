// openai.go — Lua 与 Go 插件共用信封适配，避免字段映射分叉。
package main

import (
	"encoding/json"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/openaiup"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	lua "github.com/yuin/gopher-lua"
	"google.golang.org/protobuf/encoding/protojson"
)

func newOpenAIModule(L *lua.LState) *lua.LTable {
	return tableOf(L, map[string]lua.LGFunction{"chat_body": cphOpenAIChatBody})
}
func cphOpenAIChatBody(L *lua.LState) int {
	raw := luaToGo(L.CheckTable(1)).(map[string]interface{})
	delete(raw, "credential")
	if messages, ok := raw["messages"].(map[string]interface{}); ok && len(messages) == 0 {
		raw["messages"] = []interface{}{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		L.RaiseError("chat_body: %v", err)
	}
	req := &pb.ChatRequest{}
	if err := protojson.Unmarshal(data, req); err != nil {
		L.RaiseError("chat_body: %v", err)
	}
	data, err = json.Marshal(openaiup.ChatBody(req))
	if err != nil {
		L.RaiseError("chat_body: %v", err)
	}
	var body interface{}
	if err = json.Unmarshal(data, &body); err != nil {
		L.RaiseError("chat_body: %v", err)
	}
	L.Push(goToLua(L, body))
	return 1
}
