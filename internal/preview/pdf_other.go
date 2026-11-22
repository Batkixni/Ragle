//go:build !windows

package preview

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func renderPDFNative(path string, pageIndex int, maxEdge int) (*image.RGBA, int, error) {
	// macOS QuickLook fallback for first page when pdftoppm/mutool aren't installed.
	if pageIndex == 0 {
		if bin, err := exec.LookPath("qlmanage"); err == nil {
			tmpDir, err := os.MkdirTemp("", "ragle-pdf-*")
			if err != nil {
				return nil, 0, err
			}
			defer os.RemoveAll(tmpDir)
			cmd := exec.Command(bin, "-t", "-s", strconv.Itoa(maxEdge), "-o", tmpDir, path)
			if cmd.Run() == nil {
				out := filepath.Join(tmpDir, filepath.Base(path)+".png")
				if img, err := decodeRGBA(out); err == nil {
					return img, CountPDFPagesFallback(path), nil
				}
			}
		}
	}
	return nil, CountPDFPagesFallback(path), errNoPDFRenderer
}
