package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/control"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func settingsPackage(t *testing.T, dir string, key ed25519.PrivateKey, id string) string {
	t.Helper()
	manifest := spec.Manifest{ID: id, Name: id, Version: "0.1.0", API: 1, Kind: "data", Target: "backend", Activation: "hot", Files: map[string]string{},
		Settings: []spec.Setting{{ID: "theme", Kind: "select", Label: map[string]string{"en": "Theme"}, Default: "system", Required: true,
			Options: []spec.SettingOption{{Value: "system", Label: map[string]string{"en": "System"}}, {Value: "dark", Label: map[string]string{"en": "Dark"}}}}}}
	raw, _ := json.Marshal(manifest)
	sig, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))})
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	for name, data := range map[string][]byte{"manifest.json": raw, "signature.json": sig} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, id+".cphext")
	if err := os.WriteFile(file, content.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestExtensionSettingsUseNamespacedPersistentStorage(t *testing.T) {
	cfg, ctx := testConfig(t), context.Background()
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), IDs: []string{"editor", "other"}}}
	start := func() (*App, *control.Client) {
		t.Helper()
		a, err := New(cfg, WithExtensionTrust(trust))
		if err != nil {
			t.Fatal(err)
		}
		if err = a.Start(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = a.Shutdown(ctx) })
		token, err := a.DeviceSession()
		if err != nil {
			t.Fatal(err)
		}
		client, err := control.New("http://"+a.Address(), token)
		if err != nil {
			t.Fatal(err)
		}
		return a, client
	}
	a, client := start()
	for _, id := range []string{"editor", "other"} {
		if _, err := a.extensions.Install(ctx, settingsPackage(t, t.TempDir(), private, id), nil); err != nil {
			t.Fatal(err)
		}
	}
	state, _ := a.extensions.State("editor")
	body, _ := json.Marshal(map[string]any{"hash": state.Hash, "values": map[string]any{"theme": "dark"}})
	if _, err := client.Request(ctx, "PUT", "/admin/extensions/editor/settings", "application/json", bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	var record model.Setting
	if err := a.db.Where("key = ?", setting.ExtensionConfigKey("editor")).First(&record).Error; err != nil || record.Value != `{"theme":"dark"}` {
		t.Fatalf("configuration was not stored under the extension identity: %+v %v", record, err)
	}
	other, err := a.extensions.Settings(ctx, "other")
	if err != nil || other.Values["theme"] != "system" {
		t.Fatal("one extension changed another extension's settings")
	}
	if raw, err := client.Request(ctx, "GET", "/admin/settings", "", nil); err != nil || bytes.Contains(raw, []byte(`"theme"`)) {
		t.Fatal("extension configuration leaked into the system settings response")
	}
	tokenData, err := client.Request(ctx, "POST", "/admin/tokens/scoped", "application/json", bytes.NewBufferString(`{"scopes":["extensions.read"],"ttl_seconds":60}`))
	if err != nil {
		t.Fatal(err)
	}
	var session struct{ Token string }
	json.Unmarshal(tokenData, &session)
	scoped, _ := control.New(client.URL, session.Token)
	for _, method := range []string{"GET", "PUT"} {
		_, err := scoped.Request(ctx, method, "/admin/extensions/editor/settings", "application/json", bytes.NewReader(body))
		var failure *control.HTTPError
		if !errors.As(err, &failure) || failure.Status != 403 {
			t.Fatalf("scoped %s accessed settings: %v", method, err)
		}
	}
	if err := a.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	a, client = start()
	raw, err := client.Request(ctx, "GET", "/admin/extensions/editor/settings", "", nil)
	if err != nil || !bytes.Contains(raw, []byte(`"theme":"dark"`)) {
		t.Fatalf("restart lost configuration: %s %v", raw, err)
	}
	if err := a.extensions.Uninstall(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := a.settings.Lookup(setting.ExtensionConfigKey("editor")); err != nil || !found {
		t.Fatal("uninstall removed retained settings")
	}
	if err := a.extensions.CleanData(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := a.settings.Lookup(setting.ExtensionConfigKey("editor")); err != nil || found {
		t.Fatal("cleanup left settings in the database/cache")
	}
}

func TestLegacyLuaDisableMigratesBeforeActivation(t *testing.T) {
	for _, installed := range []bool{false, true} {
		name := "not-installed"
		if installed {
			name = "installed"
		}
		t.Run(name, func(t *testing.T) {
			cfg, ctx := testConfig(t), context.Background()
			cfg.PackageDirs = []string{t.TempDir()}
			public, private, _ := ed25519.GenerateKey(rand.Reader)
			trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), Native: true, IDs: []string{"lua-runtime"}, Permissions: []string{"runtime.execute"}}}
			probes := 0
			start := func() *App {
				a, err := New(cfg, WithExtensionTrust(trust), WithRuntimeValidator(func(context.Context, string) error { probes++; return nil }))
				if err != nil {
					t.Fatal(err)
				}
				if err := a.Start(ctx); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = a.Shutdown(ctx) })
				return a
			}
			a := start()
			file := runtimeUpdatePackage(t, private, cfg.PackageDirs[0], "0.2.0", "host")
			if installed {
				if _, err := a.extensions.Install(ctx, file, []string{"runtime.execute"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.settings.SetMany(map[string]string{setting.KeyLuaEnabled: "false", setting.KeyLuaIsolation: "false"}); err != nil {
				t.Fatal(err)
			}
			if err := a.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
			previousProbes := probes
			a = start()
			state, exists := a.extensions.State("lua-runtime")
			if exists != installed || state.Enabled || state.Available || a.plugins.LuaAvailable() || probes != previousProbes {
				t.Fatal("migration activated a disabled runtime")
			}
			if _, found, err := a.settings.Lookup(setting.KeyLuaEnabled); err != nil || found {
				t.Fatal("legacy switch remained an independent setting")
			}
			if !installed {
				if _, err := a.extensions.Install(ctx, file, []string{"runtime.execute"}); err != nil {
					t.Fatal(err)
				}
			} else if err := a.extensions.SetEnabled(ctx, "lua-runtime", true); err != nil {
				t.Fatal(err)
			}
			if err := a.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
			a = start()
			if state, _ := a.extensions.State("lua-runtime"); !state.Enabled || !state.Available {
				t.Fatal("explicit enable was overridden by the legacy switch")
			}
		})
	}
}
