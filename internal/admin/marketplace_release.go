package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
)

type releaseArtifact struct {
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Format      string `json:"format,omitempty"`
}

// 新索引按发布清单选平台；只有未声明清单的历史索引才使用兼容包。
func resolveMarketArtifact(ctx context.Context, entry MarketEntry, platform string, proxy func(string) string) (MarketEntry, error) {
	if entry.ReleaseManifest == nil {
		entry.DownloadURL = proxy(entry.DownloadURL)
		return entry, nil
	}
	ref := entry.ReleaseManifest
	if err := validReleaseArtifact(*ref); err != nil {
		return entry, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy(ref.DownloadURL), nil)
	if err != nil {
		return entry, err
	}
	response, err := marketHTTPClient.Do(req)
	if err != nil {
		return entry, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return entry, fmt.Errorf("release manifest HTTP %d", response.StatusCode)
	}
	const limit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return entry, err
	}
	if len(raw) > limit {
		return entry, fmt.Errorf("release manifest exceeds size limit")
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != ref.SHA256 {
		return entry, fmt.Errorf("release manifest checksum mismatch")
	}
	var manifest struct {
		SchemaVersion   int                        `json:"schema_version"`
		Name            string                     `json:"name"`
		Version         string                     `json:"version"`
		Runtime         string                     `json:"runtime"`
		ProtocolVersion int32                      `json:"protocol_version"`
		Artifacts       map[string]releaseArtifact `json:"artifacts"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return entry, err
	}
	runtime := entry.Runtime
	if runtime == "" {
		runtime = "go"
	}
	if manifest.SchemaVersion != 1 || manifest.Name != entry.Name || manifest.Version != entry.Version || manifest.Runtime != runtime || manifest.ProtocolVersion != sdk.ProtocolVersion {
		return entry, fmt.Errorf("incompatible release manifest identity or protocol")
	}
	format := "cph-go-v1"
	if runtime == "lua" {
		platform = "any"
		format = "cph-lua-v1"
	}
	artifact, ok := manifest.Artifacts[platform]
	if !ok || artifact.Format != format {
		return entry, fmt.Errorf("no compatible plugin package for %s", platform)
	}
	if err := validReleaseArtifact(artifact); err != nil {
		return entry, err
	}
	entry.DownloadURL, entry.SHA256 = proxy(artifact.DownloadURL), artifact.SHA256
	return entry, nil
}

func validReleaseArtifact(artifact releaseArtifact) error {
	u, err := url.Parse(artifact.DownloadURL)
	if err != nil || u == nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("invalid release download URL")
	}
	digest, err := hex.DecodeString(artifact.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("invalid release checksum")
	}
	return nil
}
