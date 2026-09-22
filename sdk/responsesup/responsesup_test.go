package responsesup

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
		Model: "gpt-5", Stream: true,
		Messages: []*pb.EnvelopeMessage{
			{Role: "system", Text: "sys"},
			{Role: "user", Text: "hi"},
			{Role: "assistant", Text: "", ToolCalls: []*pb.ToolCall{
				{Id: "call_1", Name: "get_weather", Arguments: `{"city":"北京"}`},
			}},
			{Role: "tool", Text: "晴", ToolCallId: "call_1"},
		},
		Tools: []*pb.ToolDefinition{{
			Name: "get_weather", Description: "查天气",
			ParametersSchema: `{"type":"object"}`,
		}},
		ToolChoice: &pb.ToolChoice{Type: "tool", ToolName: "get_weather"},
		MaxTokens:  100,
		Extra:      map[string]string{"top_p": "0.9", "reasoning_effort": "high"},
	}
	body := ChatBody(req)
	b, _ := json.Marshal(body)
	s := string(b)

	for _, want := range []string{
		`"stream":true`, `"instructions":"sys"`,
		`"type":"input_text"`, `"text":"hi"`,
		`"type":"function_call"`, `"call_id":"call_1"`, `"name":"get_weather"`,
		`"type":"function_call_output"`, `"output":"晴"`,
		`"strict":false`, `"tool_choice":{"name":"get_weather","type":"function"}`,
		`"max_output_tokens":100`, `"top_p":0.9`, `"reasoning":{"effort":"high"}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("body missing %q\n%s", want, s)
		}
	}
	// system 不进 input；空文本 assistant 不产生 message 项
	if strings.Contains(s, `"role":"system"`) || strings.Contains(s, `"role":"assistant"`) {
		t.Errorf("unexpected role item in input\n%s", s)
	}
}

func TestChatBodyImage(t *testing.T) {
	req := &pb.ChatRequest{
		Model: "gpt-5",
		Messages: []*pb.EnvelopeMessage{{Role: "user", Parts: []*pb.ContentPart{
			{Type: "text", Text: "看图"},
			{Type: "image", MediaType: "image/png", Data: "AAAA"},
		}}},
	}
	b, _ := json.Marshal(ChatBody(req))
	s := string(b)
	for _, want := range []string{`"type":"input_image"`, `"image_url":"data:image/png;base64,AAAA"`, `"text":"看图"`} {
		if !strings.Contains(s, want) {
			t.Errorf("body missing %q\n%s", want, s)
		}
	}
}

func TestParserTextAndUsage(t *testing.T) {
	events := collect([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"你"}`,
		`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"好"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":1}}}}`,
	})
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d: %+v", len(events), events)
	}
	d1, ok := events[0].Event.(*pb.StreamEvent_ContentDelta)
	if !ok || d1.ContentDelta.Text != "你" {
		t.Errorf("first delta wrong: %+v", events[0])
	}
	fin, ok := events[2].Event.(*pb.StreamEvent_MessageFinish)
	if !ok || fin.MessageFinish.FinishReason != "stop" {
		t.Fatalf("finish wrong: %+v", events[2])
	}
	// 信封语义：input 剥离缓存读
	u := fin.MessageFinish.Usage
	if u.InputTokens != 6 || u.CachedTokens != 4 || u.OutputTokens != 2 || u.ReasoningTokens != 1 {
		t.Errorf("usage wrong: %+v", u)
	}
}

func TestParserToolCalls(t *testing.T) {
	events := collect([]string{
		`data: {"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"a\":"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"1}"}`,
		`data: {"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"a\":1}"}`,
		`data: {"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f","arguments":"{\"a\":1}"}}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
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
	// 首块 id+name，两段增量；done 不重复补发
	if len(toolEvents) != 3 {
		t.Fatalf("want 3 tool events, got %d: %+v", len(toolEvents), toolEvents)
	}
	if toolEvents[0].Id != "call_1" || toolEvents[0].Name != "f" || toolEvents[0].ArgumentsDelta != "" {
		t.Errorf("first tool event wrong: %+v", toolEvents[0])
	}
	if toolEvents[1].Id != "" || toolEvents[1].ArgumentsDelta+toolEvents[2].ArgumentsDelta != `{"a":1}` {
		t.Errorf("arguments deltas wrong: %+v %+v", toolEvents[1], toolEvents[2])
	}
	if finish == nil || finish.FinishReason != "tool_calls" {
		t.Errorf("finish wrong: %+v", finish)
	}
}

func TestParserToolArgsOnlyInDone(t *testing.T) {
	// 上游不发增量、只在 done 给全量：补发一次
	events := collect([]string{
		`data: {"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`,
		`data: {"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f","arguments":"{}"}}`,
		`data: {"type":"response.completed","response":{}}`,
	})
	var args []string
	for _, ev := range events {
		if e, ok := ev.Event.(*pb.StreamEvent_ToolCallDelta); ok && e.ToolCallDelta.ArgumentsDelta != "" {
			args = append(args, e.ToolCallDelta.ArgumentsDelta)
		}
	}
	if len(args) != 1 || args[0] != "{}" {
		t.Errorf("want single full arguments, got %v", args)
	}
}

func TestParserIncompleteAndErrors(t *testing.T) {
	events := collect([]string{
		`data: {"type":"response.output_text.delta","delta":"x"}`,
		`data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":1,"output_tokens":1}}}`,
	})
	fin, ok := events[len(events)-1].Event.(*pb.StreamEvent_MessageFinish)
	if !ok || fin.MessageFinish.FinishReason != "length" {
		t.Errorf("incomplete should finish with length: %+v", events[len(events)-1])
	}

	events = collect([]string{`data: {"type":"response.failed","response":{"error":{"message":"boom"}}}`})
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	failed, ok := events[0].Event.(*pb.StreamEvent_TaskFailed)
	if !ok || failed.TaskFailed.Error.Code != 502 || failed.TaskFailed.Error.Message != "boom" {
		t.Errorf("failed event wrong: %+v", events[0])
	}
}

func TestParserEmptyStreamFallback(t *testing.T) {
	// 上游空流：Finish 补一个 stop
	events := collect(nil)
	if len(events) != 1 {
		t.Fatalf("want 1 fallback event, got %d", len(events))
	}
	if fin, ok := events[0].Event.(*pb.StreamEvent_MessageFinish); !ok || fin.MessageFinish.FinishReason != "stop" {
		t.Errorf("fallback wrong: %+v", events[0])
	}
}
