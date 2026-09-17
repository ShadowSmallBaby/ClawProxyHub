// responses.go — OpenAI Responses 协议（Codex CLI）↔ 统一信封。
package gateway

import (
	"encoding/json"
	"fmt"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// parseResponsesRequest 把 /v1/responses 请求体转成统一信封。
func parseResponsesRequest(body []byte) (*pb.ChatRequest, error) {
	var raw struct {
		Model           string          `json:"model"`
		Instructions    string          `json:"instructions"`
		Input           json.RawMessage `json:"input"`
		Tools           []respTool      `json:"tools"`
		ToolChoice      json.RawMessage `json:"tool_choice"`
		MaxOutputTokens int32           `json:"max_output_tokens"`
		Temperature     *float64        `json:"temperature"`
		TopP            *float64        `json:"top_p"`
		Stream          bool            `json:"stream"`
		Reasoning       json.RawMessage `json:"reasoning"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	req := &pb.ChatRequest{
		Model:       raw.Model,
		Stream:      raw.Stream,
		MaxTokens:   raw.MaxOutputTokens,
		Temperature: deref(raw.Temperature),
		Extra:       map[string]string{},
	}
	if raw.TopP != nil {
		req.Extra["top_p"] = fmt.Sprintf("%g", *raw.TopP)
	}
	if len(raw.Reasoning) > 0 {
		req.Extra["reasoning"] = string(raw.Reasoning)
	}
	if raw.Instructions != "" {
		req.Messages = append(req.Messages, &pb.EnvelopeMessage{Role: "system", Text: raw.Instructions})
	}

	// input 可能是纯字符串，也可能是消息数组
	var inputText string
	if err := json.Unmarshal(raw.Input, &inputText); err == nil && inputText != "" {
		req.Messages = append(req.Messages, &pb.EnvelopeMessage{Role: "user", Text: inputText})
	} else {
		var items []struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
			// function_call（assistant 历史里的工具调用）
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			// function_call_output（工具结果）
			Output string `json:"output"`
		}
		if err := json.Unmarshal(raw.Input, &items); err != nil {
			return nil, fmt.Errorf("input must be string or message array")
		}
		for _, it := range items {
			switch it.Type {
			case "message", "":
				req.Messages = append(req.Messages, &pb.EnvelopeMessage{
					Role: it.Role, Text: extractText(it.Content),
				})
			case "function_call":
				req.Messages = append(req.Messages, &pb.EnvelopeMessage{
					Role: "assistant",
					ToolCalls: []*pb.ToolCall{{
						Id: it.CallID, Name: it.Name, Arguments: it.Arguments,
					}},
				})
			case "function_call_output":
				req.Messages = append(req.Messages, &pb.EnvelopeMessage{
					Role: "tool", Text: it.Output, ToolCallId: it.CallID,
				})
			}
		}
	}

	for _, t := range raw.Tools {
		if t.Type == "function" && t.Name != "" {
			req.Tools = append(req.Tools, &pb.ToolDefinition{
				Name: t.Name, Description: t.Description,
				ParametersSchema: string(t.Parameters),
			})
		}
	}
	if len(raw.ToolChoice) > 0 {
		var s string
		if err := json.Unmarshal(raw.ToolChoice, &s); err == nil {
			req.ToolChoice = &pb.ToolChoice{Type: s}
		}
	}
	return req, nil
}

type respTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ---------- 信封事件 → Responses SSE ----------

type responsesSSEState struct {
	model    string
	respID   string
	textItem string // 文本 output_item 的 item_id；空未开
	nextItem int
	fnItems  map[string]string // tool call id → item_id
	usage    *pb.Usage
}

func newResponsesSSEState(model string) *responsesSSEState {
	return &responsesSSEState{
		model: model, respID: "resp_" + randHex(16),
		textItem: "", fnItems: map[string]string{},
	}
}

func (s *responsesSSEState) convertEvent(ev *pb.StreamEvent) string {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		return respEvent("response.created", map[string]interface{}{
			"response": map[string]interface{}{
				"id": s.respID, "object": "response", "model": e.MessageStart.Model,
				"status": "in_progress", "output": []interface{}{},
			},
		})

	case *pb.StreamEvent_ContentDelta:
		var out string
		if s.textItem == "" {
			s.textItem = fmt.Sprintf("item_%d", s.nextItem)
			s.nextItem++
			out += respEvent("response.output_item.added", map[string]interface{}{
				"output_index": 0, "item": map[string]interface{}{
					"type": "message", "id": s.textItem, "role": "assistant", "status": "in_progress",
					"content": []interface{}{map[string]interface{}{"type": "output_text", "text": ""}},
				},
			})
		}
		out += respEvent("response.output_text.delta", map[string]interface{}{
			"item_id": s.textItem, "output_index": 0, "content_index": 0,
			"delta": e.ContentDelta.Text,
		})
		return out

	case *pb.StreamEvent_ToolCallDelta:
		itemID, ok := s.fnItems[e.ToolCallDelta.Id]
		var out string
		if !ok {
			itemID = fmt.Sprintf("item_%d", s.nextItem)
			s.nextItem++
			s.fnItems[e.ToolCallDelta.Id] = itemID
			out += respEvent("response.output_item.added", map[string]interface{}{
				"output_index": len(s.fnItems), "item": map[string]interface{}{
					"type": "function_call", "id": itemID, "call_id": e.ToolCallDelta.Id,
					"name": e.ToolCallDelta.Name, "arguments": "", "status": "in_progress",
				},
			})
		}
		if e.ToolCallDelta.ArgumentsDelta != "" {
			out += respEvent("response.function_call_arguments.delta", map[string]interface{}{
				"item_id": itemID, "output_index": len(s.fnItems),
				"delta": e.ToolCallDelta.ArgumentsDelta,
			})
		}
		return out

	case *pb.StreamEvent_MessageFinish:
		s.usage = e.MessageFinish.Usage
		var out string
		if s.textItem != "" {
			out += respEvent("response.output_text.done", map[string]interface{}{
				"item_id": s.textItem, "output_index": 0, "content_index": 0, "text": "",
			})
			out += respEvent("response.output_item.done", map[string]interface{}{
				"output_index": 0, "item": map[string]interface{}{
					"type": "message", "id": s.textItem, "role": "assistant", "status": "completed",
				},
			})
		}
		usage := map[string]interface{}{}
		if e.MessageFinish.Usage != nil {
			usage = map[string]interface{}{
				"input_tokens":  e.MessageFinish.Usage.InputTokens,
				"output_tokens": e.MessageFinish.Usage.OutputTokens,
				"total_tokens":  e.MessageFinish.Usage.InputTokens + e.MessageFinish.Usage.OutputTokens,
			}
		}
		out += respEvent("response.completed", map[string]interface{}{
			"response": map[string]interface{}{
				"id": s.respID, "object": "response", "model": s.model,
				"status": "completed", "usage": usage,
			},
		})
		return out
	}
	return ""
}

func (s *responsesSSEState) finish() string { return "" }

func respEvent(eventType string, payload map[string]interface{}) string {
	payload["type"] = eventType
	b, _ := json.Marshal(payload)
	return "event: " + eventType + "\ndata: " + string(b) + "\n\n"
}

// responsesAggregate Responses 非流式聚合。
type responsesAggregate struct {
	model  string
	text   string
	tools  map[string]*aggrTool
	finish string
	input  int64
	output int64
}

func (a *responsesAggregate) feed(ev *pb.StreamEvent) {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		a.model = e.MessageStart.Model
	case *pb.StreamEvent_ContentDelta:
		a.text += e.ContentDelta.Text
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
		a.finish = e.MessageFinish.FinishReason
		if e.MessageFinish.Usage != nil {
			a.input, a.output = e.MessageFinish.Usage.InputTokens, e.MessageFinish.Usage.OutputTokens
		}
	}
}

func (a *responsesAggregate) result() map[string]interface{} {
	var output []interface{}
	if a.text != "" {
		output = append(output, map[string]interface{}{
			"type": "message", "id": "item_0", "role": "assistant", "status": "completed",
			"content": []interface{}{map[string]interface{}{"type": "output_text", "text": a.text}},
		})
	}
	for _, id := range sortedKeys(a.tools) {
		t := a.tools[id]
		output = append(output, map[string]interface{}{
			"type": "function_call", "call_id": t.id, "name": t.name,
			"arguments": t.input, "status": "completed",
		})
	}
	return map[string]interface{}{
		"id": "resp_" + randHex(16), "object": "response", "model": a.model,
		"status": "completed", "output": output,
		"usage": map[string]interface{}{
			"input_tokens": a.input, "output_tokens": a.output,
			"total_tokens": a.input + a.output,
		},
	}
}
