package library

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"

	"raglex/internal/model"
)

// ExportOptions controls batch file export and image conversion.
type ExportOptions struct {
	DestDir string
	Format  string // "original", "png", "jpg"
	Quality int    // 1..100 for JPEG (default 90)
	MaxEdge int    // 0 = keep original dimensions
}

// ExportResult summarizes a batch export run.
type ExportResult struct {
	Exported int
	Failed   int
	Paths    []string
}

// ExportBatch writes selected assets into opts.DestDir, optionally resizing or
// converting image formats while preserving original files in the library.
func (l *Library) ExportBatch(assetIDs []string, opts ExportOptions) (ExportResult, error) {
	if strings.TrimSpace(opts.DestDir) == "" {
		return ExportResult{}, fmt.Errorf("raglex: destination directory is required")
	}
	if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
		return ExportResult{}, err
	}
	quality := opts.Quality
	if quality <= 0 || quality > 100 {
		quality = 90
	}
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "" {
		format = "original"
	}

	var res ExportResult
	for _, id := range assetIDs {
		a, err := l.Get(id)
		if err != nil || a.Missing || a.Path == "" {
			res.Failed++
			continue
		}
		dst, err := exportOneAsset(a, opts.DestDir, format, quality, opts.MaxEdge)
		if err != nil {
			res.Failed++
			continue
		}
		res.Exported++
		res.Paths = append(res.Paths, dst)
	}
	return res, nil
}

func exportOneAsset(a model.Asset, destDir, format string, quality, maxEdge int) (string, error) {
	base := a.Label()
	ext := filepath.Ext(base)
	if ext == "" {
		ext = filepath.Ext(a.Path)
	}
	stem := sanitizeFileStem(strings.TrimSuffix(base, ext))
	if stem == "" {
		stem = a.ID
	}

	isImage := model.MediaKind(a.Mime) == model.MediaImage
	needTransform := isImage && (format == "png" || format == "jpg" || maxEdge > 0)

	if !needTransform {
		dst := uniqueExportPath(destDir, stem, ext)
		if err := copyFileBytes(a.Path, dst); err != nil {
			return "", err
		}
		return dst, nil
	}

	srcImg, err := imaging.Open(a.Path, imaging.AutoOrientation(true))
	if err != nil {
		// Fall back to raw copy for exotic image formats (e.g., SVG/AVIF) that imaging cannot decode.
		dst := uniqueExportPath(destDir, stem, ext)
		if copyErr := copyFileBytes(a.Path, dst); copyErr != nil {
			return "", err
		}
		return dst, nil
	}

	outImg := resizeMaxEdge(srcImg, maxEdge)
	outExt := strings.ToLower(ext)
	switch format {
	case "png":
		outExt = ".png"
	case "jpg", "jpeg":
		outExt = ".jpg"
	default:
		if outExt != ".png" && outExt != ".jpg" && outExt != ".jpeg" {
			outExt = ".png"
		}
	}

	dst := uniqueExportPath(destDir, stem, outExt)
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	var encErr error
	if outExt == ".jpg" || outExt == ".jpeg" {
		encErr = jpeg.Encode(f, outImg, &jpeg.Options{Quality: quality})
	} else {
		encErr = png.Encode(f, outImg)
	}
	closeErr := f.Close()
	if encErr != nil {
		_ = os.Remove(dst)
		return "", encErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return "", closeErr
	}
	return dst, nil
}

func resizeMaxEdge(img image.Image, maxEdge int) image.Image {
	if maxEdge <= 0 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxEdge && h <= maxEdge {
		return img
	}
	if w >= h {
		return imaging.Resize(img, maxEdge, 0, imaging.Lanczos)
	}
	return imaging.Resize(img, 0, maxEdge, imaging.Lanczos)
}

func uniqueExportPath(dir, stem, ext string) string {
	cand := filepath.Join(dir, stem+ext)
	if _, err := os.Stat(cand); os.IsNotExist(err) {
		return cand
	}
	for n := 2; n < 10000; n++ {
		cand = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s%s", stem, model.NewID(), ext))
}

func copyFileBytes(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}
