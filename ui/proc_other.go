//go:build !windows

package ui

import "os/exec"

func hideCmdWindow(_ *exec.Cmd) {}
