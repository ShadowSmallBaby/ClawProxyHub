// Package anthropicup — Anthropic 兼容上游的通用适配：统一信封 ↔ messages 协议。
package anthropicup

import (
	"encoding/json"
	"strings"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// ChatBody 信封请求 → Anthropic Messages 请求体（stream=true）。
func ChatBody(req *pb.ChatRequest) map[string]interface{} {
	var system string
	var messages []map[string]interface{}
	for _, m := range req.Messages {
		switch {
		case m.Role == "system":
			system += m.Text
		case m.Role == "tool":
			// 工具结果以 tool_result 块包进 user 消息
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": []interface{}{map[string]interface{}{
					"type": "tool_result", "tool_use_id": m.ToolCallId, "content": m.Text,
				}},
			})
		case m.Role == "assistant" && len(m.ToolCalls) > 0:
			var blocks []interface{}
			if m.Text != "" {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": m.Text})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, map[string]interface{}{
					"type": "tool_use", "id": tc.Id, "name": tc.Name,
					"input": rawJSON(tc.Arguments),
				})
			}
			messages = append(messages, map[string]interface{}{"role": "assistant", "content": blocks})
		default:
			messages = append(messages, map[string]interface{}{
				"role":    m.Role,
				"content": []interface{}{map[string]interface{}{"type": "text", "text": m.Text}},
			})
		}
	}
	body := map[string]interface{}{
		"model":      req.Model,
		"messages":   messages,
		"max_tokens": orInt(req.MaxTokens, 8192),
		"stream":     true,
	}
	if system != "" {
		body["system"] = system
	}
	if len(req.Tools) > 0 {
		var tools []interface{}
		for _, t := range req.Tools {
			tools = append(tools, map[string]interface{}{
				"name": t.Name, "description": t.Description,
				"input_schema": rawJSON(orDefault(t.ParametersSchema, `{"type":"object"}`)),
			})
		}
		body["tools"] = tools
		if tc := req.ToolChoice; tc != nil {
			switch tc.Type {
			case "auto":
				body["tool_choice"] = map[string]interface{}{"type": "auto"}
			case "none":
				body["tool_choice"] = map[string]interface{}{"type": "none"}
			case "tool":
				body["tool_choice"] = map[string]interface{}{"type": "tool", "name": tc.ToolName}
			}
		}
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	return body
}

// Parser 把上游 Anthropic SSE 行解析为信封事件。
type Parser struct {
	emit       func(*pb.StreamEvent)
	blocks     map[int]blockInfo // content block index → 身份
	nextToolID int
	pendingUse *pb.Usage
	sentFinish bool
}

type blockInfo struct {
	kind string // text / tool_use
	id   string
	name string
}

func NewParser(emit func(*pb.StreamEvent)) *Parser {
	return &Parser{emit: emit, blocks: map[int]blockInfo{}}
}

// Feed 处理一行（"event: xxx" 与 "data: {...}"）。
func (p *Parser) Feed(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" {
		return
	}
	var ev struct {
		Type    string `json:"type"`
		Index   int    `json:"index"`
		Message struct {
			Model string `json:"model"`
			Usage struct {
				InputTokens int64 `json:"input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		ContentBlock struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
			Text  string          `json:"text"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
			StopReason  string `json:"stop_reason"`
		} `json:"delta"`
		Usage struct {
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{
			MessageStart: &pb.MessageStart{Model: ev.Message.Model},
		}})
	case "content_block_start":
		p.blocks[ev.Index] = blockInfo{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
				ContentDelta: &pb.ContentDelta{Text: ev.Delta.Text},
			}})
		case "input_json_delta":
			info := p.blocks[ev.Index]
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
				ToolCallDelta: &pb.ToolCallDelta{
					Id: info.id, Name: info.name, ArgumentsDelta: ev.Delta.PartialJSON,
				},
			}})
		}
	case "message_delta":
		// stop_reason + output_tokens 通常都在这里
		if ev.Delta.StopReason != "" {
			p.finish(mapStop(ev.Delta.StopReason), ev.Usage.OutputTokens)
		}
	case "message_stop":
		p.finish("stop", 0)
	}
}

// Finish 流结束兜底。
func (p *Parser) Finish() {
	if !p.sentFinish {
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
			MessageFinish: &pb.MessageFinish{FinishReason: "stop"},
		}})
	}
}

// FinishWithError 流异常结束：发失败事件。
func (p *Parser) FinishWithError(code int32, message string) {
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{
		TaskFailed: &pb.TaskFailed{Error: &pb.Error{Code: code, Message: message}},
	}})
}

func (p *Parser) finish(reason string, outputTokens int64) {
	if p.sentFinish {
		return
	}
	p.sentFinish = true
	if reason == "tool_use" {
		// 信封语义用 tool_calls
		reason = "tool_calls"
	}
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{
			FinishReason: reason,
			Usage:        &pb.Usage{OutputTokens: outputTokens},
		},
	}})
}

// ---------- 工具 ----------

func rawJSON(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return map[string]interface{}{}
	}
	return v
}

func mapStop(reason string) string {
	switch reason {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return "stop"
	}
}

func orInt(v, def int32) int32 {
	if v > 0 {
		return v
	}
	return def
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
