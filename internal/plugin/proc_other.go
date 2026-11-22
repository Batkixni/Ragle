//go:build !windows

package plugin

import "os/exec"

func hideCmd(_ *exec.Cmd) {}
