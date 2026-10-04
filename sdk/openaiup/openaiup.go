// Package openaiup — OpenAI 兼容上游的通用适配：统一信封 ↔ chat/completions。
// 大多数 Claw 类上游都讲 OpenAI 协议，插件作者复用本包即可只写差异部分。
package openaiup

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/requestutil"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/streamutil"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// ChatBody 信封请求 → OpenAI chat/completions 请求体。
// 始终 stream=true 并带 include_usage：上游聚合与非聚合都由事件流表达。
func ChatBody(req *pb.ChatRequest) map[string]interface{} {
	var messages []map[string]interface{}
	// tool 消息不收图片：图片攒起来，等这一组连续 tool 消息结束后以 user 消息补发
	// （tool 消息必须紧跟声明它的 assistant，中间不能插 user）
	var pendImages []map[string]interface{}
	flushImages := func() {
		if len(pendImages) == 0 {
			return
		}
		messages = append(messages, map[string]interface{}{"role": "user", "content": pendImages})
		pendImages = nil
	}
	for _, m := range req.Messages {
		if m.Role != "tool" {
			flushImages()
		}
		msg := map[string]interface{}{"role": m.Role, "content": contentOf(m)}
		if m.Role == "tool" {
			msg["content"] = m.Text
			for _, p := range m.Parts {
				if p.Type == "image" {
					pendImages = append(pendImages, imageItem(p))
				}
			}
		}
		if len(m.ToolCalls) > 0 {
			if m.Role == "assistant" {
				if m.Text == "" {
					msg["content"] = nil
				}
				var tcs []map[string]interface{}
				for _, tc := range m.ToolCalls {
					tcs = append(tcs, map[string]interface{}{
						"id": tc.Id, "type": "function",
						"function": map[string]interface{}{
							"name": tc.Name, "arguments": tc.Arguments,
						},
					})
				}
				msg["tool_calls"] = tcs
			}
		}
		if m.ToolCallId != "" {
			msg["tool_call_id"] = m.ToolCallId
		}
		if m.Role == "assistant" {
			var refusal string
			var annotations []json.RawMessage
			for _, p := range m.Parts {
				if p.Type == "refusal" {
					refusal += p.Text
				}
				if p.Source == "chat" && p.Annotations != "" {
					var values []json.RawMessage
					_ = json.Unmarshal([]byte(p.Annotations), &values)
					annotations = append(annotations, values...)
				}
			}
			if refusal != "" {
				msg["refusal"] = refusal
				if m.Text == "" {
					msg["content"] = nil
				}
			}
			if len(annotations) > 0 {
				msg["annotations"] = annotations
			}
		}
		messages = append(messages, msg)
	}
	flushImages()
	body := map[string]interface{}{
		"model":          req.Model,
		"messages":       messages,
		"stream":         true,
		"stream_options": map[string]interface{}{"include_usage": true},
	}
	if len(req.Tools) > 0 {
		var tools []map[string]interface{}
		for _, t := range req.Tools {
			function := map[string]interface{}{
				"name": t.Name, "description": t.Description,
				"parameters": rawJSON(t.ParametersSchema),
			}
			requestutil.ApplyToolStrict(req, t.Name, function)
			tools = append(tools, map[string]interface{}{
				"type":     "function",
				"function": function,
			})
		}
		body["tools"] = tools
		if tc := req.ToolChoice; tc != nil {
			switch tc.Type {
			case "auto", "none":
				body["tool_choice"] = tc.Type
			case "tool":
				if tc.ToolName == "" {
					body["tool_choice"] = "required" // Anthropic 的 any
				} else {
					body["tool_choice"] = map[string]interface{}{
						"type": "function", "function": map[string]interface{}{"name": tc.ToolName},
					}
				}
			}
		}
	}
	if req.MaxTokens > 0 {
		if req.Extra["openai_max_completion_tokens"] == "true" {
			body["max_completion_tokens"] = req.MaxTokens
		} else {
			body["max_tokens"] = req.MaxTokens
		}
	}
	if v, ok := req.Extra["temperature"]; ok && v != "" {
		body["temperature"] = jsonNumber(v) // 显式给出（含 0）
	} else if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if v, ok := req.Extra["top_p"]; ok && v != "" {
		body["top_p"] = jsonNumber(v)
	}
	if v, ok := req.Extra["stop"]; ok && v != "" {
		body["stop"] = rawJSON(v)
	}
	if v := requestutil.ReasoningEffort(req.Extra); v != "" {
		body["reasoning_effort"] = v
	} else if v := req.Extra["thinking"]; v != "" {
		// Anthropic 客户端的 thinking budget → reasoning_effort
		if effort := effortFromThinking(v); effort != "" {
			body["reasoning_effort"] = effort
		}
	}
	// OpenAI 系专有参数：原样透传（JSON 值）
	for _, k := range []string{"frequency_penalty", "presence_penalty", "seed", "parallel_tool_calls"} {
		if v := req.Extra[k]; v != "" {
			body[k] = rawJSON(v)
		}
	}
	if format := requestutil.ChatResponseFormat(req.Extra); format != nil {
		body["response_format"] = format
	}
	if v := req.Extra["user"]; v != "" {
		body["user"] = v
	}
	requestutil.ApplyNative(req, body, "chat")
	return body
}

