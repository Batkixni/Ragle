//go:build windows

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func openLocalFileOS(path string) error {
	clean := filepath.FromSlash(path)
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", clean).Start()
}

func revealLocalFileOS(path string) error {
	clean := filepath.FromSlash(path)
	cmd := exec.Command("explorer.exe")
	if info, err := os.Stat(clean); err == nil {
		if info.IsDir() {
			cmd.SysProcAttr = &syscall.SysProcAttr{
				CmdLine: fmt.Sprintf(`explorer.exe "%s"`, strings.ReplaceAll(clean, `"`, "")),
			}
		} else {
			// Pass raw Windows command line so /select, is never wrapped inside
			// quotes by Go's argv escaper (which causes Explorer to open Documents).
			cmd.SysProcAttr = &syscall.SysProcAttr{
				CmdLine: fmt.Sprintf(`explorer.exe /select,"%s"`, strings.ReplaceAll(clean, `"`, "")),
			}
		}
		return cmd.Start()
	}
	dir := filepath.Dir(clean)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`explorer.exe "%s"`, strings.ReplaceAll(dir, `"`, "")),
	}
	return cmd.Start()
}
