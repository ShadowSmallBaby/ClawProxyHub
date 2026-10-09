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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/control"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestNativeRuntimeBoundaryPreservesSharedLifecycle(t *testing.T) {
	cfg := testConfig(t)
	packages := t.TempDir()
	cfg.PackageDirs = []string{packages}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), Native: true,
		IDs: []string{"lua-runtime", "editor"}, Permissions: []string{"runtime.execute", "workspace.read"}}}
	pack := func(id, kind, version string) string {
		t.Helper()
		asset := []byte("fixture " + version)
		sum := sha256.Sum256(asset)
		manifest := spec.Manifest{ID: id, Name: id, Version: version, API: 1, Kind: kind, Target: "backend", Activation: "hot",
			Permissions: []string{"workspace.read"}, Files: map[string]string{"entry": hex.EncodeToString(sum[:])}}
		suffix := ".cphext"
		if kind == "runtime" {
			manifest.Entry, manifest.Execution, manifest.Permissions = "entry", "trusted-process", []string{"runtime.execute"}
			manifest.Settings = []spec.Setting{{ID: "isolation", Kind: "toggle", Label: map[string]string{"en": "Isolation"}, Default: true, Readonly: true}}
			suffix = ".cphhost"
		}
		raw, _ := json.Marshal(manifest)
		signed, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))})
		var content bytes.Buffer
		archive := zip.NewWriter(&content)
		for name, data := range map[string][]byte{"manifest.json": raw, "signature.json": signed, "entry": asset} {
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
		path := filepath.Join(packages, id+"-"+version+suffix)
		if err := os.WriteFile(path, content.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	host := pack("lua-runtime", "runtime", "0.2.0")
	pack("editor", "data", "0.1.0")
	ctx := context.Background()
	probes := 0
	a, err := New(cfg, WithNativeRuntimeManagement(), WithExtensionTrust(trust), WithRuntimeValidator(func(context.Context, string) error {
		probes++
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Shutdown(ctx) })
	hostState, ok := a.extensions.State("lua-runtime")
	if !ok || !hostState.Available || probes != 1 {
		t.Fatal("native startup did not install and validate the mounted runtime")
	}
	token, err := a.DeviceSession()
	if err != nil {
		t.Fatal(err)
	}
	client, err := control.New("http://"+a.Address(), token)
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/admin/extensions", "/admin/extensions/catalog"} {
		raw, err := client.Request(ctx, "GET", endpoint, "", nil)
		if err != nil || bytes.Contains(raw, []byte(`"lua-runtime"`)) || !bytes.Contains(raw, []byte(`"editor"`)) {
			t.Fatalf("functional extension list leaked runtime or lost extensions: %s, %v", raw, err)
		}
	}
	if _, err := client.Install(ctx, host, []string{"runtime.execute"}); err == nil || !strings.Contains(err.Error(), "native application") {
		t.Fatalf("HTTP upload bypassed native management: %v", err)
	}
	for _, operation := range []string{"enable", "disable", "uninstall", "cache"} {
		if _, err := client.Invoke(ctx, "core.extensions."+operation, []byte(`{"id":"lua-runtime"}`)); err == nil || !strings.Contains(err.Error(), "native application") {
			t.Fatalf("action %s bypassed native management: %v", operation, err)
		}
	}
	for _, method := range []string{"GET", "PUT"} {
		body, _ := json.Marshal(map[string]any{"hash": hostState.Hash, "values": map[string]any{"isolation": true}})
		if _, err := client.Request(ctx, method, "/admin/extensions/lua-runtime/settings", "application/json", bytes.NewReader(body)); err == nil || !strings.Contains(err.Error(), "native application") {
			t.Fatalf("HTTP runtime settings bypassed native management: %v", err)
		}
	}
	if _, err := client.Request(ctx, "PUT", "/admin/settings", "application/json", bytes.NewBufferString(`{"plugin_lua_enabled":false}`)); err == nil || !strings.Contains(err.Error(), "native application") {
		t.Fatalf("legacy switch bypassed native runtime management: %v", err)
	}
	if _, err := client.Invoke(ctx, "core.extensions.settings", []byte(`{"id":"lua-runtime"}`)); err == nil || !strings.Contains(err.Error(), "native application") {
		t.Fatalf("settings action bypassed native runtime management: %v", err)
	}
	config, err := a.NativeRuntime(ctx, NativeRuntimeRequest{Operation: "settings", ID: "lua-runtime"})
	if err != nil {
		t.Fatal(err)
	}
	rawConfig, _ := json.Marshal(config)
	if !bytes.Contains(rawConfig, []byte(`"isolation":true`)) {
		t.Fatal("native runtime settings lost the signed default")
	}
	if _, err := a.NativeRuntime(ctx, NativeRuntimeRequest{Operation: "configure", ID: "lua-runtime", SHA256: hostState.Hash, Values: map[string]any{"isolation": false}}); err == nil {
		t.Fatal("native runtime changed readonly isolation")
	}
	for _, request := range []NativeRuntimeRequest{{Operation: "disable", ID: "lua-runtime"}, {Operation: "enable", ID: "lua-runtime"}} {
		if _, err := a.NativeRuntime(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	if probes != 2 {
		t.Fatal("native activation bypassed candidate validation")
	}
	if _, err := client.Invoke(ctx, "core.extensions.disable", []byte(`{"id":"editor"}`)); err != nil {
		t.Fatalf("functional extensions are no longer manageable: %v", err)
	}
	if _, err := a.NativeRuntime(ctx, NativeRuntimeRequest{Operation: "uninstall", ID: "lua-runtime"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Invoke(ctx, "core.extensions.data", []byte(`{"id":"lua-runtime"}`)); err == nil {
		t.Fatal("uninstalled runtime data bypassed native management")
	}
	if err := a.extensions.ImportMounted(a.runtimeContext(ctx)); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.extensions.State("lua-runtime"); ok {
		t.Fatal("native uninstall lost the shared mount receipt")
	}
	if _, err := a.NativeRuntime(ctx, NativeRuntimeRequest{Operation: "mounted", ID: "lua-runtime", SHA256: hostState.Hash, Grants: []string{"runtime.execute"}}); err != nil {
		t.Fatalf("explicit native reinstall failed: %v", err)
	}
	if _, err := a.NativeRuntime(ctx, NativeRuntimeRequest{Operation: "inspect", File: filepath.Join(packages, "editor-0.1.0.cphext")}); err == nil {
		t.Fatal("runtime entry accepted a functional extension")
	}
}
