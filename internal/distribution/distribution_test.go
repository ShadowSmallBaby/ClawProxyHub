package distribution

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestFullLayoutAndMountedPackages(t *testing.T) {
	dir := t.TempDir()
	core := filepath.Join(dir, "core.bin")
	if e := os.WriteFile(core, []byte("same compiled core"), 0700); e != nil {
		t.Fatal(e)
	}
	cli := filepath.Join(dir, "cli.bin")
	os.WriteFile(cli, []byte("CLI"), 0600)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(pub), IDs: []string{"editor"}, Permissions: []string{}}}
	m := spec.Manifest{ID: "editor", Name: "editor", Version: "1.0.0", API: 1, Kind: "data", Target: "backend", Activation: "hot", Files: map[string]string{}}
	raw, _ := json.Marshal(m)
	sig, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))})
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for n, b := range map[string][]byte{"manifest.json": raw, "signature.json": sig} {
		w, _ := z.Create(n)
		w.Write(b)
	}
	z.Close()
	pkg := filepath.Join(dir, "editor.cphext")
	os.WriteFile(pkg, buf.Bytes(), 0600)
	request := Request{Profile: "full", Core: core, CLI: cli, Frontend: json.RawMessage(`{"profile":"web-full","version":"0.6.0","packages":[]}`), Version: "1.0.0", Platform: runtime.GOOS + "/" + runtime.GOARCH, Trust: trust}
	request.Packages = []string{pkg}
	fullDir := filepath.Join(dir, "full")
	full, e := Assemble(request, fullDir)
	if e != nil {
		t.Fatal(e)
	}
	if full.CoreSHA256 != hash([]byte("same compiled core")) {
		t.Fatal("assembly changed core")
	}
	if _, e = os.Stat(filepath.Join(fullDir, full.CLI)); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(fullDir, "web")); !os.IsNotExist(e) {
		t.Fatal("Web should be embedded")
	}
	bundle, e := Open(fullDir, core, "1.0.0")
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(fullDir, full.CLI), []byte("tampered"), 0600)
	if _, e = Open(fullDir, core, "1.0.0"); e == nil {
		t.Fatal("accepted tampered bundle")
	}
	os.WriteFile(filepath.Join(fullDir, full.CLI), []byte("CLI"), 0600)
	data := t.TempDir()
	manager := extension.New(data, "1.0.0", trust)
	ctx := context.Background()
	if e = manager.Load(ctx); e != nil {
		t.Fatal(e)
	}
	manager.SetPackageDirs([]string{bundle.PackageDir()})
	if e = manager.ImportMounted(ctx); e != nil {
		t.Fatal(e)
	}
	if e = manager.Uninstall(ctx, "editor"); e != nil {
		t.Fatal(e)
	}
	manager = extension.New(data, "1.0.0", trust)
	manager.Load(ctx)
	manager.SetPackageDirs([]string{bundle.PackageDir()})
	if e = manager.ImportMounted(ctx); e != nil {
		t.Fatal(e)
	}
	if _, ok := manager.State("editor"); ok {
		t.Fatal("uninstalled preinstall resurrected")
	}
	if e = os.Remove(filepath.Join(fullDir, full.Packages[0].Path)); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(fullDir, core, "1.0.0"); e != nil {
		t.Fatalf("removed offline package blocks core startup: %v", e)
	}
}

func TestAssemblyRejectsOverwriteAndUnknownProfile(t *testing.T) {
	dir := t.TempDir()
	core := filepath.Join(dir, "core")
	os.WriteFile(core, []byte("core"), 0700)
	r := Request{Core: core, CLI: core, Frontend: json.RawMessage(`{}`), Profile: "full", Version: "1.0.0", Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if _, e := Assemble(r, dir); e == nil {
		t.Fatal("overwrote existing directory")
	}
	r.Profile = "unknown"
	if _, e := Assemble(r, filepath.Join(dir, "out")); e == nil {
		t.Fatal("accepted unknown profile")
	}
}
