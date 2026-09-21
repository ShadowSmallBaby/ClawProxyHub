package gateway

import (
	"encoding/json"
	"errors"
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

// TestUsageAcrossProtocols 信封 usage（Anthropic 语义）→ 三协议出口：
// Anthropic 四字段直出；OpenAI 系 prompt/input 合回缓存读写并给出明细。
func TestUsageAcrossProtocols(t *testing.T) {
	u := &pb.Usage{InputTokens: 200, OutputTokens: 20, CachedTokens: 700, CacheCreationTokens: 100}
	fin := func() *pb.StreamEvent {
		return &pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
			MessageFinish: &pb.MessageFinish{FinishReason: "stop", Usage: u},
		}}
	}
	check := func(name, out string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(out, w) {
				t.Errorf("%s missing %q:\n%s", name, w, out)
			}
		}
	}
	anthWants := []string{`"input_tokens":200`, `"output_tokens":20`, `"cache_read_input_tokens":700`, `"cache_creation_input_tokens":100`}
	oaiWants := []string{`"prompt_tokens":1000`, `"completion_tokens":20`, `"total_tokens":1020`, `"cached_tokens":700`, `"cache_write_tokens":100`}
	respWants := []string{`"input_tokens":1000`, `"output_tokens":20`, `"total_tokens":1020`, `"input_tokens_details":{"cached_tokens":700}`}

	check("anth sse", newAnthSSEState("m").convertEvent(fin()), anthWants...)
	check("openai sse", newOpenAISSEState().convertEvent(fin()), oaiWants...)
	check("responses sse", newResponsesSSEState("m").convertEvent(fin()), respWants...)

	marshal := func(a aggregate) string {
		a.feed(fin())
		b, _ := json.Marshal(a.result())
		return string(b)
	}
	check("anth aggr", marshal(&anthAggregate{}), anthWants...)
	check("openai aggr", marshal(&openaiAggregate{}), oaiWants...)
	check("responses aggr", marshal(&responsesAggregate{}), respWants...)

	// 落库：四个计数各归各列
	log := &requestLogCtx{}
	collectUsage(log, fin())
	if log.input != 200 || log.output != 20 || log.cached != 700 || log.cacheCreation != 100 {
		t.Errorf("log usage wrong: %+v", log)
	}
}

