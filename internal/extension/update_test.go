package extension

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestSameVersionUpdatePreservesDataAndEnabledState(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "enabled"
		if !enabled {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			m := New(t.TempDir(), "1.5.2", f.trust)
			m.SetPackageDirs([]string{f.dir})
			old, err := m.Install(ctx, f.pack("editor", "0.1.0", nil, nil), []string{"workspace.read"})
			if err != nil {
				t.Fatal(err)
			}
			if !enabled {
				if err := m.SetEnabled(ctx, "editor", false); err != nil {
					t.Fatal(err)
				}
			}
			data := filepath.Join(m.dir, "data", "editor", "draft.txt")
			if err := os.MkdirAll(filepath.Dir(data), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(data, []byte("saved draft"), 0600); err != nil {
				t.Fatal(err)
			}
			f.pack("editor", "0.1.0", func(manifest *spec.Manifest) { manifest.Name = "Updated editor" }, nil)
			catalog := m.Catalog()
			if len(catalog) != 1 || catalog[0].Status != "update" || catalog[0].Error != "" {
				t.Fatalf("same-version update missing: %+v", catalog)
			}
			if err := m.ImportMounted(ctx); err != nil {
				t.Fatal(err)
			}
			if state, _ := m.State("editor"); state.Hash != old.Hash || state.Enabled != enabled {
				t.Fatal("startup replaced an existing package")
			}
			if _, err := m.InstallMounted(ctx, catalog[0].SHA256, nil); err == nil {
				t.Fatal("content update bypassed permission grants")
			}
			updated, err := m.InstallMounted(ctx, catalog[0].SHA256, []string{"workspace.read"})
			if err != nil {
				t.Fatal(err)
			}
			if updated.Hash != catalog[0].SHA256 || updated.Manifest.Version != "0.1.0" || updated.Manifest.Name != "Updated editor" || updated.Enabled != enabled || updated.Available != enabled {
				t.Fatalf("incorrect updated state: %+v", updated)
			}
			if content, err := os.ReadFile(data); err != nil || string(content) != "saved draft" {
				t.Fatal("content update changed user data")
			}
			if _, err := os.Stat(m.Archive(old)); err != nil {
				t.Fatal("previous package was overwritten")
			}
			if _, err := m.Install(ctx, f.pack("editor", "0.0.9", nil, nil), []string{"workspace.read"}); err == nil {
				t.Fatal("downgrade accepted")
			}
			if state, _ := m.State("editor"); state.Hash != updated.Hash {
				t.Fatal("rejected downgrade changed the installed package")
			}
		})
	}
}

func TestSameVersionActivationFailureRestoresPreviousPackage(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	m := New(t.TempDir(), "1.5.2", f.trust)
	old, err := m.Install(ctx, f.pack("editor", "0.1.0", nil, nil), []string{"workspace.read"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetLifecycle(func(_ context.Context, state State) error {
		if state.Manifest.Name == "Broken update" {
			return errors.New("activation failed")
		}
		return nil
	}, nil)
	file := f.pack("editor", "0.1.0", func(manifest *spec.Manifest) { manifest.Name = "Broken update" }, nil)
	if _, err := m.Install(ctx, file, []string{"workspace.read"}); err == nil {
		t.Fatal("broken update installed")
	}
	if state, _ := m.State("editor"); state.Hash != old.Hash || !state.Available {
		t.Fatal("failed content update did not restore the active package")
	}
	if _, err := m.ReadAsset("editor", old.Hash, "frontend/index.html"); err != nil {
		t.Fatal(err)
	}
}
