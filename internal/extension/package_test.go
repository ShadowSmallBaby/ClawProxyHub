package extension

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
	"os"
	"path/filepath"
	"strings"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type fixture struct {
	t     *testing.T
	key   ed25519.PrivateKey
	trust spec.TrustStore
	dir   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	return &fixture{t, key, spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(pub), Publisher: "test publisher", IDs: []string{"editor", "dependent"}, Permissions: []string{"workspace.read"}}}, t.TempDir()}
}
func (f *fixture) pack(id, version string, edit func(*spec.Manifest), extra map[string][]byte) string {
	f.t.Helper()
	asset := []byte("<h1>" + version + "</h1>")
	sum := sha256.Sum256(asset)
	m := spec.Manifest{ID: id, Name: id, Version: version, API: 1, Core: spec.VersionRange{Min: "1.0.0", MaxExclusive: "2.0.0"}, Kind: "frontend-sandbox", Target: "backend", Activation: "hot", Permissions: []string{"workspace.read"}, Files: map[string]string{"frontend/index.html": hex.EncodeToString(sum[:])}}
	if edit != nil {
		edit(&m)
	}
	raw, _ := json.Marshal(m)
	sig, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(f.key, raw))})
	files := map[string][]byte{"manifest.json": raw, "signature.json": sig, "frontend/index.html": asset}
	for k, v := range extra {
		files[k] = v
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for k, v := range files {
		entry, _ := w.Create(k)
		_, _ = entry.Write(v)
	}
	_ = w.Close()
	file := filepath.Join(f.dir, id+version+".cphext")
	if err := os.WriteFile(file, b.Bytes(), 0600); err != nil {
		f.t.Fatal(err)
	}
	return file
}
func TestVerificationRejectsTamperingAndUnsafePaths(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*spec.Manifest)
		extra map[string][]byte
	}{
		{name: "tampered", extra: map[string][]byte{"frontend/index.html": []byte("replaced")}},
		{name: "unsigned", extra: map[string][]byte{"extra.js": []byte("extra")}},
		{name: "traversal", extra: map[string][]byte{"../outside": []byte("escape")}},
		{name: "windows", extra: map[string][]byte{"CON.txt": []byte("reserved")}},
		{name: "case-duplicate", extra: map[string][]byte{"Frontend/index.html": []byte("case")}},
		{name: "permission", edit: func(m *spec.Manifest) { m.Permissions = []string{"secrets.read"} }},
		{name: "publisher", edit: func(m *spec.Manifest) { m.ID = "untrusted" }},
		{name: "native", edit: func(m *spec.Manifest) { m.Kind = "runtime" }},
		{name: "range", edit: func(m *spec.Manifest) { m.Core.Min = "2.0.0" }},
		{name: "reserved-archive", edit: func(m *spec.Manifest) { m.Files["package.cphext"] = strings.Repeat("0", 64) }},
		{name: "reserved-host-archive", edit: func(m *spec.Manifest) { m.Files["package.cphhost"] = strings.Repeat("0", 64) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			if _, err := Verify(f.pack("editor", "1.0.0", c.edit, c.extra), f.trust); err == nil {
				t.Fatal("accepted invalid package")
			}
		})
	}
	f := newFixture(t)
	file := f.pack("editor", "1.0.0", nil, nil)
	if _, err := Verify(file, f.trust); err != nil {
		t.Fatal(err)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	k := f.trust["test"]
	k.PublicKey = base64.StdEncoding.EncodeToString(pub)
	f.trust["test"] = k
	if _, err := Verify(file, f.trust); err == nil {
		t.Fatal("accepted wrong signer")
	}
}

