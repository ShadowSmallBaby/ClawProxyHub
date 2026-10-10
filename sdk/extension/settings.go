package extension

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Setting 由签名清单声明，宿主渲染控件并校验用户配置。
type Setting struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Label     map[string]string `json:"label"`
	Hint      map[string]string `json:"hint,omitempty"`
	Default   any               `json:"default,omitempty"`
	Required  bool              `json:"required,omitempty"`
	Readonly  bool              `json:"readonly,omitempty"`
	MaxLength int               `json:"max_length,omitempty"`
	Options   []SettingOption   `json:"options,omitempty"`
}

type SettingOption struct {
	Value string            `json:"value"`
	Label map[string]string `json:"label"`
}

var settingLocale = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)

func settingText(values map[string]string, limit int) bool {
	if len(values) == 0 || len(values) > 8 {
		return false
	}
	for locale, value := range values {
		if !settingLocale.MatchString(locale) || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > limit {
			return false
		}
	}
	return true
}

func ValidateSettings(fields []Setting) error {
	if len(fields) > 24 {
		return fmt.Errorf("too many extension settings")
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if !ValidID(field.ID) || field.ID == "constructor" || field.ID == "prototype" || seen[field.ID] ||
			!settingText(field.Label, 160) || len(field.Hint) > 0 && !settingText(field.Hint, 1024) {
			return fmt.Errorf("invalid setting %q", field.ID)
		}
		seen[field.ID] = true
		switch field.Kind {
		case "text", "textarea":
			if field.MaxLength < 0 || field.MaxLength > 16384 || len(field.Options) != 0 {
				return fmt.Errorf("invalid text setting %q", field.ID)
			}
		case "select":
			if len(field.Options) == 0 || len(field.Options) > 64 || field.MaxLength != 0 {
				return fmt.Errorf("invalid select setting %q", field.ID)
			}
			options := map[string]bool{}
			for _, option := range field.Options {
				if strings.TrimSpace(option.Value) == "" || utf8.RuneCountInString(option.Value) > 160 || options[option.Value] || !settingText(option.Label, 160) {
					return fmt.Errorf("invalid option for %q", field.ID)
				}
				options[option.Value] = true
			}
		case "toggle":
			if field.MaxLength != 0 || len(field.Options) != 0 {
				return fmt.Errorf("invalid toggle setting %q", field.ID)
			}
		default:
			return fmt.Errorf("unsupported setting kind %q", field.Kind)
		}
		if field.Readonly && field.Default == nil {
			return fmt.Errorf("readonly setting %q requires a default", field.ID)
		}
		if field.Default != nil {
			if err := field.ValidateValue(field.Default); err != nil {
				return fmt.Errorf("invalid default: %w", err)
			}
		}
	}
	return nil
}

func (s Setting) DefaultValue() any {
	if s.Default != nil {
		return s.Default
	}
	if s.Kind == "toggle" {
		return false
	}
	return ""
}

// ValidateValue 不接受类型转换；false 是有效的开关值。
func (s Setting) ValidateValue(value any) error {
	if s.Kind == "toggle" {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("setting %q must be a boolean", s.ID)
		}
		return nil
	}
	text, ok := value.(string)
	if !ok || s.Required && strings.TrimSpace(text) == "" {
		return fmt.Errorf("setting %q requires text", s.ID)
	}
	if s.Kind == "select" {
		if text == "" && !s.Required {
			return nil
		}
		for _, option := range s.Options {
			if text == option.Value {
				return nil
			}
		}
		return fmt.Errorf("unsupported value for setting %q", s.ID)
	}
	limit := s.MaxLength
	if limit == 0 {
		limit = 4096
	}
	if utf8.RuneCountInString(text) > limit {
		return fmt.Errorf("setting %q exceeds %d characters", s.ID, limit)
	}
	return nil
}
