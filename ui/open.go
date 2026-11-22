package ui

import (
	"os/exec"
	"path/filepath"
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

func revealLocalFile(path string) error {
	if path == "" {
		return nil
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", "/select,"+filepath.Clean(path)).Start()
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}
