package gateway

import (
	"encoding/json"
	"fmt"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// outputContent 保留块身份，确保引用偏移仍指向原块，跨协议不套用不兼容的注解。
type outputContent struct {
	parts []*outputPart
	keys  map[string]int
}

type outputPart struct {
	text        string
	refusal     bool
	annotations []json.RawMessage
}

func (c *outputContent) add(d *pb.ContentDelta, protocol string) (int, bool) {
	if d.Text == "" && !d.Refusal && (d.Source != protocol || d.Annotations == "") {
		return -1, false
	}
	if c.keys == nil {
		c.keys = map[string]int{}
	}
	key := fmt.Sprintf("%s/%s/%t", d.Source, d.BlockId, d.Refusal)
	index, exists := c.keys[key]
	if !exists {
		index = len(c.parts)
		c.keys[key] = index
		c.parts = append(c.parts, &outputPart{refusal: d.Refusal})
	}
	p := c.parts[index]
	p.text += d.Text
	if d.Source == protocol && d.Annotations != "" {
		var values []json.RawMessage
		if json.Unmarshal([]byte(d.Annotations), &values) == nil {
			p.annotations = append(p.annotations, values...)
		}
	}
	return index, !exists
}

func (p *outputPart) response() map[string]interface{} {
	if p.refusal {
		return map[string]interface{}{"type": "refusal", "refusal": p.text}
	}
	annotations := p.annotations
	if annotations == nil {
		annotations = []json.RawMessage{}
	}
	return map[string]interface{}{"type": "output_text", "text": p.text, "annotations": annotations}
}
func (c *outputContent) responses() []interface{} {
	parts := make([]interface{}, 0, len(c.parts))
	for _, p := range c.parts {
		parts = append(parts, p.response())
	}
	return parts
}
func (c *outputContent) anthropic() []interface{} {
	parts := make([]interface{}, 0, len(c.parts))
	for _, p := range c.parts {
		part := map[string]interface{}{"type": "text", "text": p.text}
		if len(p.annotations) > 0 {
			part["citations"] = p.annotations
		}
		parts = append(parts, part)
	}
	return parts
}
