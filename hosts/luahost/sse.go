// sse.go — OpenAI 兼容 SSE 解析：逐 data: 帧解出增量，驱动 stream 对象的 typed 方法。
package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// openAIChunk OpenAI chat.completions 流式分片的关注字段。
type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Id       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// streamOpenAISSE 读 SSE 并驱动 stream 方法；结束时补一次 message_finish。
func streamOpenAISSE(L *lua.LState, streamTbl *lua.LTable, body io.Reader) {
	call := func(method string, arg *lua.LTable) {
		fn := L.GetField(streamTbl, method)
		if fn == lua.LNil {
			return
		}
		L.Push(fn)
		L.Push(arg)
		_ = L.PCall(1, 0, nil)
	}
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	finish := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk openAIChunk
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		if ch.Delta.Content != "" {
			t := L.NewTable()
			t.RawSetString("text", lua.LString(ch.Delta.Content))
			call("content_delta", t)
		}
		if ch.Delta.ReasoningContent != "" {
			t := L.NewTable()
			t.RawSetString("text", lua.LString(ch.Delta.ReasoningContent))
			call("reasoning_delta", t)
		}
		for _, tc := range ch.Delta.ToolCalls {
			t := L.NewTable()
			t.RawSetString("id", lua.LString(tc.Id))
			t.RawSetString("name", lua.LString(tc.Function.Name))
			t.RawSetString("arguments_delta", lua.LString(tc.Function.Arguments))
			call("tool_call_delta", t)
		}
		if ch.FinishReason != "" {
			finish = ch.FinishReason
		}
	}
	if finish == "" {
		finish = "stop"
	}
	ft := L.NewTable()
	ft.RawSetString("finish_reason", lua.LString(finish))
	call("message_finish", ft)
}
