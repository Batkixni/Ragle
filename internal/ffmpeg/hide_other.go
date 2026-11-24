//go:build !windows

package ffmpeg

import "os/exec"

// HideWindow is a no-op on non-Windows platforms.
func HideWindow(_ *exec.Cmd) {}
