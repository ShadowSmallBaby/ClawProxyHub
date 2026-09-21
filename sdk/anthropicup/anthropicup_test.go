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
		`"system":[{"text":"sys","type":"text"}]`,
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

// TestChatBodyExtras top_p / stop / thinking 从信封 Extra 透传到 Anthropic 请求体。
func TestChatBodyExtras(t *testing.T) {
	body := ChatBody(&pb.ChatRequest{
		Model:    "claude",
		Messages: []*pb.EnvelopeMessage{{Role: "user", Text: "hi"}},
		Extra: map[string]string{
			"top_p": "0.9", "stop": `["END"]`,
			"thinking": `{"type":"enabled","budget_tokens":1024}`,
		},
	})
	b := mustJSON(body)
	for _, want := range []string{
		`"top_p":0.9`, `"stop_sequences":["END"]`, `"thinking":{"budget_tokens":1024,"type":"enabled"}`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("body missing %q\n%s", want, b)
		}
	}
}

// TestParserUsageMerge message_start 的 input/cache 打底，message_delta 补 output 并可重报全量。
func TestParserUsageMerge(t *testing.T) {
	var out []*pb.StreamEvent
	p := NewParser(func(ev *pb.StreamEvent) { out = append(out, ev) })
	for _, l := range []string{
		`data: {"type":"message_start","message":{"model":"claude","usage":{"input_tokens":12,"cache_read_input_tokens":300,"cache_creation_input_tokens":40,"output_tokens":1}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":25}}`,
		`data: {"type":"message_stop"}`,
	} {
		p.Feed(l)
	}
	p.Finish()
	var fin *pb.MessageFinish
	n := 0
	for _, ev := range out {
		if e, ok := ev.Event.(*pb.StreamEvent_MessageFinish); ok {
			fin, n = e.MessageFinish, n+1
		}
	}
	if n != 1 || fin.FinishReason != "stop" {
		t.Fatalf("want 1 stop finish, got n=%d fin=%+v", n, fin)
	}
	u := fin.Usage
	if u.InputTokens != 12 || u.CachedTokens != 300 || u.CacheCreationTokens != 40 || u.OutputTokens != 25 {
		t.Errorf("usage wrong: %+v", u)
	}
}

func TestMapStop(t *testing.T) {
	for in, want := range map[string]string{
		"end_turn": "stop", "stop_sequence": "stop_sequence", "tool_use": "tool_calls",
		"max_tokens": "length", "pause_turn": "length", "refusal": "content_filter",
	} {
		if got := mapStop(in); got != want {
			t.Errorf("mapStop(%s)=%s want %s", in, got, want)
		}
	}
}

// TestChatBodyParts 图片 → source 块；带签名 thinking 回放、无签名丢弃；redacted_thinking 原样。
func TestChatBodyParts(t *testing.T) {
	body := ChatBody(&pb.ChatRequest{
		Model: "claude",
		Messages: []*pb.EnvelopeMessage{
			{Role: "user", Text: "看", Parts: []*pb.ContentPart{
				{Type: "text", Text: "看"},
				{Type: "image", MediaType: "image/png", Data: "AAAA"},
				{Type: "image", Url: "https://x/y.png"},
			}},
			{Role: "assistant", Text: "答", Parts: []*pb.ContentPart{
				{Type: "thinking", Text: "signed", Signature: "sig"},
				{Type: "thinking", Text: "unsigned"},
				{Type: "redacted_thinking", Data: "blob"},
				{Type: "text", Text: "答"},
			}, ToolCalls: []*pb.ToolCall{{Id: "tu", Name: "f", Arguments: "{}"}}},
		},
	})
	b := mustJSON(body)
	for _, want := range []string{
		`"source":{"data":"AAAA","media_type":"image/png","type":"base64"}`,
		`"source":{"type":"url","url":"https://x/y.png"}`,
		`{"signature":"sig","thinking":"signed","type":"thinking"}`,
		`{"data":"blob","type":"redacted_thinking"}`,
		`"type":"tool_use"`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("body missing %q\n%s", want, b)
		}
	}
	if strings.Contains(b, "unsigned") {
		t.Errorf("unsigned thinking must be dropped:\n%s", b)
	}
	// 顺序：thinking 在 text 前、text 在 tool_use 前
	if strings.Index(b, `"type":"thinking"`) > strings.Index(b, `"text":"答"`) || strings.Index(b, `"text":"答"`) > strings.Index(b, `"type":"tool_use"`) {
		t.Errorf("block order wrong:\n%s", b)
	}
}

