package luahost

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// writeFixture 落一个临时 lua 插件目录（manifest.json + main.lua），返回目录。
func writeFixture(t *testing.T, manifest, lua string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte(lua), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestHandshakeIdentityFromManifest 回归：脚本 handshake 硬编码的 name/version/author 必须被
// manifest.json 覆盖（防 AUDD 拷贝 autoclaw 后 name="autoclaw" 覆盖真实实例的事故）；
// 协议版本以协商值为准；label/capabilities 等非身份字段仍以脚本声明为准。
func TestHandshakeIdentityFromManifest(t *testing.T) {
	dir := writeFixture(t,
		`{"name":"real","version":"latest","author":"real-author","runtime":"lua","label":{"zh":"真"}}`,
		`local M = {}
function M.handshake(req)
  return { manifest = {
    name = "evil", version = "9.9.9", author = "evil-author",
    label = { zh = "脚本名", en = "ScriptName" },
    capabilities = { "chat", "models" },
    endpoints = { "messages" },
  } }
end
return M`)
	h := &luahost{dir: dir, pool: newVMPool(dir, nil)}

	resp, err := h.Handshake(context.Background(), &pb.HandshakeRequest{ProtocolVersion: 2})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("handshake error: %s", resp.Error.Message)
	}
	m := resp.Manifest
	if m.Name != "real" {
		t.Errorf("Name = %q, want real（manifest.json 覆盖脚本硬编码）", m.Name)
	}
	if m.Version != "latest" {
		t.Errorf("Version = %q, want latest", m.Version)
	}
	if m.Author != "real-author" {
		t.Errorf("Author = %q, want real-author", m.Author)
	}
	if m.ProtocolVersion != 2 {
		t.Errorf("ProtocolVersion = %d, want 2（协商值）", m.ProtocolVersion)
	}
	// 非身份字段仍取脚本声明
	if len(m.Capabilities) == 0 || m.Capabilities[0] != "chat" {
		t.Errorf("Capabilities = %v, want 取自脚本", m.Capabilities)
	}
	if m.Label["zh"] != "脚本名" {
		t.Errorf("Label.zh = %q, want 脚本名（label 非身份，脚本为准）", m.Label["zh"])
	}
}

// TestHandshakeFallbackManifest 脚本无 handshake() 时回退磁盘 manifest.json，协议版本用宿主当前值。
func TestHandshakeFallbackManifest(t *testing.T) {
	dir := writeFixture(t,
		`{"name":"fb","version":"1.0.0","author":"a","runtime":"lua","capabilities":["chat"]}`,
		`local M = {}
function M.models() return { models = {} } end
return M`)
	h := &luahost{dir: dir, pool: newVMPool(dir, nil)}

	resp, err := h.Handshake(context.Background(), &pb.HandshakeRequest{ProtocolVersion: 2})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("handshake error: %s", resp.Error.Message)
	}
	if resp.Manifest.Name != "fb" || resp.Manifest.Version != "1.0.0" {
		t.Errorf("fallback manifest = %q/%q, want fb/1.0.0", resp.Manifest.Name, resp.Manifest.Version)
	}
	if resp.Manifest.ProtocolVersion != sdk.ProtocolVersion {
		t.Errorf("ProtocolVersion = %d, want %d", resp.Manifest.ProtocolVersion, sdk.ProtocolVersion)
	}
}
