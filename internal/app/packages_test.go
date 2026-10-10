package app

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestPackageInstallPolicyPreservesCatalogAndInstalledState(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		name := "default"
		if disabled {
			name = "manual"
		}
		t.Run(name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.DisablePackageAutoInstall = disabled
			cfg.PackageDirs = []string{t.TempDir()}
			public, private, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			asset := []byte("<p>Optional editor</p>")
			sum := sha256.Sum256(asset)
			manifest, _ := json.Marshal(spec.Manifest{ID: "editor", Name: "Editor", Version: "0.1.0", API: 1,
				Kind: "frontend-sandbox", Target: "backend", Activation: "hot", Core: spec.VersionRange{Min: "1.0.0"},
				Permissions: []string{"workspace.read"}, Files: map[string]string{"frontend/index.html": hex.EncodeToString(sum[:])}})
			signature, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(private, manifest))})
			file, err := os.Create(filepath.Join(cfg.PackageDirs[0], "editor.cphext"))
			if err != nil {
				t.Fatal(err)
			}
			archive := zip.NewWriter(file)
			for path, data := range map[string][]byte{"manifest.json": manifest, "signature.json": signature, "frontend/index.html": asset} {
				entry, err := archive.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = entry.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err = archive.Close(); err != nil {
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
			trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), IDs: []string{"editor"}, Permissions: []string{"workspace.read"}}}
			ctx := context.Background()
			start := func() *App {
				t.Helper()
				a, err := New(cfg, WithExtensionTrust(trust))
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
			if _, installed := a.extensions.State("editor"); installed == disabled {
				t.Fatal("startup did not apply the installation policy")
			}
			catalog := a.extensions.Catalog()
			if len(catalog) != 1 || catalog[0].Manifest == nil {
				t.Fatal("manual package catalog is unavailable")
			}
			if disabled {
				if _, err = a.extensions.InstallMounted(ctx, catalog[0].SHA256, []string{"workspace.read"}); err != nil {
					t.Fatal(err)
				}
			} else if err = a.extensions.SetEnabled(ctx, "editor", false); err != nil {
				t.Fatal(err)
			}
			if err = a.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
			cfg.DisablePackageAutoInstall = true
			a = start()
			state, exists := a.extensions.State("editor")
			if !exists || state.Enabled != disabled || state.Available != disabled {
				t.Fatal("disabling automatic installation changed the existing enabled state")
			}
		})
	}
}