// effortFromThinking Anthropic thinking 配置 → reasoning_effort：≤1024 low、≤8192 medium、其余 high；
// 未开启返回空。
func effortFromThinking(raw string) string {
	var t struct {
		Type   string `json:"type"`
		Budget int    `json:"budget_tokens"`
	}
	if json.Unmarshal([]byte(raw), &t) != nil || t.Type != "enabled" {
		return ""
	}
	switch {
	case t.Budget <= 0:
		return "high" // 自适应（无 budget）按高强度
	case t.Budget <= 1024:
		return "low"
	case t.Budget <= 8192:
		return "medium"
	default:
		return "high"
	}
}

// contentOf 消息内容：无 parts 用纯文本；含图片时用 parts 数组（text + image_url）。
// 推理块不回传：DeepSeek 等上游对输入里的 reasoning_content 直接 400。
func contentOf(m *pb.EnvelopeMessage) interface{} {
	hasImage := false
	for _, p := range m.Parts {
		if p.Type == "image" {
			hasImage = true
			break
		}
	}
	if !hasImage {
		return m.Text
	}
	var items []map[string]interface{}
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			items = append(items, map[string]interface{}{"type": "text", "text": p.Text})
		case "image":
			items = append(items, imageItem(p))
		}
	}
	return items
}

// imageItem 图片块 → OpenAI image_url 项（base64 重组为 data URL）。
func imageItem(p *pb.ContentPart) map[string]interface{} {
	url := p.Url
	if url == "" {
		url = "data:" + p.MediaType + ";base64," + p.Data
	}
	image := map[string]interface{}{"url": url}
	if p.ImageDetail != "" {
		image["detail"] = p.ImageDetail
	}
	return map[string]interface{}{"type": "image_url", "image_url": image}
}

// Parser 把上游 OpenAI SSE 行解析为信封事件。
// 用法：每读一行调 Feed；流结束时调 Finish 把挂起的 finish_reason 落地。
type Parser struct {
	emit        func(*pb.StreamEvent)
	pendingStop string
	toolIDs     map[int]string // tool_calls index → 调用身份
	tools       map[int]*pendingTool
	usage       *pb.Usage // 跨块合并的用量（部分上游每块都带累计 usage）
	sentFinish  bool
	sawDone     bool
	annotations streamutil.AnnotationSet
	refused     bool
}

func NewParser(emit func(*pb.StreamEvent)) *Parser {
	return &Parser{emit: emit, toolIDs: map[int]string{}, tools: map[int]*pendingTool{}}
}

type pendingTool struct {
	id, name, args string
	emitted        bool
}

// chunkUsage OpenAI 方言的 usage 块：prompt_tokens 含缓存读写；缓存明细在
// prompt_tokens_details（OpenAI / New API），DeepSeek 用顶层 prompt_cache_hit_tokens。
type chunkUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptCacheHit      int64 `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails struct {
		CachedTokens     int64 `json:"cached_tokens"`
		CacheWriteTokens int64 `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

// toEnvelope OpenAI 语义 → 信封（Anthropic）语义：input 剥离缓存读写，不为负。
func (u *chunkUsage) toEnvelope() *pb.Usage {
	cached := u.PromptTokensDetails.CachedTokens
	if cached == 0 {
		cached = u.PromptCacheHit
	}
	creation := u.PromptTokensDetails.CacheWriteTokens
	input := u.PromptTokens - cached - creation
	if input < 0 {
		input = 0
	}
	return &pb.Usage{
		InputTokens: input, OutputTokens: u.CompletionTokens,
		CachedTokens: cached, CacheCreationTokens: creation,
		ReasoningTokens: u.CompletionTokensDetails.ReasoningTokens,
	}
}

