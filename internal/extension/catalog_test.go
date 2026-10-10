package extension

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestMountedPackagesPreserveInstallDisableAndRemoval(t *testing.T) {
	f := newFixture(t)
	f.pack("editor", "0.1.0", nil, nil)
	dir, ctx := t.TempDir(), context.Background()
	manager := New(dir, "1.5.2", f.trust)
	manager.SetPackageDirs([]string{f.dir})
	if err := manager.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err := manager.ImportMounted(ctx); err != nil {
		t.Fatal(err)
	}
	initial, ok := manager.State("editor")
	if !ok || !initial.Available {
		t.Fatal("package was not imported")
	}
	if err := manager.SetEnabled(ctx, "editor", false); err != nil {
		t.Fatal(err)
	}
	upgrade := f.pack("editor", "0.2.0", nil, nil)
	if err := manager.ImportMounted(ctx); err != nil {
		t.Fatal(err)
	}
	state, _ := manager.State("editor")
	if state.Hash != initial.Hash || state.Enabled {
		t.Fatal("startup changed an existing installation")
	}
	verified, err := manager.Inspect(upgrade)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.InstallMounted(ctx, verified.SHA256, nil); err == nil {
		t.Fatal("mounted install bypassed grants")
	}
	if _, err = manager.InstallMounted(ctx, verified.SHA256, []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
	state, _ = manager.State("editor")
	if state.Manifest.Version != "0.2.0" || state.Enabled {
		t.Fatal("explicit upgrade did not preserve disabled state")
	}
	if err = manager.Uninstall(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	manager = New(dir, "1.5.2", f.trust)
	manager.SetPackageDirs([]string{f.dir})
	if err = manager.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err = manager.ImportMounted(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.State("editor"); ok {
		t.Fatal("uninstalled package returned after restart")
	}
	if _, err = manager.InstallMounted(ctx, verified.SHA256, []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
}

func TestMountedDependenciesAndInvalidPackageIsolation(t *testing.T) {
	f := newFixture(t)
	f.pack("dependent", "0.1.0", func(m *spec.Manifest) { m.Dependencies = map[string]spec.VersionRange{"editor": {Min: "0.2.0"}} }, nil)
	f.pack("editor", "0.1.0", nil, nil)
	f.pack("editor", "0.2.0", nil, nil)
	if err := os.WriteFile(filepath.Join(f.dir, "broken.cphext"), []byte("invalid archive"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(t.TempDir(), "1.5.2", f.trust)
	m.SetPackageDirs([]string{f.dir, filepath.Join(f.dir, "absent")})
	if err := m.ImportMounted(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"editor", "dependent"} {
		if s, ok := m.State(id); !ok || !s.Available {
			t.Fatalf("dependency installation failed: %s", id)
		}
	}
	if s, _ := m.State("editor"); s.Manifest.Version != "0.2.0" {
		t.Fatal("did not select newest compatible package")
	}
	invalid := false
	for _, item := range m.Catalog() {
		invalid = invalid || item.Status == "invalid" && item.Error != ""
	}
	if !invalid {
		t.Fatal("invalid package disappeared from catalog")
	}
}

func TestMountedSameVersionConflictAndChangedInput(t *testing.T) {
	f := newFixture(t)
	first := f.pack("editor", "0.1.0", nil, nil)
	if err := os.Rename(first, filepath.Join(f.dir, "original.cphext")); err != nil {
		t.Fatal(err)
	}
	f.pack("editor", "0.1.0", func(m *spec.Manifest) { m.Name = "different source" }, nil)
	m := New(t.TempDir(), "1.5.2", f.trust)
	m.SetPackageDirs([]string{f.dir})
	if err := m.ImportMounted(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatal("ambiguous package was installed")
	}
	for _, item := range m.Catalog() {
		if item.Status != "conflict" {
			t.Fatalf("conflict not shown: %s", item.Status)
		}
	}
	if err := os.Remove(filepath.Join(f.dir, "original.cphext")); err != nil {
		t.Fatal(err)
	}
	catalog := m.Catalog()
	if err := os.WriteFile(first, []byte("changed after inspection"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.InstallMounted(context.Background(), catalog[0].SHA256, []string{"workspace.read"}); err == nil {
		t.Fatal("installed changed source")
	}
}

func TestOmittedUpperBoundAcceptsNewerCore(t *testing.T) {
	f := newFixture(t)
	file := f.pack("editor", "0.1.0", func(m *spec.Manifest) { m.Core.MaxExclusive = "" }, nil)
	m := New(t.TempDir(), "9.0.0", f.trust)
	if _, err := m.Install(context.Background(), file, []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
}
