// install.go — .cphplugin 包的安装与卸载。
// 包格式（zip）：manifest.json + plugin-<os>-<arch>[.exe] 二进制（可多平台）。
package plugin

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
)

// PackageManifest 包内 manifest.json。
type PackageManifest struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Author          string            `json:"author"`
	Label           map[string]string `json:"label"` // 品牌名（多语言）
	ProtocolVersion int32             `json:"protocol_version"`
	MinCoreVersion  string            `json:"min_core_version"`
	// Runtime 运行时：空=Go 插件（包内自带二进制）；"lua"=脚本插件（包内只含脚本，
	// 安装时由核心注入内置 luahost 作为 plugin-<os>-<arch>）。
	Runtime string `json:"runtime"`
	// Entry 脚本入口（lua 固定 main.lua）；luahost 读同目录 main.lua，此字段仅声明。
	Entry string `json:"entry"`
	// Icon 插件图标：包内相对路径（如 "icon.png"，建议正方形 PNG 128–256px）。
	// 安装时解出到插件目录，前端经 /assets/plugins/<name>/icon 读取。
	Icon string `json:"icon"`
}

// Installed 磁盘上已安装的全部插件（含未运行的），按落盘 manifest.json 读取；管理页据此列出可启动项。
func (m *Manager) Installed() []PackageManifest {
	var out []PackageManifest
	for _, dir := range m.pluginDirs() {
		data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			continue
		}
		var mf PackageManifest
		if json.Unmarshal(data, &mf) == nil && mf.Name != "" {
			out = append(out, mf)
		}
	}
	return out
}

// manifestAuthor 读插件目录落盘 manifest.json 的 author；作者统一以此为准，
// Go 插件 main.go / lua 脚本握手声明的 author 均由核心在启动后覆盖。读不到返回空。
func manifestAuthor(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return ""
	}
	var mf PackageManifest
	if json.Unmarshal(data, &mf) != nil {
		return ""
	}
	return mf.Author
}

// Install 阶段（InstallZip 的进度回调值）。
const (
	PhaseStopping   = "stopping"   // 升级：停旧进程
	PhaseInstalling = "installing" // 解压落盘
	PhaseStarting   = "starting"   // 启动子进程 + 握手
)

