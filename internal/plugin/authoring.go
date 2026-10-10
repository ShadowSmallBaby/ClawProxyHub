// authoring.go — 用户自建 Lua 插件：落盘 data/plugins/local/<name>/，仅 main.lua 可在线编辑。
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/luasource"
)

// LocalNamespace 自建插件命名空间目录名（<dir>/local/<name>/）。
const LocalNamespace = "local"

// scaffoldTemplate 内置脚手架：一个最小可跑的 echo Lua 插件（manifest + main.lua）。
const scaffoldTemplate = `{
  "name": "%s",
  "version": "0.1.0",
  "author": "local",
  "label": { "zh": "%s", "en": "%s" },
  "runtime": "lua",
  "entry": "main.lua",
  "protocol_version": 2,
  "capabilities": ["chat", "models"],
  "endpoints": ["chat_completions", "messages"],
  "auth_methods": [
    {
      "id": "token",
      "label": { "zh": "令牌", "en": "Token" },
      "fields": [
        { "name": "token", "label": { "zh": "API 令牌", "en": "API Token" }, "type": "password", "required": true }
      ]
    }
  ]
}
`

const scaffoldLuaTemplate = `-- main.lua — %s（自建脚手架）
-- ── 插件身份（保存时由此回显表单）────────────────────────
local PLUGIN_NAME     = "%s"   -- 插件名（字母/数字/-/_，全局唯一，建档后不可改）
local PLUGIN_LABEL_ZH = "%s"   -- 显示名（中文）
local PLUGIN_LABEL_EN = ""     -- 显示名（英文，缺省用 PLUGIN_NAME）
-- ────────────────────────────────────────────────────────
-- 约定函数全列于此（用不到的自己删；缺失则该能力降级）。
-- 宿主能力 cph.*：http.request/stream、json、hash、time、random、openai.chat_body、log。
-- stream 方法：message_start / content_delta / reasoning_delta / tool_call_delta / message_finish / failed。
-- 凭据形态：授权时 login 返回的 blob（JSON），chat/refresh 经 req.credential.blob 读回。

local plugin = {}

local UPSTREAM = "https://example.com/v1/chat/completions"

-- 握手：声明能力与授权方式（缺失回退 manifest.json；显示名以本文件为准）。
-- 身份（name/version/author/protocol_version）由宿主托管，脚本不声明。
function plugin.handshake(req)
  return {
    manifest = {
      label = { zh = PLUGIN_LABEL_ZH, en = PLUGIN_LABEL_EN ~= "" and PLUGIN_LABEL_EN or PLUGIN_NAME },
      capabilities = { "chat", "models", "login" },
      endpoints = { "chat_completions", "messages" },
      auth_methods = {
        {
          id = "token",
          label = { zh = "令牌", en = "Token" },
          fields = {
            { name = "token", label = { zh = "API 令牌", en = "API Token" }, type = "password", required = true },
          },
        },
      },
    },
  }
end

function plugin.models(cred)
  return { models = {
    { id = "my-model", context_window = 32768, supports_tools = true, supports_stream = true },
  } }
end

function plugin.login(req)
  local token = (req.form or {}).token or ""
  if token == "" then
    return { error = { code = 400, message = "请填写令牌" } }
  end
  return {
    blob = cph.json.encode({ token = token }),
    profile = { display_name = PLUGIN_LABEL_ZH, healthy = true, quota = {} },
  }
end

function plugin.chat(req, stream)
  local cred = cph.json.decode((req.credential or {}).blob or "") or {}
  local body = cph.openai.chat_body(req)
  stream.message_start({ model = req.model })
  local ok, err = cph.http.stream({
    method = "POST",
    url = UPSTREAM,
    headers = {
      ["Content-Type"] = "application/json",
      ["Authorization"] = "Bearer " .. (cred.token or ""),
    },
    body = body,
    stream = stream,
    format = "openai",
  })
  if not ok then
    stream.failed({ code = 502, message = tostring(err) })
  end
end

function plugin.refresh(cred)
  return {} -- 按需返回 { blob, profile }；无刷新逻辑可删
end

function plugin.profile(cred)
  return { display_name = PLUGIN_LABEL_ZH, healthy = true, quota = {} }
end

function plugin.tasks()
  return { capabilities = {} } -- 任务能力声明；无任务可删
end

function plugin.task(req)
  return { summary = "" } -- req={capability_id, credential, context}；无任务可删
end

return plugin
`

