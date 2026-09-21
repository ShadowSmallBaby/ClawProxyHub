package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func TestParseResponsesRequest(t *testing.T) {
	body := `{
		"model": "gpt-5",
		"instructions": "你是助手",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "你好"}]},
			{"type": "function_call", "call_id": "call_1", "name": "f", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "结果"}
		],
		"tools": [{"type": "function", "name": "f", "parameters": {"type": "object"}}],
		"max_output_tokens": 512,
		"stream": true
	}`
	req, err := parseResponsesRequest([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	// instructions(system) + user + function_call + function_call_output = 4
	if len(req.Messages) != 4 {
		t.Fatalf("want 4 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Text != "你是助手" {
		t.Errorf("instructions wrong: %+v", req.Messages[0])
	}
	if req.Messages[2].Role != "assistant" || req.Messages[2].ToolCalls[0].Id != "call_1" {
		t.Errorf("function_call wrong: %+v", req.Messages[2])
	}
	if req.Messages[3].Role != "tool" || req.Messages[3].ToolCallId != "call_1" {
		t.Errorf("function_call_output wrong: %+v", req.Messages[3])
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "f" {
		t.Errorf("tools wrong: %+v", req.Tools)
	}
	if req.MaxTokens != 512 || !req.Stream {
		t.Errorf("basic fields wrong")
	}
}

// TestParseResponsesRequestCodex 复刻 Codex CLI 的实际请求：developer 角色 +
// input_text 内容块 + reasoning.effort，回归三处修复（文本不再被丢空 / 角色归一 / effort 透传）。
func TestParseResponsesRequestCodex(t *testing.T) {
	body := `{
		"model": "deepseek-flash",
		"instructions": "system prompt",
		"input": [
			{"type": "message", "role": "developer", "content": [{"type": "input_text", "text": "dev rule"}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "今天是几号了"}]}
		],
		"reasoning": {"effort": "xhigh"},
		"stream": true
	}`
	req, err := parseResponsesRequest([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	// instructions(system) + developer(→system) + user = 3
	if len(req.Messages) != 3 {
		t.Fatalf("want 3 messages, got %d: %+v", len(req.Messages), req.Messages)
	}
	if req.Messages[1].Role != "system" || req.Messages[1].Text != "dev rule" {
		t.Errorf("developer 未归一或文本丢失: %+v", req.Messages[1])
	}
	if req.Messages[2].Role != "user" || req.Messages[2].Text != "今天是几号了" {
		t.Errorf("input_text 提取失败: %+v", req.Messages[2])
	}
	// reasoning.effort 不得透传上游：Codex 的 "xhigh" 上游不认会 500。
	if _, ok := req.Extra["reasoning_effort"]; ok {
		t.Errorf("reasoning.effort 不应透传上游: %q", req.Extra["reasoning_effort"])
	}
}

func TestParseResponsesRequestStringInput(t *testing.T) {
	req, err := parseResponsesRequest([]byte(`{"model":"m","input":"纯文本输入"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Text != "纯文本输入" {
		t.Errorf("string input wrong: %+v", req.Messages)
	}
}

func TestResponsesSSE(t *testing.T) {
	st := newResponsesSSEState("gpt-5")
	var sb strings.Builder
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{
		MessageStart: &pb.MessageStart{Model: "gpt-5"},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
		ContentDelta: &pb.ContentDelta{Text: "hello"},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
		ToolCallDelta: &pb.ToolCallDelta{Id: "call_1", Name: "f", ArgumentsDelta: `{"x":1}`},
	}}))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{FinishReason: "tool_calls",
			Usage: &pb.Usage{InputTokens: 3, OutputTokens: 4}},
	}}))
	out := sb.String()
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		`"delta":"hello"`,
		`"type":"function_call"`,
		"event: response.function_call_arguments.delta",
		// 工具调用必须 finalize，否则 Codex 收不到完整调用不执行
		"event: response.function_call_arguments.done",
		`"arguments":"{\"x\":1}"`,
		"event: response.output_item.done",
		"event: response.completed",
		// output_text.done 必须带完整文本，而非空串
		`"text":"hello"`,
		// response.completed 必须带 output 数组，Codex 从此读最终输出
		`"output":[`,
		`"input_tokens":3`,
		`"output_tokens":4`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("responses sse missing %q\n%s", want, out)
		}
	}
	// completed 的 output 里应同时含 message 与 function_call 两项
	idx := strings.Index(out, "event: response.completed")
	if idx < 0 || !strings.Contains(out[idx:], `"type":"function_call"`) || !strings.Contains(out[idx:], `"type":"message"`) {
		t.Errorf("response.completed output 缺少 message/function_call:\n%s", out[max(idx, 0):])
	}
}

// TestResponsesSSEToolCallSplit 复刻 Codex 真实流式：工具调用首块带 id+name，
// 续块 id 置空只带 arguments（openaiup 契约）。回归——空 id 必须归入当前调用，
// 不得开新块，否则参数流进无名孤儿 item，Codex 报 "failed to parse arguments: EOF"。
func TestResponsesSSEToolCallSplit(t *testing.T) {
	st := newResponsesSSEState("deepseek-flash")
	feed := func(id, name, argsDelta string) string {
		return st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
			ToolCallDelta: &pb.ToolCallDelta{Id: id, Name: name, ArgumentsDelta: argsDelta},
		}})
	}
	var sb strings.Builder
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{
		MessageStart: &pb.MessageStart{Model: "deepseek-flash"}}}))
	// 调用1：首块 id+name，续块空 id 分块带参数
	sb.WriteString(feed("call_00", "exec_command", ""))
	sb.WriteString(feed("", "", `{"cmd":"ls`))
	sb.WriteString(feed("", "", ` -la"}`))
	// 调用2：新 id 开新块，续块空 id 带参数
	sb.WriteString(feed("call_01", "exec_command", ""))
	sb.WriteString(feed("", "", `{"cmd":"pwd"}`))
	sb.WriteString(st.convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{FinishReason: "tool_calls",
			Usage: &pb.Usage{InputTokens: 1, OutputTokens: 2}}}}))
	out := sb.String()

	// completed 的 output 数组是权威终态：只应有 2 个 function_call（不能因空 id 开孤儿块）
	ci := strings.Index(out, "event: response.completed")
	if ci < 0 {
		t.Fatalf("无 response.completed:\n%s", out)
	}
	if n := strings.Count(out[ci:], `"type":"function_call"`); n != 2 {
		t.Fatalf("completed.output want 2 function_call, got %d\n%s", n, out[ci:])
	}
	// added 事件也应恰好 2 次（每个真实调用开一块，无孤儿）
	if n := strings.Count(out, "event: response.output_item.added"); n != 2 {
		t.Fatalf("want 2 output_item.added, got %d\n%s", n, out)
	}
	// 空 id / 空 name 的孤儿块绝不允许出现
	if strings.Contains(out, `"call_id":""`) {
		t.Errorf("出现空 call_id 孤儿块:\n%s", out)
	}
	// 两个调用的完整参数都要在 arguments.done 里完整落地
	for _, want := range []string{
		`"call_id":"call_00"`, `"call_id":"call_01"`,
		`"arguments":"{\"cmd\":\"ls -la\"}"`, `"arguments":"{\"cmd\":\"pwd\"}"`,
		`"name":"exec_command"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
	// name 不得为空（孤儿块的典型症状）
	if strings.Contains(out, `"name":""`) {
		t.Errorf("function_call name 为空:\n%s", out)
	}
}

func TestResponsesAggregate(t *testing.T) {
	a := &responsesAggregate{}
	a.feed(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Text: "a"}}})
	a.feed(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Text: "b"}}})
	a.feed(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{Usage: &pb.Usage{InputTokens: 1, OutputTokens: 2}},
	}})
	res := a.result()
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"text":"ab"`) {
		t.Errorf("aggregate text wrong: %s", b)
	}
	if !strings.Contains(string(b), `"total_tokens":3`) {
		t.Errorf("aggregate usage wrong: %s", b)
	}
}

// TestParseResponsesToolChoice 字符串 required → tool；对象 function → 指定工具。
func TestParseResponsesToolChoice(t *testing.T) {
	req, err := parseResponsesRequest([]byte(`{"model":"m","input":"hi","tool_choice":"required"}`))
	if err != nil || req.ToolChoice == nil || req.ToolChoice.Type != "tool" {
		t.Errorf("required wrong: err=%v tc=%+v", err, req.ToolChoice)
	}
	req, err = parseResponsesRequest([]byte(`{"model":"m","input":"hi","tool_choice":{"type":"function","name":"shell"}}`))
	if err != nil || req.ToolChoice == nil || req.ToolChoice.Type != "tool" || req.ToolChoice.ToolName != "shell" {
		t.Errorf("object wrong: err=%v tc=%+v", err, req.ToolChoice)
	}
}
