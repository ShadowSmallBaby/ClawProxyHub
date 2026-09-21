// Package anthropicup — Anthropic 兼容上游的通用适配：统一信封 ↔ messages 协议。
package anthropicup

import (
	"encoding/json"
	"strings"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// ChatBody 信封请求 → Anthropic Messages 请求体（stream=true）。
func ChatBody(req *pb.ChatRequest) map[string]interface{} {
	var system []interface{}
	var messages []map[string]interface{}
	// appendMsg 同角色相邻消息合并为一条（tool_result 后紧跟 user 文本是常态；
	// 官方 API 会自动合并，但兼容层/中转不一定，这里主动做）
	appendMsg := func(role string, blocks []interface{}) {
		if len(blocks) == 0 {
			return // 空消息上游拒绝
		}
		if n := len(messages); n > 0 && messages[n-1]["role"] == role {
			messages[n-1]["content"] = append(messages[n-1]["content"].([]interface{}), blocks...)
			return
		}
		messages = append(messages, map[string]interface{}{"role": role, "content": blocks})
	}
	for _, m := range req.Messages {
		switch {
		case m.Role == "system":
			system = append(system, blocksOf(m)...)
		case m.Role == "tool":
			// 工具结果以 tool_result 块包进 user 消息；带图片时 content 为 text/image 块数组，
			// 缓存断点从末块提到 tool_result 自身
			result := map[string]interface{}{"type": "tool_result", "tool_use_id": m.ToolCallId}
			if len(m.Parts) > 0 {
				blocks := blocksOf(m)
				if last, ok := blocks[len(blocks)-1].(map[string]interface{}); ok && last["cache_control"] != nil {
					result["cache_control"] = last["cache_control"]
					delete(last, "cache_control")
					if last["type"] == "text" && last["text"] == "" {
						blocks = blocks[:len(blocks)-1] // 只为承载断点补的空文本块
					}
				}
				result["content"] = blocks
			} else {
				result["content"] = m.Text
			}
			if m.ToolError {
				result["is_error"] = true
			}
			appendMsg("user", []interface{}{result})
		default:
			blocks := blocksOf(m)
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, map[string]interface{}{
					"type": "tool_use", "id": tc.Id, "name": tc.Name,
					"input": rawJSON(tc.Arguments),
				})
			}
			appendMsg(m.Role, blocks)
		}
	}
	body := map[string]interface{}{
		"model":      req.Model,
		"messages":   messages,
		"max_tokens": orInt(req.MaxTokens, 8192),
		"stream":     true,
	}
	if len(system) > 0 {
		body["system"] = system
	}
	if len(req.Tools) > 0 {
		var tools []interface{}
		for _, t := range req.Tools {
			tool := map[string]interface{}{
				"name": t.Name, "description": t.Description,
				"input_schema": rawJSON(orDefault(t.ParametersSchema, `{"type":"object"}`)),
			}
			if t.CacheControl != "" {
				tool["cache_control"] = rawJSON(t.CacheControl)
			}
			tools = append(tools, tool)
		}
		body["tools"] = tools
		if tc := req.ToolChoice; tc != nil {
			switch tc.Type {
			case "auto", "none":
				body["tool_choice"] = map[string]interface{}{"type": tc.Type}
			case "tool":
				if tc.ToolName == "" {
					body["tool_choice"] = map[string]interface{}{"type": "any"} // OpenAI 的 required
				} else {
					body["tool_choice"] = map[string]interface{}{"type": "tool", "name": tc.ToolName}
				}
			}
			if v := req.Extra["parallel_tool_calls"]; v == "false" {
				if tcm, ok := body["tool_choice"].(map[string]interface{}); ok {
					tcm["disable_parallel_tool_use"] = true
				}
			}
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
	if v, ok := req.Extra["top_k"]; ok && v != "" {
		body["top_k"] = jsonNumber(v)
	}
	if v, ok := req.Extra["stop"]; ok && v != "" {
		// 信封 stop 为 JSON（字符串或数组），Anthropic 只收数组
		var seqs []string
		if err := json.Unmarshal([]byte(v), &seqs); err != nil {
			var single string
			if json.Unmarshal([]byte(v), &single) == nil && single != "" {
				seqs = []string{single}
			}
		}
		if len(seqs) > 0 {
			body["stop_sequences"] = seqs
		}
	}
	if v, ok := req.Extra["thinking"]; ok && v != "" {
		body["thinking"] = rawJSON(v)
	} else if effort := req.Extra["reasoning_effort"]; effort != "" {
		// OpenAI 客户端的 reasoning_effort → 按 max_tokens 比例折算 budget
		if t := thinkingFromEffort(effort, body["max_tokens"].(int32)); t != nil {
			body["thinking"] = t
		}
	}
	if v := req.Extra["user"]; v != "" {
		body["metadata"] = map[string]interface{}{"user_id": v}
	}
	return body
}

// thinkingFromEffort reasoning_effort → Anthropic thinking 配置；budget 取 max_tokens 的比例，
// 下限 1024、必须小于 max_tokens；none / 未知值不开启。
func thinkingFromEffort(effort string, maxTokens int32) map[string]interface{} {
	var pct int32
	switch effort {
	case "minimal":
		pct = 5
	case "low":
		pct = 20
	case "medium":
		pct = 50
	case "high":
		pct = 80
	case "xhigh", "max":
		pct = 95
	default:
		return nil
	}
	budget := maxTokens * pct / 100
	if budget < 1024 {
		budget = 1024
	}
	if budget >= maxTokens {
		return nil // max_tokens 太小容不下最低 budget，放弃开启
	}
	return map[string]interface{}{"type": "enabled", "budget_tokens": budget}
}

// blocksOf 消息内容块：无 parts 用单个 text 块；有 parts 按序转换（保留 cache_control）。
// thinking 块只在带签名时回放（Anthropic 校验签名，无签名 / 他家签名一律 400），
// redacted_thinking 原样回传。
func blocksOf(m *pb.EnvelopeMessage) []interface{} {
	if len(m.Parts) == 0 {
		if m.Text == "" {
			return nil
		}
		return []interface{}{map[string]interface{}{"type": "text", "text": m.Text}}
	}
	var blocks []interface{}
	for _, p := range m.Parts {
		var block map[string]interface{}
		switch p.Type {
		case "text":
			if p.Text == "" && p.CacheControl == "" {
				continue
			}
			block = map[string]interface{}{"type": "text", "text": p.Text}
		case "image":
			var source map[string]interface{}
			if p.Url != "" {
				source = map[string]interface{}{"type": "url", "url": p.Url}
			} else {
				source = map[string]interface{}{"type": "base64", "media_type": p.MediaType, "data": p.Data}
			}
			block = map[string]interface{}{"type": "image", "source": source}
		case "thinking":
			if p.Signature != "" && m.Role == "assistant" {
				block = map[string]interface{}{"type": "thinking", "thinking": p.Text, "signature": p.Signature}
			}
		case "redacted_thinking":
			if p.Data != "" && m.Role == "assistant" {
				block = map[string]interface{}{"type": "redacted_thinking", "data": p.Data}
			}
		}
		if block == nil {
			continue
		}
		if p.CacheControl != "" {
			block["cache_control"] = rawJSON(p.CacheControl)
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// Parser 把上游 Anthropic SSE 行解析为信封事件。
type Parser struct {
	emit       func(*pb.StreamEvent)
	blocks     map[int]blockInfo // content block index → 身份
	usage      *pb.Usage         // message_start 打底、message_delta 累计覆盖
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

// anthUsage Anthropic usage 块（message_start.message.usage 与 message_delta.usage 同构）。
type anthUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

func (u *anthUsage) toEnvelope() *pb.Usage {
	return &pb.Usage{
		InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CachedTokens: u.CacheReadInputTokens, CacheCreationTokens: u.CacheCreationInputTokens,
	}
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
			Model string     `json:"model"`
			Usage *anthUsage `json:"usage"`
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
			Thinking    string `json:"thinking"`
			Signature   string `json:"signature"`
			StopReason  string `json:"stop_reason"`
			StopSeq     string `json:"stop_sequence"`
		} `json:"delta"`
		Usage *anthUsage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		start := &pb.MessageStart{Model: ev.Message.Model}
		if ev.Message.Usage != nil {
			p.usage = mergeUsage(p.usage, ev.Message.Usage.toEnvelope())
			start.Usage = ev.Message.Usage.toEnvelope() // 客户端 message_start 可先拿到输入/缓存计数
		}
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageStart{MessageStart: start}})
	case "content_block_start":
		p.blocks[ev.Index] = blockInfo{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{
				ContentDelta: &pb.ContentDelta{Text: ev.Delta.Text},
			}})
		case "thinking_delta":
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ReasoningDelta{
				ReasoningDelta: &pb.ReasoningDelta{Text: ev.Delta.Thinking},
			}})
		case "signature_delta":
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ReasoningDelta{
				ReasoningDelta: &pb.ReasoningDelta{Signature: ev.Delta.Signature},
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
		// stop_reason + 累计 usage（新版 API 在此重报 input/cache 字段）都在这里
		if ev.Usage != nil {
			p.usage = mergeUsage(p.usage, ev.Usage.toEnvelope())
		}
		if ev.Delta.StopReason != "" {
			p.finish(mapStop(ev.Delta.StopReason), ev.Delta.StopSeq)
		}
	case "message_stop":
		p.finish("stop", "")
	}
}

