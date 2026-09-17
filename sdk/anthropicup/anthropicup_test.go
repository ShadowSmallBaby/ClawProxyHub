package anthropicup

import (
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func TestChatBody(t *testing.T) {
	req := &pb.ChatRequest{
		Model: "kimi-k3",
		Messages: []*pb.EnvelopeMessage{
			{Role: "system", Text: "sys"},
			{Role: "user", Text: "hi"},
			{Role: "assistant", Text: "calling", ToolCalls: []*pb.ToolCall{
				{Id: "tu_1", Name: "f", Arguments: `{"x":1}`},
			}},
			{Role: "tool", Text: "result", ToolCallId: "tu_1"},
		},
		Tools: []*pb.ToolDefinition{{Name: "f", ParametersSchema: `{"type":"object"}`}},
	}
	body := ChatBody(req)
	b := mustJSON(body)
	for _, want := range []string{
		`"system":"sys"`,
		`"text":"hi"`,
		`"type":"tool_use"`,
		`"type":"tool_result"`,
		`"input_schema"`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("body missing %q\n%s", want, b)
		}
	}
}

func TestParser(t *testing.T) {
	var out []*pb.StreamEvent
	p := NewParser(func(ev *pb.StreamEvent) { out = append(out, ev) })
	lines := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"model":"kimi-k3","usage":{"input_tokens":10}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好"}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_1","name":"f","input":{}}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
	}
	for _, l := range lines {
		p.Feed(l)
	}
	p.Finish()

	var hasStart, hasText, hasTool, hasFinish bool
	for _, ev := range out {
		switch e := ev.Event.(type) {
		case *pb.StreamEvent_MessageStart:
			hasStart = e.MessageStart.Model == "kimi-k3"
		case *pb.StreamEvent_ContentDelta:
			hasText = e.ContentDelta.Text == "你好"
		case *pb.StreamEvent_ToolCallDelta:
			hasTool = e.ToolCallDelta.Id == "tu_1" && e.ToolCallDelta.Name == "f"
		case *pb.StreamEvent_MessageFinish:
			hasFinish = e.MessageFinish.FinishReason == "tool_calls" &&
				e.MessageFinish.Usage.OutputTokens == 5
		}
	}
	if !hasStart || !hasText || !hasTool || !hasFinish {
		t.Errorf("events incomplete: start=%v text=%v tool=%v finish=%v", hasStart, hasText, hasTool, hasFinish)
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
