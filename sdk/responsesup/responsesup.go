// Package responsesup 上游为 OpenAI Responses API 时的请求组装与 SSE 解析。
// 与 anthropicup / openaiup 并列：ChatBody 组请求体，NewParser 把上游事件流转成信封事件。
package responsesup

import (
	"encoding/json"
	"strings"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// ChatBody 信封请求 → Responses 请求体：system 归入 instructions，工具往返展开为
// function_call / function_call_output 项，图片走 input_image。
func ChatBody(req *pb.ChatRequest) map[string]interface{} {
	var instructions []string
	var input []map[string]interface{}
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			instructions = append(instructions, m.Text)
		case "tool":
			input = append(input, map[string]interface{}{
				"type": "function_call_output", "call_id": m.ToolCallId, "output": m.Text,
			})
		case "assistant":
			if m.Text != "" {
				input = append(input, map[string]interface{}{
					"role":    "assistant",
					"content": []map[string]interface{}{{"type": "output_text", "text": m.Text}},
				})
			}
			for _, tc := range m.ToolCalls {
				input = append(input, map[string]interface{}{
					"type": "function_call", "call_id": tc.Id, "name": tc.Name, "arguments": tc.Arguments,
				})
			}
		default:
			input = append(input, map[string]interface{}{"role": "user", "content": inputParts(m)})
		}
	}
	body := map[string]interface{}{"model": req.Model, "input": input, "stream": true}
	if len(instructions) > 0 {
		body["instructions"] = strings.Join(instructions, "\n\n")
	}
	if len(req.Tools) > 0 {
		var tools []interface{}
		for _, t := range req.Tools {
			tools = append(tools, map[string]interface{}{
				"type": "function", "name": t.Name, "description": t.Description,
				"parameters": rawJSON(orDefault(t.ParametersSchema, `{"type":"object"}`)), "strict": false,
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
					body["tool_choice"] = map[string]interface{}{"type": "function", "name": tc.ToolName}
				}
			}
		}
	}
	if req.MaxTokens > 0 {
		body["max_output_tokens"] = req.MaxTokens
	}
	if v := req.Extra["temperature"]; v != "" {
		body["temperature"] = rawJSON(v) // 显式给出（含 0）
	} else if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if v := req.Extra["top_p"]; v != "" {
		body["top_p"] = rawJSON(v)
	}
	if v := req.Extra["reasoning_effort"]; v != "" {
		body["reasoning"] = map[string]interface{}{"effort": v}
	}
	if v := req.Extra["parallel_tool_calls"]; v != "" {
		body["parallel_tool_calls"] = rawJSON(v)
	}
	if v := req.Extra["user"]; v != "" {
		body["user"] = v
	}
	return body
}

// inputParts user 消息内容：text → input_text，image → input_image（base64 重组为 data URL）。
func inputParts(m *pb.EnvelopeMessage) []map[string]interface{} {
	if len(m.Parts) == 0 {
		return []map[string]interface{}{{"type": "input_text", "text": m.Text}}
	}
	var items []map[string]interface{}
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			items = append(items, map[string]interface{}{"type": "input_text", "text": p.Text})
		case "image":
			url := p.Url
			if url == "" {
				url = "data:" + p.MediaType + ";base64," + p.Data
			}
			items = append(items, map[string]interface{}{"type": "input_image", "image_url": url})
		}
	}
	return items
}

// Parser 把上游 Responses SSE 行解析为信封事件（data 自带 type，不依赖 event 行）。
// 用法：每读一行调 Feed；流结束时调 Finish 兜底收尾。
type Parser struct {
	emit       func(*pb.StreamEvent)
	usage      *pb.Usage
	sawTool    bool
	argSeen    map[string]bool // item_id → 已收到 arguments 增量（done 时不再补发全量）
	stop       string
	sentFinish bool
}

func NewParser(emit func(*pb.StreamEvent)) *Parser {
	return &Parser{emit: emit, argSeen: map[string]bool{}}
}

// event 上游事件的公共字段（按 type 取用）。
type event struct {
	Type      string `json:"type"`
	Delta     string `json:"delta"`
	ItemID    string `json:"item_id"`
	Arguments string `json:"arguments"`
	Item      struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
	Response struct {
		Usage             *responseUsage `json:"usage"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// responseUsage Responses 的 usage：input_tokens 含缓存读；缓存与推理明细在 *_details。
type responseUsage struct {
	InputTokens        int64 `json:"input_tokens"`
	OutputTokens       int64 `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// toEnvelope Responses 语义 → 信封（Anthropic）语义：input 剥离缓存读，不为负。
func (u *responseUsage) toEnvelope() *pb.Usage {
	input := u.InputTokens - u.InputTokensDetails.CachedTokens
	if input < 0 {
		input = 0
	}
	return &pb.Usage{
		InputTokens: input, OutputTokens: u.OutputTokens,
		CachedTokens:    u.InputTokensDetails.CachedTokens,
		ReasoningTokens: u.OutputTokensDetails.ReasoningTokens,
	}
}

// Feed 处理一行（"data: {...}"）。
func (p *Parser) Feed(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}
	var ev event
	if json.Unmarshal([]byte(payload), &ev) != nil {
		return
	}
	switch ev.Type {
	case "response.output_text.delta":
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
			ContentDelta: &pb.ContentDelta{Text: ev.Delta},
		}})
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ReasoningDelta{
			ReasoningDelta: &pb.ReasoningDelta{Text: ev.Delta},
		}})
	case "response.output_item.added":
		if ev.Item.Type == "function_call" {
			p.sawTool = true
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
				ToolCallDelta: &pb.ToolCallDelta{Id: ev.Item.CallID, Name: ev.Item.Name},
			}})
		}
	case "response.function_call_arguments.delta":
		p.argSeen[ev.ItemID] = true
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
			ToolCallDelta: &pb.ToolCallDelta{ArgumentsDelta: ev.Delta}, // 后续增量不带 id，避免信封侧误开新块
		}})
	case "response.function_call_arguments.done":
		p.toolArgsDone(ev.ItemID, ev.Arguments)
	case "response.output_item.done":
		if ev.Item.Type == "function_call" {
			p.toolArgsDone(ev.Item.ID, ev.Item.Arguments)
		}
	case "response.completed", "response.incomplete":
		if u := ev.Response.Usage; u != nil {
			p.usage = u.toEnvelope()
		}
		if ev.Type == "response.incomplete" && ev.Response.IncompleteDetails.Reason == "max_output_tokens" {
			p.stop = "length"
		}
		p.finish()
	case "response.failed":
		p.FinishWithError(502, orDefault(ev.Response.Error.Message, "upstream response failed"))
	case "error":
		p.FinishWithError(502, orDefault(ev.Error.Message, "upstream error"))
	}
}

// toolArgsDone 上游只在 done 给全量参数时补发一次（已走增量则跳过）。
func (p *Parser) toolArgsDone(itemID, args string) {
	if p.argSeen[itemID] || args == "" {
		return
	}
	p.argSeen[itemID] = true
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
		ToolCallDelta: &pb.ToolCallDelta{ArgumentsDelta: args},
	}})
}

// Finish 流结束：未收到 completed 时按已有状态收尾。
func (p *Parser) Finish() {
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
	p.sentFinish = true
	reason := p.stop
	if reason == "" {
		reason = "stop"
		if p.sawTool {
			reason = "tool_calls"
		}
	}
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{FinishReason: reason, Usage: p.usage},
	}})
}

// ---------- 工具 ----------

func rawJSON(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
