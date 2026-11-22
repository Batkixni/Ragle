package preview

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var rePDFPageObj = regexp.MustCompile(`/Type\s*/Page\b`)

// CountPDFPagesFallback estimates page count by scanning raw PDF page objects.
func CountPDFPagesFallback(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 1
	}
	// Strip /Type /Pages before counting /Type /Page.
	cleaned := bytes.ReplaceAll(raw, []byte("/Pages"), []byte("/_____"))
	n := len(rePDFPageObj.FindAllIndex(cleaned, -1))
	if n < 1 {
		return 1
	}
	return n
}

// RenderPDFPage renders 0-based pageIndex of the PDF at path into an RGBA image
// and returns (image, totalPages, err).
func RenderPDFPage(path string, pageIndex int, maxEdge int) (*image.RGBA, int, error) {
	if pageIndex < 0 {
		pageIndex = 0
	}
	if maxEdge < 256 {
		maxEdge = 1280
	}
	// Fast path 1: poppler pdftoppm (cross-platform if installed).
	if bin, err := exec.LookPath("pdftoppm"); err == nil {
		if img, total, rerr := renderPDFWithPdftoppm(bin, path, pageIndex, maxEdge); rerr == nil {
			return img, total, nil
		}
	}
	// Fast path 2: MuPDF mutool (cross-platform if installed).
	if bin, err := exec.LookPath("mutool"); err == nil {
		if img, total, rerr := renderPDFWithMutool(bin, path, pageIndex, maxEdge); rerr == nil {
			return img, total, nil
		}
	}
	// Native OS renderer (Windows 10/11 built-in Windows.Data.Pdf WinRT; macOS qlmanage).
	return renderPDFNative(path, pageIndex, maxEdge)
}

func renderPDFWithPdftoppm(bin, path string, pageIndex, maxEdge int) (*image.RGBA, int, error) {
	tmpDir, err := os.MkdirTemp("", "ragle-pdf-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(tmpDir)

	prefix := filepath.Join(tmpDir, "page")
	pageNum := strconv.Itoa(pageIndex + 1)
	cmd := exec.Command(bin, "-png", "-singlefile", "-scale-to", strconv.Itoa(maxEdge), "-f", pageNum, "-l", pageNum, path, prefix)
	hideCmd(cmd)
	if err := cmd.Run(); err != nil {
		return nil, 0, err
	}
	img, err := decodeRGBA(prefix + ".png")
	if err != nil {
		return nil, 0, err
	}
	total := pdfInfoPages(path)
	if total < pageIndex+1 {
		total = max(pageIndex+1, CountPDFPagesFallback(path))
	}
	return img, total, nil
}

func renderPDFWithMutool(bin, path string, pageIndex, maxEdge int) (*image.RGBA, int, error) {
	tmpDir, err := os.MkdirTemp("", "ragle-pdf-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(tmpDir)

	out := filepath.Join(tmpDir, "page.png")
	cmd := exec.Command(bin, "draw", "-r", "144", "-o", out, path, strconv.Itoa(pageIndex+1))
	hideCmd(cmd)
	if err := cmd.Run(); err != nil {
		return nil, 0, err
	}
	img, err := decodeRGBA(out)
	if err != nil {
		return nil, 0, err
	}
	total := CountPDFPagesFallback(path)
	if total < pageIndex+1 {
		total = pageIndex + 1
	}
	return img, total, nil
}

func pdfInfoPages(path string) int {
	bin, err := exec.LookPath("pdfinfo")
	if err != nil {
		return 0
	}
	cmd := exec.Command(bin, path)
	hideCmd(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
			if n > 0 {
				return n
			}
		}
	}
	return 0
}

func decodeRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba, nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst, nil
}

var errNoPDFRenderer = errors.New("no PDF renderer available on this system")

func errPDFMsg(err error) error {
	if err == nil {
		return errNoPDFRenderer
	}
	return fmt.Errorf("render pdf: %w", err)
}
