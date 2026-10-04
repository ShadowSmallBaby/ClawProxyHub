// Package responsesup 上游为 OpenAI Responses API 时的请求组装与 SSE 解析。
// 与 anthropicup / openaiup 并列：ChatBody 组请求体，NewParser 把上游事件流转成信封事件。
package responsesup

import (
	"encoding/json"
	"fmt"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/requestutil"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/streamutil"
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
			var output interface{} = m.Text
			if len(m.Parts) > 0 {
				output = inputParts(m)
			}
			input = append(input, map[string]interface{}{
				"type": "function_call_output", "call_id": m.ToolCallId, "output": output,
			})
		case "assistant":
			for _, part := range m.Parts {
				if part.Type == "responses_reasoning" {
					item := map[string]interface{}{"type": "reasoning", "summary": []map[string]interface{}{{"type": "summary_text", "text": part.Text}}}
					if part.Signature != "" {
						item["encrypted_content"] = part.Signature
					}
					input = append(input, item)
				}
			}

			var content []map[string]interface{}
			for _, part := range m.Parts {
				switch part.Type {
				case "text":
					item := map[string]interface{}{"type": "output_text", "text": part.Text}
					if part.Source == "responses" && part.Annotations != "" {
						item["annotations"] = json.RawMessage(part.Annotations)
					}
					content = append(content, item)
				case "refusal":
					content = append(content, map[string]interface{}{"type": "refusal", "refusal": part.Text})
				}
			}
			if len(content) == 0 && m.Text != "" {
				content = append(content, map[string]interface{}{"type": "output_text", "text": m.Text})
			}
			if len(content) > 0 {
				input = append(input, map[string]interface{}{"role": "assistant", "content": content})
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
			tool := map[string]interface{}{
				"type": "function", "name": t.Name, "description": t.Description,
				"parameters": rawJSON(orDefault(t.ParametersSchema, `{"type":"object"}`)), "strict": false,
			}
			requestutil.ApplyToolStrict(req, t.Name, tool)
			tools = append(tools, tool)
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
	if v := requestutil.ReasoningEffort(req.Extra); v != "" {
		body["reasoning"] = map[string]interface{}{"effort": v}
	}
	if v := req.Extra["parallel_tool_calls"]; v != "" {
		body["parallel_tool_calls"] = rawJSON(v)
	}
	if v := req.Extra["responses_reasoning"]; v != "" {
		body["reasoning"] = rawJSON(v)
	}
	if v := requestutil.ResponsesText(req.Extra); v != nil {
		body["text"] = v
	}
	if v := req.Extra["user"]; v != "" {
		body["user"] = v
	}
	requestutil.ApplyNative(req, body, "responses")
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
			image := map[string]interface{}{"type": "input_image", "image_url": url}
			if p.ImageDetail != "" {
				image["detail"] = p.ImageDetail
			}
			items = append(items, image)
		}
	}
	return items
}

// Parser 把上游 Responses SSE 行解析为信封事件（data 自带 type，不依赖 event 行）。
// 用法：每读一行调 Feed；流结束时调 Finish 兜底收尾。
type Parser struct {
	emit        func(*pb.StreamEvent)
	usage       *pb.Usage
	sawTool     bool
	toolIDs     map[string]string
	callItems   map[string]string // call_id → item_id，防止不同输出项混入同一次调用
	argSeen     map[string]bool   // item_id → 已收到 arguments 增量（done 时不再补发全量）
	toolArgs    map[string]string
	toolNames   map[string]string
	text        map[string]string
	signatures  map[string]string
	stop        string
	annotations streamutil.AnnotationSet
	sentFinish  bool
}

func NewParser(emit func(*pb.StreamEvent)) *Parser {
	return &Parser{emit: emit, argSeen: map[string]bool{}, toolIDs: map[string]string{}, callItems: map[string]string{}, toolArgs: map[string]string{}, toolNames: map[string]string{}, text: map[string]string{}, signatures: map[string]string{}}
}

