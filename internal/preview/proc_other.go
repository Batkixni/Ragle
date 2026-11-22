//go:build !windows

package preview

import "os/exec"

func hideCmd(_ *exec.Cmd) {}
