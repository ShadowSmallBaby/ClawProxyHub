package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/control"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extstore"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestGoExtensionLifecycleAndStorage(t *testing.T) {
	ctx := context.Background()
	executable := filepath.Join(t.TempDir(), "service")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.Command("go", "build", "-trimpath", "-o", executable, "./testdata/extension-service")
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "CGO_ENABLED" && key != "GOOS" && key != "GOARCH" && key != "GOWORK" {
			build.Env = append(build.Env, entry)
		}
	}
	build.Env = append(build.Env, "CGO_ENABLED=0", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test backend: %v\n%s", err, output)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	permissions := []string{"service.execute", "storage.read", "storage.write"}
	trust := spec.TrustStore{"service-test": {PublicKey: base64.StdEncoding.EncodeToString(public), IDs: []string{"service-a", "service-b"}, Permissions: permissions, Native: true}}
	makeManifest := func(id, version string, schema int, tables ...string) spec.Manifest {
		manifest := spec.Manifest{ID: id, Name: "Service test", Version: version, API: 1, Kind: "service", Target: "backend", Activation: "hot", Execution: "trusted-process", Permissions: permissions,
			Backend: &spec.Backend{Language: "go", Protocol: 1, Entries: map[string]string{}, BackgroundPermissions: []string{"storage.read", "storage.write"}},
			Storage: &spec.StorageSchema{Version: schema}, Files: map[string]string{}}
		for _, name := range tables {
			manifest.Storage.Tables = append(manifest.Storage.Tables, spec.StorageTable{Name: name, Columns: []spec.StorageColumn{{Name: "id", Type: "string", PrimaryKey: true}, {Name: "value", Type: "string"}}})
		}
		for _, id := range []string{"read", "write", "batch", "inspect", "remember", "reuse", "wait", "crash"} {
			permission, effect := "storage.read", "read"
			if id == "write" || id == "batch" {
				permission, effect = "storage.write", "write"
			}
			if id == "crash" {
				permission, effect = "service.execute", "execute"
			}
			manifest.Actions = append(manifest.Actions, spec.Action{ID: id, Title: id, Permission: permission, Effect: effect, TimeoutMS: 5000, Input: &spec.Schema{Type: "object", AdditionalProperties: true}, Output: &spec.Schema{Type: "object", AdditionalProperties: true}})
		}
		return manifest
	}
	pack := func(manifest spec.Manifest) string {
		t.Helper()
		files := map[string][]byte{}
		// 本测试只执行当前平台；其他入口复用同一测试程序以覆盖清单矩阵。
		for _, platform := range spec.DesktopPlatforms {
			entry := "backend/" + strings.ReplaceAll(platform, "/", "-") + "/service"
			if strings.HasPrefix(platform, "windows/") {
				entry += ".exe"
			}
			manifest.Backend.Entries[platform] = entry
			files[entry] = binary
			hash := sha256.Sum256(binary)
			manifest.Files[entry] = hex.EncodeToString(hash[:])
		}
		raw, _ := json.Marshal(manifest)
		signature, _ := json.Marshal(spec.Signature{KeyID: "service-test", Value: base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))})
		files["manifest.json"], files["signature.json"] = raw, signature
		var buffer bytes.Buffer
		archive := zip.NewWriter(&buffer)
		for name, data := range files {
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
		path := filepath.Join(t.TempDir(), "test.cphext")
		if err = os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cfg := testConfig(t)
	var app *App
	var client *control.Client
	start := func() {
		t.Helper()
		var err error
		app, err = New(cfg, WithExtensionTrust(trust))
		if err != nil {
			t.Fatal(err)
		}
		if err = app.Start(ctx); err != nil {
			t.Fatal(err)
		}
		running := app
		t.Cleanup(func() { _ = running.Shutdown(ctx) })
		token, err := app.DeviceSession()
		if err != nil {
			t.Fatal(err)
		}
		client, err = control.New("http://"+app.Address(), token)
		if err != nil {
			t.Fatal(err)
		}
	}
	start()
	first := makeManifest("service-a", "0.1.0", 1, "notes", "legacy")
	firstPackage := pack(first)
	for _, path := range []string{firstPackage, pack(makeManifest("service-b", "0.1.0", 1, "notes", "legacy"))} {
		if _, err := client.Install(ctx, path, permissions); err != nil {
			t.Fatal(err)
		}
	}
	invoke := func(id, input string) json.RawMessage {
		t.Helper()
		result, err := client.Invoke(ctx, id, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		return result
	}
	contains := func(id, input, wanted string) {
		t.Helper()
		if result := invoke(id, input); !strings.Contains(string(result), wanted) {
			t.Fatalf("%s: got %s, want %s", id, result, wanted)
		}
	}
	query := `{"op":"query","table":"notes"}`
	contains("service-a.inspect", `{}`, `"background_alive":true`)
	invoke("service-a.write", `{"op":"insert","table":"notes","values":{"id":"a","value":"before backup"}}`)
	invoke("service-a.write", `{"op":"insert","table":"legacy","values":{"value":"retained legacy data"}}`)
	invoke("service-b.write", `{"op":"insert","table":"notes","values":{"id":"b","value":"other extension"}}`)
	contains("service-a.read", query, "before backup")
	if result := invoke("service-b.read", query); strings.Contains(string(result), "before backup") {
		t.Fatal("storage leaked across extensions")
	}
	for _, input := range []string{`{"op":"insert","table":"notes","values":{"value":"read action write"}}`, `{"op":"query","table":"accounts"}`, `{"op":"query","table":"notes","columns":["_cph_size"]}`, `{"op":"query","table":"notes","extension_id":"service-b"}`} {
		if _, err := client.Invoke(ctx, "service-a.read", json.RawMessage(input)); err == nil {
			t.Fatalf("unauthorized storage request accepted: %s", input)
		}
	}
	raw, err := client.Request(ctx, http.MethodPost, "/admin/tokens/scoped", "application/json", strings.NewReader(`{"scopes":["storage.read"],"ttl_seconds":60}`))
	if err != nil {
		t.Fatal(err)
	}
	var token struct{ Token string }
	if err = json.Unmarshal(raw, &token); err != nil {
		t.Fatal(err)
	}
	scoped, err := control.New(client.URL, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = scoped.Invoke(ctx, "service-a.read", json.RawMessage(query)); err != nil {
		t.Fatal(err)
	}
	if _, err = scoped.Invoke(ctx, "service-a.write", json.RawMessage(`{"op":"insert","table":"notes","values":{"value":"scope escape"}}`)); err == nil {
		t.Fatal("scoped caller gained storage.write")
	}
	if _, err = client.Invoke(ctx, "service-a.batch", json.RawMessage(`{"requests":[{"op":"insert","table":"notes","values":{"value":"partial batch"}},{"op":"query","table":"undeclared"}]}`)); err == nil {
		t.Fatal("invalid batch accepted")
	}
	if result := invoke("service-a.read", query); strings.Contains(string(result), "partial batch") {
		t.Fatal("failed batch partially committed")
	}
	invoke("service-a.remember", `{}`)
	if _, err = client.Invoke(ctx, "service-a.reuse", nil); err == nil {
		t.Fatal("expired invocation capability reused")
	}
	waitFor := func(condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for extension lifecycle")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	call, cancel := context.WithCancel(ctx)
	completed := make(chan error, 1)
	go func() { _, err := client.Invoke(call, "service-a.wait", nil); completed <- err }()
	waitFor(func() bool { return strings.Contains(string(invoke("service-a.inspect", `{}`)), `"waiting":1`) })
	cancel()
	if err = <-completed; err == nil {
		t.Fatal("canceled invocation succeeded")
	}
	waitFor(func() bool { return strings.Contains(string(invoke("service-a.inspect", `{}`)), `"waiting":0`) })
	old, _ := app.extensions.State("service-a")
	failed := makeManifest("service-a", "0.2.0", 2, "notes", "candidate")
	missing := failed.Actions[0]
	missing.ID = "missing"
	failed.Actions = append(failed.Actions, missing)
	if _, err = client.Install(ctx, pack(failed), permissions); err == nil {
		t.Fatal("backend without declared handler installed")
	}
	if state, _ := app.extensions.State("service-a"); state.Hash != old.Hash || !state.Available {
		t.Fatalf("failed upgrade did not restore old backend: %+v", state)
	}
	contains("service-a.read", `{"op":"query","table":"legacy"}`, "retained legacy data")
	obsolete, err := app.extensions.ObsoleteTables(ctx, "service-a", false, "")
	if err != nil || len(obsolete.Tables) != 0 {
		t.Fatalf("failed installation changed obsolete markers: %+v %v", obsolete, err)
	}
	if _, err = client.Install(ctx, pack(makeManifest("service-a", "0.2.0", 3, "notes")), permissions); err != nil {
		t.Fatal(err)
	}
	obsolete, err = app.extensions.ObsoleteTables(ctx, "service-a", false, "")
	if err != nil || !reflect.DeepEqual(obsolete.Tables, []string{"legacy"}) {
		t.Fatalf("wrong obsolete tables after successful upgrade: %+v %v", obsolete, err)
	}
	if _, err = client.Install(ctx, firstPackage, permissions); err == nil || !strings.Contains(err.Error(), "downgrades") {
		t.Fatalf("active downgrade was not rejected: %v", err)
	}
	if _, err = app.extensions.ObsoleteTables(ctx, "service-a", true, old.Hash); err == nil {
		t.Fatal("obsolete cleanup accepted a stale package hash")
	}
	backup := extensionBackupRequest(t, client, nil, http.StatusOK)
	archive, err := zip.NewReader(bytes.NewReader(backup), int64(len(backup)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, entry := range archive.File {
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name], err = io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"cph.db", "cph.ext.db", "secret.key"} {
		if len(files[name]) == 0 {
			t.Fatalf("system backup omitted %s", name)
		}
	}
	snapshot := filepath.Join(t.TempDir(), "cph.ext.db")
	if err = os.WriteFile(snapshot, files["cph.ext.db"], 0600); err != nil {
		t.Fatal(err)
	}
	if err = extstore.ValidateBackup(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	invoke("service-a.write", `{"op":"update","table":"notes","where":[{"column":"id","op":"eq","value":"a"}],"values":{"value":"after backup"}}`)
	extensionBackupRequest(t, client, backup, http.StatusOK)
	var invalid bytes.Buffer
	invalidZIP := zip.NewWriter(&invalid)
	for name, content := range map[string][]byte{"cph.db": files["cph.db"], "secret.key": files["secret.key"], "cph.ext.db": []byte("invalid SQLite")} {
		writer, err := invalidZIP.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err = invalidZIP.Close(); err != nil {
		t.Fatal(err)
	}
	extensionBackupRequest(t, client, invalid.Bytes(), http.StatusBadRequest)
	if pending, err := os.ReadFile(filepath.Join(cfg.DataDir, "restore", "cph.ext.db")); err != nil || !bytes.Equal(pending, files["cph.ext.db"]) {
		t.Fatal("invalid upload changed previously staged extension backup", err)
	}
	if err = app.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	start()
	contains("service-a.read", query, "before backup")
	contains("service-b.read", query, "other extension")
	if err = app.extensions.SetEnabled(ctx, "service-b", false); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Install(ctx, pack(makeManifest("service-b", "0.2.0", 2, "notes", "archive")), permissions); err != nil {
		t.Fatal(err)
	}
	if state, _ := app.extensions.State("service-b"); state.Enabled || state.Available {
		t.Fatal("updating a disabled extension started its backend")
	}
	disabledSnapshot := filepath.Join(t.TempDir(), "cph.ext.db")
	if err = app.extensionServices.store.Snapshot(ctx, disabledSnapshot); err != nil {
		t.Fatal(err)
	}
	verify, err := sql.Open("sqlite3", disabledSnapshot+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	err = verify.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name LIKE 'e_%_archive'`).Scan(&count)
	verify.Close()
	if err != nil || count != 1 {
		t.Fatal("disabled installation did not prepare its declared table", count, err)
	}
	if err = app.extensions.SetEnabled(ctx, "service-b", true); err != nil {
		t.Fatal(err)
	}
	contains("service-b.read", query, "other extension")
	cleanup, _ := json.Marshal(map[string]string{"id": "service-a", "hash": obsolete.Hash})
	if _, err = client.Invoke(ctx, "core.extensions.clean-obsolete", cleanup); err != nil {
		t.Fatal(err)
	}
	contains("core.extensions.obsolete", `{"id":"service-a"}`, `"tables":[]`)
	if err = app.extensionServices.store.Check(ctx, "service-a", "service-test", first.Storage); err == nil {
		t.Fatal("cleaned schema can still be used for rollback")
	}
	app.extensionServices.mu.Lock()
	previous := app.extensionServices.bindings["service-a"]
	app.extensionServices.mu.Unlock()
	if err = app.extensions.SetEnabled(ctx, "service-a", false); err != nil {
		t.Fatal(err)
	}
	if previous.process.Alive() {
		t.Fatal("disabled extension process is still running")
	}
	if _, err = previous.storage.Execute(ctx, []spec.StorageRequest{{Op: "query", Table: "notes"}}, permissions); !errors.Is(err, extstore.ErrUnavailable) {
		t.Fatalf("disabled storage session remains usable: %v", err)
	}
	if err = app.extensions.SetEnabled(ctx, "service-a", true); err != nil {
		t.Fatal(err)
	}
	state, _ := app.extensions.State("service-a")
	if err = app.extensions.BackendExited(ctx, "service-a", state.Hash, errors.New("late exit from old process"), func() bool {
		app.extensionServices.mu.Lock()
		defer app.extensionServices.mu.Unlock()
		return app.extensionServices.bindings["service-a"].process == previous.process
	}); err != nil {
		t.Fatal(err)
	}
	contains("service-a.read", query, "before backup")
	if _, err = client.Invoke(ctx, "service-a.crash", nil); err == nil {
		t.Fatal("crashed backend returned success")
	}
	waitFor(func() bool {
		state, _ := app.extensions.State("service-a")
		return state.Status == "failed" && !state.Available
	})
	if err = app.extensions.SetEnabled(ctx, "service-a", true); err != nil {
		t.Fatal(err)
	}
	contains("service-a.read", query, "before backup")
	if err = app.extensions.Uninstall(ctx, "service-a"); err != nil {
		t.Fatal(err)
	}
	if err = app.extensions.CleanData(ctx, "service-a"); err != nil {
		t.Fatal(err)
	}
	contains("service-b.read", query, "other extension")
}

// extensionBackupRequest 使用系统设置调用的同一导出、导入端点。
func extensionBackupRequest(t *testing.T, client *control.Client, backup []byte, status int) []byte {
	t.Helper()
	method, path := http.MethodGet, "/admin/system/backup"
	var body bytes.Buffer
	contentType := ""
	if backup != nil {
		method, path = http.MethodPost, "/admin/system/restore"
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile("file", "backup.zip")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(backup); err != nil {
			t.Fatal(err)
		}
		if err = form.Close(); err != nil {
			t.Fatal(err)
		}
		contentType = form.FormDataContentType()
	}
	request, err := http.NewRequest(method, client.URL+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+client.Token)
	request.Header.Set("Content-Type", contentType)
	response, err := client.HTTP.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("%s: %d, want %d: %s", path, response.StatusCode, status, raw)
	}
	return raw
}