// localDir 自建插件根目录（<dir>/local）。
func (m *Manager) localDir() string { return filepath.Join(m.dir, LocalNamespace) }

// IsLocalPlugin 插件是否用户自建（落盘在 local 命名空间下）。
func (m *Manager) IsLocalPlugin(name string) bool {
	if !validPluginName(name) {
		return false
	}
	st, err := os.Stat(filepath.Join(m.localDir(), name, "manifest.json"))
	return err == nil && !st.IsDir()
}

// localPluginDir 自建插件目录（不存在返回 ok=false）。
func (m *Manager) localPluginDir(name string) (string, bool) {
	if !m.IsLocalPlugin(name) {
		return "", false
	}
	return filepath.Join(m.localDir(), name), true
}

// ScaffoldLua 渲染脚手架 main.lua（真实插件名由 PLUGIN_NAME 行在保存时回显）。
func (m *Manager) ScaffoldLua(label string) string {
	if label == "" {
		label = "my-plugin"
	}
	return fmt.Sprintf(scaffoldLuaTemplate, label, label, label)
}

// CreateLocalPlugin 保留旧 SDK 的创建并运行语义；工作区 API 使用 CreateLocalWorkspace。
func (m *Manager) CreateLocalPlugin(ctx context.Context, name, label, lua string, icon []byte) (string, error) {
	created, err := m.CreateLocalWorkspace(name, label, lua, icon)
	if err != nil {
		return "", err
	}
	if err = m.ReloadLocalSource(ctx, name); err != nil {
		return created, fmt.Errorf("created but failed to start: %w", err)
	}
	return created, nil
}

