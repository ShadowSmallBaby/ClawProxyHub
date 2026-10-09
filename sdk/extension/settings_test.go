package extension

import (
	"encoding/json"
	"testing"
)

func TestSettingsMetadataValidation(t *testing.T) {
	valid := []Setting{
		{ID: "theme", Kind: "select", Label: map[string]string{"zh": "主题", "en": "Theme"}, Default: "system", Required: true,
			Options: []SettingOption{{Value: "system", Label: map[string]string{"en": "Follow system"}}, {Value: "dark", Label: map[string]string{"en": "Dark"}}}},
		{ID: "isolation", Kind: "toggle", Label: map[string]string{"en": "Isolation"}, Default: true, Readonly: true},
	}
	if err := ValidateSettings(valid); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func([]Setting){
		"duplicate":        func(f []Setting) { f[1].ID = f[0].ID },
		"reserved":         func(f []Setting) { f[0].ID = "constructor" },
		"missing-label":    func(f []Setting) { f[0].Label = nil },
		"invalid-locale":   func(f []Setting) { f[0].Label = map[string]string{"bad!": "Theme"} },
		"invalid-default":  func(f []Setting) { f[0].Default = "unknown" },
		"duplicate-choice": func(f []Setting) { f[0].Options[1].Value = "system" },
		"wrong-type":       func(f []Setting) { f[1].Default = "true" },
		"readonly-default": func(f []Setting) { f[1].Default = nil },
		"unsupported":      func(f []Setting) { f[1].Kind = "image" },
		"wrong-options":    func(f []Setting) { f[1].Options = f[0].Options },
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(valid)
			var fields []Setting
			json.Unmarshal(raw, &fields)
			change(fields)
			if err := ValidateSettings(fields); err == nil {
				t.Fatal("accepted invalid signed settings declaration")
			}
		})
	}
	if err := (Setting{ID: "enabled", Kind: "toggle", Required: true}).ValidateValue(false); err != nil {
		t.Fatal("false must remain a valid boolean setting")
	}
}
