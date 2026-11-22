//go:build !windows

package laya

import "os/exec"

func hideConsoleWindow(_ *exec.Cmd) {}
