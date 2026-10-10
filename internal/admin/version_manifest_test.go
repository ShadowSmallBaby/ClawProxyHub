package admin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUpdateManifestUsesDesktopFieldsAndConfiguredRepository(t *testing.T) {
	repository := "https://github.com/example/fork"
	raw := `{"schema_version":1,"tag":"v2.0.0","version":"1.6.0","release_url":"` + repository + `/releases/tag/v2.0.0",
		"artifacts":{"linux/amd64":{"full":{"name":"cph-linux-amd64-full.zip","download_url":"` + repository + `/releases/download/v2.0.0/cph-linux-amd64-full.zip","sha256":"` + strings.Repeat("a", 64) + `","size":123}}},
		"platform":{"android":{"version":"99.0.0","version_code":999}},"changelog":{"items":[{"en":"Desktop change"}]}}`
	var manifest remoteManifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatal(err)
	}
	if !manifest.valid(repository) || manifest.Version != "1.6.0" || manifest.Changelog.Items[0]["en"] != "Desktop change" {
		t.Fatal("desktop metadata was not selected")
	}
	if manifest.valid("https://github.com/another/project") {
		t.Fatal("accepted another release repository")
	}
	for _, corrupt := range []func(*remoteManifest){
		func(m *remoteManifest) { m.Schema = 2 },
		func(m *remoteManifest) { m.Version = "invalid" },
		func(m *remoteManifest) { m.Tag = "../another" },
		func(m *remoteManifest) { m.ReleaseURL = "https://example.com" },
		func(m *remoteManifest) {
			a := m.Artifacts["linux/amd64"]["full"]
			a.SHA256 = "bad"
			m.Artifacts["linux/amd64"]["full"] = a
		},
		func(m *remoteManifest) {
			a := m.Artifacts["linux/amd64"]["full"]
			a.DownloadURL = "https://example.com/file"
			m.Artifacts["linux/amd64"]["full"] = a
		},
	} {
		var candidate remoteManifest
		_ = json.Unmarshal([]byte(raw), &candidate)
		corrupt(&candidate)
		if candidate.valid(repository) {
			t.Fatal("accepted invalid update metadata")
		}
	}
}
