// Package openaiup — OpenAI 兼容上游的通用适配：统一信封 ↔ chat/completions。
// 大多数 Claw 类上游都讲 OpenAI 协议，插件作者复用本包即可只写差异部分。
package openaiup

import (
	"encoding/json"
	"strings"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// ChatBody 信封请求 → OpenAI chat/completions 请求体。
// 始终 stream=true 并带 include_usage：上游聚合与非聚合都由事件流表达。
func ChatBody(req *pb.ChatRequest) map[string]interface{} {
	var messages []map[string]interface{}
	for _, m := range req.Messages {
		msg := map[string]interface{}{"role": m.Role, "content": m.Text}
		if len(m.ToolCalls) > 0 {
			if m.Role == "assistant" {
				msg["content"] = nilIfEmpty(m.Text)
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
		messages = append(messages, msg)
	}
	body := map[string]interface{}{
		"model":          req.Model,
		"messages":       messages,
		"stream":         true,
		"stream_options": map[string]interface{}{"include_usage": true},
	}
	if len(req.Tools) > 0 {
		var tools []map[string]interface{}
		for _, t := range req.Tools {
			tools = append(tools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": t.Name, "description": t.Description,
					"parameters": rawJSON(t.ParametersSchema),
				},
			})
		}
		body["tools"] = tools
		if tc := req.ToolChoice; tc != nil {
			switch tc.Type {
			case "auto", "none":
				body["tool_choice"] = tc.Type
			case "tool":
				body["tool_choice"] = map[string]interface{}{
					"type": "function", "function": map[string]interface{}{"name": tc.ToolName},
				}
			}
		}
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if v, ok := req.Extra["top_p"]; ok && v != "" {
		body["top_p"] = jsonNumber(v)
	}
	if v, ok := req.Extra["stop"]; ok && v != "" {
		body["stop"] = rawJSON(v)
	}
	if v, ok := req.Extra["reasoning_effort"]; ok && v != "" {
		body["reasoning_effort"] = v
	}
	return body
}

// Parser 把上游 OpenAI SSE 行解析为信封事件。
// 用法：每读一行调 Feed；流结束时调 Finish 把挂起的 finish_reason 落地。
type Parser struct {
	emit        func(*pb.StreamEvent)
	pendingStop string
	toolSeen    map[int]bool // tool_calls index → 是否已发过 name
	sentFinish  bool
}

func NewParser(emit func(*pb.StreamEvent)) *Parser {
	return &Parser{emit: emit, toolSeen: map[int]bool{}}
}

// Feed 处理一行（"data: {...}" 或 "data: [DONE]"）。
func (p *Parser) Feed(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
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
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return
	}
	for _, c := range chunk.Choices {
		if c.Delta.Content != "" {
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
				ContentDelta: &pb.ContentDelta{Text: c.Delta.Content},
			}})
		}
		for _, tc := range c.Delta.ToolCalls {
			ev := &pb.ToolCallDelta{
				Id:             tc.ID,
				Name:           tc.Function.Name,
				ArgumentsDelta: tc.Function.Arguments,
			}
			if tc.ID == "" && p.toolSeen[tc.Index] {
				ev.Id = "" // 后续增量不带 id，避免信封侧误开新块
			}
			p.toolSeen[tc.Index] = true
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: ev}})
		}
		if c.FinishReason != nil && *c.FinishReason != "" {
			p.pendingStop = *c.FinishReason
			// usage 一般随终止块或其后的 usage 块到达；若已到则立即收尾
			if chunk.Usage != nil {
				p.finish(chunk.Usage)
			}
		}
	}
	if chunk.Usage != nil {
		p.finish(chunk.Usage)
	}
}

// Finish 流结束：把挂起的 finish_reason 落地（从未发过时补一个空 usage 的收尾）。
func (p *Parser) Finish() {
	if !p.sentFinish {
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
			MessageFinish: &pb.MessageFinish{FinishReason: orDefault(p.pendingStop, "stop")},
		}})
	}
}

// FinishWithError 流异常结束：发失败事件。
func (p *Parser) FinishWithError(code int32, message string) {
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{
		TaskFailed: &pb.TaskFailed{Error: &pb.Error{Code: code, Message: message}},
	}})
}

func (p *Parser) finish(u *struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}) {
	if p.sentFinish {
		return
	}
	p.sentFinish = true
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{
			FinishReason: orDefault(p.pendingStop, "stop"),
			Usage:        &pb.Usage{InputTokens: u.PromptTokens, OutputTokens: u.CompletionTokens},
		},
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

func jsonNumber(s string) interface{} {
	var v json.Number
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