// CreateLocalWorkspace 只建档保存，不下载运行时或启动脚本。
func (m *Manager) CreateLocalWorkspace(name, label, lua string, icon []byte) (string, error) {
	if !validPluginName(name) {
		return "", fmt.Errorf("插件名仅允许字母、数字、- 与 _")
	}
	if m.IsLocalPlugin(name) {
		return "", fmt.Errorf("插件 %q 已存在", name)
	}
	if len(lua) > 2<<20 || len(icon) > 2<<20 || len(label) > 128 {
		return "", fmt.Errorf("工作区内容超过限制")
	}
	if lua == "" {
		lua = m.ScaffoldLua(name)
	}
	if identity, _ := ParseLuaIdentity(lua); identity != "" && identity != name {
		return "", fmt.Errorf("代码侧 PLUGIN_NAME 与插件身份不一致")
	}
	if err := os.MkdirAll(m.localDir(), 0755); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(m.localDir(), ".workspace-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	files := map[string][]byte{"manifest.json": []byte(m.ScaffoldManifestFile(name, label)), "main.lua": []byte(lua)}
	if len(icon) > 0 {
		files["icon.png"] = icon
	}
	for path, data := range files {
		if err = os.WriteFile(filepath.Join(stage, path), data, 0600); err != nil {
			return "", err
		}
	}
	if err = os.Rename(stage, filepath.Join(m.localDir(), name)); err != nil {
		return "", err
	}
	return name, nil
}

// ScaffoldManifestFile 按插件名/品牌名渲染 manifest 内容。
func (m *Manager) ScaffoldManifestFile(name, label string) string {
	if label == "" {
		label = name
	}
	encoded, _ := json.Marshal(label)
	escaped := string(encoded[1 : len(encoded)-1])
	return fmt.Sprintf(scaffoldTemplate, name, escaped, escaped)
}

// syncLocalLabel 代码侧 PLUGIN_LABEL_ZH/EN 变更时同步进 manifest.json 的 label.zh/en（label 非身份）。
func (m *Manager) syncLocalLabel(dir, name, luaContent string) {
	changed := false
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var mf PackageManifest
	if json.Unmarshal(data, &mf) != nil {
		return
	}
	metadata, _ := luasource.Metadata(luaContent)
	for lang, varName := range map[string]string{"zh": "PLUGIN_LABEL_ZH", "en": "PLUGIN_LABEL_EN"} {
		v := metadata[varName]
		if v == "" || mf.Label[lang] == v {
			continue
		}
		if mf.Label == nil {
			mf.Label = map[string]string{}
		}
		mf.Label[lang] = v
		changed = true
	}
	if !changed {
		return
	}
	if out, err := json.MarshalIndent(mf, "", "  "); err == nil {
		_ = os.WriteFile(path, out, 0o644)
	}
}

// ParseLuaIdentity 提取 main.lua 顶部的 PLUGIN_NAME / PLUGIN_LABEL_ZH 声明（保存时回显表单）。
func ParseLuaIdentity(content string) (name, label string) {
	metadata, _ := luasource.Metadata(content)
	return metadata["PLUGIN_NAME"], metadata["PLUGIN_LABEL_ZH"]
}

// ReadLocalSource 读自建插件源码（仅 main.lua；manifest 由核心建档生成，不开放编辑）。
func (m *Manager) ReadLocalSource(name, file string) (string, error) {
	if file != "main.lua" {
		return "", fmt.Errorf("only main.lua editable")
	}
	_, ok := m.localPluginDir(name)
	if !ok {
		return "", fmt.Errorf("插件 %q 不是可编辑的自建插件", name)
	}
	root, err := os.OpenRoot(m.localDir())
	if err != nil {
		return "", err
	}
	defer root.Close()
	data, err := root.ReadFile(name + "/" + file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteLocalSource 保留旧 SDK 保存后重载语义，新工作区将两者分开调用。
func (m *Manager) WriteLocalSource(ctx context.Context, name, file, content string) error {
	if err := m.SaveLocalSource(name, file, content); err != nil {
		return err
	}
	return m.ReloadLocalSource(ctx, name)
}

// SaveLocalSource 原子替换 main.lua，不改变当前运行实例。
func (m *Manager) SaveLocalSource(name, file, content string) error {
	if file != "main.lua" {
		return fmt.Errorf("only main.lua editable")
	}
	dir, ok := m.localPluginDir(name)
	if !ok {
		return fmt.Errorf("插件 %q 不是可编辑的自建插件", name)
	}
	if len(content) > 2<<20 {
		return fmt.Errorf("源码超过限制")
	}
	if codeName, _ := ParseLuaIdentity(content); codeName != "" && codeName != name {
		return fmt.Errorf("代码侧 PLUGIN_NAME %q 与插件身份不一致（不可改名，需删除重建）", codeName)
	}
	root, err := os.OpenRoot(m.localDir())
	if err != nil {
		return err
	}
	defer root.Close()
	// OpenRoot 阻止工作区符号链接越界；临时文件与目标在同一目录，崩溃不截断原稿。
	tmpName := fmt.Sprintf("%s/.source-%d", name, time.Now().UnixNano())
	f, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmpName)
	_, err = f.WriteString(content)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = root.Rename(tmpName, name+"/"+file); err != nil {
		return err
	}
	m.syncLocalLabel(dir, name, content)
	return nil
}

// ReloadLocalSource 执行单独授权，未安装/禁用运行时时由运行适配器返回明确错误。
func (m *Manager) ReloadLocalSource(ctx context.Context, name string) error {
	dir, ok := m.localPluginDir(name)
	if !ok {
		return fmt.Errorf("插件不是本地工作区")
	}
	if _, running := m.Get(name); running {
		m.Stop(name, false)
	}
	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := m.Start(startCtx, dir); err != nil {
		return fmt.Errorf("源码已保存，运行失败: %w", err)
	}
	return nil
}
