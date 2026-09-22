// Package version — 核心版本号，随发布手动维护，可经 ldflags 覆盖。
package version

// Core 当前核心版本（与 CHANGELOG 顶部对齐）；构建时 -ldflags "-X .../version.Core=x" 可覆盖。
var Core = "1.1.2"
