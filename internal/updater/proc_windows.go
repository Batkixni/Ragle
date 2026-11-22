//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

func detachCmd(cmd *exec.Cmd) {
	// DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200,
	}
}
