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
		StopSequences []string        `json:"stop_sequences"`
		Tools         []anthTool      `json:"tools"`
		ToolChoice    json.RawMessage `json:"tool_choice"`
		Stream        bool            `json:"stream"`
		Thinking      json.RawMessage `json:"thinking"`
		Metadata      json.RawMessage `json:"metadata"`
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

	// system 可能是 string 或 blocks
	if sys := extractText(raw.System); sys != "" {
		req.Messages = append(req.Messages, &pb.EnvelopeMessage{Role: "system", Text: sys})
	}

	for i := range raw.Messages {
		req.Messages = append(req.Messages, convertAnthMessage(&raw.Messages[i])...)
	}

	for _, t := range raw.Tools {
		req.Tools = append(req.Tools, &pb.ToolDefinition{
			Name:             t.Name,
			Description:      t.Description,
			ParametersSchema: string(t.InputSchema),
		})
	}
	if tc, err := convertAnthToolChoice(raw.ToolChoice); err == nil && tc != nil {
		req.ToolChoice = tc
	}

	if raw.TopP != nil {
		req.Extra["top_p"] = fmt.Sprintf("%g", *raw.TopP)
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

	return req, nil
}

type anthMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// convertAnthMessage 单条 Anthropic 消息 → 一到多条信封消息
// （tool_result 块会拆出独立的 role=tool 消息）。
func convertAnthMessage(m *anthMessage) []*pb.EnvelopeMessage {
	// 纯文本 content
	if text := extractText(m.Content); text != "" && !isArray(m.Content) {
		return []*pb.EnvelopeMessage{{Role: m.Role, Text: text, Raw: m.Content}}
	}

	var out []*pb.EnvelopeMessage
	var assistantToolCalls []*pb.ToolCall
	var blockTexts []string

	var blocks []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return []*pb.EnvelopeMessage{{Role: m.Role, Raw: m.Content}}
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			blockTexts = append(blockTexts, b.Text)
		case "tool_use":
			assistantToolCalls = append(assistantToolCalls, &pb.ToolCall{
				Id: b.ID, Name: b.Name, Arguments: compactJSON(b.Input),
			})
		case "tool_result":
			out = append(out, &pb.EnvelopeMessage{
				Role: "tool", Text: extractText(b.Content), ToolCallId: b.ToolUseID,
			})
		}
	}

	if m.Role == "assistant" {
		out = append(out, &pb.EnvelopeMessage{
			Role: "assistant", Text: joinTexts(blockTexts), ToolCalls: assistantToolCalls,
			Raw: m.Content,
		})
	} else if len(blockTexts) > 0 {
		out = append(out, &pb.EnvelopeMessage{
			Role: m.Role, Text: joinTexts(blockTexts), Raw: m.Content,
		})
	}
	return out
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
	model        string
	msgID        string
	textBlock    int // 当前文本块 index；-1 未开
	nextBlock    int
	toolBlocks   map[string]int // tool_call id → block index
	stopReason   string
	inputTokens  int64
	outputTokens int64
}

func newAnthSSEState(model string) *anthSSEState {
	return &anthSSEState{
		model: model, msgID: "msg_" + randHex(12),
		textBlock: -1, toolBlocks: map[string]int{},
	}
}

// startMessage 返回 message_start 事件（流开始时发一次）。
func (s *anthSSEState) startMessage() string {
	return anthEvent("message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id": s.msgID, "type": "message", "role": "assistant", "model": s.model,
			"content": []interface{}{}, "stop_reason": nil, "usage": map[string]interface{}{
				"input_tokens": s.inputTokens, "output_tokens": 0,
			},
		},
	})
}

// convertEvent 把信封事件转成 Anthropic SSE 行（可能多行，\n 分隔）。
// 流结束时额外返回 message_stop 尾部。
func (s *anthSSEState) convertEvent(ev *pb.StreamEvent) string {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		s.model = e.MessageStart.Model
		return s.startMessage()

	case *pb.StreamEvent_ContentDelta:
		var out string
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
		idx, ok := s.toolBlocks[e.ToolCallDelta.Id]
		if !ok {
			idx = s.nextBlock
			s.nextBlock++
			s.toolBlocks[e.ToolCallDelta.Id] = idx
		}
		var out string
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
		var out string
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
		usage := map[string]interface{}{"input_tokens": s.inputTokens}
		if e.MessageFinish.Usage != nil {
			s.outputTokens = e.MessageFinish.Usage.OutputTokens
			usage = map[string]interface{}{
				"input_tokens":  e.MessageFinish.Usage.InputTokens,
				"output_tokens": e.MessageFinish.Usage.OutputTokens,
			}
		}
		out += anthEvent("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": s.stopReason},
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
	default:
		return "end_turn"
	}
}

func anthEvent(eventType string, payload map[string]interface{}) string {
	payload["type"] = eventType
	b, _ := json.Marshal(payload)
	return "event: " + eventType + "\ndata: " + string(b) + "\n\n"
}

// ---------- 非流式聚合 ----------

// anthAggregate 聚合信封事件为完整 Message 响应对象。
type anthAggregate struct {
	model   string
	text    string
	tools   map[string]*aggrTool
	stop    string
	input   int64
	output  int64
	started bool
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
		if e.MessageFinish.Usage != nil {
			a.input, a.output = e.MessageFinish.Usage.InputTokens, e.MessageFinish.Usage.OutputTokens
		}
	}
}

// result 生成非流式 Message JSON。
func (a *anthAggregate) result() map[string]interface{} {
	var content []interface{}
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
	return map[string]interface{}{
		"id": "msg_" + randHex(12), "type": "message", "role": "assistant",
		"model": a.model, "content": content, "stop_reason": a.stop,
		"usage": map[string]interface{}{
			"input_tokens": a.input, "output_tokens": a.output,
		},
	}
}
