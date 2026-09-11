//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

func StartTracked(cmd *exec.Cmd) (uintptr, error) {
	SetProcessGroupKill(cmd)
	return 0, cmd.Start()
}

func SetProcessGroupKill(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// A new session gives the tracked tree its own process group and prevents an
	// interactive child from acquiring the runtime's controlling terminal.
	cmd.SysProcAttr.Setpgid = false
	cmd.SysProcAttr.Setsid = true
}

func KillTracked(cmd *exec.Cmd, _ uintptr) {
	KillTree(cmd)
}

func ReapTracked(cmd *exec.Cmd, _ uintptr) {
	KillTree(cmd)
}

func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
