// schema.go — 宿主与扩展共用的有界 JSON 结构验证。
package extension

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

const MaxJSONBytes = 2 << 20

// Schema 是明确支持的 JSON Schema 子集；对象拒绝未知字段。
type Schema struct {
	Type                 string            `json:"type"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	Required             []string          `json:"required,omitempty"`
	AdditionalProperties bool              `json:"additionalProperties"`
	Items                *Schema           `json:"items,omitempty"`
	MaxLength            int               `json:"maxLength,omitempty"`
	MaxItems             int               `json:"maxItems,omitempty"`
	Enum                 []string          `json:"enum,omitempty"`
}

func (s Schema) Check() error {
	nodes := 0
	if err := s.check(0, &nodes); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > 64<<10 {
		return errors.New("action schema too large")
	}
	return nil
}

func (s Schema) check(depth int, nodes *int) error {
	*nodes++
	if depth > 16 || *nodes > 2048 || s.MaxLength < 0 || s.MaxItems < 0 {
		return errors.New("action schema exceeds structural limits")
	}
	switch s.Type {
	case "object":
		for _, v := range s.Properties {
			if err := v.check(depth+1, nodes); err != nil {
				return err
			}
		}
		for _, name := range s.Required {
			if _, ok := s.Properties[name]; !ok {
				return fmt.Errorf("unknown required property %s", name)
			}
		}
	case "array":
		if s.Items == nil {
			return errors.New("missing item schema")
		}
		return s.Items.check(depth+1, nodes)
	case "string", "boolean", "number", "integer":
	default:
		return fmt.Errorf("unsupported schema type %s", s.Type)
	}
	return nil
}
func (s Schema) validate(v any) error {
	invalid := func() error { return fmt.Errorf("value must satisfy %s schema", s.Type) }
	switch s.Type {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return invalid()
		}
		for _, k := range s.Required {
			if _, ok = obj[k]; !ok {
				return fmt.Errorf("missing %s", k)
			}
		}
		for k, value := range obj {
			property, ok := s.Properties[k]
			if !ok {
				if !s.AdditionalProperties {
					return fmt.Errorf("unknown field %s", k)
				}
				continue
			}
			if err := property.validate(value); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
	case "array":
		values, ok := v.([]any)
		if !ok || s.MaxItems > 0 && len(values) > s.MaxItems {
			return invalid()
		}
		for _, value := range values {
			if err := s.Items.validate(value); err != nil {
				return err
			}
		}
	case "string":
		value, ok := v.(string)
		if !ok || s.MaxLength > 0 && len([]rune(value)) > s.MaxLength {
			return invalid()
		}
		if len(s.Enum) > 0 {
			if slices.Contains(s.Enum, value) {
				return nil
			}
			return invalid()
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return invalid()
		}
	case "number", "integer":
		n, ok := v.(json.Number)
		if !ok {
			return invalid()
		}
		if s.Type == "integer" {
			if _, err := n.Int64(); err != nil {
				return invalid()
			}
		} else if _, err := n.Float64(); err != nil {
			return invalid()
		}
	default:
		return invalid()
	}
	return nil
}
func (s Schema) Validate(raw json.RawMessage) error {
	if len(raw) > MaxJSONBytes {
		return errors.New("action payload too large")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing action JSON")
	}
	return s.validate(v)
}
