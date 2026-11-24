//go:build windows

package ffmpeg

import (
	"os/exec"
	"syscall"
)

// HideWindow prevents spawning a flashing console window on Windows.
func HideWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
