package luahost

import (
	lua "github.com/yuin/gopher-lua"
	"strings"
	"testing"
)

func TestMultilineSSEBridge(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	stream := L.NewTable()
	text, finished, failed := "", 0, 0
	L.SetField(stream, "content_delta", L.NewFunction(func(L *lua.LState) int { text += L.GetField(L.CheckTable(1), "text").String(); return 0 }))
	L.SetField(stream, "message_finish", L.NewFunction(func(L *lua.LState) int { finished++; return 0 }))
	L.SetField(stream, "failed", L.NewFunction(func(L *lua.LState) int { failed++; return 0 }))
	streamOpenAISSE(L, stream, strings.NewReader("data: {\ndata: \"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	if text != "hello" || finished != 1 || failed != 0 {
		t.Fatalf("text=%q finished=%d failed=%d", text, finished, failed)
	}
}
