package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pluginpkg "github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestRuntimeDisableStopsLuaAndRecoversOnFailure(t *testing.T) {
	for _, native := range []bool{false, true} {
		name := "desktop"
		if native {
			name = "native"
		}
		t.Run(name, func(t *testing.T) {
			cfg, ctx := testConfig(t), context.Background()
			public, private, _ := ed25519.GenerateKey(rand.Reader)
			trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), Native: true, IDs: []string{"lua-runtime"}, Permissions: []string{"runtime.execute"}}}
			connector := &updateConnector{stateFile: filepath.Join(cfg.DataDir, "extensions", "state.json"), history: map[string][]string{}, released: map[string]int{}}
			options := []Option{WithExtensionTrust(trust), WithPluginRuntime(pluginpkg.NewServiceRuntime(connector)), WithRuntimeValidator(func(context.Context, string) error { return nil })}
			if native {
				options = append(options, WithNativeRuntimeManagement())
			}
			a, err := New(cfg, options...)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.Start(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.Shutdown(ctx) })
			runtimeCtx := a.runtimeContext(ctx)
			old, err := a.extensions.Install(runtimeCtx, runtimeUpdatePackage(t, private, t.TempDir(), "0.2.0", "old"), []string{"runtime.execute"})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"script", "native"} {
				dir := filepath.Join(cfg.PluginDir, name)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				runtime := "lua"
				if name == "native" {
					runtime = "go"
				}
				data, _ := json.Marshal(map[string]string{"name": name, "runtime": runtime})
				if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
					t.Fatal(err)
				}
				if _, err := a.plugins.Start(ctx, dir); err != nil {
					t.Fatal(err)
				}
			}
			nativePlugin, _ := a.plugins.Get("native")
			var script model.Plugin
			if err := a.db.Where("name = ?", "script").First(&script).Error; err != nil {
				t.Fatal(err)
			}
			rule := model.TaskRule{PluginID: script.ID, Enabled: true, TriggerType: "manual"}
			if err := a.db.Create(&rule).Error; err != nil {
				t.Fatal(err)
			}
			if err := a.extensions.SetEnabled(runtimeCtx, "lua-runtime", false); err == nil {
				t.Fatal("disable ignored enabled task dependency")
			}
			if _, ok := a.plugins.Get("script"); !ok {
				t.Fatal("blocked disable interrupted the script")
			}
			if err := a.db.Delete(&rule).Error; err != nil {
				t.Fatal(err)
			}
			failStop := true
			a.extensions.SetLifecycle(func(_ context.Context, state extension.State) error {
				return a.plugins.SetLuaRuntime(func() (string, error) { return a.extensions.Executable(state) })
			}, func(context.Context, string) error {
				if failStop {
					return errors.New("runtime still in use")
				}
				return a.plugins.SetLuaRuntime(nil)
			})
			if err := a.extensions.SetEnabled(runtimeCtx, "lua-runtime", false); err == nil {
				t.Fatal("failed deactivation reported success")
			}
			if state, _ := a.extensions.State("lua-runtime"); !state.Available || state.Hash != old.Hash {
				t.Fatal("failed disable lost runtime state")
			}
			if _, ok := a.plugins.Get("script"); !ok {
				t.Fatal("failed disable did not restore the original script")
			}
			failStop = false
			if native {
				_, err = a.NativeRuntime(ctx, NativeRuntimeRequest{Operation: "disable", ID: "lua-runtime"})
			} else {
				err = a.extensions.SetEnabled(ctx, "lua-runtime", false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := a.plugins.Get("script"); ok {
				t.Fatal("disabled runtime kept a Lua session")
			}
			if running, _ := a.plugins.Get("native"); running != nativePlugin {
				t.Fatal("runtime disable affected an unrelated Go plugin")
			}
			if _, err := a.plugins.Start(ctx, filepath.Join(cfg.PluginDir, "script")); err == nil {
				t.Fatal("disabled runtime allowed new Lua sessions")
			}
			if err := a.db.First(&script, script.ID).Error; err != nil || !script.Enabled {
				t.Fatal("runtime switch overwrote per-plugin preference")
			}
			if err := a.extensions.SetEnabled(runtimeCtx, "lua-runtime", true); err != nil {
				t.Fatal(err)
			}
			if _, err := a.plugins.Start(ctx, filepath.Join(cfg.PluginDir, "script")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
