// util.go — gateway 内部共用的小工具。
package gateway

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func deref(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// normalizeRole 把 developer 角色归一为 system（上游多不认 developer）。
func normalizeRole(role string) string {
	if role == "developer" {
		return "system"
	}
	return role
}

// extractText 从 string / blocks 数组 / nil 里提取文本拼接。
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var texts []string
		for _, b := range blocks {
			if b.Type == "text" || b.Type == "input_text" || b.Type == "output_text" {
				texts = append(texts, b.Text)
			}
		}
		return joinTexts(texts)
	}
	return ""
}

func isArray(raw json.RawMessage) bool {
	for _, c := range raw {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == '['
	}
	return false
}

// compactJSON 压掉无关空白，保证 Arguments 是规范 JSON 文本。
func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

func joinTexts(texts []string) string {
	out := ""
	for i, t := range texts {
		if i > 0 {
			out += "\n"
		}
		out += t
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedValues[V int](m map[string]V) []V {
	vals := make([]V, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
	return vals
}

// ---------- 内容块（图片 / 推理） ----------

// imagePart 由 URL 构造图片块：data URL 拆成 media_type + base64，其余保留远程地址。
func imagePart(url string) *pb.ContentPart {
	if strings.HasPrefix(url, "data:") {
		meta, data, ok := strings.Cut(url[len("data:"):], ",")
		if ok {
			return &pb.ContentPart{
				Type: "image", MediaType: strings.TrimSuffix(meta, ";base64"), Data: data,
			}
		}
	}
	return &pb.ContentPart{Type: "image", Url: url}
}

// finishParts 决定信封是否保留 parts：全是无缓存断点的文本块则丢弃（以 text 为准），
// 否则保留完整有序内容。
func finishParts(parts []*pb.ContentPart) []*pb.ContentPart {
	for _, p := range parts {
		if p.Type != "text" || p.CacheControl != "" {
			return parts
		}
	}
	return nil
}

// setTemperature 显式给出的 temperature 记进 Extra（含 0），插件据此区分"未设置"与"设为 0"。
func setTemperature(req *pb.ChatRequest, t *float64) {
	if t != nil {
		req.Extra["temperature"] = strconv.FormatFloat(*t, 'g', -1, 64)
	}
}

// partsText 拼接内容块里的文本。
func partsText(parts []*pb.ContentPart) string {
	var texts []string
	for _, p := range parts {
		if p.Type == "text" {
			texts = append(texts, p.Text)
		}
	}
	return joinTexts(texts)
}
