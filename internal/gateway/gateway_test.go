package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func TestParseAnthropicRequest(t *testing.T) {
	body := `{
		"model": "kimi-k3",
		"system": "你是个助手",
		"max_tokens": 1024,
		"temperature": 0.7,
		"stream": true,
		"messages": [
			{"role": "user", "content": "你好"},
			{"role": "assistant", "content": [
				{"type": "text", "text": "我来调用工具"},
				{"type": "tool_use", "id": "tu_1", "name": "get_weather", "input": {"city": "北京"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "tu_1", "content": "晴"}
			]}
		],
		"tools": [{"name": "get_weather", "description": "查天气", "input_schema": {"type": "object"}}],
		"tool_choice": {"type": "auto"}
	}`
	req, err := parseAnthropicRequest([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "kimi-k3" || !req.Stream || req.MaxTokens != 1024 || req.Temperature != 0.7 {
		t.Fatalf("basic fields wrong: %+v", req)
	}
	// system + user + assistant + tool_result拆出的tool = 4条
	if len(req.Messages) != 4 {
		t.Fatalf("want 4 messages, got %d: %+v", len(req.Messages), req.Messages)
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Text != "你是个助手" {
		t.Errorf("system message wrong: %+v", req.Messages[0])
	}
	asst := req.Messages[2]
	if asst.Role != "assistant" || len(asst.ToolCalls) != 1 || asst.ToolCalls[0].Id != "tu_1" {
		t.Errorf("assistant tool_calls wrong: %+v", asst)
	}
	if asst.ToolCalls[0].Arguments != `{"city":"北京"}` {
		t.Errorf("tool arguments wrong: %s", asst.ToolCalls[0].Arguments)
	}
	toolMsg := req.Messages[3]
	if toolMsg.Role != "tool" || toolMsg.ToolCallId != "tu_1" || toolMsg.Text != "晴" {
		t.Errorf("tool_result message wrong: %+v", toolMsg)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "get_weather" {
		t.Errorf("tools wrong: %+v", req.Tools)
	}
	if req.ToolChoice == nil || req.ToolChoice.Type != "auto" {
		t.Errorf("tool_choice wrong: %+v", req.ToolChoice)
	}
}

func TestAnthropicSSE(t *testing.T) {
	st := newAnthSSEState("kimi-k3")
	var sb strings.Builder

	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{
		MessageStart: &pb.MessageStart{Model: "kimi-k3"},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
		ContentDelta: &pb.ContentDelta{Text: "你好"},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
		ContentDelta: &pb.ContentDelta{Text: "，世界"},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
		ToolCallDelta: &pb.ToolCallDelta{Id: "tu_1", Name: "get_weather", ArgumentsDelta: `{"city":`},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
		ToolCallDelta: &pb.ToolCallDelta{Id: "tu_1", ArgumentsDelta: `"北京"}`},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{
			FinishReason: "tool_calls",
			Usage:        &pb.Usage{InputTokens: 10, OutputTokens: 20},
		},
	}}))

	out := sb.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"\"text_delta\"",
		"\"tool_use\"",
		"\"input_json_delta\"",
		"\"partial_json\":\"{\\\"city\\\":\"",
		"event: content_block_stop",
		"event: message_delta",
		"\"stop_reason\":\"tool_use\"",
		"\"input_tokens\":10",
		"\"output_tokens\":20",
		"event: message_stop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("SSE missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestParseChatCompletions(t *testing.T) {
	body := `{
		"model": "kimi-k3",
		"messages": [
			{"role": "system", "content": "sys"},
			{"role": "user", "content": [{"type": "text", "text": "hi"}]},
			{"role": "assistant", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "f", "arguments": "{}"}}
			], "content": null},
			{"role": "tool", "tool_call_id": "call_1", "content": "result"}
		],
		"tools": [{"type": "function", "function": {"name": "f", "parameters": {"type": "object"}}}],
		"tool_choice": "auto",
		"stream": false
	}`
	req, err := parseChatCompletions([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 4 {
		t.Fatalf("want 4 messages, got %d", len(req.Messages))
	}
	if req.Messages[1].Text != "hi" {
		t.Errorf("user text wrong: %q", req.Messages[1].Text)
	}
	if len(req.Messages[2].ToolCalls) != 1 || req.Messages[2].ToolCalls[0].Id != "call_1" {
		t.Errorf("assistant tool_calls wrong: %+v", req.Messages[2])
	}
	if req.Messages[3].Role != "tool" || req.Messages[3].ToolCallId != "call_1" {
		t.Errorf("tool message wrong: %+v", req.Messages[3])
	}
	if req.Tools[0].ParametersSchema == "" {
		t.Errorf("parameters schema empty")
	}
	if req.ToolChoice.Type != "auto" {
		t.Errorf("tool_choice wrong: %+v", req.ToolChoice)
	}
}

// TestToolIDFillerAllEncoders 出口层 id 补齐器：工具调用续块 id 置空只带 arguments
// （openaiup 契约）。补齐后 openai/anthropic 编码器都应只产 1 个工具块、参数完整，
// 不因空 id 开孤儿块。覆盖协议间转换链路（各上游方言 → 客户端协议）。
func TestToolIDFillerAllEncoders(t *testing.T) {
	// 每次构造全新事件：filler.fill 会原地改 id，不能共享
	freshSeq := func() []*pb.StreamEvent {
		return []*pb.StreamEvent{
			{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{Id: "call_9", Name: "exec_command"}}},
			{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{ArgumentsDelta: `{"cmd":"ls`}}},
			{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{ArgumentsDelta: ` -la"}`}}},
			{Event: &pb.StreamEvent_MessageFinish{MessageFinish: &pb.MessageFinish{FinishReason: "tool_calls", Usage: &pb.Usage{}}}},
		}
	}
	run := func(enc streamEncoder) string {
		f := &toolIDFiller{}
		var sb strings.Builder
		for _, ev := range freshSeq() {
			f.fill(ev) // 出口层补齐（模拟 streamOut/nonStreamOut）
			sb.WriteString(enc.convertEvent(ev))
		}
		return sb.String()
	}

	// OpenAI：tool_calls 按 index，只应有一个 index 0，参数完整拼接
	oai := run(newOpenAISSEState())
	if strings.Count(oai, `"index":1`) != 0 {
		t.Errorf("openai 出现第二个工具块(孤儿):\n%s", oai)
	}
	if !strings.Contains(oai, `"name":"exec_command"`) {
		t.Errorf("openai 工具 name 丢失:\n%s", oai)
	}

	// Anthropic：tool_use content_block，只应有一个 content_block_start(tool_use)
	anth := run(newAnthSSEState("m"))
	if n := strings.Count(anth, `"type":"tool_use"`); n != 1 {
		t.Errorf("anthropic want 1 tool_use block, got %d:\n%s", n, anth)
	}
	if !strings.Contains(anth, `"id":"call_9"`) || !strings.Contains(anth, `"name":"exec_command"`) {
		t.Errorf("anthropic tool_use id/name 丢失:\n%s", anth)
	}

	// 续块空 id 未被补齐时的反证：不经 filler，openai 会开出 index 1 孤儿
	var bad strings.Builder
	st := newOpenAISSEState()
	for _, ev := range freshSeq() {
		bad.WriteString(st.convertEvent(ev))
	}
	if !strings.Contains(bad.String(), `"index":1`) {
		t.Errorf("反证失败：未补齐时本应出现孤儿 index 1，说明测试序列无效")
	}
}

func TestOpenAISSEAndAggregate(t *testing.T) {
	st := newOpenAISSEState()
	var sb strings.Builder
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
		ContentDelta: &pb.ContentDelta{Text: "hello"},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{FinishReason: "stop", Usage: &pb.Usage{InputTokens: 5, OutputTokens: 1}},
	}}))
	out := sb.String()
	if !strings.Contains(out, `"content":"hello"`) {
		t.Errorf("missing content delta:\n%s", out)
	}
	if !strings.Contains(out, `"finish_reason":"stop"`) {
		t.Errorf("missing finish_reason:\n%s", out)
	}
	if !strings.Contains(out, `"total_tokens":6`) || !strings.Contains(out, "data: [DONE]") {
		// usage 块在 finish 里，[DONE] 由 server 层补，这里只验证 usage
		if !strings.Contains(out, `"total_tokens":6`) {
			t.Errorf("missing usage chunk:\n%s", out)
		}
	}

	// 非流式聚合
	agg := &openaiAggregate{}
	agg.feed(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{MessageStart: &pb.MessageStart{Model: "m"}}})
	agg.feed(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Text: "a"}}})
	agg.feed(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Text: "b"}}})
	agg.feed(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{FinishReason: "stop", Usage: &pb.Usage{InputTokens: 1, OutputTokens: 2}},
	}})
	res := agg.result()
	if res["model"] != "m" {
		t.Errorf("model wrong: %v", res["model"])
	}
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"content":"ab"`) {
		t.Errorf("aggregated content wrong: %s", b)
	}
}
