//go:build !cph_no_web

package web

import "embed"

//go:embed all:build-web
var distFS embed.FS

const distDir = "build-web"
