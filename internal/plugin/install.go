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
	Name            string `json:"name"`
	Version         string `json:"version"`
	Author          string `json:"author"`
	ProtocolVersion int32  `json:"protocol_version"`
	MinCoreVersion  string `json:"min_core_version"`
	// Icon 插件图标：包内相对路径（如 "icon.png"，建议正方形 PNG 128–256px）。
	// 安装时解出到插件目录，前端经 /assets/plugins/<name>/icon 读取。
	Icon string `json:"icon"`
}

// InstallZip 安装一个 .cphplugin 包：校验 → 解压到插件目录 → 启动。
// 返回插件名。已存在时覆盖安装（升级）。
func (m *Manager) InstallZip(ctx context.Context, zipPath string) (string, error) {
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
	if manifest.ProtocolVersion != 0 && manifest.ProtocolVersion != sdk.ProtocolVersion {
		return "", fmt.Errorf("protocol version mismatch: package=%d core=%d", manifest.ProtocolVersion, sdk.ProtocolVersion)
	}

	// 2. 升级场景：先停旧进程
	if _, running := m.Get(manifest.Name); running {
		m.Stop(manifest.Name)
	}

	// 3. 解压到 <dir>/<name>/（清掉旧目录）
	target := filepath.Join(m.dir, manifest.Name)
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

	// 4. 启动
	if _, err := m.Start(ctx, binPath); err != nil {
		return manifest.Name, fmt.Errorf("installed but failed to start: %w", err)
	}
	return manifest.Name, nil
}

// Uninstall 停止并删除一个插件的全部本地文件。
func (m *Manager) Uninstall(name string) error {
	if !validPluginName(name) {
		return fmt.Errorf("invalid plugin name")
	}
	if _, running := m.Get(name); running {
		m.Stop(name)
	}
	return os.RemoveAll(filepath.Join(m.dir, name))
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
	if !validPluginName(name) {
		return "", false
	}
	dir := filepath.Join(m.dir, name)
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
