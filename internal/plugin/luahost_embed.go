//go:build luahost_embed

package plugin

import _ "embed"

// luahostBin 内置的通用 luahost 二进制（当前平台）：以 -tags luahost_embed 构建时嵌入
// internal/plugin/luahost.bin。安装 runtime=lua 插件时写出为插件目录的 plugin-<os>-<arch>（方案 A）。
// 发布构建需先编 hosts/luahost（对应 GOOS/GOARCH）落到 luahost.bin，再编核心。
//
//go:embed luahost.bin
var luahostBin []byte
