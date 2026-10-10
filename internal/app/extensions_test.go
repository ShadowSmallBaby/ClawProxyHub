package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/control"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestMountedExtensionIsInstalledBeforeServing(t *testing.T) {
	cfg := testConfig(t)
	packages := filepath.Join(cfg.DataDir, "packages")
	if err := os.MkdirAll(packages, 0700); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := spec.TrustStore{"test": {PublicKey: base64.StdEncoding.EncodeToString(public), IDs: []string{"example"}}}
	manifest := spec.Manifest{ID: "example", Version: "0.1.0", Name: "Example", API: 1, Kind: "data", Target: "backend", Activation: "hot", Files: map[string]string{}}
	raw, _ := json.Marshal(manifest)
	sig, _ := json.Marshal(spec.Signature{KeyID: "test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))})
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	for name, data := range map[string][]byte{"manifest.json": raw, "signature.json": sig} {
		entry, err := archive.Create(name)
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
	if err = os.WriteFile(filepath.Join(packages, "example.cphext"), content.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(cfg, WithExtensionTrust(trust))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Shutdown(ctx) })
	state, ok := a.extensions.State("example")
	if !ok || !state.Available {
		t.Fatal("mounted package was not activated")
	}
	token, err := a.DeviceSession()
	if err != nil {
		t.Fatal(err)
	}
	client, err := control.New("http://"+a.Address(), token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Request(ctx, "GET", "/admin/extensions/catalog", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Invoke(ctx, "core.workspace.scaffold", nil); err != nil {
		t.Fatal(err)
	}
}

func TestManagedTrustSurvivesRestartAndRequiresFullAdmin(t *testing.T) {
	cfg := testConfig(t)
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := spec.TrustKey{PublicKey: base64.StdEncoding.EncodeToString(public), Publisher: "Example", IDs: []string{"example"}, Permissions: []string{}}
	fixed := spec.TrustStore{"distribution": key}
	ctx := context.Background()
	start := func() (*App, *control.Client) {
		t.Helper()
		a, err := New(cfg, WithExtensionTrust(fixed))
		if err != nil {
			t.Fatal(err)
		}
		if err = a.Start(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = a.Shutdown(ctx) })
		token, err := a.DeviceSession()
		if err != nil {
			t.Fatal(err)
		}
		client, err := control.New("http://"+a.Address(), token)
		if err != nil {
			t.Fatal(err)
		}
		return a, client
	}
	a, client := start()
	body, _ := json.Marshal(key)
	request := func(client *control.Client, method, path string, body []byte, want int) []byte {
		t.Helper()
		data, err := client.Request(ctx, method, path, "application/json", bytes.NewReader(body))
		if want == 200 {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			var failure *control.HTTPError
			if !errors.As(err, &failure) || failure.Status != want {
				t.Fatalf("%s %s: %v, want %d", method, path, err, want)
			}
		}
		return data
	}
	request(client, "PUT", "/admin/extension-trust/author", body, 200)
	request(client, "PUT", "/admin/extension-trust/distribution", body, 400)
	request(client, "PUT", "/admin/extension-trust/invalid", []byte(`{"public_key":"bad"}`), 400)
	raw := request(client, "POST", "/admin/tokens/scoped", []byte(`{"scopes":["extensions.manage"],"ttl_seconds":60}`), 200)
	var session struct{ Token string }
	if err = json.Unmarshal(raw, &session); err != nil {
		t.Fatal(err)
	}
	scoped, _ := control.New(client.URL, session.Token)
	request(scoped, "PUT", "/admin/extension-trust/author", body, 403)
	request(scoped, "DELETE", "/admin/extension-trust/author", nil, 403)
	request(scoped, "GET", "/admin/extension-trust", nil, 403)
	unauthenticated := *client
	unauthenticated.Token = ""
	request(&unauthenticated, "GET", "/admin/extension-trust", nil, http.StatusUnauthorized)
	if err = a.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	_, client = start()
	var response struct{ Identities []extension.TrustEntry }
	if err = json.Unmarshal(request(client, "GET", "/admin/extension-trust", nil, 200), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Identities) != 2 || response.Identities[0].Code != "author" || response.Identities[0].ReadOnly || !response.Identities[1].ReadOnly {
		t.Fatalf("signing identities lost or changed after restart: %+v", response.Identities)
	}
	request(client, "DELETE", "/admin/extension-trust/author", nil, 200)
	if strings.Contains(string(request(client, "GET", "/admin/extension-trust", nil, 200)), `"author"`) {
		t.Fatal("deleted identity is still trusted")
	}
	if _, err = os.Stat(filepath.Join(cfg.DataDir, "extensions-trust.json")); !os.IsNotExist(err) {
		t.Fatal("managed trust should be persisted in the database")
	}
}

func TestInvalidExtensionTrustIsRejected(t *testing.T) {
	cfg := testConfig(t)
	trust := filepath.Join(cfg.DataDir, "extensions-trust.json")
	if err := os.WriteFile(trust, []byte("invalid trust"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Start(context.Background()); err == nil {
		t.Fatal("invalid trust accepted")
	}
	if data, err := os.ReadFile(trust); err != nil || string(data) != "invalid trust" {
		t.Fatal("trust configuration was modified")
	}
}