// Feed 处理一行（"data: {...}" 或 "data: [DONE]"）。
func (p *Parser) Feed(line string) {
	if p.sentFinish {
		return
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "[DONE]" {
		p.sawDone = true
		p.finish()
		return
	}
	if payload == "" {
		return
	}
	var chunk struct {
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Status  int32  `json:"status"`
		} `json:"error"`
		Choices []struct {
			Delta struct {
				Role             string            `json:"role"`
				Content          string            `json:"content"`
				Refusal          string            `json:"refusal"`
				Annotations      []json.RawMessage `json:"annotations"`
				ReasoningContent string            `json:"reasoning_content"` // DeepSeek / New API
				Reasoning        string            `json:"reasoning"`         // OpenRouter
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *chunkUsage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		p.FinishWithError(502, "invalid upstream JSON")
		return
	}
	if chunk.Error != nil {
		p.FinishWithError(streamutil.ErrorCode(chunk.Error.Type, chunk.Error.Status), chunk.Error.Message)
		return
	}
	for _, c := range chunk.Choices {
		if raw := p.annotations.Add("text", c.Delta.Annotations); raw != "" {
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Annotations: raw, Source: "chat"}}})
		}
		if c.Delta.Refusal != "" {
			p.refused = true
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Text: c.Delta.Refusal, Refusal: true, Source: "chat"}}})
		}
		if r := orDefault(c.Delta.ReasoningContent, c.Delta.Reasoning); r != "" {
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ReasoningDelta{
				ReasoningDelta: &pb.ReasoningDelta{Text: r},
			}})
		}
		if c.Delta.Content != "" {
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
				ContentDelta: &pb.ContentDelta{Text: c.Delta.Content, Source: "chat"},
			}})
		}
		for _, tc := range c.Delta.ToolCalls {
			tool := p.tools[tc.Index]
			if tool == nil {
				tool = &pendingTool{}
				p.tools[tc.Index] = tool
			}
			if tc.ID != "" && tool.id != "" && tool.id != tc.ID {
				p.FinishWithError(502, "conflicting tool call identity")
				return
			}
			if tc.ID != "" {
				for index, id := range p.toolIDs {
					if index != tc.Index && id == tc.ID {
						p.FinishWithError(502, "duplicate tool call identity")
						return
					}
				}
				p.toolIDs[tc.Index] = tc.ID
				tool.id = tc.ID
			}
			if tool.id == "" {
				p.FinishWithError(502, "tool continuation has no call identity")
				return
			}
			if tc.Function.Name != "" {
				if tool.emitted && tc.Function.Name != tool.name {
					p.FinishWithError(502, "tool name changed after arguments started")
					return
				}
				if !tool.emitted {
					tool.name += tc.Function.Name
				}
			}
			tool.args += tc.Function.Arguments
			if len(tool.args)+len(tool.name) > 32<<20 {
				p.FinishWithError(502, "tool call exceeds 32 MiB")
				return
			}
			if tool.name != "" && tool.args != "" {
				p.flushTool(tool)
			}
		}
		if c.FinishReason != nil && *c.FinishReason != "" {
			p.pendingStop = *c.FinishReason
		}
	}
	if chunk.Usage != nil {
		p.usage = mergeUsage(p.usage, chunk.Usage.toEnvelope())
	}
	// 等待 [DONE] 或 EOF，避免早先的累计 usage 遮蔽 finish_reason 之后的最终用量。
}

// Finish 流结束：把挂起的 finish_reason 落地（从未发过时按已合并的 usage 收尾）。
func (p *Parser) Finish() {
	if p.sentFinish {
		return
	}
	if p.pendingStop == "" && !p.sawDone {
		p.FinishWithError(502, "upstream ended before a terminal event")
		return
	}
	p.finish()
}

// FinishWithError 流异常结束：发失败事件。
func (p *Parser) FinishWithError(code int32, message string) {
	if p.sentFinish {
		return
	}
	p.sentFinish = true
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{
		TaskFailed: &pb.TaskFailed{Error: &pb.Error{Code: code, Message: message}},
	}})
}

func (p *Parser) finish() {
	if p.sentFinish {
		return
	}
	indices := make([]int, 0, len(p.tools))
	for index := range p.tools {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		tool := p.tools[index]
		if tool.name == "" {
			p.FinishWithError(502, "tool call has no name")
			return
		}
		if !tool.emitted && tool.args == "" {
			tool.args = "{}"
		}
		if !tool.emitted || tool.args != "" {
			p.flushTool(tool)
		}
	}
	p.sentFinish = true
	if p.refused && (p.pendingStop == "" || p.pendingStop == "stop") {
		p.pendingStop = "content_filter"
	}
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{
			FinishReason: orDefault(p.pendingStop, "stop"),
			Usage:        p.usage,
		},
	}})
}

func (p *Parser) flushTool(tool *pendingTool) {
	name := ""
	if !tool.emitted {
		name = tool.name
	}
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{Id: tool.id, Name: name, ArgumentsDelta: tool.args}}})
	tool.emitted, tool.args = true, ""
}

// mergeUsage 后到的非零字段覆盖，零值不擦除已有计数。
func mergeUsage(cur, in *pb.Usage) *pb.Usage {
	if cur == nil {
		cur = &pb.Usage{}
	}
	if in == nil {
		return cur
	}
	if in.InputTokens > 0 {
		cur.InputTokens = in.InputTokens
	}
	if in.OutputTokens > 0 {
		cur.OutputTokens = in.OutputTokens
	}
	if in.CachedTokens > 0 {
		cur.CachedTokens = in.CachedTokens
	}
	if in.CacheCreationTokens > 0 {
		cur.CacheCreationTokens = in.CacheCreationTokens
	}
	if in.ReasoningTokens > 0 {
		cur.ReasoningTokens = in.ReasoningTokens
	}
	return cur
}

// ---------- 工具 ----------

func rawJSON(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}

func jsonNumber(s string) interface{} {
	var v json.Number
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
