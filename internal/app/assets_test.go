package app

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginIconWithoutEmbeddedWeb(t *testing.T) {
	cfg := testConfig(t)
	directory := filepath.Join(cfg.PluginDir, "demo")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"manifest.json": `{"name":"demo","version":"1.0.0","icon":"icon.png"}`, "icon.png": "icon-data"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := New(cfg, WithoutWeb())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })
	response, err := http.Get("http://" + a.Address() + "/assets/plugins/demo/icon")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || string(data) != "icon-data" {
		t.Fatalf("icon response %d: %s", response.StatusCode, data)
	}
}
