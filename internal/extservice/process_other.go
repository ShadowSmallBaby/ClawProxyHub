//go:build !windows

package extservice

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