// InstallZip 安装一个 .cphplugin 包：校验 → 解压到插件目录 → 启动。
// namespace 为来源命名空间（官方源/手动上传为空 → <dir>/<name>；其他源 → <dir>/<namespace>/<name>）。
// 返回插件名。同名同命名空间时覆盖安装（升级）；同名插件已装在其他命名空间时拒绝。
// onPhase 非 nil 时在各阶段开始前回调（前端进度展示）。
func (m *Manager) InstallZip(ctx context.Context, zipPath, namespace string, onPhase func(string)) (string, error) {
	m.installMu.Lock()
	defer m.installMu.Unlock()
	report := func(phase string) {
		if onPhase != nil {
			onPhase(phase)
		}
	}
	if namespace != "" && !validPluginName(namespace) {
		return "", fmt.Errorf("invalid namespace")
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	entries := make(map[string]*zip.File)
	var total uint64
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			name = strings.TrimSuffix(name, "/")
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") || f.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe archive path %q", name)
		}
		if f.FileInfo().IsDir() {
			continue // ZIP 目录条目不解压，文件写入时创建父目录。
		}
		if _, exists := entries[name]; exists {
			return "", fmt.Errorf("duplicate archive path %q", name)
		}
		if f.UncompressedSize64 > 512<<20 || total > (512<<20)-f.UncompressedSize64 {
			return "", fmt.Errorf("package exceeds extraction limit")
		}
		total += f.UncompressedSize64
		entries[name] = f
	}
	mf := entries["manifest.json"]
	if mf == nil || mf.UncompressedSize64 > 1<<20 {
		return "", fmt.Errorf("missing or oversized manifest")
	}
	rc, err := mf.Open()
	if err != nil {
		return "", err
	}
	var manifest PackageManifest
	err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&manifest)
	rc.Close()
	if err != nil || !validPluginName(manifest.Name) {
		return "", fmt.Errorf("invalid manifest name")
	}
	if manifest.Runtime != "" && manifest.Runtime != "go" && manifest.Runtime != "lua" {
		return "", fmt.Errorf("unsupported runtime")
	}
	if pv := manifest.ProtocolVersion; pv != 0 && (pv < sdk.MinProtocolVersion || pv > sdk.ProtocolVersion) {
		return "", fmt.Errorf("unsupported protocol %d", pv)
	}
	if manifest.Runtime == "lua" && m.host.settings != nil && m.db != nil && !m.host.settings.LuaEnabled() {
		return "", fmt.Errorf("Lua runtime is disabled")
	}
	target, err := installTarget(m.dir, namespace, manifest.Name)
	if err != nil {
		return "", err
	}
	if existing, ok := m.pluginDir(manifest.Name); ok {
		existing, _ = filepath.EvalSymlinks(existing)
		existing, _ = filepath.Abs(existing)
		if existing != target {
			return "", fmt.Errorf("plugin already installed from another source")
		}
	}
	report(PhaseInstalling)
	stage, err := os.MkdirTemp(filepath.Dir(target), ".install-"+manifest.Name+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	bin := fmt.Sprintf("plugin-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if manifest.Runtime == "lua" {
		if entries["main.lua"] == nil {
			return "", fmt.Errorf("missing main.lua")
		}
		if _, err = m.ensureLuahost(); err != nil {
			return "", err
		}
		for name, f := range entries {
			if strings.HasSuffix(name, ".lua") {
				dst := filepath.Join(stage, filepath.FromSlash(name))
				if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
					return "", err
				}
				if err = extractTo(f, dst); err != nil {
					return "", err
				}
			}
		}
	} else {
		f := entries[bin]
		if f == nil {
			return "", fmt.Errorf("missing binary for %s", bin)
		}
		if err = extractTo(f, filepath.Join(stage, bin)); err != nil {
			return "", err
		}
		if err = os.Chmod(filepath.Join(stage, bin), 0755); err != nil {
			return "", err
		}
	}
	if err = extractTo(mf, filepath.Join(stage, "manifest.json")); err != nil {
		return "", err
	}
	if manifest.Icon != "" {
		if !fs.ValidPath(manifest.Icon) || strings.Contains(manifest.Icon, "\\") {
			return "", fmt.Errorf("invalid icon path")
		}
		if f := entries[manifest.Icon]; f != nil {
			if err = extractTo(f, filepath.Join(stage, filepath.Base(manifest.Icon))); err != nil {
				return "", err
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	_, running := m.Get(manifest.Name)
	if running {
		report(PhaseStopping)
		m.Stop(manifest.Name, false)
	}
	backup := stage + "-previous"
	hadOld := false
	if _, err = os.Lstat(target); err == nil {
		if err = os.Rename(target, backup); err != nil {
			return "", err
		}
		hadOld = true
	} else if !os.IsNotExist(err) {
		return "", err
	}
	rollback := func(cause error) (string, error) {
		if err := removeWithRetry(target); err != nil {
			return "", fmt.Errorf("%w; rollback cleanup: %v (backup %s)", cause, err, backup)
		}
		if hadOld {
			if err := os.Rename(backup, target); err != nil {
				return "", fmt.Errorf("%w; rollback: %v (backup %s)", cause, err, backup)
			}
		}
		if running && hadOld {
			recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := m.Start(recovery, target); err != nil {
				return "", fmt.Errorf("%w; previous plugin restart: %v", cause, err)
			}
		}
		return "", cause
	}
	if err = os.Rename(stage, target); err != nil {
		return rollback(err)
	}
	report(PhaseStarting)
	if _, err = m.Start(ctx, target); err != nil {
		return rollback(err)
	}
	if hadOld {
		if err = removeWithRetry(backup); err != nil {
			return manifest.Name, fmt.Errorf("installed; old backup cleanup: %w", err)
		}
	}
	return manifest.Name, nil
}

// installTarget 在任何替换操作前验证命名空间和真实父路径，禁止符号链接越界。
func installTarget(root, namespace, name string) (string, error) {
	if !validPluginName(name) || (namespace != "" && !validPluginName(namespace)) {
		return "", fmt.Errorf("invalid install name")
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	parent := filepath.Join(root, namespace)
	if err = os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	if resolved != parent {
		return "", fmt.Errorf("namespace may not be a symbolic link")
	}
	target := filepath.Join(parent, name)
	if st, err := os.Lstat(target); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("target may not be a symbolic link")
	}
	return target, nil
}

// Uninstall 停止并删除一个插件的全部本地文件（根目录或命名空间目录；仅停进程，插件记录随级联删除清库）。
func (m *Manager) Uninstall(name string) error {
	if !validPluginName(name) {
		return fmt.Errorf("invalid plugin name")
	}
	if _, running := m.Get(name); running {
		m.Stop(name, false)
	}
	dir, ok := m.pluginDir(name)
	if !ok {
		return nil
	}
	return removeWithRetry(dir)
}

// removeWithRetry 带重试删除：Windows 下进程退出到文件锁释放有延迟（Access is denied），
// 退避重试几轮再报错。
func removeWithRetry(dir string) error {
	var err error
	for i := 0; i < 5; i++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
	}
	return err
}

// pluginDir 按插件名定位落盘目录：先 <dir>/<name>，再 <dir>/<source>/<name>（命名空间安装）。
func (m *Manager) pluginDir(name string) (string, bool) {
	if !validPluginName(name) {
		return "", false
	}
	if isPluginDir(filepath.Join(m.dir, name)) {
		return filepath.Join(m.dir, name), true
	}
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(m.dir, e.Name(), name)
		if isPluginDir(p) {
			return p, true
		}
	}
	return "", false
}

// isPluginDir 目录含 manifest.json 即视为插件目录（命名空间目录本身没有）。
func isPluginDir(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "manifest.json"))
	return err == nil && !st.IsDir()
}

// validPluginName 防路径穿越。
func validPluginName(name string) bool {
	if len(name) == 0 || len(name) > 128 || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	return true
}

// IconFile 插件图标文件路径：以落盘 manifest.json 的 icon 声明为准（文件存在才返回）。
func (m *Manager) IconFile(name string) (string, bool) {
	dir, ok := m.pluginDir(name)
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return "", false
	}
	var mf PackageManifest
	if json.Unmarshal(data, &mf) != nil || mf.Icon == "" {
		return "", false
	}
	icon := filepath.Base(mf.Icon) // 只认基名，防路径穿越
	switch strings.ToLower(filepath.Ext(icon)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg":
	default:
		return "", false
	}
	p := filepath.Join(dir, icon)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

func extractTo(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	n, err := io.Copy(out, io.LimitReader(rc, (512<<20)+1))
	if err == nil && n > 512<<20 {
		err = fmt.Errorf("archive entry too large")
	}
	if err != nil {
		return err
	}
	return out.Close()
}
