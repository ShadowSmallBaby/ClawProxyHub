package admin

import (
	"encoding/json"
	"testing"
)

func TestOfflineMarketIndex(t *testing.T) {
	var entries []MarketEntry
	if err := json.Unmarshal(offlineMarketJSON, &entries); err != nil || len(entries) == 0 {
		t.Fatalf("offline market broken: %v, len=%d", err, len(entries))
	}
	for _, e := range entries {
		if e.Name == "" || e.DownloadURL == "" {
			t.Fatalf("bad entry: %+v", e)
		}
	}
}

func TestIsGitHubURL(t *testing.T) {
	for _, u := range []string{
		"https://github.com/ShadowSmallBaby/ClawProxyHubPlugins/releases/latest/download/x.cphplugin",
		"https://raw.githubusercontent.com/ShadowSmallBaby/ClawProxyHubPlugins/main/index.json",
		"https://objects.githubusercontent.com/x",
	} {
		if !isGitHubURL(u) {
			t.Fatalf("should be github url: %s", u)
		}
	}
	for _, u := range []string{"https://example.com/x", "http://127.0.0.1:8080/y", "not-a-url"} {
		if isGitHubURL(u) {
			t.Fatalf("should not be github url: %s", u)
		}
	}
}