// event 上游事件的公共字段（按 type 取用）。
type event struct {
	Code         string            `json:"code"`
	Message      string            `json:"message"`
	Status       int32             `json:"status"`
	Type         string            `json:"type"`
	Delta        string            `json:"delta"`
	ItemID       string            `json:"item_id"`
	Arguments    string            `json:"arguments"`
	Text         string            `json:"text"`
	Refusal      string            `json:"refusal"`
	Annotation   json.RawMessage   `json:"annotation"`
	Annotations  []json.RawMessage `json:"annotations"`
	ContentIndex int               `json:"content_index"`
	SummaryIndex int               `json:"summary_index"`
	Item         responseItem      `json:"item"`
	Response     struct {
		Output            []responseItem `json:"output"`
		Usage             *responseUsage `json:"usage"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Error responseError `json:"error"`
	} `json:"response"`
	Error responseError `json:"error"`
}

type responsePart struct {
	Type        string            `json:"type"`
	Text        string            `json:"text"`
	Refusal     string            `json:"refusal"`
	Annotations []json.RawMessage `json:"annotations"`
}

type responseItem struct {
	ID               string         `json:"id"`
	Type             string         `json:"type"`
	CallID           string         `json:"call_id"`
	Name             string         `json:"name"`
	Arguments        string         `json:"arguments"`
	EncryptedContent string         `json:"encrypted_content"`
	Content          []responsePart `json:"content"`
	Summary          []responsePart `json:"summary"`
}

type responseError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int32  `json:"status"`
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
	if p.sentFinish {
		return
	}
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
		p.FinishWithError(502, "invalid upstream JSON")
		return
	}
	switch ev.Type {
	case "response.refusal.delta":
		p.emitText(ev.ItemID, ev.ContentIndex, "refusal", ev.Delta, false)
	case "response.output_text.delta":
		p.emitText(ev.ItemID, ev.ContentIndex, "text", ev.Delta, false)
	case "response.output_text.annotation.added":
		p.emitAnnotations(ev.ItemID, ev.ContentIndex, []json.RawMessage{ev.Annotation})
	case "response.output_text.done":
		p.emitText(ev.ItemID, ev.ContentIndex, "text", ev.Text, true)
		p.emitAnnotations(ev.ItemID, ev.ContentIndex, ev.Annotations)
	case "response.refusal.done":
		p.emitText(ev.ItemID, ev.ContentIndex, "refusal", ev.Refusal, true)
	case "response.reasoning_summary_text.delta":
		p.emitText(ev.ItemID, ev.SummaryIndex, "reasoning_summary", ev.Delta, false)
	case "response.reasoning_summary_text.done":
		p.emitText(ev.ItemID, ev.SummaryIndex, "reasoning_summary", ev.Text, true)
	case "response.reasoning_text.delta":
		p.emitText(ev.ItemID, ev.ContentIndex, "reasoning", ev.Delta, false)
	case "response.reasoning_text.done":
		p.emitText(ev.ItemID, ev.ContentIndex, "reasoning", ev.Text, true)
	case "response.output_item.added":
		if ev.Item.Type == "function_call" {
			p.registerTool(ev.Item)
			if ev.Item.Arguments != "" {
				p.toolArgsDone(ev.Item.ID, ev.Item.Arguments)
			}
		}
	case "response.function_call_arguments.delta":
		if p.toolIDs[ev.ItemID] == "" {
			p.FinishWithError(502, "tool continuation has no call identity")
			return
		}
		p.argSeen[ev.ItemID] = true
		p.toolArgs[ev.ItemID] += ev.Delta
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
			ToolCallDelta: &pb.ToolCallDelta{Id: p.toolIDs[ev.ItemID], ArgumentsDelta: ev.Delta},
		}})
	case "response.function_call_arguments.done":
		p.toolArgsDone(ev.ItemID, ev.Arguments)
	case "response.output_item.done":
		p.completeItem(ev.Item)
	case "response.completed", "response.incomplete":
		for _, item := range ev.Response.Output {
			p.completeItem(item)
			if p.sentFinish {
				return
			}
		}
		if u := ev.Response.Usage; u != nil {
			p.usage = u.toEnvelope()
		}
		if ev.Type == "response.incomplete" {
			switch ev.Response.IncompleteDetails.Reason {
			case "max_output_tokens":
				p.stop = "length"
			case "content_filter":
				p.stop = "content_filter"
			default:
				p.FinishWithError(502, "upstream response incomplete: "+ev.Response.IncompleteDetails.Reason)
				return
			}
		}
		p.finish()
	case "response.failed":
		e := ev.Response.Error
		p.FinishWithError(streamutil.ErrorCode(orDefault(e.Code, e.Type), e.Status), orDefault(e.Message, "upstream response failed"))
	case "error":
		e := ev.Error
		if ev.Message != "" || ev.Code != "" {
			e = responseError{Code: ev.Code, Message: ev.Message, Status: ev.Status}
		}
		p.FinishWithError(streamutil.ErrorCode(orDefault(e.Code, e.Type), e.Status), orDefault(e.Message, "upstream error"))
	}
}

// toolArgsDone 上游只在 done 给全量参数时补发一次（已走增量则跳过）。
func (p *Parser) toolArgsDone(itemID, args string) {
	if p.sentFinish {
		return
	}
	if p.toolIDs[itemID] == "" {
		p.FinishWithError(502, "tool completion has no call identity")
		return
	}
	if args == "" && p.argSeen[itemID] {
		return
	}
	if args == "" {
		args = "{}"
	}
	previous := p.toolArgs[itemID]
	if !strings.HasPrefix(args, previous) {
		p.FinishWithError(502, "conflicting final tool arguments")
		return
	}
	p.toolArgs[itemID] = args
	p.argSeen[itemID] = true
	args = strings.TrimPrefix(args, previous)
	if args == "" {
		return
	}
	p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{
		ToolCallDelta: &pb.ToolCallDelta{Id: p.toolIDs[itemID], ArgumentsDelta: args},
	}})
}

// emitText 按内容块合并增量与快照，仅补发尚未收到的后缀。
func (p *Parser) emitText(id string, index int, kind, value string, complete bool) {
	if p.sentFinish {
		return
	}
	key := fmt.Sprintf("%s/%d/%s", id, index, kind)
	previous := p.text[key]
	if complete {
		if !strings.HasPrefix(value, previous) {
			p.FinishWithError(502, "conflicting final response content")
			return
		}
		value = strings.TrimPrefix(value, previous)
	}
	p.text[key] += value
	if kind == "refusal" {
		p.stop = "content_filter"
	}
	if value == "" {
		return
	}
	if kind == "reasoning" || kind == "reasoning_summary" {
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ReasoningDelta{ReasoningDelta: &pb.ReasoningDelta{Text: value}}})
	} else {
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Text: value, Refusal: kind == "refusal", Source: "responses", BlockId: fmt.Sprintf("%s/%d/%s", id, index, kind)}}})
	}
}

func (p *Parser) registerTool(item responseItem) {
	if p.sentFinish {
		return
	}
	if item.ID == "" || item.CallID == "" {
		p.FinishWithError(502, "tool item has no call identity")
		return
	}
	oldID, oldName := p.toolIDs[item.ID], p.toolNames[item.ID]
	if otherItem := p.callItems[item.CallID]; otherItem != "" && otherItem != item.ID {
		p.FinishWithError(502, "duplicate tool call identity")
		return
	}
	if (oldID != "" && oldID != item.CallID) || (oldName != "" && item.Name != "" && oldName != item.Name) {
		p.FinishWithError(502, "conflicting tool identity")
		return
	}
	p.sawTool = true
	p.toolIDs[item.ID] = item.CallID
	p.callItems[item.CallID] = item.ID
	if oldID == "" || (oldName == "" && item.Name != "") {
		p.toolNames[item.ID] = item.Name
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{Id: item.CallID, Name: item.Name}}})
	}
}

func (p *Parser) completeItem(item responseItem) {
	if p.sentFinish {
		return
	}
	switch item.Type {
	case "function_call":
		p.registerTool(item)
		p.toolArgsDone(item.ID, item.Arguments)
	case "message":
		for i, part := range item.Content {
			switch part.Type {
			case "output_text":
				p.emitText(item.ID, i, "text", part.Text, true)
				p.emitAnnotations(item.ID, i, part.Annotations)
			case "refusal":
				p.emitText(item.ID, i, "refusal", part.Refusal, true)
			default:
				p.FinishWithError(502, "unsupported response content: "+part.Type)
			}
		}
	case "reasoning":
		for i, part := range item.Summary {
			p.emitText(item.ID, i, "reasoning_summary", part.Text, true)
		}
		for i, part := range item.Content {
			if part.Type != "reasoning_text" {
				p.FinishWithError(502, "unsupported reasoning content: "+part.Type)
				return
			}
			p.emitText(item.ID, i, "reasoning", part.Text, true)
		}
		if item.EncryptedContent != "" && p.signatures[item.ID] != item.EncryptedContent && !p.sentFinish {
			p.signatures[item.ID] = item.EncryptedContent
			p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ReasoningDelta{ReasoningDelta: &pb.ReasoningDelta{Signature: streamutil.ResponsesSignature(item.EncryptedContent)}}})
		}
	default:
		p.FinishWithError(502, "unsupported response item: "+item.Type)
	}
}

// Finish 流结束：缺少终止事件时报告失败。
func (p *Parser) Finish() {
	if !p.sentFinish {
		p.FinishWithError(502, "upstream ended before a terminal event")
	}
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
	for itemID := range p.toolIDs {
		if strings.TrimSpace(p.toolNames[itemID]) == "" {
			p.FinishWithError(502, "completed tool call has no name")
			return
		}
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

func (p *Parser) emitAnnotations(id string, index int, values []json.RawMessage) {
	if p.sentFinish {
		return
	}
	key := fmt.Sprintf("%s/%d/text", id, index)
	if raw := p.annotations.Add(key, values); raw != "" {
		p.emit(&pb.StreamEvent{Event: &pb.StreamEvent_ContentDelta{ContentDelta: &pb.ContentDelta{Annotations: raw, Source: "responses", BlockId: key}}})
	}
}