// TestParseChatCompletionsNewFields max_completion_tokens 回退 + reasoning_effort 透传。
func TestParseChatCompletionsNewFields(t *testing.T) {
	req, err := parseChatCompletions([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":321,"reasoning_effort":"high"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 321 || req.Extra["reasoning_effort"] != "high" {
		t.Errorf("fields wrong: max=%d extra=%v", req.MaxTokens, req.Extra)
	}
}

// TestPartsParsing 三协议入口的图片 / 推理块 → 信封 parts（纯文本不产生 parts）。
func TestPartsParsing(t *testing.T) {
	// Anthropic：user 图片 + assistant 带签名 thinking
	req, err := parseAnthropicRequest([]byte(`{"model":"m","max_tokens":1,"messages":[
		{"role":"user","content":[{"type":"text","text":"看图"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]},
		{"role":"assistant","content":[{"type":"thinking","thinking":"想想","signature":"sig1"},{"type":"text","text":"好"}]},
		{"role":"user","content":"纯文本"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	u := req.Messages[0]
	if u.Text != "看图" || len(u.Parts) != 2 || u.Parts[1].Type != "image" || u.Parts[1].MediaType != "image/png" || u.Parts[1].Data != "AAAA" {
		t.Errorf("anthropic image parts wrong: %+v", u)
	}
	a := req.Messages[1]
	if a.Text != "好" || len(a.Parts) != 2 || a.Parts[0].Type != "thinking" || a.Parts[0].Signature != "sig1" {
		t.Errorf("anthropic thinking parts wrong: %+v", a)
	}
	if len(req.Messages[2].Parts) != 0 {
		t.Errorf("plain text should not carry parts: %+v", req.Messages[2])
	}

	// OpenAI：image_url data URL 拆解 + reasoning_content 进 thinking（无签名）
	req, err = parseChatCompletions([]byte(`{"model":"m","messages":[
		{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,BBBB"}}]},
		{"role":"assistant","content":"ok","reasoning_content":"thought"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if p := req.Messages[0].Parts; len(p) != 2 || p[1].MediaType != "image/jpeg" || p[1].Data != "BBBB" {
		t.Errorf("openai image parts wrong: %+v", p)
	}
	if p := req.Messages[1].Parts; len(p) != 2 || p[0].Type != "thinking" || p[0].Text != "thought" || p[1].Text != "ok" {
		t.Errorf("openai reasoning parts wrong: %+v", p)
	}

	// Responses：input_image + reasoning item 与后续 message/function_call 合并进同一 assistant
	req, err = parseResponsesRequest([]byte(`{"model":"m","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"q"},{"type":"input_image","image_url":"https://x/y.png"}]},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"plan"}],"encrypted_content":"enc"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"do"}]},
		{"type":"function_call","call_id":"c1","name":"f","arguments":"{}"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d: %+v", len(req.Messages), req.Messages)
	}
	if p := req.Messages[0].Parts; len(p) != 2 || p[1].Url != "https://x/y.png" {
		t.Errorf("responses image parts wrong: %+v", p)
	}
	a = req.Messages[1]
	if a.Text != "do" || len(a.ToolCalls) != 1 || len(a.Parts) != 2 || a.Parts[0].Type != "thinking" || a.Parts[0].Signature != "enc" {
		t.Errorf("responses assistant merge wrong: %+v", a)
	}
}

// TestReasoningAcrossProtocols ReasoningDelta → 三协议出口：Anthropic thinking 块先于 text 关闭；
// OpenAI reasoning_content；Responses reasoning item + summary_text。
func TestReasoningAcrossProtocols(t *testing.T) {
	seq := func() []*pb.StreamEvent {
		return []*pb.StreamEvent{
			{Event: &pb.StreamEvent_ReasoningDelta{ReasoningDelta: &pb.ReasoningDelta{Text: "思"}}},
			{Event: &pb.StreamEvent_ReasoningDelta{ReasoningDelta: &pb.ReasoningDelta{Text: "考"}}},
			{Event: &pb.StreamEvent_ReasoningDelta{ReasoningDelta: &pb.ReasoningDelta{Signature: "sig"}}},
			{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Text: "答"}}},
			{Event: &pb.StreamEvent_MessageFinish{MessageFinish: &pb.MessageFinish{FinishReason: "stop", Usage: &pb.Usage{}}}},
		}
	}
	run := func(enc streamEncoder) string {
		var sb strings.Builder
		for _, ev := range seq() {
			sb.WriteString(enc.convertEvent(ev))
		}
		return sb.String()
	}
	anth := run(newAnthSSEState("m"))
	for _, w := range []string{`{"thinking":"","type":"thinking"}`, `{"thinking":"思","type":"thinking_delta"}`, `{"signature":"sig","type":"signature_delta"}`} {
		if !strings.Contains(anth, w) {
			t.Errorf("anthropic missing %q:\n%s", w, anth)
		}
	}
	// thinking(0) 的 stop 必须在 text(1) 的 start 之前
	stop0 := strings.Index(anth, `{"index":0,"type":"content_block_stop"}`)
	start1 := strings.Index(anth, `"type":"text"},"index":1,"type":"content_block_start"`)
	if stop0 < 0 || start1 < 0 || stop0 > start1 {
		t.Errorf("thinking block not closed before text: stop0=%d start1=%d\n%s", stop0, start1, anth)
	}
	if n := strings.Count(anth, `{"index":0,"type":"content_block_stop"}`); n != 1 {
		t.Errorf("thinking block closed %d times", n)
	}

	oai := run(newOpenAISSEState())
	if !strings.Contains(oai, `"reasoning_content":"思"`) || strings.Contains(oai, "sig") {
		t.Errorf("openai reasoning wrong:\n%s", oai)
	}

	resp := run(newResponsesSSEState("m"))
	for _, w := range []string{`"type":"reasoning"`, `response.reasoning_summary_text.delta`, `"summary_index":0`,
		`response.reasoning_summary_text.done`, `"summary":[{"text":"思考","type":"summary_text"}]`} {
		if !strings.Contains(resp, w) {
			t.Errorf("responses missing %q:\n%s", w, resp)
		}
	}

	// 聚合
	for name, a := range map[string]aggregate{"anth": &anthAggregate{}, "oai": &openaiAggregate{}, "resp": &responsesAggregate{}} {
		for _, ev := range seq() {
			a.feed(ev)
		}
		b, _ := json.Marshal(a.result())
		if !strings.Contains(string(b), "思考") {
			t.Errorf("%s aggregate lost reasoning: %s", name, b)
		}
	}
}

// TestToolResultImages tool_result / function_call_output 内嵌图片 → tool 消息的 parts。
func TestToolResultImages(t *testing.T) {
	req, err := parseAnthropicRequest([]byte(`{"model":"m","max_tokens":1,"messages":[
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":[
			{"type":"text","text":"截图"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"IMG"}}]}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","content":"plain"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	m := req.Messages[0]
	if m.Role != "tool" || m.ToolCallId != "tu1" || m.Text != "截图" || len(m.Parts) != 2 || m.Parts[1].Data != "IMG" {
		t.Errorf("anthropic tool_result image wrong: %+v", m)
	}
	if m2 := req.Messages[1]; m2.Text != "plain" || len(m2.Parts) != 0 {
		t.Errorf("anthropic plain tool_result wrong: %+v", m2)
	}

	req, err = parseResponsesRequest([]byte(`{"model":"m","input":[
		{"type":"function_call_output","call_id":"c1","output":[{"type":"input_text","text":"shot"},{"type":"input_image","image_url":"data:image/png;base64,IMG"}]},
		{"type":"function_call_output","call_id":"c2","output":"plain"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	m = req.Messages[0]
	if m.Role != "tool" || m.Text != "shot" || len(m.Parts) != 2 || m.Parts[1].MediaType != "image/png" || m.Parts[1].Data != "IMG" {
		t.Errorf("responses function_call_output image wrong: %+v", m)
	}
	if m2 := req.Messages[1]; m2.Text != "plain" || len(m2.Parts) != 0 {
		t.Errorf("responses plain output wrong: %+v", m2)
	}
}

// TestAnthropicParamsAndCache 提示缓存断点 / is_error / top_k / temperature=0 / disable_parallel_tool_use / metadata。
func TestAnthropicParamsAndCache(t *testing.T) {
	req, err := parseAnthropicRequest([]byte(`{"model":"m","max_tokens":1,"temperature":0,"top_k":5,
		"system":[{"type":"text","text":"s1"},{"type":"text","text":"s2","cache_control":{"type":"ephemeral"}}],
		"tools":[{"name":"f","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}],
		"tool_choice":{"type":"any","disable_parallel_tool_use":true},
		"metadata":{"user_id":"u1"},
		"messages":[
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu","content":"boom","is_error":true,"cache_control":{"type":"ephemeral"}}]},
			{"role":"user","content":[{"type":"text","text":"q","cache_control":{"type":"ephemeral"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	sys := req.Messages[0]
	if sys.Role != "system" || sys.Text != "s1\ns2" || len(sys.Parts) != 2 || sys.Parts[1].CacheControl == "" || sys.Parts[0].CacheControl != "" {
		t.Errorf("system parts wrong: %+v", sys)
	}
	if req.Tools[0].CacheControl == "" {
		t.Errorf("tool cache_control lost")
	}
	if req.ToolChoice == nil || req.ToolChoice.Type != "tool" || req.ToolChoice.ToolName != "" {
		t.Errorf("tool_choice any wrong: %+v", req.ToolChoice)
	}
	for k, want := range map[string]string{"temperature": "0", "top_k": "5", "parallel_tool_calls": "false", "user": "u1"} {
		if req.Extra[k] != want {
			t.Errorf("extra %s=%q want %q", k, req.Extra[k], want)
		}
	}
	tr := req.Messages[1]
	if !tr.ToolError || len(tr.Parts) != 1 || tr.Parts[0].CacheControl == "" || tr.Text != "boom" {
		t.Errorf("tool_result wrong: %+v", tr)
	}
	if u := req.Messages[2]; len(u.Parts) != 1 || u.Parts[0].CacheControl == "" {
		t.Errorf("user text cache_control lost: %+v", u)
	}
}

// TestOpenAIExtrasPassthrough OpenAI 专有参数进 Extra；temperature=0 显式记录。
func TestOpenAIExtrasPassthrough(t *testing.T) {
	req, err := parseChatCompletions([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],
		"temperature":0,"seed":42,"frequency_penalty":0.5,"parallel_tool_calls":false,
		"response_format":{"type":"json_object"},"user":"u"}`))
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"temperature": "0", "seed": "42", "frequency_penalty": "0.5",
		"parallel_tool_calls": "false", "response_format": `{"type":"json_object"}`, "user": "u"} {
		if req.Extra[k] != want {
			t.Errorf("extra %s=%q want %q", k, req.Extra[k], want)
		}
	}
	if _, ok := req.Extra["presence_penalty"]; ok {
		t.Errorf("absent field must not be set")
	}
}

// TestStreamFailureAndEarlyUsage 流中失败 → 各协议错误事件；message_start 带上游早期 usage；reasoning_tokens 透出。
func TestStreamFailureAndEarlyUsage(t *testing.T) {
	for name, enc := range map[string]streamEncoder{"anth": newAnthSSEState("m"), "oai": newOpenAISSEState(), "resp": newResponsesSSEState("m")} {
		out := enc.failure("upstream exploded")
		if !strings.Contains(out, "upstream exploded") {
			t.Errorf("%s failure missing message: %s", name, out)
		}
	}
	anth := newAnthSSEState("m").failure("x")
	if !strings.HasPrefix(anth, "event: error\n") || !strings.Contains(anth, `"type":"error"`) {
		t.Errorf("anthropic error event wrong: %s", anth)
	}
	if resp := newResponsesSSEState("m").failure("x"); !strings.Contains(resp, "response.failed") || !strings.Contains(resp, `"status":"failed"`) {
		t.Errorf("responses failed event wrong: %s", resp)
	}

	start := newAnthSSEState("m").convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{
		MessageStart: &pb.MessageStart{Model: "m", Usage: &pb.Usage{InputTokens: 7, CachedTokens: 900}},
	}})
	if !strings.Contains(start, `"input_tokens":7`) || !strings.Contains(start, `"cache_read_input_tokens":900`) {
		t.Errorf("message_start early usage lost: %s", start)
	}

	u := &pb.Usage{InputTokens: 1, OutputTokens: 50, ReasoningTokens: 30}
	if b, _ := json.Marshal(openaiUsage(u)); !strings.Contains(string(b), `"reasoning_tokens":30`) {
		t.Errorf("openai reasoning_tokens lost: %s", b)
	}
	if b, _ := json.Marshal(responsesUsage(u)); !strings.Contains(string(b), `"reasoning_tokens":30`) {
		t.Errorf("responses reasoning_tokens lost: %s", b)
	}
	if b, _ := json.Marshal(errBody("api_error", errors.New("e"))); !strings.Contains(string(b), `"type":"error"`) {
		t.Errorf("errBody missing top-level type: %s", b)
	}
}

// TestStopSequencePassthrough Anthropic 出口保留 stop_sequence 原值；OpenAI / Responses 出口折成 stop。
func TestStopSequencePassthrough(t *testing.T) {
	fin := func() *pb.StreamEvent {
		return &pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{MessageFinish: &pb.MessageFinish{
			FinishReason: "stop_sequence", StopSequence: "END_MARK", Usage: &pb.Usage{},
		}}}
	}
	anth := newAnthSSEState("m").convertEvent(fin())
	if !strings.Contains(anth, `"stop_reason":"stop_sequence","stop_sequence":"END_MARK"`) {
		t.Errorf("anthropic sse lost stop_sequence:\n%s", anth)
	}
	ag := &anthAggregate{}
	ag.feed(fin())
	if b, _ := json.Marshal(ag.result()); !strings.Contains(string(b), `"stop_reason":"stop_sequence","stop_sequence":"END_MARK"`) {
		t.Errorf("anthropic aggregate lost stop_sequence: %s", b)
	}
	// 普通结束仍是 null
	plain := newAnthSSEState("m").convertEvent(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{FinishReason: "stop", Usage: &pb.Usage{}}}})
	if !strings.Contains(plain, `"stop_reason":"end_turn","stop_sequence":null`) {
		t.Errorf("plain stop wrong:\n%s", plain)
	}
	oai := newOpenAISSEState().convertEvent(fin())
	if !strings.Contains(oai, `"finish_reason":"stop"`) || strings.Contains(oai, "stop_sequence") {
		t.Errorf("openai should fold to stop:\n%s", oai)
	}
	for name, a := range map[string]aggregate{"oai": &openaiAggregate{}, "resp": &responsesAggregate{}} {
		a.feed(fin())
		if b, _ := json.Marshal(a.result()); strings.Contains(string(b), "stop_sequence") {
			t.Errorf("%s aggregate leaked stop_sequence: %s", name, b)
		}
	}
}
