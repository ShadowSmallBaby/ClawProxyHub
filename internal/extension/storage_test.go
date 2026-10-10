package extension

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func runtimeFixture(t *testing.T) (*fixture, string) {
	f := newFixture(t)
	key := f.trust["test"]
	key.IDs = append(key.IDs, "lua-runtime")
	key.Native = true
	key.Permissions = append(key.Permissions, "runtime.execute")
	f.trust["test"] = key
	file := f.pack("lua-runtime", "1.0.0", func(m *spec.Manifest) {
		m.Kind, m.Execution, m.Entry = "runtime", "trusted-process", "frontend/index.html"
		m.Permissions = append(m.Permissions, "runtime.execute")
	}, nil)
	return f, file
}

func TestRuntimeStorageAndExtensionLifecycleAreSeparate(t *testing.T) {
	f, runtime := runtimeFixture(t)
	data := t.TempDir()
	m := New(data, "1.5.2", f.trust)
	m.SetLifecycle(func(context.Context, State) error { return nil }, nil)
	ctx := context.Background()
	host, err := m.Install(ctx, runtime, []string{"workspace.read", "runtime.execute"})
	if err != nil {
		t.Fatal(err)
	}
	editor, err := m.Install(ctx, f.pack("editor", "1.0.0", nil, nil), []string{"workspace.read"})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		m.Archive(host):   filepath.Join(data, "hosts", "versions", host.Manifest.ID, host.Hash, "package.cphhost"),
		m.Archive(editor): filepath.Join(data, "extensions", "versions", editor.Manifest.ID, editor.Hash, "package.cphext"),
	} {
		if path != want {
			t.Fatalf("installation directory: %s != %s", path, want)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	if err = m.SetEnabled(ctx, "lua-runtime", false); err != nil {
		t.Fatal(err)
	}
	reloaded := New(data, "1.5.2", f.trust)
	if err = reloaded.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if state, ok := reloaded.State("lua-runtime"); !ok || state.Enabled {
		t.Fatal("disabled runtime state lost")
	}
	if err = reloaded.Uninstall(ctx, "lua-runtime"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(m.Archive(host)); !os.IsNotExist(err) {
		t.Fatal("runtime files remain installed")
	}
	if _, err = reloaded.ReadAsset("editor", editor.Hash, "frontend/index.html"); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDirectoryMigrationPreservesDisabledState(t *testing.T) {
	f, file := runtimeFixture(t)
	v, err := Verify(file, f.trust)
	if err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	old := filepath.Join(data, "extensions", "versions", v.Manifest.ID, v.SHA256)
	if err = os.MkdirAll(old, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(old, "package.cphext"), v.Archive, 0600); err != nil {
		t.Fatal(err)
	}
	state := State{Manifest: v.Manifest, Hash: v.SHA256, Signer: v.Signature.KeyID, Enabled: false}
	raw, _ := json.Marshal(map[string]State{v.Manifest.ID: state})
	if err = os.WriteFile(filepath.Join(data, "extensions", "state.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	m := New(data, "1.5.2", f.trust)
	if err = m.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, ok := m.State(v.Manifest.ID)
	if !ok || state.Enabled || state.Status != "disabled" {
		t.Fatalf("migration changed state: %+v", state)
	}
	if _, err = Verify(m.Archive(state), f.trust); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old runtime directory remains")
	}
}
