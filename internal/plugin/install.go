// install.go — .cphplugin 包的安装与卸载。
// 包格式（zip）：manifest.json + plugin-<os>-<arch>[.exe] 二进制（可多平台）。
package plugin

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
func (m *Manager) InstallZip(ctx context.Context, zipPath, namespace string, onPhase func(phase string)) (string, error) {
	report := func(phase string) {
		if onPhase != nil {
			onPhase(phase)
		}
	}
	if namespace != "" && !validPluginName(namespace) {
		return "", fmt.Errorf("invalid source namespace")
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open package: %w", err)
	}
	defer zr.Close()

	// 1. 读 manifest 与平台二进制
	var manifest *PackageManifest
	var binFile *zip.File
	platformBin := fmt.Sprintf("plugin-%s-%s", runtime.GOOS, runtime.GOARCH)
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		switch {
		case name == "manifest.json":
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			manifest = &PackageManifest{}
			err = json.NewDecoder(rc).Decode(manifest)
			rc.Close()
			if err != nil {
				return "", fmt.Errorf("invalid manifest.json: %w", err)
			}
		case name == platformBin || (runtime.GOOS == "windows" && name == platformBin+".exe"):
			binFile = f
		}
	}
	if manifest == nil {
		return "", fmt.Errorf("package missing manifest.json")
	}
	if manifest.Name == "" {
		return "", fmt.Errorf("manifest missing name")
	}
	if binFile == nil {
		return "", fmt.Errorf("package missing binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if pv := manifest.ProtocolVersion; pv != 0 && (pv < sdk.MinProtocolVersion || pv > sdk.ProtocolVersion) {
		return "", fmt.Errorf("protocol version %d unsupported (core accepts %d..%d)", pv, sdk.MinProtocolVersion, sdk.ProtocolVersion)
	}

	// 2. 同名冲突：已装在其他命名空间的同名插件拒绝（插件身份 = manifest.name，全局唯一）
	target := filepath.Join(m.dir, namespace, manifest.Name)
	if existing, ok := m.pluginDir(manifest.Name); ok && existing != target {
		return "", fmt.Errorf("同名插件 %q 已从其他来源安装（%s），请先卸载", manifest.Name, filepath.Base(filepath.Dir(existing)))
	}

	// 3. 升级场景：先停旧进程（仅停本次进程，升级后照常拉起）
	if _, running := m.Get(manifest.Name); running {
		report(PhaseStopping)
		m.Stop(manifest.Name, false)
	}
	report(PhaseInstalling)

	// 4. 解压到目标目录（清掉旧目录）
	if err := os.RemoveAll(target); err != nil {
		return "", fmt.Errorf("clean old install: %w", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	binPath := filepath.Join(target, platformBin)
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if err := extractTo(binFile, binPath); err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}
	if runtime.GOOS != "windows" {
		os.Chmod(binPath, 0o755)
	}
	// manifest 一并落盘（卸载/诊断用）
	if mf := zipEntry(zr, "manifest.json"); mf != nil {
		_ = extractTo(mf, filepath.Join(target, "manifest.json"))
	}
	// icon（包内 manifest.icon 声明）一并解出
	if manifest.Icon != "" {
		if ic := zipEntry(zr, filepath.Base(manifest.Icon)); ic != nil {
			_ = extractTo(ic, filepath.Join(target, filepath.Base(manifest.Icon)))
		}
	}

	// 5. 启动
	report(PhaseStarting)
	if _, err := m.Start(ctx, binPath); err != nil {
		return manifest.Name, fmt.Errorf("installed but failed to start: %w", err)
	}
	return manifest.Name, nil
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
	return os.RemoveAll(dir)
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
		if !e.IsDir() {
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
	if name == "" || strings.ContainsAny(name, `/\..`) {
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

func zipEntry(zr *zip.ReadCloser, base string) *zip.File {
	for _, f := range zr.File {
		if filepath.Base(f.Name) == base {
			return f
		}
	}
	return nil
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
	_, err = io.Copy(out, rc)
	return err
}
