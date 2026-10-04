package requestutil

import (
	"encoding/json"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// NativeFields 仅保留目标协议可直接消费的选项，不透传任意请求字段。
var NativeFields = map[string][]string{
	"chat":      {"metadata", "service_tier", "store", "prompt_cache_key", "prompt_cache_retention", "safety_identifier"},
	"responses": {"metadata", "service_tier", "store", "prompt_cache_key", "prompt_cache_retention", "safety_identifier", "truncation"},
	"anthropic": {"metadata", "service_tier", "cache_control"},
}

func CaptureNative(req *pb.ChatRequest, raw []byte, protocol string) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	if req.Extra == nil {
		req.Extra = map[string]string{}
	}
	for _, key := range NativeFields[protocol] {
		if v := fields[key]; len(v) > 0 && string(v) != "null" {
			req.Extra["native_"+protocol+"_"+key] = string(v)
		}
	}
}

func ApplyNative(req *pb.ChatRequest, body map[string]interface{}, protocol string) {
	for _, key := range NativeFields[protocol] {
		raw := req.Extra["native_"+protocol+"_"+key]
		if raw == "" {
			continue
		}
		var value interface{}
		// RawMessage 保留 false、0 和 JSON 数字精度。
		if json.Unmarshal([]byte(raw), &value) == nil {
			body[key] = json.RawMessage(raw)
		}
	}
}
