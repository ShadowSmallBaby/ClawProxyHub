package openaiup

import (
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func collect(lines []string) []*pb.StreamEvent {
	var out []*pb.StreamEvent
	p := NewParser(func(ev *pb.StreamEvent) { out = append(out, ev) })
	for _, l := range lines {
		p.Feed(l)
	}
	p.Finish()
	return out
}

func TestChatBody(t *testing.T) {
	req := &pb.ChatRequest{
		Model: "kimi-k3", Stream: true,
		Messages: []*pb.EnvelopeMessage{
			{Role: "system", Text: "sys"},
			{Role: "assistant", Text: "", ToolCalls: []*pb.ToolCall{
				{Id: "call_1", Name: "get_weather", Arguments: `{"city":"北京"}`},
			}},
			{Role: "tool", Text: "晴", ToolCallId: "call_1"},
		},
		Tools: []*pb.ToolDefinition{{
			Name: "get_weather", Description: "查天气",
			ParametersSchema: `{"type":"object"}`,
		}},
		ToolChoice: &pb.ToolChoice{Type: "auto"},
		MaxTokens:  100,
		Extra:      map[string]string{"top_p": "0.9", "stop": `["end"]`},
	}
	body := ChatBody(req)
	b, _ := json.Marshal(body)
	s := string(b)

	for _, want := range []string{
		`"stream":true`, `"include_usage":true`,
		`"role":"system"`, `"role":"assistant"`, `"tool_call_id":"call_1"`,
		`"get_weather"`, `"tool_choice":"auto"`, `"max_tokens":100`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("body missing %q\n%s", want, s)
		}
	}
}

func TestParserTextAndUsage(t *testing.T) {
	events := collect([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"你"}}]}`,
		`data: {"choices":[{"delta":{"content":"好"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
		`data: [DONE]`,
	})
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d: %+v", len(events), events)
	}
	d1, ok := events[0].Event.(*pb.StreamEvent_ContentDelta)
	if !ok || d1.ContentDelta.Text != "你" {
		t.Errorf("first delta wrong: %+v", events[0])
	}
	fin, ok := events[2].Event.(*pb.StreamEvent_MessageFinish)
	if !ok || fin.MessageFinish.FinishReason != "stop" ||
		fin.MessageFinish.Usage.InputTokens != 5 || fin.MessageFinish.Usage.OutputTokens != 2 {
		t.Errorf("finish wrong: %+v", events[2])
	}
}

func TestParserToolCalls(t *testing.T) {
	events := collect([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"f","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"more\""}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`,
	})
	var toolEvents []*pb.ToolCallDelta
	var finish *pb.MessageFinish
	for _, ev := range events {
		switch e := ev.Event.(type) {
		case *pb.StreamEvent_ToolCallDelta:
			toolEvents = append(toolEvents, e.ToolCallDelta)
		case *pb.StreamEvent_MessageFinish:
			finish = e.MessageFinish
		}
	}
	if len(toolEvents) != 2 {
		t.Fatalf("want 2 tool events, got %d", len(toolEvents))
	}
	if toolEvents[0].Name != "f" || toolEvents[0].Id != "call_1" {
		t.Errorf("first tool event wrong: %+v", toolEvents[0])
	}
	if toolEvents[1].ArgumentsDelta != `"more"` {
		t.Errorf("second tool delta wrong: %+v", toolEvents[1])
	}
	if finish == nil || finish.FinishReason != "tool_calls" {
		t.Errorf("finish wrong: %+v", finish)
	}
}

func TestParserEmptyStreamFallback(t *testing.T) {
	// 上游空流 / 只有 [DONE]：Finish 补一个 stop
	events := collect([]string{`data: [DONE]`})
	if len(events) != 1 {
		t.Fatalf("want 1 fallback event, got %d", len(events))
	}
	fin, ok := events[0].Event.(*pb.StreamEvent_MessageFinish)
	if !ok || fin.MessageFinish.FinishReason != "stop" {
		t.Errorf("fallback finish wrong: %+v", events[0])
	}
}
