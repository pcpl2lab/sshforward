//go:build windows

package tunnel

import (
	"os/exec"
	"syscall"
)

const _DETACHED_PROCESS = 0x00000008

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | _DETACHED_PROCESS,
	}
}
