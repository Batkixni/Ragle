//go:build windows

package updater

import (
	"os/exec"
)

func detachCmd(cmd *exec.Cmd) {
	// GUI executables on Windows do not need DETACHED_PROCESS; passing it can
	// prevent foreground window activation on relaunch.
	cmd.Stdout = nil
	cmd.Stderr = nil
}
