//go:build !luahost_embed

package plugin

// luahostBin 未内置：默认构建不嵌入 luahost。此时 lua 插件的市场安装会被拒绝
// （提示以 -tags luahost_embed 构建核心）。开发期可用 `pack -install` 本地注入 luahost。
var luahostBin []byte
