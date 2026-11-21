//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detachCommand Windows 侧编译补位：新建进程组，使脚本不随应用退出被组杀。
// 应用内自动更新在 darwin 之外由 InstallAppUpdate 运行时拒绝，本函数当前
// 不会被调用；保留实现是为未来解锁该平台时不缺件。
func detachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
