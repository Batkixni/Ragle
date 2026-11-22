//go:build !windows

package thumbnail

import "os/exec"

func hideCmd(_ *exec.Cmd) {}
