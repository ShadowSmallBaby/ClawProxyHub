// Package requestutil 保留统一信封未建模的协议选项，供网关与上游适配器共享。
package requestutil

import (
	"encoding/json"
	"strings"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// SetToolStrict 通过 extra 保留显式 strict，兼容现有插件 RPC 协议。
func SetToolStrict(req *pb.ChatRequest, name string, strict *bool) {
	if strict == nil {
		return
	}
	if req.Extra == nil {
		req.Extra = map[string]string{}
	}
	flags := map[string]bool{}
	_ = json.Unmarshal([]byte(req.Extra["tool_strict"]), &flags)
	if flags == nil {
		flags = map[string]bool{}
	}
	flags[name] = *strict
	b, _ := json.Marshal(flags)
	req.Extra["tool_strict"] = string(b)
}

// ApplyToolStrict 不覆盖各目标协议对省略 strict 的默认处理。
func ApplyToolStrict(req *pb.ChatRequest, name string, tool map[string]interface{}) {
	var flags map[string]bool
	if json.Unmarshal([]byte(req.Extra["tool_strict"]), &flags) == nil {
		if value, ok := flags[name]; ok {
			tool["strict"] = value
		}
	}
}

// ChatResponseFormat 将 Responses 的扁平 format 转回 Chat 的 json_schema 包装。
func ChatResponseFormat(extra map[string]string) interface{} {
	if v := object(extra["response_format"]); v != nil {
		return v
	}
	text := object(extra["responses_text"])
	format, ok := text["format"].(map[string]interface{})
	if !ok {
		return nil
	}
	if format["type"] != "json_schema" {
		return format
	}
	schema := make(map[string]interface{}, len(format))
	for key, value := range format {
		if key != "type" {
			schema[key] = value
		}
	}
	return map[string]interface{}{"type": "json_schema", "json_schema": schema}
}

// ResponsesText 将 Chat 的结构化输出映射到 Responses，保留 false 与 schema 原值。
func ResponsesText(extra map[string]string) interface{} {
	if v := object(extra["responses_text"]); v != nil {
		return v
	}
	format := object(extra["response_format"])
	if format == nil {
		return nil
	}
	if format["type"] == "json_schema" {
		if schema, ok := format["json_schema"].(map[string]interface{}); ok {
			delete(format, "json_schema")
			for key, value := range schema {
				if key != "type" {
					format[key] = value
				}
			}
		}
	}
	return map[string]interface{}{"format": format}
}

// ReasoningEffort 统一解析显式 effort、Responses reasoning 与 Anthropic thinking 配置。
func ReasoningEffort(extra map[string]string) string {
	if effort := extra["reasoning_effort"]; effort != "" {
		return effort
	}
	effort, _ := object(extra["responses_reasoning"])["effort"].(string)
	if effort != "" {
		return effort
	}
	// thinking 的开关优先于同源 output_config 的强度，避免关闭推理后被重新启用。
	if object(extra["thinking"])["type"] == "disabled" {
		return "none"
	}
	if effort, _ = object(extra["anthropic_output_config"])["effort"].(string); effort != "" {
		return effort
	}
	var thinking struct {
		Type   string `json:"type"`
		Budget int    `json:"budget_tokens"`
	}
	if json.Unmarshal([]byte(extra["thinking"]), &thinking) != nil {
		return ""
	}
	switch thinking.Type {
	case "disabled":
		return "none"
	case "adaptive":
		return "high"
	case "enabled":
		switch {
		case thinking.Budget <= 0:
			return "high"
		case thinking.Budget <= 1024:
			return "low"
		case thinking.Budget <= 8192:
			return "medium"
		default:
			return "high"
		}
	}
	return ""
}

func object(raw string) map[string]interface{} {
	var value map[string]interface{}
	// schema 内的大整数不能经 float64 解码后失真。
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return nil
	}
	return value
}
