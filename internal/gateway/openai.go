// openai.go — OpenAI Chat Completions 协议 ↔ 统一信封。
package gateway

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/requestutil"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// parseChatCompletions 把 /v1/chat/completions 请求体转成统一信封。
func parseChatCompletions(body []byte) (*pb.ChatRequest, error) {
	if err := validateRequestContent(body, "chat"); err != nil {
		return nil, err
	}
	var raw struct {
		Model               string          `json:"model"`
		Messages            []openaiMessage `json:"messages"`
		MaxTokens           int32           `json:"max_tokens"`
		MaxCompletionTokens int32           `json:"max_completion_tokens"` // 新版字段，max_tokens 的替代
		N                   *int            `json:"n"`
		Temperature         *float64        `json:"temperature"`
		TopP                *float64        `json:"top_p"`
		Stop                json.RawMessage `json:"stop"`
		Tools               []openaiTool    `json:"tools"`
		ToolChoice          json.RawMessage `json:"tool_choice"`
		Stream              bool            `json:"stream"`
		ReasoningEffort     string          `json:"reasoning_effort"`
		// 只对 OpenAI 系上游有意义的采样/控制参数：原样透传
		FrequencyPenalty  json.RawMessage `json:"frequency_penalty"`
		PresencePenalty   json.RawMessage `json:"presence_penalty"`
		Seed              json.RawMessage `json:"seed"`
		ParallelToolCalls json.RawMessage `json:"parallel_tool_calls"`
		ResponseFormat    json.RawMessage `json:"response_format"`
		User              string          `json:"user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if len(raw.Messages) == 0 {
		return nil, fmt.Errorf("messages is required")
	}
	if raw.N != nil && *raw.N != 1 {
		return nil, fmt.Errorf("only n=1 is supported")
	}
	if raw.MaxCompletionTokens != 0 {
		raw.MaxTokens = raw.MaxCompletionTokens
	}

	req := &pb.ChatRequest{
		Model:       raw.Model,
		Stream:      raw.Stream,
		MaxTokens:   raw.MaxTokens,
		Temperature: deref(raw.Temperature),
		Extra:       map[string]string{},
	}
	setTemperature(req, raw.Temperature)
	requestutil.CaptureNative(req, body, "chat")
	if raw.MaxCompletionTokens != 0 {
		req.Extra["openai_max_completion_tokens"] = "true"
	}
	if raw.TopP != nil {
		req.Extra["top_p"] = fmt.Sprintf("%g", *raw.TopP)
	}
	if len(raw.Stop) > 0 {
		req.Extra["stop"] = string(raw.Stop)
	}
	if raw.ReasoningEffort != "" {
		req.Extra["reasoning_effort"] = raw.ReasoningEffort
	}
	for k, v := range map[string]json.RawMessage{
		"frequency_penalty": raw.FrequencyPenalty, "presence_penalty": raw.PresencePenalty,
		"seed": raw.Seed, "parallel_tool_calls": raw.ParallelToolCalls, "response_format": raw.ResponseFormat,
	} {
		if len(v) > 0 && string(v) != "null" {
			req.Extra[k] = string(v)
		}
	}
	if raw.User != "" {
		req.Extra["user"] = raw.User
	}

	for i := range raw.Messages {
		m := &raw.Messages[i]
		parts := openaiParts(m.Content)
		if m.Refusal != "" {
			parts = append(parts, &pb.ContentPart{Type: "refusal", Text: m.Refusal})
		}
		if len(m.Annotations) > 0 && string(m.Annotations) != "null" {
			if len(parts) == 0 {
				parts = append(parts, &pb.ContentPart{Type: "text"})
			}
			parts[0].Annotations = string(m.Annotations)
			parts[0].Source = "chat"
		}
		em := &pb.EnvelopeMessage{Role: normalizeRole(m.Role), Text: partsText(parts), Raw: m.Content}
		if m.ReasoningContent != "" && em.Role == "assistant" {
			// 推理正文放最前（Anthropic 要求 thinking 块先于 text）
			parts = append([]*pb.ContentPart{{Type: "thinking", Text: m.ReasoningContent}}, parts...)
		}
		em.Parts = finishParts(parts)
		for _, tc := range m.ToolCalls {
			args := tc.Function.Arguments
			if args == "" {
				args = "{}" // 空 arguments 跨方言会变成非法 tool_use.input
			} else if !json.Valid([]byte(args)) {
				return nil, fmt.Errorf("tool_call %q arguments must be valid JSON", tc.ID)
			}
			em.ToolCalls = append(em.ToolCalls, &pb.ToolCall{
				Id: tc.ID, Name: tc.Function.Name, Arguments: args,
			})
		}
		if m.ToolCallID != "" {
			em.ToolCallId = m.ToolCallID
		}
		req.Messages = append(req.Messages, em)
	}

	for _, t := range raw.Tools {
		if t.Type != "function" || t.Function.Name == "" {
			return nil, fmt.Errorf("unsupported tool type or missing function name: %s", t.Type)
		}
		requestutil.SetToolStrict(req, t.Function.Name, t.Function.Strict)
		if t.Function.Name != "" {
			req.Tools = append(req.Tools, &pb.ToolDefinition{
				Name: t.Function.Name, Description: t.Function.Description,
				ParametersSchema: string(t.Function.Parameters),
			})
		}
	}
	tc, err := convertOpenAIToolChoice(raw.ToolChoice)
	if err != nil {
		return nil, err
	}
	req.ToolChoice = tc
	return req, validateToolHistory(req)
}

type openaiMessage struct {
	Role        string          `json:"role"`
	Refusal     string          `json:"refusal"`
	Annotations json.RawMessage `json:"annotations"`
	Content     json.RawMessage `json:"content"`
	ToolCalls   []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
	ToolCallID       string `json:"tool_call_id"`
	ReasoningContent string `json:"reasoning_content"` // DeepSeek / New API 方言的历史推理
}

// openaiParts OpenAI content（string 或 parts 数组）→ 内容块：text / image_url。
func openaiParts(raw json.RawMessage) []*pb.ContentPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []*pb.ContentPart{{Type: "text", Text: s}}
	}
	var items []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL    string `json:"url"`
			Detail string `json:"detail"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	var parts []*pb.ContentPart
	for _, it := range items {
		switch it.Type {
		case "text":
			parts = append(parts, &pb.ContentPart{Type: "text", Text: it.Text})
		case "image_url":
			if it.ImageURL.URL != "" {
				part := imagePart(it.ImageURL.URL)
				part.ImageDetail = it.ImageURL.Detail
				parts = append(parts, part)
			}
		}
	}
	return parts
}

type openaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
		Strict      *bool           `json:"strict"`
	} `json:"function"`
}

