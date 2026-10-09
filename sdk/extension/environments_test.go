package extension

import (
	"encoding/json"
	"testing"
)

func TestManifestEnvironmentValidation(t *testing.T) {
	for _, test := range []struct {
		name, declaration string
		valid             bool
	}{
		{"unrestricted", "", true},
		{"all", `,"environments":["app","web-desktop","web-mobile"]`, true},
		{"desktop", `,"environments":["web-desktop"]`, true},
		{"app", `,"environments":["app"]`, true},
		{"empty", `,"environments":[]`, false},
		{"unknown", `,"environments":["mobile"]`, false},
		{"duplicate", `,"environments":["app","app"]`, false},
		{"scalar", `,"environments":"app"`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var manifest Manifest
			err := json.Unmarshal([]byte(`{"id":"example","name":"Example","version":"0.1.0","api":1,"kind":"data","target":"backend","activation":"hot","core":{},"permissions":[],"files":{}`+test.declaration+`}`), &manifest)
			if err == nil {
				err = manifest.Validate()
			}
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}
