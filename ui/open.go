package ui

import (
	"os/exec"
	"runtime"
)

func openLocalFile(path string) error {
	if path == "" {
		return nil
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