func convertOpenAIToolChoice(raw json.RawMessage) (*pb.ToolChoice, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto", "none":
			return &pb.ToolChoice{Type: s}, nil
		case "required":
			return &pb.ToolChoice{Type: "tool"}, nil
		}
		return nil, fmt.Errorf("unsupported tool_choice: %s", s)
	}
	var tc struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
	}
	if tc.Type == "function" && tc.Function.Name != "" {
		return &pb.ToolChoice{Type: "tool", ToolName: tc.Function.Name}, nil
	}
	return nil, fmt.Errorf("unsupported tool_choice or missing function name: %s", tc.Type)
}

// ---------- 信封事件 → OpenAI SSE ----------

type openaiSSEState struct {
	model   string
	id      string
	created int64
	// tool call 聚合（OpenAI 的 delta.tool_calls 用 index 标识）
	toolIdx map[string]int
	nextIdx int
}

func newOpenAISSEState() *openaiSSEState {
	return &openaiSSEState{id: "chatcmpl-" + randHex(12), created: time.Now().Unix(), toolIdx: map[string]int{}}
}

// convertEvent 信封事件 → OpenAI chat.completion.chunk SSE 行。
func (s *openaiSSEState) convertEvent(ev *pb.StreamEvent) string {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		s.model = e.MessageStart.Model
		return ""

	case *pb.StreamEvent_ReasoningDelta:
		// 签名对 OpenAI 客户端无意义，只透推理文本
		if e.ReasoningDelta.Text == "" {
			return ""
		}
		return s.chunk(map[string]interface{}{
			"role": "assistant", "reasoning_content": e.ReasoningDelta.Text,
		}, "")

	case *pb.StreamEvent_ContentDelta:
		delta := map[string]interface{}{"role": "assistant"}
		if e.ContentDelta.Refusal {
			delta["refusal"] = e.ContentDelta.Text
		} else if e.ContentDelta.Text != "" {
			delta["content"] = e.ContentDelta.Text
		}
		if e.ContentDelta.Source == "chat" && e.ContentDelta.Annotations != "" {
			delta["annotations"] = json.RawMessage(e.ContentDelta.Annotations)
		}
		if len(delta) == 1 {
			return ""
		}
		return s.chunk(delta, "")

	case *pb.StreamEvent_ToolCallDelta:
		idx, ok := s.toolIdx[e.ToolCallDelta.Id]
		if !ok {
			idx = s.nextIdx
			s.nextIdx++
			s.toolIdx[e.ToolCallDelta.Id] = idx
		}
		fn := map[string]interface{}{"arguments": e.ToolCallDelta.ArgumentsDelta}
		tool := map[string]interface{}{"index": idx, "function": fn}
		if !ok {
			fn["name"] = e.ToolCallDelta.Name
			tool["id"] = e.ToolCallDelta.Id
			tool["type"] = "function"
		}
		return s.chunk(map[string]interface{}{
			"tool_calls": []interface{}{tool},
		}, "")

	case *pb.StreamEvent_MessageFinish:
		out := s.chunk(map[string]interface{}{}, openaiFinish(e.MessageFinish.FinishReason))
		// 带 usage 的终止块（stream_options.include_usage 的客户端靠它收用量）
		if e.MessageFinish.Usage != nil {
			out += s.chunkRaw(map[string]interface{}{
				"id": s.id, "object": "chat.completion.chunk", "created": s.created,
				"model": s.model, "choices": []interface{}{},
				"usage": openaiUsage(e.MessageFinish.Usage),
			})
		}
		return out
	}
	return ""
}

