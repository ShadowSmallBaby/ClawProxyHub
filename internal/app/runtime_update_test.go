package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pluginpkg "github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
)

// 与原生加载器一致，从已持久化的状态选择包，验证更新与回滚的顺序。
type updateConnector struct {
	mu        sync.Mutex
	stateFile string
	history   map[string][]string
	released  map[string]int
	fail      func(string, string) error
}

func (*updateConnector) Available(string) bool { return true }
func (c *updateConnector) Connect(ctx context.Context, dir string) (*pluginpkg.ServiceBinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, hash := filepath.Base(dir), "native"
	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	if bytes.Contains(manifest, []byte(`"runtime":"lua"`)) {
		data, err := os.ReadFile(c.stateFile)
		if err != nil {
			return nil, err
		}
		var states map[string]extension.State
		if err = json.Unmarshal(data, &states); err != nil {
			return nil, err
		}
		state := states["lua-runtime"]
		if !state.Available || !state.Enabled {
			return nil, errors.New("persisted runtime is unavailable")
		}
		hash = state.Hash
	}
	c.mu.Lock()
	c.history[name] = append(c.history[name], hash)
	c.mu.Unlock()
	if c.fail != nil {
		if err := c.fail(name, hash); err != nil {
			return nil, err
		}
	}
	a, b := net.Pipe()
	d, e := net.Pipe()
	peer, err := transport.ServePlugin(b, e, &appPlugin{name: name})
	if err != nil {
		a.Close()
		d.Close()
		return nil, err
	}
	return &pluginpkg.ServiceBinding{Requests: a, Callbacks: d, Protocol: sdk.ProtocolVersion, Release: func() error {
		c.mu.Lock()
		c.released[name]++
		c.mu.Unlock()
		peer.Close()
		return nil
	}}, nil
}

func runtimeUpdatePackage(t *testing.T, key ed25519.PrivateKey, dir, version, name string) string {
	t.Helper()
	asset := []byte(version + "/" + name)
	sum := sha256.Sum256(asset)
	manifest := spec.Manifest{ID: "lua-runtime", Name: name, Version: version, API: 1, Kind: "runtime", Target: "backend", Activation: "hot",
		Entry: "entry", Execution: "trusted-process", Permissions: []string{"runtime.execute"}, Files: map[string]string{"entry": hex.EncodeToString(sum[:])}}
	raw, _ := json.Marshal(manifest)
	signature, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))})
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	for name, contents := range map[string][]byte{"manifest.json": raw, "signature.json": signature, "entry": asset} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, name+".cphhost")
	if err := os.WriteFile(file, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestRuntimeUpdateRestoresRunningPlugins(t *testing.T) {
	for _, scenario := range []string{"same-version", "upgrade", "native-upgrade", "validation-failure", "restart-failure", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := testConfig(t)
			ctx := context.Background()
			public, private, _ := ed25519.GenerateKey(rand.Reader)
			trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), Native: true, IDs: []string{"lua-runtime"}, Permissions: []string{"runtime.execute"}}}
			connector := &updateConnector{stateFile: filepath.Join(cfg.DataDir, "extensions", "state.json"), history: map[string][]string{}, released: map[string]int{}}
			invalid := false
			options := []Option{WithExtensionTrust(trust), WithPluginRuntime(pluginpkg.NewServiceRuntime(connector)), WithRuntimeValidator(func(context.Context, string) error {
				if invalid {
					return errors.New("invalid candidate")
				}
				return nil
			})}
			if scenario == "native-upgrade" {
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
			packages := t.TempDir()
			old, err := a.extensions.Install(a.runtimeContext(ctx), runtimeUpdatePackage(t, private, packages, "0.2.0", "old"), []string{"runtime.execute"})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"alpha", "beta", "stopped", "native"} {
				dir := filepath.Join(cfg.PluginDir, name)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				runtime := "lua"
				if name == "native" {
					runtime = "go"
				}
				if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(fmt.Sprintf(`{"name":%q,"runtime":%q}`, name, runtime)), 0644); err != nil {
					t.Fatal(err)
				}
				if name == "stopped" {
					if err := a.db.Create(&model.Plugin{Name: name, Enabled: false}).Error; err != nil {
						t.Fatal(err)
					}
					if err := a.db.Model(&model.Plugin{}).Where("name = ?", name).Update("enabled", false).Error; err != nil {
						t.Fatal(err)
					}
				} else if _, err := a.plugins.Start(ctx, dir); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.extensions.Uninstall(a.runtimeContext(ctx), "lua-runtime"); err == nil {
				t.Fatal("uninstall bypassed running-plugin protection")
			}
			version := "0.3.0"
			if scenario == "same-version" {
				version = "0.2.0"
			}
			file := runtimeUpdatePackage(t, private, packages, version, "new")
			candidate, err := a.extensions.Inspect(file)
			if err != nil {
				t.Fatal(err)
			}
			updateCtx, cancel := context.WithCancel(a.runtimeContext(ctx))
			defer cancel()
			invalid = scenario == "validation-failure"
			connector.fail = func(name, hash string) error {
				if name == "beta" && hash == candidate.SHA256 {
					if scenario == "cancelled" {
						cancel()
						return context.Canceled
					}
					if scenario == "restart-failure" {
						return errors.New("plugin cannot use candidate runtime")
					}
				}
				return nil
			}
			if scenario == "native-upgrade" {
				_, err = a.NativeRuntime(updateCtx, NativeRuntimeRequest{Operation: "install", File: file, SHA256: candidate.SHA256, Grants: []string{"runtime.execute"}})
			} else {
				_, err = a.extensions.Install(updateCtx, file, []string{"runtime.execute"})
			}
			failed := scenario == "validation-failure" || scenario == "restart-failure" || scenario == "cancelled"
			if (err != nil) != failed {
				t.Fatalf("update: %v, expected failure=%v", err, failed)
			}
			wantHash := candidate.SHA256
			if failed {
				wantHash = old.Hash
			}
			current, _ := a.extensions.State("lua-runtime")
			if current.Hash != wantHash || !current.Available || !current.Enabled {
				t.Fatalf("incorrect final runtime: %+v", current)
			}
			data, err := os.ReadFile(connector.stateFile)
			if err != nil {
				t.Fatal(err)
			}
			var persisted map[string]extension.State
			if err := json.Unmarshal(data, &persisted); err != nil || persisted["lua-runtime"].Hash != wantHash {
				t.Fatal("runtime rollback was not persisted")
			}
			for _, name := range []string{"alpha", "beta", "native"} {
				if _, ok := a.plugins.Get(name); !ok {
					t.Fatalf("%s was not restored", name)
				}
			}
			if _, ok := a.plugins.Get("stopped"); ok {
				t.Fatal("update started a previously stopped plugin")
			}
			var stopped model.Plugin
			if err := a.db.Where("name = ?", "stopped").First(&stopped).Error; err != nil || stopped.Enabled {
				t.Fatal("update changed persisted plugin state")
			}
			connector.mu.Lock()
			defer connector.mu.Unlock()
			if len(connector.history["native"]) != 1 || connector.released["native"] != 0 || len(connector.history["stopped"]) != 0 {
				t.Fatal("update interrupted unrelated plugins")
			}
			for _, name := range []string{"alpha", "beta"} {
				history := connector.history[name]
				if history[len(history)-1] != wantHash {
					t.Fatalf("%s resumed before the correct runtime was persisted: %v", name, history)
				}
				if invalid && (len(history) != 1 || connector.released[name] != 0) {
					t.Fatal("invalid candidate interrupted an active plugin")
				}
			}
		})
	}
}
