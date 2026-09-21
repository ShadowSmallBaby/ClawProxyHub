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
		Model             string          `json:"model"`
		Instructions      string          `json:"instructions"`
		Input             json.RawMessage `json:"input"`
		Tools             []respTool      `json:"tools"`
		ToolChoice        json.RawMessage `json:"tool_choice"`
		MaxOutputTokens   int32           `json:"max_output_tokens"`
		Temperature       *float64        `json:"temperature"`
		TopP              *float64        `json:"top_p"`
		Stream            bool            `json:"stream"`
		Reasoning         json.RawMessage `json:"reasoning"`
		ParallelToolCalls json.RawMessage `json:"parallel_tool_calls"`
		User              string          `json:"user"`
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
	setTemperature(req, raw.Temperature)
	if raw.TopP != nil {
		req.Extra["top_p"] = fmt.Sprintf("%g", *raw.TopP)
	}
	if len(raw.ParallelToolCalls) > 0 && string(raw.ParallelToolCalls) != "null" {
		req.Extra["parallel_tool_calls"] = string(raw.ParallelToolCalls)
	}
	if raw.User != "" {
		req.Extra["user"] = raw.User
	}
	// 不透传 reasoning.effort：Codex 的 "xhigh" 等私有值上游不认，会 500。
	// 对齐原项目——Responses 无顶层 reasoning_effort，该字段直接丢弃。
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
			// function_call_output（工具结果：string 或 input_text/input_image 数组）
			Output json.RawMessage `json:"output"`
			// reasoning（assistant 历史里的推理项，Codex 会原样回传）
			Summary []struct {
				Text string `json:"text"`
			} `json:"summary"`
			EncryptedContent string `json:"encrypted_content"`
		}
		if err := json.Unmarshal(raw.Input, &items); err != nil {
			return nil, fmt.Errorf("input must be string or message array")
		}
		// 合并相邻 assistant message / reasoning / function_call：并行调用须归入同一条 assistant
		// 的 tool_calls，否则 tool 消息与声明它的 assistant 错位，上游拒绝。
		var pendParts []*pb.ContentPart
		var pendAssistant bool
		var pendTools []*pb.ToolCall
		flushAssistant := func() {
			if !pendAssistant && len(pendTools) == 0 {
				return
			}
			req.Messages = append(req.Messages, &pb.EnvelopeMessage{
				Role: "assistant", Text: partsText(pendParts), Parts: finishParts(pendParts), ToolCalls: pendTools,
			})
			pendParts, pendAssistant, pendTools = nil, false, nil
		}
		for _, it := range items {
			switch it.Type {
			case "message", "":
				role := normalizeRole(it.Role)
				parts := responsesParts(it.Content)
				if role == "assistant" {
					pendParts = append(pendParts, parts...)
					pendAssistant = true
					continue
				}
				flushAssistant()
				req.Messages = append(req.Messages, &pb.EnvelopeMessage{
					Role: role, Text: partsText(parts), Parts: finishParts(parts),
				})
			case "reasoning":
				// 推理摘要作 thinking 块归入 assistant；encrypted_content 是 OpenAI 私有签名，
				// 存到 signature 供 openai 系上游回放（anthropic 上游会因签名不匹配而丢弃）
				var texts []string
				for _, s := range it.Summary {
					texts = append(texts, s.Text)
				}
				pendParts = append(pendParts, &pb.ContentPart{
					Type: "thinking", Text: joinTexts(texts), Signature: it.EncryptedContent,
				})
				pendAssistant = true
			case "function_call":
				args := it.Arguments
				if args == "" {
					args = "{}" // 上游要求 arguments 是合法 JSON 文本
				}
				pendTools = append(pendTools, &pb.ToolCall{Id: it.CallID, Name: it.Name, Arguments: args})
			case "function_call_output":
				flushAssistant()
				parts := responsesParts(it.Output)
				req.Messages = append(req.Messages, &pb.EnvelopeMessage{
					Role: "tool", Text: partsText(parts), Parts: finishParts(parts), ToolCallId: it.CallID,
				})
			}
		}
		flushAssistant()
	}

	for _, t := range raw.Tools {
		if t.Type == "function" && t.Name != "" {
			req.Tools = append(req.Tools, &pb.ToolDefinition{
				Name: t.Name, Description: t.Description,
				ParametersSchema: string(t.Parameters),
			})
		}
	}
	if tc, err := convertResponsesToolChoice(raw.ToolChoice); err == nil && tc != nil {
		req.ToolChoice = tc
	}
	return req, nil
}

// convertResponsesToolChoice Responses tool_choice → 信封 ToolChoice。
// 字符串：auto / none / required；对象：{"type":"function","name":"x"}。
func convertResponsesToolChoice(raw json.RawMessage) (*pb.ToolChoice, error) {
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
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
	}
	if tc.Type == "function" {
		return &pb.ToolChoice{Type: "tool", ToolName: tc.Name}, nil
	}
	return &pb.ToolChoice{Type: tc.Type}, nil
}

type respTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// responsesParts Responses message content（string 或 items）→ 内容块：
// input_text / output_text / input_image（image_url 为字符串）。
func responsesParts(raw json.RawMessage) []*pb.ContentPart {
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
		ImageURL string `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	var parts []*pb.ContentPart
	for _, it := range items {
		switch it.Type {
		case "input_text", "output_text", "text":
			parts = append(parts, &pb.ContentPart{Type: "text", Text: it.Text})
		case "input_image":
			if it.ImageURL != "" {
				parts = append(parts, imagePart(it.ImageURL))
			}
		}
	}
	return parts
}

// ---------- 信封事件 → Responses SSE ----------

type responsesSSEState struct {
	model      string
	respID     string
	reasonItem string // reasoning output_item 的 item_id；空未开
	reasonIdx  int
	reasoning  string // 累计推理文本，供 summary_text.done 与 completed.output 回填
	textItem   string // 文本 output_item 的 item_id；空未开
	textIdx    int    // 文本 item 的 output_index
	text       string // 累计正文，供 output_text.done 与 completed.output 回填
	nextItem   int    // 下一个 output item 的序号（同时用作 item_id 与 output_index）
	fnOrder    []string
	fnCalls    map[string]*respFnCall // tool call id → 累积中的函数调用
	curFn      *respFnCall            // 当前打开的调用：后续增量 id 为空时归入它
	usage      *pb.Usage
}

// respFnCall 累积中的 function_call output item（name/args 分块到齐后于 finish 补发 done）。
type respFnCall struct {
	itemID string
	callID string
	name   string
	args   string
	idx    int // output_index
}

func newResponsesSSEState(model string) *responsesSSEState {
	return &responsesSSEState{
		model: model, respID: "resp_" + randHex(16),
		fnCalls: map[string]*respFnCall{},
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

	case *pb.StreamEvent_ReasoningDelta:
		// Codex 读 reasoning_summary_text.delta 展示思考过程；签名不透
		if e.ReasoningDelta.Text == "" {
			return ""
		}
		var out string
		if s.reasonItem == "" {
			s.reasonItem = fmt.Sprintf("rs_%d", s.nextItem)
			s.reasonIdx = s.nextItem
			s.nextItem++
			out += respEvent("response.output_item.added", map[string]interface{}{
				"output_index": s.reasonIdx, "item": map[string]interface{}{
					"type": "reasoning", "id": s.reasonItem, "status": "in_progress", "summary": []interface{}{},
				},
			})
		}
		s.reasoning += e.ReasoningDelta.Text
		out += respEvent("response.reasoning_summary_text.delta", map[string]interface{}{
			"item_id": s.reasonItem, "output_index": s.reasonIdx, "summary_index": 0,
			"delta": e.ReasoningDelta.Text,
		})
		return out

	case *pb.StreamEvent_ContentDelta:
		var out string
		if s.textItem == "" {
			s.textItem = fmt.Sprintf("item_%d", s.nextItem)
			s.textIdx = s.nextItem
			s.nextItem++
			out += respEvent("response.output_item.added", map[string]interface{}{
				"output_index": s.textIdx, "item": map[string]interface{}{
					"type": "message", "id": s.textItem, "role": "assistant", "status": "in_progress",
					"content": []interface{}{map[string]interface{}{"type": "output_text", "text": ""}},
				},
			})
		}
		s.text += e.ContentDelta.Text
		out += respEvent("response.output_text.delta", map[string]interface{}{
			"item_id": s.textItem, "output_index": s.textIdx, "content_index": 0,
			"delta": e.ContentDelta.Text,
		})
		return out

	case *pb.StreamEvent_ToolCallDelta:
		var out string
		// 续块空 id 归入当前打开的调用，否则参数流进无名孤儿 item。
		id := e.ToolCallDelta.Id
		fc := s.curFn
		if id != "" {
			existing, ok := s.fnCalls[id]
			if !ok {
				existing = &respFnCall{
					itemID: fmt.Sprintf("item_%d", s.nextItem), callID: id,
					name: e.ToolCallDelta.Name, idx: s.nextItem,
				}
				s.nextItem++
				s.fnCalls[id] = existing
				s.fnOrder = append(s.fnOrder, id)
				out += respEvent("response.output_item.added", map[string]interface{}{
					"output_index": existing.idx, "item": map[string]interface{}{
						"type": "function_call", "id": existing.itemID, "call_id": existing.callID,
						"name": existing.name, "arguments": "", "status": "in_progress",
					},
				})
			}
			fc = existing
			s.curFn = fc
		} else if fc == nil {
			// 兜底：首块就没 id，合成 key 避免丢事件
			key := fmt.Sprintf("__anon_%d", s.nextItem)
			fc = &respFnCall{itemID: fmt.Sprintf("item_%d", s.nextItem), callID: key, idx: s.nextItem}
			s.nextItem++
			s.fnCalls[key] = fc
			s.fnOrder = append(s.fnOrder, key)
			out += respEvent("response.output_item.added", map[string]interface{}{
				"output_index": fc.idx, "item": map[string]interface{}{
					"type": "function_call", "id": fc.itemID, "call_id": fc.callID,
					"name": fc.name, "arguments": "", "status": "in_progress",
				},
			})
			s.curFn = fc
		}
		if fc.name == "" && e.ToolCallDelta.Name != "" {
			fc.name = e.ToolCallDelta.Name // name 可能晚于首个 delta 到达
		}
		if e.ToolCallDelta.ArgumentsDelta != "" {
			fc.args += e.ToolCallDelta.ArgumentsDelta
			out += respEvent("response.function_call_arguments.delta", map[string]interface{}{
				"item_id": fc.itemID, "output_index": fc.idx,
				"delta": e.ToolCallDelta.ArgumentsDelta,
			})
		}
		return out

	case *pb.StreamEvent_MessageFinish:
		s.usage = e.MessageFinish.Usage
		var out string
		var output []interface{}
		// 推理 item 收尾：summary_text.done + output_item.done（summary 带完整文本）。
		if s.reasonItem != "" {
			out += respEvent("response.reasoning_summary_text.done", map[string]interface{}{
				"item_id": s.reasonItem, "output_index": s.reasonIdx, "summary_index": 0, "text": s.reasoning,
			})
			item := map[string]interface{}{
				"type": "reasoning", "id": s.reasonItem, "status": "completed",
				"summary": []interface{}{map[string]interface{}{"type": "summary_text", "text": s.reasoning}},
			}
			out += respEvent("response.output_item.done", map[string]interface{}{
				"output_index": s.reasonIdx, "item": item,
			})
			output = append(output, item)
		}
		// 文本 item 收尾：output_text.done 带完整文本，output_item.done 带完整 content。
		if s.textItem != "" {
			out += respEvent("response.output_text.done", map[string]interface{}{
				"item_id": s.textItem, "output_index": s.textIdx, "content_index": 0, "text": s.text,
			})
			item := map[string]interface{}{
				"type": "message", "id": s.textItem, "role": "assistant", "status": "completed",
				"content": []interface{}{map[string]interface{}{"type": "output_text", "text": s.text}},
			}
			out += respEvent("response.output_item.done", map[string]interface{}{
				"output_index": s.textIdx, "item": item,
			})
			output = append(output, item)
		}
		// 工具调用 item 收尾：补 arguments.done 与 output_item.done，否则 Codex 不执行。
		for _, id := range s.fnOrder {
			fc := s.fnCalls[id]
			out += respEvent("response.function_call_arguments.done", map[string]interface{}{
				"item_id": fc.itemID, "output_index": fc.idx, "arguments": fc.args,
			})
			item := map[string]interface{}{
				"type": "function_call", "id": fc.itemID, "call_id": fc.callID,
				"name": fc.name, "arguments": fc.args, "status": "completed",
			}
			out += respEvent("response.output_item.done", map[string]interface{}{
				"output_index": fc.idx, "item": item,
			})
			output = append(output, item)
		}
		if output == nil {
			output = []interface{}{}
		}
		// usage 为必填字段，缺失时补零值（Codex 严格反序列化，否则断流）。
		// response.completed 必须带 output：Codex 从这里读最终 message + function_call。
		out += respEvent("response.completed", map[string]interface{}{
			"response": map[string]interface{}{
				"id": s.respID, "object": "response", "model": s.model,
				"status": "completed", "output": output, "usage": responsesUsage(e.MessageFinish.Usage),
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
	model     string
	reasoning string
	text      string
	tools     map[string]*aggrTool
	finish    string
	usage     *pb.Usage
}

func (a *responsesAggregate) feed(ev *pb.StreamEvent) {
	switch e := ev.Event.(type) {
	case *pb.StreamEvent_MessageStart:
		a.model = e.MessageStart.Model
	case *pb.StreamEvent_ReasoningDelta:
		a.reasoning += e.ReasoningDelta.Text
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
		a.finish = openaiFinish(e.MessageFinish.FinishReason)
		a.usage = e.MessageFinish.Usage
	}
}

func (a *responsesAggregate) result() map[string]interface{} {
	var output []interface{}
	if a.reasoning != "" {
		output = append(output, map[string]interface{}{
			"type": "reasoning", "id": "rs_0", "status": "completed",
			"summary": []interface{}{map[string]interface{}{"type": "summary_text", "text": a.reasoning}},
		})
	}
	if a.text != "" {
		output = append(output, map[string]interface{}{
			"type": "message", "id": "item_1", "role": "assistant", "status": "completed",
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
	if output == nil {
		output = []interface{}{}
	}
	return map[string]interface{}{
		"id": "resp_" + randHex(16), "object": "response", "model": a.model,
		"status": "completed", "output": output,
		"usage": responsesUsage(a.usage),
	}
}
