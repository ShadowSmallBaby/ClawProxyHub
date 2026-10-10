//go:build cph_no_web

package web

import "embed"

// 原生宿主自行提供界面，无需将 Web 资源编入核心库。
var distFS embed.FS

const distDir = "."
