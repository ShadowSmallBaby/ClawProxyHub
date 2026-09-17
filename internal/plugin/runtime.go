// Package plugin — 平台与进程辅助（manager.go 依赖，拆分以保持单文件聚焦）。
package plugin

import (
	"os/exec"
	"runtime"
)

// runtimeOS 返回 go-plugin 二进制命名用的 os 段。
func runtimeOS() string { return runtime.GOOS }

// runtimeArch 返回 go-plugin 二进制命名用的 arch 段。
func runtimeArch() string { return runtime.GOARCH }

// execCommand 构造插件子进程命令。
func execCommand(binPath string) *exec.Cmd {
	return exec.Command(binPath)
}