// TestParserThinking thinking_delta / signature_delta → ReasoningDelta。
func TestParserThinking(t *testing.T) {
	var out []*pb.StreamEvent
	p := NewParser(func(ev *pb.StreamEvent) { out = append(out, ev) })
	for _, l := range []string{
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"ok"}}`,
	} {
		p.Feed(l)
	}
	var texts, sigs []string
	for _, ev := range out {
		if e, ok := ev.Event.(*pb.StreamEvent_ReasoningDelta); ok {
			if e.ReasoningDelta.Text != "" {
				texts = append(texts, e.ReasoningDelta.Text)
			}
			if e.ReasoningDelta.Signature != "" {
				sigs = append(sigs, e.ReasoningDelta.Signature)
			}
		}
	}
	if len(texts) != 1 || texts[0] != "hmm" || len(sigs) != 1 || sigs[0] != "sig" {
		t.Errorf("reasoning events wrong: texts=%v sigs=%v", texts, sigs)
	}
}

// TestChatBodyToolResultImage tool 消息带图片 → tool_result.content 为 text/image 块数组。
func TestChatBodyToolResultImage(t *testing.T) {
	body := ChatBody(&pb.ChatRequest{Model: "c", Messages: []*pb.EnvelopeMessage{
		{Role: "tool", ToolCallId: "tu", Text: "shot", Parts: []*pb.ContentPart{
			{Type: "text", Text: "shot"}, {Type: "image", MediaType: "image/png", Data: "IMG"},
		}},
	}})
	b := mustJSON(body)
	if !strings.Contains(b, `"tool_use_id":"tu"`) || !strings.Contains(b, `"content":[{"text":"shot","type":"text"},{"source":{"data":"IMG","media_type":"image/png","type":"base64"},"type":"image"}]`) {
		t.Errorf("tool_result blocks wrong:\n%s", b)
	}
}

// TestChatBodyParity 同角色合并 / tool_choice any / temperature=0 / top_k / is_error /
// cache_control（system、块、tool_result、tools）/ metadata.user_id / effort→thinking。
func TestChatBodyParity(t *testing.T) {
	body := ChatBody(&pb.ChatRequest{
		Model: "claude", MaxTokens: 10000,
		Messages: []*pb.EnvelopeMessage{
			{Role: "system", Text: "s1\ns2", Parts: []*pb.ContentPart{
				{Type: "text", Text: "s1"}, {Type: "text", Text: "s2", CacheControl: `{"type":"ephemeral"}`},
			}},
			{Role: "user", Text: "q1"},
			{Role: "assistant", Text: "a", ToolCalls: []*pb.ToolCall{{Id: "tu", Name: "f", Arguments: "{}"}}},
			{Role: "tool", ToolCallId: "tu", Text: "boom", ToolError: true, Parts: []*pb.ContentPart{
				{Type: "text", Text: "boom", CacheControl: `{"type":"ephemeral"}`},
			}},
			{Role: "user", Text: "q2"}, // 紧跟 tool_result 的 user：应合并进同一条 user
		},
		Tools:      []*pb.ToolDefinition{{Name: "f", CacheControl: `{"type":"ephemeral"}`}},
		ToolChoice: &pb.ToolChoice{Type: "tool"},
		Extra: map[string]string{
			"temperature": "0", "top_k": "3", "parallel_tool_calls": "false",
			"user": "u1", "reasoning_effort": "medium",
		},
	})
	b := mustJSON(body)
	for _, want := range []string{
		`"system":[{"text":"s1","type":"text"},{"cache_control":{"type":"ephemeral"},"text":"s2","type":"text"}]`,
		`{"cache_control":{"type":"ephemeral"},"content":[{"text":"boom","type":"text"}],"is_error":true,"tool_use_id":"tu","type":"tool_result"},{"text":"q2","type":"text"}]`,
		`"tools":[{"cache_control":{"type":"ephemeral"},"description":"","input_schema":{"type":"object"},"name":"f"}]`,
		`"tool_choice":{"disable_parallel_tool_use":true,"type":"any"}`,
		`"temperature":0`, `"top_k":3`, `"metadata":{"user_id":"u1"}`,
		`"thinking":{"budget_tokens":5000,"type":"enabled"}`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("body missing %q\n%s", want, b)
		}
	}
	msgs := body["messages"].([]map[string]interface{})
	if len(msgs) != 3 {
		t.Errorf("want 3 messages after merge, got %d", len(msgs))
	}
	// 显式 thinking 优先于 effort 折算；max_tokens 太小则不开
	b2 := mustJSON(ChatBody(&pb.ChatRequest{Model: "c", MaxTokens: 1000, Messages: []*pb.EnvelopeMessage{{Role: "user", Text: "q"}},
		Extra: map[string]string{"reasoning_effort": "high"}}))
	if strings.Contains(b2, "thinking") {
		t.Errorf("thinking must not be enabled when max_tokens too small:\n%s", b2)
	}
}

// TestParserStartUsage message_start 的输入/缓存计数随 MessageStart 事件透出。
func TestParserStartUsage(t *testing.T) {
	var out []*pb.StreamEvent
	p := NewParser(func(ev *pb.StreamEvent) { out = append(out, ev) })
	p.Feed(`data: {"type":"message_start","message":{"model":"c","usage":{"input_tokens":3,"cache_read_input_tokens":400}}}`)
	st, ok := out[0].Event.(*pb.StreamEvent_MessageStart)
	if !ok || st.MessageStart.Usage == nil || st.MessageStart.Usage.InputTokens != 3 || st.MessageStart.Usage.CachedTokens != 400 {
		t.Errorf("start usage wrong: %+v", out[0])
	}
}

// TestParserStopSequence 命中停止序列时原值随 MessageFinish 透出。
func TestParserStopSequence(t *testing.T) {
	var fin *pb.MessageFinish
	p := NewParser(func(ev *pb.StreamEvent) {
		if e, ok := ev.Event.(*pb.StreamEvent_MessageFinish); ok {
			fin = e.MessageFinish
		}
	})
	p.Feed(`data: {"type":"message_delta","delta":{"stop_reason":"stop_sequence","stop_sequence":"</answer>"},"usage":{"output_tokens":3}}`)
	p.Finish()
	if fin == nil || fin.FinishReason != "stop_sequence" || fin.StopSequence != "</answer>" {
		t.Errorf("finish wrong: %+v", fin)
	}
}
