// backend.go — 功能扩展的 Go 后端与平台入口，不复用业务插件协议。
package extension

import (
	"fmt"
	"slices"
	"strings"
)

const BackendProtocol = 1

var DesktopPlatforms = []string{"windows/amd64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"}
var AndroidPlatforms = []string{"android/arm64", "android/amd64"}

// AndroidBackend 的 JNI 入口由应用私有服务加载，业务处理器仍使用同一套 Go 协议。
type AndroidBackend struct {
	Protocol int `json:"protocol"`
	MinSDK   int `json:"min_sdk"`
}

type Backend struct {
	Language              string            `json:"language"`
	Protocol              int               `json:"protocol"`
	Entries               map[string]string `json:"entries"`
	BackgroundPermissions []string          `json:"background_permissions,omitempty"`
	Android               *AndroidBackend   `json:"android,omitempty"`
}

func (b Backend) Platforms() []string {
	platforms := append([]string{}, DesktopPlatforms...)
	if b.Android != nil {
		platforms = append(platforms, "android/arm64")
		if b.Entries["android/amd64"] != "" {
			platforms = append(platforms, "android/amd64")
		}
	}
	return platforms
}

func HasPermission(grants []string, permission string) bool {
	return slices.Contains(grants, permission)
}

func (m Manifest) validateBackend() error {
	if m.Backend == nil {
		if m.Kind == "service" {
			return fmt.Errorf("service requires a Go backend")
		}
		return nil
	}
	b := m.Backend
	if m.Kind != "service" || m.Execution != "trusted-process" || m.Activation != "hot" || m.Target != "backend" || m.Entry != "" || m.Android != nil || b.Language != "go" || b.Protocol != BackendProtocol || !HasPermission(m.Permissions, "service.execute") {
		return fmt.Errorf("invalid Go backend declaration")
	}
	platforms := b.Platforms()
	if b.Android != nil && (b.Android.Protocol != BackendProtocol || b.Android.MinSDK < 24) {
		return fmt.Errorf("invalid Android backend protocol or minimum SDK")
	}
	if len(b.Entries) != len(platforms) {
		return fmt.Errorf("Go backend requires five desktop entries, Android arm64 when declared, and optional Android amd64 for testing")
	}
	seen := map[string]bool{}
	for _, platform := range platforms {
		entry := b.Entries[platform]
		if !SafePath(entry) || m.Files[entry] == "" || seen[entry] {
			return fmt.Errorf("missing or duplicate backend entry for %s", platform)
		}
		if strings.HasPrefix(platform, "android/") && !strings.HasSuffix(entry, ".so") {
			return fmt.Errorf("Android backend requires a shared library")
		}
		seen[entry] = true
	}
	if len(m.Platforms) > 0 {
		if len(m.Platforms) != len(platforms) {
			return fmt.Errorf("backend platform list differs from entries")
		}
		seenPlatforms := map[string]bool{}
		for _, platform := range m.Platforms {
			if b.Entries[platform] == "" || seenPlatforms[platform] {
				return fmt.Errorf("invalid backend platform")
			}
			seenPlatforms[platform] = true
		}
	}
	seen = map[string]bool{}
	for _, p := range b.BackgroundPermissions {
		if (p != "storage.read" && p != "storage.write") || !HasPermission(m.Permissions, p) || seen[p] {
			return fmt.Errorf("invalid background storage permission")
		}
		seen[p] = true
	}
	return nil
}
