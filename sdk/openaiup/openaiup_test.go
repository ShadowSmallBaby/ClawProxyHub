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

func lastFinish(t *testing.T, events []*pb.StreamEvent) *pb.MessageFinish {
	t.Helper()
	var fin *pb.MessageFinish
	n := 0
	for _, ev := range events {
		if e, ok := ev.Event.(*pb.StreamEvent_MessageFinish); ok {
			fin, n = e.MessageFinish, n+1
		}
	}
	if n != 1 {
		t.Fatalf("want exactly 1 finish, got %d", n)
	}
	return fin
}

// TestParserCachedUsage OpenAI 语义 prompt_tokens 含缓存读写 → 信封拆成非缓存输入 + 缓存读 + 缓存写。
func TestParserCachedUsage(t *testing.T) {
	fin := lastFinish(t, collect([]string{
		`data: {"choices":[{"delta":{"content":"x"},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":700,"cache_write_tokens":100}}}`,
		`data: [DONE]`,
	}))
	u := fin.Usage
	if u.InputTokens != 200 || u.CachedTokens != 700 || u.CacheCreationTokens != 100 || u.OutputTokens != 20 {
		t.Errorf("usage wrong: %+v", u)
	}
}

// TestParserDeepSeekCacheHit DeepSeek 顶层 prompt_cache_hit_tokens 也算缓存读。
func TestParserDeepSeekCacheHit(t *testing.T) {
	fin := lastFinish(t, collect([]string{
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":50,"completion_tokens":5,"prompt_cache_hit_tokens":30,"prompt_cache_miss_tokens":20}}`,
	}))
	if fin.Usage.InputTokens != 20 || fin.Usage.CachedTokens != 30 {
		t.Errorf("usage wrong: %+v", fin.Usage)
	}
}

// TestParserCumulativeUsage 每块都带累计 usage 的上游：finish_reason 之前只合并不收尾，
// 最终 usage 取最后非零值，finish_reason 不丢。
func TestParserCumulativeUsage(t *testing.T) {
	events := collect([]string{
		`data: {"choices":[{"delta":{"content":"a"}}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`,
		`data: {"choices":[{"delta":{"content":"b"}}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`,
	})
	fin := lastFinish(t, events)
	if fin.FinishReason != "tool_calls" || fin.Usage.OutputTokens != 3 || fin.Usage.InputTokens != 10 {
		t.Errorf("finish wrong: %+v", fin)
	}
	// finish 必须是最后一个事件（不能在第一块 usage 时提前收尾）
	if _, ok := events[len(events)-1].Event.(*pb.StreamEvent_MessageFinish); !ok {
		t.Errorf("finish not last: %+v", events)
	}
}

// TestChatBodyParts 含图片时 content 变 parts 数组（data URL 重组）；推理块不回传。
func TestChatBodyParts(t *testing.T) {
	body := ChatBody(&pb.ChatRequest{
		Model: "m",
		Messages: []*pb.EnvelopeMessage{
			{Role: "user", Text: "看", Parts: []*pb.ContentPart{
				{Type: "text", Text: "看"},
				{Type: "image", MediaType: "image/png", Data: "AAAA"},
			}},
			{Role: "assistant", Text: "答", Parts: []*pb.ContentPart{
				{Type: "thinking", Text: "secret"}, {Type: "text", Text: "答"},
			}},
		},
	})
	b, _ := json.Marshal(body)
	s := string(b)
	if !strings.Contains(s, `{"image_url":{"url":"data:image/png;base64,AAAA"},"type":"image_url"}`) {
		t.Errorf("image part missing:\n%s", s)
	}
	if !strings.Contains(s, `"content":"答"`) || strings.Contains(s, "secret") {
		t.Errorf("assistant should be plain text without reasoning:\n%s", s)
	}
}

// TestParserReasoning reasoning_content（DeepSeek）与 reasoning（OpenRouter）都进 ReasoningDelta。
func TestParserReasoning(t *testing.T) {
	events := collect([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"a"}}]}`,
		`data: {"choices":[{"delta":{"reasoning":"b"}}]}`,
		`data: {"choices":[{"delta":{"content":"c"},"finish_reason":"stop"}]}`,
	})
	var reasoning string
	for _, ev := range events {
		if e, ok := ev.Event.(*pb.StreamEvent_ReasoningDelta); ok {
			reasoning += e.ReasoningDelta.Text
		}
	}
	if reasoning != "ab" {
		t.Errorf("reasoning wrong: %q", reasoning)
	}
}

// TestChatBodyToolResultImage tool 消息图片挪到连续 tool 组之后的 user 消息，tool 本身只留文本。
func TestChatBodyToolResultImage(t *testing.T) {
	body := ChatBody(&pb.ChatRequest{Model: "m", Messages: []*pb.EnvelopeMessage{
		{Role: "assistant", ToolCalls: []*pb.ToolCall{{Id: "c1", Name: "f", Arguments: "{}"}, {Id: "c2", Name: "g", Arguments: "{}"}}},
		{Role: "tool", ToolCallId: "c1", Text: "shot", Parts: []*pb.ContentPart{
			{Type: "text", Text: "shot"}, {Type: "image", MediaType: "image/png", Data: "IMG"},
		}},
		{Role: "tool", ToolCallId: "c2", Text: "ok"},
		{Role: "user", Text: "next"},
	}})
	msgs := body["messages"].([]map[string]interface{})
	roles := ""
	for _, m := range msgs {
		roles += m["role"].(string) + ","
	}
	if roles != "assistant,tool,tool,user,user," {
		t.Fatalf("order wrong: %s", roles)
	}
	if msgs[1]["content"] != "shot" {
		t.Errorf("tool content should be text only: %v", msgs[1]["content"])
	}
	b, _ := json.Marshal(msgs[3])
	if !strings.Contains(string(b), `"url":"data:image/png;base64,IMG"`) {
		t.Errorf("deferred image message wrong: %s", b)
	}
}

// TestChatBodyParity tool_choice required / temperature=0 / 专有参数透传 / thinking→effort / 推理块不回传。
func TestChatBodyParity(t *testing.T) {
	body := ChatBody(&pb.ChatRequest{
		Model: "m", Messages: []*pb.EnvelopeMessage{{Role: "user", Text: "q"}},
		Tools: []*pb.ToolDefinition{{Name: "f"}}, ToolChoice: &pb.ToolChoice{Type: "tool"},
		Extra: map[string]string{
			"temperature": "0", "seed": "42", "parallel_tool_calls": "false",
			"response_format": `{"type":"json_object"}`, "user": "u",
			"thinking": `{"type":"enabled","budget_tokens":4000}`,
		},
	})
	b, _ := json.Marshal(body)
	s := string(b)
	for _, want := range []string{
		`"tool_choice":"required"`, `"temperature":0`, `"seed":42`, `"parallel_tool_calls":false`,
		`"response_format":{"type":"json_object"}`, `"user":"u"`, `"reasoning_effort":"medium"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("body missing %q\n%s", want, s)
		}
	}
}

// TestParserReasoningTokens completion_tokens_details.reasoning_tokens → Usage.ReasoningTokens。
func TestParserReasoningTokens(t *testing.T) {
	fin := lastFinish(t, collect([]string{
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":50,"completion_tokens_details":{"reasoning_tokens":30}}}`,
	}))
	if fin.Usage.ReasoningTokens != 30 || fin.Usage.OutputTokens != 50 {
		t.Errorf("usage wrong: %+v", fin.Usage)
	}
}
