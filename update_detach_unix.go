//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detachCommand 让子进程脱离应用进程组，应用退出后脚本继续运行。
// unix 用 Setsid；syscall.SysProcAttr 在 Windows 上无该字段，故按平台拆分。
func detachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
