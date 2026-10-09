package extension

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func testSettings(manifest *spec.Manifest) {
	manifest.Settings = []spec.Setting{
		{ID: "theme", Kind: "select", Label: map[string]string{"en": "Theme"}, Default: "system", Required: true,
			Options: []spec.SettingOption{{Value: "system", Label: map[string]string{"en": "System"}}, {Value: "dark", Label: map[string]string{"en": "Dark"}}}},
		{ID: "isolation", Kind: "toggle", Label: map[string]string{"en": "Isolation"}, Default: true, Readonly: true},
		{ID: "title", Kind: "text", Label: map[string]string{"en": "Title"}, MaxLength: 8},
	}
}

func TestSettingsSurvivePackageLifecycle(t *testing.T) {
	f := newFixture(t)
	dir, ctx := t.TempDir(), context.Background()
	stored := map[string][]byte{}
	failSave := false
	configure := func(m *Manager) {
		m.SetSettingsStore(func(id string) (map[string]any, error) {
			values := map[string]any{}
			if data := stored[id]; data != nil {
				if err := json.Unmarshal(data, &values); err != nil {
					return nil, err
				}
			}
			return values, nil
		}, func(id string, values map[string]any) error {
			if failSave {
				return errors.New("storage failed")
			}
			if values == nil {
				delete(stored, id)
				return nil
			}
			data, err := json.Marshal(values)
			if err == nil {
				stored[id] = data
			}
			return err
		})
	}
	m := New(dir, "1.5.2", f.trust)
	configure(m)
	old, err := m.Install(ctx, f.pack("editor", "0.1.0", testSettings, nil), []string{"workspace.read"})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := m.Settings(ctx, "editor")
	if err != nil || initial.Values["theme"] != "system" || initial.Values["isolation"] != true || len(stored) != 0 {
		t.Fatalf("defaults were not read from the signed manifest: %+v %v", initial, err)
	}
	for _, invalid := range []map[string]any{
		{"unknown": true}, {"theme": "light"}, {"theme": true}, {"theme": nil}, {"theme": ""}, {"isolation": false}, {"title": "too long title"}, nil,
	} {
		if _, err := m.UpdateSettings(ctx, "editor", old.Hash, invalid); err == nil || len(stored) != 0 {
			t.Fatalf("invalid settings persisted: %v", invalid)
		}
	}
	if _, err := m.UpdateSettings(ctx, "editor", old.Hash, map[string]any{"theme": "dark", "isolation": true}); err != nil {
		t.Fatal(err)
	}
	failSave = true
	if _, err := m.UpdateSettings(ctx, "editor", old.Hash, map[string]any{"theme": "system"}); err == nil {
		t.Fatal("ignored persistence failure")
	}
	failSave = false
	if err := m.CleanData(ctx, "editor"); err == nil {
		t.Fatal("installed settings could be cleared")
	}
	if err := m.SetEnabled(ctx, "editor", false); err != nil {
		t.Fatal(err)
	}
	next, err := m.Install(ctx, f.pack("editor", "0.2.0", testSettings, nil), []string{"workspace.read"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpdateSettings(ctx, "editor", old.Hash, map[string]any{"theme": "system"}); err == nil {
		t.Fatal("stale settings form overwrote a new package")
	}
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	m = New(dir, "1.5.2", f.trust)
	configure(m)
	if err := m.Load(ctx); err != nil {
		t.Fatal(err)
	}
	config, err := m.Settings(ctx, "editor")
	if err != nil || config.Hash != next.Hash || config.Values["theme"] != "dark" {
		t.Fatalf("update/restart lost config: %+v %v", config, err)
	}
	if err := m.Uninstall(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	if len(stored["editor"]) == 0 {
		t.Fatal("uninstall deleted settings")
	}
	if _, err := m.Settings(ctx, "editor"); err == nil {
		t.Fatal("uninstalled extension settings remained accessible")
	}
	if _, err := m.Install(ctx, f.pack("editor", "0.2.0", testSettings, nil), []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
	config, _ = m.Settings(ctx, "editor")
	if config.Values["theme"] != "dark" {
		t.Fatal("reinstall reset user settings")
	}
	if err := m.Uninstall(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	if err := m.CleanData(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatal("explicit data cleanup did not remove settings")
	}
}

func TestSettingsRespectNewSchemaAndPlatformPolicy(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	m := New(t.TempDir(), "1.5.2", f.trust)
	stored := map[string]any{"theme": "removed-choice", "isolation": false, "undeclared": "private"}
	m.SetSettingsStore(func(string) (map[string]any, error) { return stored, nil }, func(_ string, value map[string]any) error { stored = value; return nil })
	s, err := m.Install(ctx, f.pack("editor", "0.1.0", testSettings, nil), []string{"workspace.read"})
	if err != nil {
		t.Fatal(err)
	}
	config, err := m.Settings(ctx, "editor")
	if err != nil || config.Values["theme"] != "system" || config.Values["isolation"] != true || config.Values["undeclared"] != nil {
		t.Fatalf("settings from another schema were exposed: %+v %v", config, err)
	}
	m.SetManagementPolicy(func(context.Context, spec.Manifest) error { return errors.New("native only") })
	if _, err := m.Settings(ctx, "editor"); err == nil {
		t.Fatal("settings read bypassed platform policy")
	}
	if _, err := m.UpdateSettings(ctx, "editor", s.Hash, map[string]any{}); err == nil {
		t.Fatal("settings write bypassed platform policy")
	}
}
