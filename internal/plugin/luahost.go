// luahost.go — lua 插件的共享 luahost 管理（P1）：一份 luahost 二进制置于 data/hosts/，
// 各 lua 插件进程以 `--dir <插件目录>` 启动，避免逐插件拷贝；也是 P0（升级刷新）的落点。
package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// hostsDir 共享 host 二进制目录（data/hosts，与 data/plugins 同级）。
func (m *Manager) hostsDir() string { return filepath.Join(filepath.Dir(m.dir), "hosts") }

// luahostFile 当前平台共享 luahost 路径。
func (m *Manager) luahostFile() string {
	name := fmt.Sprintf("luahost-%s-%s", runtimeOS(), runtimeArch())
	if runtimeOS() == "windows" {
		name += ".exe"
	}
	return filepath.Join(m.hostsDir(), name)
}

// luahostManual 是否存在手动上传标记（存在则不被内置字节自动覆盖，保留用户上传版本）。
func (m *Manager) luahostManual() bool {
	_, err := os.Stat(filepath.Join(m.hostsDir(), ".manual"))
	return err == nil
}

// ensureLuahost 确保共享 luahost 就位并返回路径：缺失、或与内置字节 sha256 不一致（且非手动上传）时，
// 用内置字节覆盖（P0 升级刷新）。内置为空（未 -tags luahost_embed）且文件也缺失时报错。
func (m *Manager) ensureLuahost() (string, error) {
	path := m.luahostFile()
	if cur, err := os.ReadFile(path); err == nil {
		if m.luahostManual() || len(luahostBin) == 0 || sha256.Sum256(cur) == sha256.Sum256(luahostBin) {
			return path, nil // 已就位且无需刷新
		}
	}
	if len(luahostBin) == 0 {
		return "", fmt.Errorf("核心未内置 luahost（请以 -tags luahost_embed 构建），且 %s 不存在", path)
	}
	if err := os.MkdirAll(m.hostsDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, luahostBin, 0o755); err != nil {
		return "", fmt.Errorf("write luahost: %w", err)
	}
	return path, nil
}

// manifestRuntimeAt 读插件目录 manifest.json 的 runtime（读不到即空 = Go 插件）。
func manifestRuntimeAt(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return ""
	}
	var mf struct {
		Runtime string `json:"runtime"`
	}
	_ = json.Unmarshal(data, &mf)
	return mf.Runtime
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// launchable 目录是否可启动：lua → 有内置/共享 luahost，或插件目录自带 luahost（旧式逐插件拷贝）；
// go → 目录内有当前平台二进制。
func (m *Manager) launchable(dir string) bool {
	if manifestRuntimeAt(dir) == "lua" {
		if len(luahostBin) > 0 || fileExists(m.luahostFile()) {
			return true
		}
		_, err := pluginBinary(dir)
		return err == nil
	}
	_, err := pluginBinary(dir)
	return err == nil
}

// resolveLaunch 由插件目录解析启动命令与插件名：lua 优先共享 luahost + `--dir 插件目录`（P1），
// 无内置/共享时回退插件目录自带的 luahost（pack -install 旧式，不带 --dir 走 exeDir）；go → 目录内二进制。
func (m *Manager) resolveLaunch(dir string) (string, *exec.Cmd, error) {
	name := filepath.Base(dir)
	if manifestRuntimeAt(dir) == "lua" {
		if len(luahostBin) > 0 || fileExists(m.luahostFile()) {
			host, err := m.ensureLuahost()
			if err != nil {
				return "", nil, err
			}
			return name, execCommand(host, "--dir", dir), nil
		}
		if bin, err := pluginBinary(dir); err == nil {
			return name, execCommand(bin), nil // 旧式：luahost 读自身所在目录
		}
		return "", nil, fmt.Errorf("lua 插件 %s 无可用 luahost（内置/共享/自带均缺）", name)
	}
	bin, err := pluginBinary(dir)
	if err != nil {
		return "", nil, err
	}
	return name, execCommand(bin), nil
}

// ReplaceLuahost 手动替换共享 luahost：先停在跑的 lua 插件（释放文件锁），覆盖二进制并打 .manual 标记
// （此后不被内置字节自动刷新），再拉起原先在跑的插件以启用新二进制。
func (m *Manager) ReplaceLuahost(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("空的 luahost 二进制")
	}
	var toRestart []string
	for _, dir := range m.pluginDirs() {
		if manifestRuntimeAt(dir) != "lua" {
			continue
		}
		name := filepath.Base(dir)
		if _, running := m.Get(name); running {
			m.Stop(name, false) // 临时停（非持久），换完拉起
			toRestart = append(toRestart, dir)
		}
	}
	if err := os.MkdirAll(m.hostsDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.luahostFile(), data, 0o755); err != nil {
		return fmt.Errorf("write luahost: %w", err)
	}
	_ = os.WriteFile(filepath.Join(m.hostsDir(), ".manual"), []byte("manual"), 0o644)
	for _, dir := range toRestart {
		_, _ = m.Start(context.Background(), dir)
	}
	return nil
}