func TestVerificationReportsManifestAndSignatureErrorsSeparately(t *testing.T) {
	for _, test := range []struct {
		name, file, raw, want string
	}{
		{"business-runtime-field", "manifest.json", `{"runtime":"host"}`, `invalid extension manifest: json: unknown field "runtime"`},
		{"business-protocol-field", "manifest.json", `{"protocol_version":2}`, `invalid extension manifest: json: unknown field "protocol_version"`},
		{"broken-signature", "signature.json", `{`, "invalid extension signature metadata:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			_, err := Verify(f.pack("editor", "1.0.0", nil, map[string][]byte{test.file: []byte(test.raw)}), f.trust)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestFailedValidationPreservesActiveVersion(t *testing.T) {
	f := newFixture(t)
	m := New(filepath.Join(f.dir, "installed"), "1.5.2", f.trust)
	ctx := context.Background()
	stops := 0
	m.SetLifecycle(func(context.Context, State) error { return nil }, func(context.Context, string) error { stops++; return nil })
	old, err := m.Install(ctx, f.pack("editor", "1.0.0", nil, nil), []string{"workspace.read"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetValidator(func(context.Context, State) error { return errors.New("runtime cannot start") })
	if _, err := m.Install(ctx, f.pack("editor", "1.1.0", nil, nil), []string{"workspace.read"}); err == nil {
		t.Fatal("invalid candidate installed")
	}
	current, _ := m.State("editor")
	if current.Hash != old.Hash || !current.Available || stops != 0 {
		t.Fatal("invalid candidate interrupted the active version")
	}
	if err := m.SetEnabled(ctx, "editor", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled(ctx, "editor", true); err == nil {
		t.Fatal("enable skipped runtime validation")
	}
}

func TestLocalizedMetadataSurvivesSignedPackageVerification(t *testing.T) {
	f := newFixture(t)
	file := f.pack("editor", "1.0.0", func(m *spec.Manifest) {
		m.Label = map[string]string{"zh": "Lua 编辑器", "en": "Lua Editor"}
		m.Desc = map[string]string{"zh": "编辑 Lua 源码", "en": "Edit Lua source"}
		m.Environments = []string{"app", "web-desktop", "web-mobile"}
		m.Pages = []spec.Page{{ID: "editor", Title: "Editor", Labels: map[string]string{"zh": "编辑器", "en": "Editor"}, Entry: "frontend/index.html"}}
		m.Contributions = []spec.Contribution{{ID: "new", Location: "plugins.toolbar", Label: "New", Labels: map[string]string{"zh": "新建", "en": "New"}, Page: "editor"}}
		m.Actions = []spec.Action{{ID: "read", Target: "core.workspace.read", Title: "Read source", Labels: map[string]string{"zh": "读取源码", "en": "Read source"}}}
	}, nil)
	verified, err := Verify(file, f.trust)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Manifest.Label["zh"] != "Lua 编辑器" || verified.Manifest.Desc["en"] != "Edit Lua source" || verified.Manifest.Contributions[0].Labels["zh"] != "新建" {
		t.Fatal("localized signed metadata was lost")
	}
	if verified.Manifest.Pages[0].Labels["zh"] != "编辑器" || verified.Manifest.Actions[0].Labels["en"] != "Read source" {
		t.Fatal("localized page or action names were lost")
	}
	if strings.Join(verified.Manifest.Environments, ",") != "app,web-desktop,web-mobile" {
		t.Fatal("signed environments were lost")
	}
}

func TestStartupValidationFailurePersistsUnavailableState(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.dir, "installed")
	m := New(dir, "1.5.2", f.trust)
	ctx := context.Background()
	if _, err := m.Install(ctx, f.pack("editor", "1.0.0", nil, nil), []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
	restarted := New(dir, "1.5.2", f.trust)
	restarted.SetValidator(func(context.Context, State) error { return errors.New("runtime cannot start") })
	if err := restarted.Load(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "extensions", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var states map[string]State
	if err = json.Unmarshal(raw, &states); err != nil {
		t.Fatal(err)
	}
	if states["editor"].Available || states["editor"].Status != "failed" {
		t.Fatal("platform loaders can still observe the stale active state")
	}
}
func TestLifecycleDependenciesRollbackAndTamperedAsset(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	m := New(dir, "1.5.2", f.trust)
	if err := m.Load(ctx); err != nil {
		t.Fatal(err)
	}
	file := f.pack("editor", "1.0.0", nil, nil)
	if _, err := m.Install(ctx, file, nil); err == nil {
		t.Fatal("missing grant accepted")
	}
	old, err := m.Install(ctx, file, []string{"workspace.read"})
	if err != nil {
		t.Fatal(err)
	}
	// 回调读取管理器不会死锁；失败更新保留旧版本和资源。
	m.SetLifecycle(func(ctx context.Context, s State) error {
		m.List()
		if s.Manifest.Version == "1.1.0" {
			return errors.New("activation failed")
		}
		return nil
	}, func(context.Context, string) error { return nil })
	if _, err = m.Install(ctx, f.pack("editor", "1.1.0", nil, nil), []string{"workspace.read"}); err == nil {
		t.Fatal("expected activation failure")
	}
	s, _ := m.State("editor")
	if s.Hash != old.Hash || !s.Available {
		t.Fatalf("rollback failed: %+v", s)
	}
	if _, err = m.ReadAsset("editor", s.Hash, "frontend/index.html"); err != nil {
		t.Fatal(err)
	}
	dependent := f.pack("dependent", "1.0.0", func(m *spec.Manifest) {
		m.Dependencies = map[string]spec.VersionRange{"editor": {MaxExclusive: "1.1.0"}}
	}, nil)
	if _, err = m.Install(ctx, dependent, []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
	if err = m.SetEnabled(ctx, "editor", false); err == nil {
		t.Fatal("disabled dependency")
	}
	if _, err = m.Install(ctx, f.pack("editor", "1.2.0", nil, nil), []string{"workspace.read"}); err == nil {
		t.Fatal("upgraded past dependent range")
	}
	cycle := f.pack("editor", "1.0.1", func(m *spec.Manifest) { m.Dependencies = map[string]spec.VersionRange{"dependent": {}} }, nil)
	if _, err = m.Install(ctx, cycle, []string{"workspace.read"}); err == nil {
		t.Fatal("accepted cycle")
	}
	if err = m.Uninstall(ctx, "dependent"); err != nil {
		t.Fatal(err)
	}
	if err = m.SetEnabled(ctx, "editor", false); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ReadAsset("editor", s.Hash, "frontend/index.html"); err == nil {
		t.Fatal("disabled asset accessible")
	}
	if err = m.SetEnabled(ctx, "editor", true); err != nil {
		t.Fatal(err)
	}
	restarted := New(dir, "1.5.2", f.trust)
	if err = restarted.Load(ctx); err != nil {
		t.Fatal(err)
	}
	s, _ = restarted.State("editor")
	if !s.Available {
		t.Fatal(s)
	}
	asset := filepath.Join(dir, "extensions", "versions", "editor", s.Hash, "frontend", "index.html")
	if err = os.WriteFile(asset, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.ReadAsset("editor", s.Hash, "frontend/index.html"); err == nil {
		t.Fatal("tampered installed asset accepted")
	}
	data := filepath.Join(dir, "data", "editor", "draft.txt")
	if err = os.MkdirAll(filepath.Dir(data), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(data, []byte("keep my draft"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Uninstall(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "extensions", "versions", "editor")); !os.IsNotExist(err) {
		t.Fatal("uninstalled code remains on disk")
	}
	if content, err := os.ReadFile(data); err != nil || string(content) != "keep my draft" {
		t.Fatal("uninstall removed user data")
	}
	if err = restarted.CleanCache(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
}
func TestPendingActivationAndRollbackFailure(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	m := New(dir, "1.5.2", f.trust)
	_ = m.Load(ctx)
	pending := f.pack("editor", "1.0.0", func(m *spec.Manifest) { m.Activation = "system-install" }, nil)
	if _, err := m.Install(ctx, pending, []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
	restarted := New(dir, "1.5.2", f.trust)
	if err := restarted.Load(ctx); err != nil {
		t.Fatal(err)
	}
	s, _ := restarted.State("editor")
	if s.Status != "pending-system-install" || !s.Enabled || s.Available {
		t.Fatalf("pending state lost: %+v", s)
	}
	_ = m.Uninstall(ctx, "editor")
	if _, err := m.Install(ctx, f.pack("editor", "1.0.1", nil, nil), []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
	m.SetLifecycle(func(context.Context, State) error { return errors.New("executor unavailable") }, nil)
	if _, err := m.Install(ctx, f.pack("editor", "1.0.2", nil, nil), []string{"workspace.read"}); err == nil {
		t.Fatal("expected failure")
	}
	s, _ = m.State("editor")
	if s.Available || s.Status != "failed" || !strings.Contains(s.Error, "rollback") {
		t.Fatalf("rollback failure not reported: %+v", s)
	}
}
