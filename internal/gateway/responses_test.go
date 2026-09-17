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
		"event: response.output_item.done",
		"event: response.completed",
		`"input_tokens":3`,
		`"output_tokens":4`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("responses sse missing %q\n%s", want, out)
		}
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
