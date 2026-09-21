// anthropic.go — Anthropic Messages 协议 ↔ 统一信封。
package gateway

import (
	"encoding/json"
	"fmt"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// parseAnthropicRequest 把 /v1/messages 请求体转成统一信封。
func parseAnthropicRequest(body []byte) (*pb.ChatRequest, error) {
	var raw struct {
		Model         string          `json:"model"`
		System        json.RawMessage `json:"system"`
		Messages      []anthMessage   `json:"messages"`
		MaxTokens     int32           `json:"max_tokens"`
		Temperature   *float64        `json:"temperature"`
		TopP          *float64        `json:"top_p"`
		TopK          *int            `json:"top_k"`
		StopSequences []string        `json:"stop_sequences"`
		Tools         []anthTool      `json:"tools"`
		ToolChoice    json.RawMessage `json:"tool_choice"`
		Stream        bool            `json:"stream"`
		Thinking      json.RawMessage `json:"thinking"`
		Metadata      struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if len(raw.Messages) == 0 {
		return nil, fmt.Errorf("messages is required")
	}

	req := &pb.ChatRequest{
		Model:       raw.Model,
		Stream:      raw.Stream,
		MaxTokens:   raw.MaxTokens,
		Temperature: deref(raw.Temperature),
		Extra:       map[string]string{},
	}
	setTemperature(req, raw.Temperature)

	// system 可能是 string 或 blocks；blocks 带 cache_control 时保留 parts（提示缓存断点）
	if parts := anthParts(raw.System); len(parts) > 0 {
		req.Messages = append(req.Messages, &pb.EnvelopeMessage{
			Role: "system", Text: partsText(parts), Parts: finishParts(parts),
		})
	}

	for i := range raw.Messages {
		req.Messages = append(req.Messages, convertAnthMessage(&raw.Messages[i])...)
	}

	for _, t := range raw.Tools {
		req.Tools = append(req.Tools, &pb.ToolDefinition{
			Name:             t.Name,
			Description:      t.Description,
			ParametersSchema: string(t.InputSchema),
			CacheControl:     string(t.CacheControl),
		})
	}
	if tc, err := convertAnthToolChoice(raw.ToolChoice); err == nil && tc != nil {
		req.ToolChoice = tc
	}
	if len(raw.ToolChoice) > 0 {
		var dp struct {
			DisableParallel bool `json:"disable_parallel_tool_use"`
		}
		if json.Unmarshal(raw.ToolChoice, &dp) == nil && dp.DisableParallel {
			req.Extra["parallel_tool_calls"] = "false"
		}
	}

	if raw.TopP != nil {
		req.Extra["top_p"] = fmt.Sprintf("%g", *raw.TopP)
	}
	if raw.TopK != nil {
		req.Extra["top_k"] = fmt.Sprintf("%d", *raw.TopK)
	}
	if len(raw.StopSequences) > 0 {
		if b, err := json.Marshal(raw.StopSequences); err == nil {
			req.Extra["stop"] = string(b)
		}
	}
	// thinking 块原样透传给插件
	if len(raw.Thinking) > 0 {
		req.Extra["thinking"] = string(raw.Thinking)
	}
	if raw.Metadata.UserID != "" {
		req.Extra["user"] = raw.Metadata.UserID
	}

	return req, nil
}

type anthMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	CacheControl json.RawMessage `json:"cache_control"`
}

// convertAnthMessage 单条 Anthropic 消息 → 一到多条信封消息
// （tool_result 块会拆出独立的 role=tool 消息；图片 / 推理块进 parts）。
func convertAnthMessage(m *anthMessage) []*pb.EnvelopeMessage {
	// 纯文本 content
	if text := extractText(m.Content); text != "" && !isArray(m.Content) {
		return []*pb.EnvelopeMessage{{Role: m.Role, Text: text, Raw: m.Content}}
	}

	var out []*pb.EnvelopeMessage
	var assistantToolCalls []*pb.ToolCall
	var parts []*pb.ContentPart

	var blocks []struct {
		Type         string          `json:"type"`
		Text         string          `json:"text"`
		ID           string          `json:"id"`
		Name         string          `json:"name"`
		Input        json.RawMessage `json:"input"`
		ToolUseID    string          `json:"tool_use_id"`
		Content      json.RawMessage `json:"content"`
		IsError      bool            `json:"is_error"`
		Thinking     string          `json:"thinking"`
		Signature    string          `json:"signature"`
		Data         string          `json:"data"` // redacted_thinking
		CacheControl json.RawMessage `json:"cache_control"`
		Source       struct {
			Type      string `json:"type"` // base64 / url
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
			URL       string `json:"url"`
		} `json:"source"`
	}
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return []*pb.EnvelopeMessage{{Role: m.Role, Raw: m.Content}}
	}

	for _, b := range blocks {
		cc := string(b.CacheControl)
		switch b.Type {
		case "text":
			parts = append(parts, &pb.ContentPart{Type: "text", Text: b.Text, CacheControl: cc})
		case "image":
			if b.Source.Type == "url" {
				parts = append(parts, &pb.ContentPart{Type: "image", Url: b.Source.URL, CacheControl: cc})
			} else {
				parts = append(parts, &pb.ContentPart{Type: "image", MediaType: b.Source.MediaType, Data: b.Source.Data, CacheControl: cc})
			}
		case "thinking":
			parts = append(parts, &pb.ContentPart{Type: "thinking", Text: b.Thinking, Signature: b.Signature})
		case "redacted_thinking":
			parts = append(parts, &pb.ContentPart{Type: "redacted_thinking", Data: b.Data})
		case "tool_use":
			assistantToolCalls = append(assistantToolCalls, &pb.ToolCall{
				Id: b.ID, Name: b.Name, Arguments: compactJSON(b.Input),
			})
		case "tool_result":
			rp := anthParts(b.Content)
			if cc != "" {
				// tool_result 自身的缓存断点挂到末块（无块时补一个空文本块承载）
				if len(rp) == 0 {
					rp = []*pb.ContentPart{{Type: "text"}}
				}
				rp[len(rp)-1].CacheControl = cc
			}
			out = append(out, &pb.EnvelopeMessage{
				Role: "tool", Text: partsText(rp), Parts: finishParts(rp), ToolCallId: b.ToolUseID, ToolError: b.IsError,
			})
		}
	}

	if m.Role == "assistant" {
		out = append(out, &pb.EnvelopeMessage{
			Role: "assistant", Text: partsText(parts), ToolCalls: assistantToolCalls,
			Parts: finishParts(parts), Raw: m.Content,
		})
	} else if len(parts) > 0 {
		out = append(out, &pb.EnvelopeMessage{
			Role: m.Role, Text: partsText(parts), Parts: finishParts(parts), Raw: m.Content,
		})
	}
	return out
}