// Finish 流结束兜底。
func (p *Parser) Finish() {
	p.finish("stop", "")
}

// FinishWithError 流异常结束：发失败事件。
func (p *Parser) FinishWithError(code int32, message string) {
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{
		TaskFailed: &pb.TaskFailed{Error: &pb.Error{Code: code, Message: message}},
	}})
}

func (p *Parser) finish(reason, stopSeq string) {
	if p.sentFinish {
		return
	}
	p.sentFinish = true
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{
		MessageFinish: &pb.MessageFinish{FinishReason: reason, Usage: p.usage, StopSequence: stopSeq},
	}})
}

// mergeUsage 后到的非零字段覆盖，零值不擦除已有计数（message_start 的 input 不被
// message_delta 的省略字段抹掉）。
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
	return cur
}

// ---------- 工具 ----------

func rawJSON(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return map[string]interface{}{}
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

// mapStop Anthropic stop_reason → 信封 finish_reason。
// stop_sequence 保留原值：Anthropic 直通无损，其它协议出口自行折成 stop。
func mapStop(reason string) string {
	switch reason {
	case "tool_use":
		return "tool_calls"
	case "stop_sequence":
		return "stop_sequence"
	case "max_tokens", "pause_turn":
		// pause_turn 是服务端工具循环的可续状态，信封无对应值，按未完成处理
		return "length"
	case "refusal":
		return "content_filter"
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
