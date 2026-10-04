package gateway

import (
	"encoding/json"
	"fmt"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"strings"
)

// validateToolHistory 避免把缺少身份或非对象参数的工具历史转为不可回放的请求。
func validateToolHistory(req *pb.ChatRequest) error {
	for i, message := range req.Messages {
		if message.Role == "tool" && message.ToolCallId == "" {
			return fmt.Errorf("messages[%d]: tool result has no call identity", i)
		}
		seen := map[string]bool{}
		for _, tool := range message.ToolCalls {
			if tool.Id == "" || tool.Name == "" {
				return fmt.Errorf("messages[%d]: tool call requires id and name", i)
			}
			if seen[tool.Id] {
				return fmt.Errorf("messages[%d]: duplicate tool call %q", i, tool.Id)
			}
			seen[tool.Id] = true
			args := strings.TrimSpace(tool.Arguments)
			if !strings.HasPrefix(args, "{") || !json.Valid([]byte(args)) {
				return fmt.Errorf("tool call %q arguments must be a JSON object", tool.Id)
			}
		}
	}
	return nil
}

// validateRequestContent 在归一化前拒绝无法表达的内容，避免静默截去附件或托管工具。
func validateRequestContent(body []byte, protocol string) error {
	var req struct {
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Input json.RawMessage `json:"input"`
		Tools []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	if protocol == "anthropic" {
		if err := validateContent(req.System, protocol, "system", false); err != nil {
			return err
		}
		for i, tool := range req.Tools {
			if (tool.Type != "" && tool.Type != "custom") || tool.Name == "" {
				return fmt.Errorf("tools[%d]: unsupported hosted tool type or missing name: %s", i, tool.Type)
			}
		}
	}
	for i, message := range req.Messages {
		if err := validateContent(message.Content, protocol, fmt.Sprintf("messages[%d].content", i), protocol == "anthropic"); err != nil {
			return err
		}
	}
	if protocol == "responses" {
		var text string
		if json.Unmarshal(req.Input, &text) == nil {
			return nil
		}
		var items []struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
			Output  json.RawMessage `json:"output"`
		}
		if err := json.Unmarshal(req.Input, &items); err != nil {
			return fmt.Errorf("input must be string or message array")
		}
		for i, item := range items {
			value := item.Content
			if item.Type == "function_call_output" {
				value = item.Output
			}
			if err := validateContent(value, protocol, fmt.Sprintf("input[%d].content", i), false); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateContent(raw json.RawMessage, protocol, path string, tools bool) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return nil
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return fmt.Errorf("%s must be string or content array", path)
	}
	for i, block := range blocks {
		var kind string
		_ = json.Unmarshal(block["type"], &kind)
		at := fmt.Sprintf("%s[%d]", path, i)
		supported := false
		switch protocol {
		case "chat":
			supported = kind == "text" || kind == "image_url"
		case "responses":
			supported = kind == "text" || kind == "input_text" || kind == "output_text" || kind == "input_image" || kind == "refusal"
		case "anthropic":
			supported = kind == "text" || (tools && (kind == "image" || kind == "thinking" || kind == "redacted_thinking" || kind == "tool_use" || kind == "tool_result"))
		case "anthropic_result":
			supported = kind == "text" || kind == "image"
		}
		if !supported {
			return fmt.Errorf("%s: unsupported content type %q", at, kind)
		}
		switch kind {
		case "text", "input_text", "output_text":
			if json.Unmarshal(block["text"], &text) != nil || string(block["text"]) == "null" {
				return fmt.Errorf("%s.text must be a string", at)
			}
		case "refusal":
			if json.Unmarshal(block["refusal"], &text) != nil {
				return fmt.Errorf("%s.refusal must be a string", at)
			}
		case "tool_result":
			if err := validateContent(block["content"], "anthropic_result", at+".content", false); err != nil {
				return err
			}
		case "image_url":
			var image struct {
				URL    string `json:"url"`
				Detail string `json:"detail"`
			}
			if json.Unmarshal(block[kind], &image) != nil || strings.TrimSpace(image.URL) == "" {
				return fmt.Errorf("%s.image_url.url is required", at)
			}
			if image.Detail != "" && image.Detail != "auto" && image.Detail != "low" && image.Detail != "high" {
				return fmt.Errorf("%s: invalid image detail", at)
			}
		case "input_image":
			if json.Unmarshal(block["image_url"], &text) != nil || strings.TrimSpace(text) == "" {
				return fmt.Errorf("%s.image_url is required; file_id images are unsupported", at)
			}
			if detail, ok := block["detail"]; ok {
				var d string
				if json.Unmarshal(detail, &d) != nil || (d != "auto" && d != "low" && d != "high") {
					return fmt.Errorf("%s: invalid image detail", at)
				}
			}
		case "image":
			var source struct {
				Type, URL, Data string
				MediaType       string `json:"media_type"`
			}
			if json.Unmarshal(block["source"], &source) != nil || !((source.Type == "url" && source.URL != "") || (source.Type == "base64" && source.Data != "" && source.MediaType != "")) {
				return fmt.Errorf("%s: invalid image source", at)
			}
		}
	}
	return nil
}
