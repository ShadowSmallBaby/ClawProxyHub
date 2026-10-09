// Package version 从项目配置读取核心版本与更新源，构建时可用 ldflags 覆盖。
package version

import (
	"strings"

	project "github.com/ShadowSmallBaby/ClawProxyHub"
)

var Core string
var UpdateRepository string

func init() {
	if Core == "" {
		Core = project.CoreVersion
	}
	if UpdateRepository == "" {
		UpdateRepository = project.UpdateRepository
	}
	UpdateRepository = strings.TrimSuffix(strings.TrimRight(UpdateRepository, "/"), ".git")
}

func LatestReleaseURL() string  { return UpdateRepository + "/releases/latest" }
func UpdateManifestURL() string { return LatestReleaseURL() + "/download/update-manual.json" }
