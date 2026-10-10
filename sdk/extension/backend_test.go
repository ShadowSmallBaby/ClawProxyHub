package extension

import (
	"encoding/json"
	"strings"
	"testing"
)

func backendManifest() Manifest {
	manifest := Manifest{ID: "example", Name: "Example", Version: "1.0.0", API: 1, Kind: "service", Target: "backend", Activation: "hot", Execution: "trusted-process", Permissions: []string{"service.execute", "storage.read"}, Files: map[string]string{}, Backend: &Backend{Language: "go", Protocol: 1, Entries: map[string]string{}}}
	for _, platform := range DesktopPlatforms {
		entry := "backend/" + platform + "/service"
		manifest.Backend.Entries[platform] = entry
		manifest.Files[entry] = strings.Repeat("a", 64)
	}
	return manifest
}

func TestBackendRequiresCompleteGoDeclaration(t *testing.T) {
	if err := backendManifest().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(*Manifest)
	}{
		{"missing platform", func(m *Manifest) { delete(m.Backend.Entries, "darwin/arm64") }},
		{"unsigned entry", func(m *Manifest) { delete(m.Files, m.Backend.Entries["windows/amd64"]) }},
		{"duplicate entry", func(m *Manifest) { m.Backend.Entries["darwin/amd64"] = m.Backend.Entries["linux/amd64"] }},
		{"language", func(m *Manifest) { m.Backend.Language = "python" }},
		{"protocol", func(m *Manifest) { m.Backend.Protocol = 2 }},
		{"untrusted execution", func(m *Manifest) { m.Execution = "" }},
		{"missing native grant", func(m *Manifest) { m.Permissions = []string{"storage.read"} }},
		{"background escalation", func(m *Manifest) { m.Backend.BackgroundPermissions = []string{"storage.write"} }},
		{"background core access", func(m *Manifest) {
			m.Permissions = append(m.Permissions, "workspace.read")
			m.Backend.BackgroundPermissions = []string{"workspace.read"}
		}},
		{"platform mismatch", func(m *Manifest) { m.Platforms = []string{"windows/amd64"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest := backendManifest()
			test.edit(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("invalid backend accepted")
			}
		})
	}
}

func TestBackendActionsAndSchemasAreValidated(t *testing.T) {
	manifest := backendManifest()
	manifest.Actions = []Action{{ID: "list", Title: "List", Permission: "storage.read", Effect: "read", TimeoutMS: 5000, Input: &Schema{Type: "object"}, Output: &Schema{Type: "object", AdditionalProperties: true}}}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	manifest.Actions[0].Permission = "workspace.read"
	if err := manifest.Validate(); err == nil {
		t.Fatal("undeclared action grant accepted")
	}
	cyclic := &Schema{Type: "array"}
	cyclic.Items = cyclic
	if err := cyclic.Check(); err == nil {
		t.Fatal("unbounded schema accepted")
	}
	raw, _ := json.Marshal(backendManifest())
	raw = append([]byte(`{"install_command":"run arbitrary code",`), raw[1:]...)
	var decoded Manifest
	if err := json.Unmarshal(raw, &decoded); err == nil {
		t.Fatal("unknown manifest command silently accepted")
	}
}

func TestAndroidBackendRequiresARM64AndAllowsTestABI(t *testing.T) {
	makeManifest := func() Manifest {
		m := backendManifest()
		m.Backend.Android = &AndroidBackend{Protocol: 1, MinSDK: 24}
		for _, platform := range AndroidPlatforms {
			entry := "backend/" + platform + "/libservice.so"
			m.Backend.Entries[platform] = entry
			m.Files[entry] = strings.Repeat("b", 64)
		}
		m.Platforms = m.Backend.Platforms()
		return m
	}
	if err := makeManifest().Validate(); err != nil {
		t.Fatal(err)
	}
	production := makeManifest()
	delete(production.Files, production.Backend.Entries["android/amd64"])
	delete(production.Backend.Entries, "android/amd64")
	production.Platforms = production.Backend.Platforms()
	if err := production.Validate(); err != nil {
		t.Fatal("ARM64-only Android release must be valid:", err)
	}
	for _, edit := range []func(*Manifest){
		func(m *Manifest) { m.Backend.Android = nil },
		func(m *Manifest) { m.Backend.Android.Protocol++ },
		func(m *Manifest) { m.Backend.Android.MinSDK = 23 },
		func(m *Manifest) { delete(m.Backend.Entries, "android/arm64") },
		func(m *Manifest) { delete(m.Files, m.Backend.Entries["android/arm64"]) },
		func(m *Manifest) {
			m.Backend.Entries["android/arm64"] = "backend/android/service"
			m.Files["backend/android/service"] = strings.Repeat("b", 64)
		},
		func(m *Manifest) { m.Platforms = DesktopPlatforms },
	} {
		m := makeManifest()
		edit(&m)
		if err := m.Validate(); err == nil {
			t.Fatal("incomplete Android backend accepted")
		}
	}
}