// chunk 生成一个 choices[0] 带 delta 与可选 finish_reason 的 chunk。
func (s *openaiSSEState) chunk(delta map[string]interface{}, finish string) string {
	choice := map[string]interface{}{"index": 0, "delta": delta}
	if finish != "" {
		choice["finish_reason"] = finish
	}
	return s.chunkRaw(map[string]interface{}{
		"id": s.id, "object": "chat.completion.chunk", "created": s.created,
		"model": s.model, "choices": []interface{}{choice},
	})
}

func (s *openaiSSEState) chunkRaw(payload map[string]interface{}) string {
	b, _ := json.Marshal(payload)
	return "data: " + string(b) + "\n\n"
}

// openaiAggregate 非流式聚合。
type openaiAggregate struct {
	refusal     string
	annotations []json.RawMessage
	model       string
	reasoning   string
	text        string
	tools       map[string]*aggrTool
	finish      string
	usage       *pb.Usage
}

func (a *openaiAggregate) feed(ev *pb.StreamEvent) {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		a.model = e.MessageStart.Model
	case *pb.StreamEvent_ReasoningDelta:
		a.reasoning += e.ReasoningDelta.Text
	case *pb.StreamEvent_ContentDelta:
		if e.ContentDelta.Refusal {
			a.refusal += e.ContentDelta.Text
		} else {
			a.text += e.ContentDelta.Text
		}
		if e.ContentDelta.Source == "chat" && e.ContentDelta.Annotations != "" {
			var values []json.RawMessage
			_ = json.Unmarshal([]byte(e.ContentDelta.Annotations), &values)
			a.annotations = append(a.annotations, values...)
		}
	case *pb.StreamEvent_ToolCallDelta:
		if a.tools == nil {
			a.tools = map[string]*aggrTool{}
		}
		t, ok := a.tools[e.ToolCallDelta.Id]
		if !ok {
			t = &aggrTool{id: e.ToolCallDelta.Id, name: e.ToolCallDelta.Name}
			a.tools[e.ToolCallDelta.Id] = t
		}
		t.input += e.ToolCallDelta.ArgumentsDelta
	case *pb.StreamEvent_MessageFinish:
		a.finish = openaiFinish(e.MessageFinish.FinishReason)
		a.usage = e.MessageFinish.Usage
	}
}

func (a *openaiAggregate) result() map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant", "content": a.text}
	if a.refusal != "" {
		msg["refusal"] = a.refusal
		if a.text == "" {
			msg["content"] = nil
		}
	}
	if len(a.annotations) > 0 {
		msg["annotations"] = a.annotations
	}
	if a.reasoning != "" {
		msg["reasoning_content"] = a.reasoning
	}
	if len(a.tools) > 0 {
		msg["content"] = nil
		var tcs []interface{}
		for _, id := range sortedKeys(a.tools) {
			t := a.tools[id]
			tcs = append(tcs, map[string]interface{}{
				"id": t.id, "type": "function",
				"function": map[string]interface{}{"name": t.name, "arguments": t.input},
			})
		}
		msg["tool_calls"] = tcs
	}
	var choices []interface{}
	choices = append(choices, map[string]interface{}{
		"index": 0, "message": msg, "finish_reason": a.finish,
	})
	return map[string]interface{}{
		"id": "chatcmpl-" + randHex(12), "object": "chat.completion",
		"created": time.Now().Unix(), "model": a.model, "choices": choices,
		"usage": openaiUsage(a.usage),
	}
}