// anthParts system / tool_result 的 content（string 或 text/image 块数组）→ 内容块（保留 cache_control）。
func anthParts(raw json.RawMessage) []*pb.ContentPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return []*pb.ContentPart{{Type: "text", Text: s}}
	}
	var blocks []struct {
		Type         string          `json:"type"`
		Text         string          `json:"text"`
		CacheControl json.RawMessage `json:"cache_control"`
		Source       struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
			URL       string `json:"url"`
		} `json:"source"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	var parts []*pb.ContentPart
	for _, b := range blocks {
		cc := string(b.CacheControl)
		switch b.Type {
		case "text":
			parts = append(parts, &pb.ContentPart{Type: "text", Text: b.Text, CacheControl: cc})
		case "image":
			if b.Source.Type == "url" {
				parts = append(parts, &pb.ContentPart{Type: "image", Url: b.Source.URL, CacheControl: cc})
			} else {
				parts = append(parts, &pb.ContentPart{Type: "image", MediaType: b.Source.MediaType, Data: b.Source.Data, CacheControl: cc})
			}
		}
	}
	return parts
}

// convertAnthToolChoice Anthropic tool_choice → 信封 ToolChoice。
func convertAnthToolChoice(raw json.RawMessage) (*pb.ToolChoice, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return &pb.ToolChoice{Type: "auto"}, nil
		case "any", "required":
			return &pb.ToolChoice{Type: "tool"}, nil
		}
		return nil, fmt.Errorf("unsupported tool_choice: %s", s)
	}
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
	}
	if tc.Type == "auto" || tc.Type == "none" {
		return &pb.ToolChoice{Type: tc.Type}, nil
	}
	return &pb.ToolChoice{Type: "tool", ToolName: tc.Name}, nil
}

// ---------- 信封事件 → Anthropic SSE ----------

type anthSSEState struct {
	model      string
	msgID      string
	thinkBlock int // 当前 thinking 块 index；-1 未开 / 已关
	textBlock  int // 当前文本块 index；-1 未开
	nextBlock  int
	toolBlocks map[string]int // tool_call id → block index
	stopReason string
}

func newAnthSSEState(model string) *anthSSEState {
	return &anthSSEState{
		model: model, msgID: "msg_" + randHex(12),
		thinkBlock: -1, textBlock: -1, toolBlocks: map[string]int{},
	}
}

// closeThinking thinking 块必须先于 text / tool_use 关闭（Anthropic 块顺序约束）。
func (s *anthSSEState) closeThinking() string {
	if s.thinkBlock < 0 {
		return ""
	}
	idx := s.thinkBlock
	s.thinkBlock = -1
	return anthEvent("content_block_stop", map[string]interface{}{
		"type": "content_block_stop", "index": idx,
	})
}

// startMessage 返回 message_start 事件（流开始时发一次）。
// usage 用上游流开头已知的计数（Anthropic 上游有输入/缓存，OpenAI 上游为零值占位）；
// 真实用量在 message_delta 全量重报，客户端以 message_delta 为准。
func (s *anthSSEState) startMessage(usage *pb.Usage) string {
	return anthEvent("message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id": s.msgID, "type": "message", "role": "assistant", "model": s.model,
			"content": []interface{}{}, "stop_reason": nil, "usage": anthUsage(usage),
		},
	})
}

// convertEvent 把信封事件转成 Anthropic SSE 行（可能多行，\n 分隔）。
// 流结束时额外返回 message_stop 尾部。
func (s *anthSSEState) convertEvent(ev *pb.StreamEvent) string {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		s.model = e.MessageStart.Model
		return s.startMessage(e.MessageStart.Usage)

	case *pb.StreamEvent_ReasoningDelta:
		var out string
		if s.thinkBlock < 0 {
			s.thinkBlock = s.nextBlock
			s.nextBlock++
			out += anthEvent("content_block_start", map[string]interface{}{
				"type": "content_block_start", "index": s.thinkBlock,
				"content_block": map[string]interface{}{"type": "thinking", "thinking": ""},
			})
		}
		if e.ReasoningDelta.Text != "" {
			out += anthEvent("content_block_delta", map[string]interface{}{
				"type": "content_block_delta", "index": s.thinkBlock,
				"delta": map[string]interface{}{"type": "thinking_delta", "thinking": e.ReasoningDelta.Text},
			})
		}
		if e.ReasoningDelta.Signature != "" {
			out += anthEvent("content_block_delta", map[string]interface{}{
				"type": "content_block_delta", "index": s.thinkBlock,
				"delta": map[string]interface{}{"type": "signature_delta", "signature": e.ReasoningDelta.Signature},
			})
		}
		return out

	case *pb.StreamEvent_ContentDelta:
		out := s.closeThinking()
		if s.textBlock < 0 {
			s.textBlock = s.nextBlock
			s.nextBlock++
			out += anthEvent("content_block_start", map[string]interface{}{
				"type": "content_block_start", "index": s.textBlock,
				"content_block": map[string]interface{}{"type": "text", "text": ""},
			})
		}
		out += anthEvent("content_block_delta", map[string]interface{}{
			"type": "content_block_delta", "index": s.textBlock,
			"delta": map[string]interface{}{"type": "text_delta", "text": e.ContentDelta.Text},
		})
		return out

	case *pb.StreamEvent_ToolCallDelta:
		out := s.closeThinking()
		idx, ok := s.toolBlocks[e.ToolCallDelta.Id]
		if !ok {
			idx = s.nextBlock
			s.nextBlock++
			s.toolBlocks[e.ToolCallDelta.Id] = idx
		}
		if !ok {
			out += anthEvent("content_block_start", map[string]interface{}{
				"type": "content_block_start", "index": idx,
				"content_block": map[string]interface{}{
					"type": "tool_use", "id": e.ToolCallDelta.Id,
					"name": e.ToolCallDelta.Name, "input": map[string]interface{}{},
				},
			})
		}
		if e.ToolCallDelta.ArgumentsDelta != "" {
			out += anthEvent("content_block_delta", map[string]interface{}{
				"type": "content_block_delta", "index": idx,
				"delta": map[string]interface{}{
					"type": "input_json_delta", "partial_json": e.ToolCallDelta.ArgumentsDelta,
				},
			})
		}
		return out

	case *pb.StreamEvent_MessageFinish:
		s.stopReason = mapStopReason(e.MessageFinish.FinishReason)
		out := s.closeThinking()
		if s.textBlock >= 0 {
			out += anthEvent("content_block_stop", map[string]interface{}{
				"type": "content_block_stop", "index": s.textBlock,
			})
		}
		for _, idx := range sortedValues(s.toolBlocks) {
			out += anthEvent("content_block_stop", map[string]interface{}{
				"type": "content_block_stop", "index": idx,
			})
		}
		usage := anthUsage(e.MessageFinish.Usage)
		out += anthEvent("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": s.stopReason, "stop_sequence": stopSeqValue(e.MessageFinish)},
			"usage": usage,
		})
		out += anthEvent("message_stop", map[string]interface{}{"type": "message_stop"})
		return out
	}
	return ""
}

// mapStopReason 信封 finish_reason → Anthropic stop_reason。
func mapStopReason(reason string) string {
	switch reason {
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "refusal"
	case "stop_sequence":
		return "stop_sequence"
	default:
		return "end_turn"
	}
}

// stopSeqValue Anthropic stop_sequence 字段：命中序列时为原值，否则 null。
func stopSeqValue(fin *pb.MessageFinish) interface{} {
	if fin.FinishReason == "stop_sequence" && fin.StopSequence != "" {
		return fin.StopSequence
	}
	return nil
}

// openaiFinish 信封 finish_reason → OpenAI finish_reason（stop_sequence 折成 stop）。
func openaiFinish(reason string) string {
	if reason == "stop_sequence" {
		return "stop"
	}
	return reason
}

func anthEvent(eventType string, payload map[string]interface{}) string {
	payload["type"] = eventType
	b, _ := json.Marshal(payload)
	return "event: " + eventType + "\ndata: " + string(b) + "\n\n"
}

// ---------- 非流式聚合 ----------

// anthAggregate 聚合信封事件为完整 Message 响应对象。
type anthAggregate struct {
	model     string
	thinking  string
	signature string
	text      string
	tools     map[string]*aggrTool
	stop      string
	stopSeq   interface{} // 命中的停止序列原值（未命中 nil）
	usage     *pb.Usage
}

type aggrTool struct {
	id    string
	name  string
	input string
}

func (a *anthAggregate) feed(ev *pb.StreamEvent) {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		a.model = e.MessageStart.Model
	case *pb.StreamEvent_ReasoningDelta:
		a.thinking += e.ReasoningDelta.Text
		a.signature += e.ReasoningDelta.Signature
	case *pb.StreamEvent_ContentDelta:
		a.text += e.ContentDelta.Text
	case *pb.StreamEvent_ToolCallDelta:
		t, ok := a.tools[e.ToolCallDelta.Id]
		if !ok {
			if a.tools == nil {
				a.tools = map[string]*aggrTool{}
			}
			t = &aggrTool{id: e.ToolCallDelta.Id, name: e.ToolCallDelta.Name}
			a.tools[e.ToolCallDelta.Id] = t
		}
		t.input += e.ToolCallDelta.ArgumentsDelta
	case *pb.StreamEvent_MessageFinish:
		a.stop = mapStopReason(e.MessageFinish.FinishReason)
		a.stopSeq = stopSeqValue(e.MessageFinish)
		a.usage = e.MessageFinish.Usage
	}
}

// result 生成非流式 Message JSON（thinking 块在 text / tool_use 之前）。
func (a *anthAggregate) result() map[string]interface{} {
	var content []interface{}
	if a.thinking != "" || a.signature != "" {
		content = append(content, map[string]interface{}{
			"type": "thinking", "thinking": a.thinking, "signature": a.signature,
		})
	}
	if a.text != "" {
		content = append(content, map[string]interface{}{"type": "text", "text": a.text})
	}
	for _, id := range sortedKeys(a.tools) {
		t := a.tools[id]
		content = append(content, map[string]interface{}{
			"type": "tool_use", "id": t.id, "name": t.name,
			"input": json.RawMessage(t.input),
		})
	}
	if content == nil {
		content = []interface{}{}
	}
	return map[string]interface{}{
		"id": "msg_" + randHex(12), "type": "message", "role": "assistant",
		"model": a.model, "content": content, "stop_reason": a.stop, "stop_sequence": a.stopSeq,
		"usage": anthUsage(a.usage),
	}
}
