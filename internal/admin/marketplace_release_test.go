package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestReleaseManifestSelectsPlatformAndVerifiesDownloads(t *testing.T) {
	payload := []byte("platform package")
	digest := sha256.Sum256(payload)
	var raw []byte
	legacyRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			w.Write(raw)
		case "/windows":
			w.Write(payload)
		default:
			legacyRequests++
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	manifest := map[string]any{"schema_version": 1, "name": "demo", "version": "1.0.0", "runtime": "go", "protocol_version": 2,
		"artifacts": map[string]releaseArtifact{"windows-amd64": {DownloadURL: server.URL + "/windows", SHA256: hex.EncodeToString(digest[:]), Format: "cph-go-v1"}}}
	raw, _ = json.Marshal(manifest)
	sum := sha256.Sum256(raw)
	entry := MarketEntry{Name: "demo", Version: "1.0.0", DownloadURL: server.URL + "/legacy", ReleaseManifest: &releaseArtifact{DownloadURL: server.URL + "/manifest.json", SHA256: hex.EncodeToString(sum[:])}}
	proxy := func(url string) string { return url }
	download, err := resolveMarketArtifact(context.Background(), entry, "windows-amd64", proxy)
	if err != nil {
		t.Fatal(err)
	}
	path, err := downloadToTemp(context.Background(), &download, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if data, _ := os.ReadFile(path); string(data) != string(payload) {
		t.Fatal("wrong platform downloaded")
	}
	if _, err := resolveMarketArtifact(context.Background(), entry, "linux-arm64", proxy); err == nil {
		t.Fatal("missing platform fell back to legacy")
	}
	entry.ReleaseManifest.SHA256 = strings.Repeat("0", 64)
	if _, err := resolveMarketArtifact(context.Background(), entry, "windows-amd64", proxy); err == nil {
		t.Fatal("tampered manifest accepted")
	}
	entry.ReleaseManifest = nil
	legacy, err := resolveMarketArtifact(context.Background(), entry, "windows-amd64", proxy)
	if err != nil || legacy.DownloadURL != entry.DownloadURL || legacyRequests != 0 {
		t.Fatal("legacy compatibility changed")
	}
	manifest["runtime"] = "lua"
	manifest["artifacts"] = map[string]releaseArtifact{"any": {DownloadURL: server.URL + "/windows", SHA256: hex.EncodeToString(digest[:]), Format: "cph-lua-v1"}}
	raw, _ = json.Marshal(manifest)
	sum = sha256.Sum256(raw)
	entry.Runtime = "lua"
	entry.ReleaseManifest = &releaseArtifact{DownloadURL: server.URL + "/manifest.json", SHA256: hex.EncodeToString(sum[:])}
	if _, err := resolveMarketArtifact(context.Background(), entry, "android-arm64", proxy); err != nil {
		t.Fatal(err)
	}
	manifest["name"] = "different"
	raw, _ = json.Marshal(manifest)
	sum = sha256.Sum256(raw)
	entry.ReleaseManifest.SHA256 = hex.EncodeToString(sum[:])
	if _, err := resolveMarketArtifact(context.Background(), entry, "android-arm64", proxy); err == nil {
		t.Fatal("wrong plugin identity accepted")
	}
}
