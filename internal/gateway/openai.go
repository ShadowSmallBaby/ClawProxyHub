// openai.go — OpenAI Chat Completions 协议 ↔ 统一信封。
package gateway

import (
	"encoding/json"
	"fmt"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// parseChatCompletions 把 /v1/chat/completions 请求体转成统一信封。
func parseChatCompletions(body []byte) (*pb.ChatRequest, error) {
	var raw struct {
		Model       string          `json:"model"`
		Messages    []openaiMessage `json:"messages"`
		MaxTokens   int32           `json:"max_tokens"`
		Temperature *float64        `json:"temperature"`
		TopP        *float64        `json:"top_p"`
		Stop        json.RawMessage `json:"stop"`
		Tools       []openaiTool    `json:"tools"`
		ToolChoice  json.RawMessage `json:"tool_choice"`
		Stream      bool            `json:"stream"`
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
	if raw.TopP != nil {
		req.Extra["top_p"] = fmt.Sprintf("%g", *raw.TopP)
	}
	if len(raw.Stop) > 0 {
		req.Extra["stop"] = string(raw.Stop)
	}

	for i := range raw.Messages {
		m := &raw.Messages[i]
		em := &pb.EnvelopeMessage{Role: normalizeRole(m.Role), Text: extractText(m.Content), Raw: m.Content}
		for _, tc := range m.ToolCalls {
			em.ToolCalls = append(em.ToolCalls, &pb.ToolCall{
				Id: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
			})
		}
		if m.ToolCallID != "" {
			em.ToolCallId = m.ToolCallID
		}
		req.Messages = append(req.Messages, em)
	}

	for _, t := range raw.Tools {
		if t.Function.Name != "" {
			req.Tools = append(req.Tools, &pb.ToolDefinition{
				Name: t.Function.Name, Description: t.Function.Description,
				ParametersSchema: string(t.Function.Parameters),
			})
		}
	}
	if tc, err := convertOpenAIToolChoice(raw.ToolChoice); err == nil && tc != nil {
		req.ToolChoice = tc
	}
	return req, nil
}

type openaiMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
	ToolCallID string `json:"tool_call_id"`
}

type openaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

func convertOpenAIToolChoice(raw json.RawMessage) (*pb.ToolChoice, error) {
	if len(raw) == 0 {
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
	if tc.Type == "function" {
		return &pb.ToolChoice{Type: "tool", ToolName: tc.Function.Name}, nil
	}
	return &pb.ToolChoice{Type: tc.Type}, nil
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
	return &openaiSSEState{id: "chatcmpl-" + randHex(12), toolIdx: map[string]int{}}
}

// convertEvent 信封事件 → OpenAI chat.completion.chunk SSE 行。
func (s *openaiSSEState) convertEvent(ev *pb.StreamEvent) string {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		s.model = e.MessageStart.Model
		return ""

	case *pb.StreamEvent_ContentDelta:
		return s.chunk(map[string]interface{}{
			"role": "assistant", "content": e.ContentDelta.Text,
		}, "")

	case *pb.StreamEvent_ToolCallDelta:
		idx, ok := s.toolIdx[e.ToolCallDelta.Id]
		if !ok {
			idx = s.nextIdx
			s.nextIdx++
			s.toolIdx[e.ToolCallDelta.Id] = idx
		}
		fn := map[string]interface{}{"arguments": e.ToolCallDelta.ArgumentsDelta}
		if !ok {
			fn["name"] = e.ToolCallDelta.Name
		}
		return s.chunk(map[string]interface{}{
			"tool_calls": []interface{}{map[string]interface{}{
				"index": idx, "id": e.ToolCallDelta.Id, "type": "function", "function": fn,
			}},
		}, "")

	case *pb.StreamEvent_MessageFinish:
		out := s.chunk(map[string]interface{}{}, e.MessageFinish.FinishReason)
		// 带 usage 的终止块（stream_options.include_usage 的客户端靠它收用量）
		if e.MessageFinish.Usage != nil {
			out += s.chunkRaw(map[string]interface{}{
				"id": s.id, "object": "chat.completion.chunk", "created": s.created,
				"model": s.model, "choices": []interface{}{},
				"usage": map[string]interface{}{
					"prompt_tokens":     e.MessageFinish.Usage.InputTokens,
					"completion_tokens": e.MessageFinish.Usage.OutputTokens,
					"total_tokens":      e.MessageFinish.Usage.InputTokens + e.MessageFinish.Usage.OutputTokens,
				},
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
	model  string
	text   string
	tools  map[string]*aggrTool
	finish string
	input  int64
	output int64
}

func (a *openaiAggregate) feed(ev *pb.StreamEvent) {
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

func (a *openaiAggregate) result() map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant", "content": a.text}
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
		"created": 0, "model": a.model, "choices": choices,
		"usage": map[string]interface{}{
			"prompt_tokens": a.input, "completion_tokens": a.output,
			"total_tokens": a.input + a.output,
		},
	}
}
