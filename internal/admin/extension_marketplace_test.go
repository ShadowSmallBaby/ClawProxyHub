package admin

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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type extensionTransport func(*http.Request) (*http.Response, error)

func (f extensionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestExtensionIndexCachesOfflineAndVerifiesDownloads(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { _ = sql.Close() })
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := spec.Manifest{ID: "example", Version: "0.1.0", Name: "Example", API: 1, Kind: "data", Target: "backend", Activation: "hot", Files: map[string]string{}}
	manifest.Environments = []string{"web-mobile"}
	raw, _ := json.Marshal(manifest)
	signature, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))})
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	for name, data := range map[string][]byte{"manifest.json": raw, "signature.json": signature} {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	payload := content.Bytes()
	sum := sha256.Sum256(payload)
	s := &Server{dataDir: dir, settings: setting.New(db), extensions: extension.New(dir, version.Core,
		spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), Publisher: "Test", IDs: []string{"example"}}})}
	item := releasePackage{ID: "example", Version: "0.1.0", Kind: "data", Target: "backend", updateArtifact: updateArtifact{Name: "example-0.1.0.cphext", Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])}}
	item.Environments = manifest.Environments
	item.DownloadURL = version.UpdateRepository + "/releases/download/v1.5.2/" + item.Name
	app := releasePackage{ID: "frontend.app", Version: "0.6.0", Kind: "frontend-trusted", Target: "client", updateArtifact: updateArtifact{
		Name: "app-0.6.0.cphui", Size: 1024, SHA256: strings.Repeat("b", 64)}}
	app.DownloadURL = version.UpdateRepository + "/releases/download/v1.5.2/" + app.Name
	remote := remoteManifest{Schema: 1, Tag: "v1.5.2", Version: "1.5.2", ReleaseURL: version.UpdateRepository + "/releases/tag/v1.5.2", Packages: []releasePackage{item, app}}
	remoteJSON, _ := json.Marshal(remote)
	online := true
	oldIndex, oldDownload := marketHTTPClient, downloadHTTPClient
	t.Cleanup(func() { marketHTTPClient, downloadHTTPClient = oldIndex, oldDownload })
	client := &http.Client{Transport: extensionTransport(func(r *http.Request) (*http.Response, error) {
		if !online {
			return nil, fmt.Errorf("offline")
		}
		var data []byte
		switch r.URL.String() {
		case version.UpdateManifestURL():
			data = remoteJSON
		case item.DownloadURL:
			data = payload
		default:
			return nil, fmt.Errorf("unexpected download: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})}
	marketHTTPClient, downloadHTTPClient = client, client
	if _, cached, err := s.extensionIndex(ctx, true); err != nil || cached {
		t.Fatalf("fetch index: cached=%v, %v", cached, err)
	}
	online = false
	s.settings = setting.New(db)
	if index, cached, err := s.extensionIndex(ctx, true); err != nil || !cached || len(index.Manifest.Packages) != 2 {
		t.Fatalf("offline persisted index: cached=%v, %v", cached, err)
	}
	request := httptest.NewRequest("GET", "/admin/extensions/marketplace", nil)
	request = request.WithContext(context.WithValue(request.Context(), ctxKeyRole{}, "admin"))
	response := httptest.NewRecorder()
	s.extensionMarketplace(response, request)
	var catalog struct{ Packages []releasePackage }
	if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil || response.Code != 200 || len(catalog.Packages) != 1 || catalog.Packages[0].ID != item.ID {
		t.Fatalf("client interface leaked into backend catalog: %s", response.Body.String())
	}
	online = true
	inspect := func(hash string, want int) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/admin/extensions/inspect-market", strings.NewReader(`{"sha256":"`+hash+`"}`))
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyRole{}, "admin"))
		w := httptest.NewRecorder()
		s.inspectMarketExtension(w, r)
		if w.Code != want {
			t.Fatalf("inspect status %d, want %d: %s", w.Code, want, w.Body.String())
		}
		var result struct{ Ticket string }
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return result.Ticket
	}
	inspect(strings.Repeat("0", 64), 400)
	inspect(app.SHA256, 400)
	ticket := inspect(item.SHA256, 200)
	if ticket == "" {
		t.Fatal("missing verified upload ticket")
	}
	if installed, err := s.extensions.InstallUpload(ctx, ticket, []string{}); err != nil || installed.Manifest.ID != item.ID || strings.Join(installed.Manifest.Environments, ",") != "web-mobile" {
		t.Fatalf("install verified download: %v", err)
	}
	if _, err := s.extensions.InstallUpload(ctx, ticket, []string{}); err == nil {
		t.Fatal("upload ticket was reusable")
	}
	remote.Packages[0].SHA256 = strings.Repeat("c", 64)
	remoteJSON, _ = json.Marshal(remote)
	request = httptest.NewRequest("GET", "/admin/extensions/marketplace?refresh=true", nil)
	request = request.WithContext(context.WithValue(request.Context(), ctxKeyRole{}, "admin"))
	response = httptest.NewRecorder()
	s.extensionMarketplace(response, request)
	var updates struct {
		Packages []struct {
			Status           string `json:"status"`
			InstalledVersion string `json:"installed_version"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &updates); err != nil || response.Code != 200 || len(updates.Packages) != 1 || updates.Packages[0].Status != "update" || updates.Packages[0].InstalledVersion != item.Version {
		t.Fatalf("same-version update missing from online catalog: %s", response.Body.String())
	}
	remote.Packages[0] = item
	remoteJSON, _ = json.Marshal(remote)
	if _, _, err := s.extensionIndex(ctx, true); err != nil {
		t.Fatal(err)
	}
	payload = bytes.Repeat([]byte("x"), len(payload))
	inspect(item.SHA256, 400)
	cached := cachedExtensionCatalog{URL: "https://github.com/another/fork/releases/latest/download/update-manual.json", Manifest: remote}
	wrongSource, _ := json.Marshal(cached)
	if err := s.settings.Set(setting.KeyExtensionCatalog, string(wrongSource)); err != nil {
		t.Fatal(err)
	}
	online = false
	if _, _, err := s.extensionIndex(ctx, false); err == nil {
		t.Fatal("offline index from another repository accepted")
	}
}

func TestReleasePackageValidationAndPlatforms(t *testing.T) {
	repo := "https://github.com/example/project"
	item := releasePackage{ID: "lua-runtime", Version: "0.2.0", Kind: "runtime", Target: "backend", updateArtifact: updateArtifact{
		Name: "lua-runtime.cphhost", DownloadURL: repo + "/releases/download/v1.0.0/lua-runtime.cphhost", SHA256: strings.Repeat("a", 64), Size: 1024}}
	if !item.valid(repo, "v1.0.0") || !item.onThisPlatform() {
		t.Fatal("valid universal package rejected")
	}
	item.Platforms = []string{runtime.GOOS + "/" + runtime.GOARCH}
	if !item.onThisPlatform() {
		t.Fatal("native package hidden")
	}
	item.Platforms = []string{"another/platform"}
	if item.onThisPlatform() {
		t.Fatal("foreign package offered")
	}
	for _, mutate := range []func(*releasePackage){
		func(v *releasePackage) { v.DownloadURL = "https://other.example/package" },
		func(v *releasePackage) { v.Name = "../lua-runtime.cphhost" },
		func(v *releasePackage) { v.SHA256 = "bad" },
		func(v *releasePackage) { v.Size = extension.MaxPackageBytes + 1 },
		func(v *releasePackage) { v.Version = "unknown" },
		func(v *releasePackage) { v.Environments = []string{} },
		func(v *releasePackage) { v.Environments = []string{"web"} },
		func(v *releasePackage) { v.Environments = []string{"app", "app"} },
	} {
		bad := item
		mutate(&bad)
		if bad.valid(repo, "v1.0.0") {
			t.Fatal("invalid package metadata accepted")
		}
	}
}
