// Package ffmpeg resolves a usable ffmpeg / ffprobe binary with zero external
// user setup: first checking binaries bundled alongside Raglex (or inside
// Raglex.app/Contents/Resources), then Raglex's app-managed cache directory,
// and finally the system PATH. It can also download a verified static ffmpeg
// binary on demand for standalone portable builds.
package ffmpeg

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	SourceNone    = ""
	SourceBundled = "bundled"
	SourceManaged = "managed"
	SourceSystem  = "system"
)

const staticReleaseTag = "b6.1.1"

var (
	cacheMu     sync.RWMutex
	cachedReady bool
	cachedPath  string
	cachedSrc   string

	probeMu       sync.RWMutex
	probeReady    bool
	probeCached   string

	// Overridable hooks for deterministic unit tests.
	execPathFn      = os.Executable
	userConfigDirFn = os.UserConfigDir
	lookPathFn      = exec.LookPath
	httpClient      = &http.Client{Timeout: 10 * time.Minute}
)

// ResetCache clears cached ffmpeg/ffprobe lookup results. Call this after
// installing or removing a managed binary.
func ResetCache() {
	cacheMu.Lock()
	cachedReady = false
	cachedPath = ""
	cachedSrc = ""
	cacheMu.Unlock()

	probeMu.Lock()
	probeReady = false
	probeCached = ""
	probeMu.Unlock()
}

// Available reports whether a usable ffmpeg executable is available.
func Available() bool {
	return Find() != ""
}

// Find returns the resolved path to ffmpeg, or "" if unavailable.
func Find() string {
	p, _ := Detect()
	return p
}

// Detect returns both the resolved ffmpeg path and where it was found:
// SourceBundled, SourceManaged, SourceSystem, or SourceNone.
func Detect() (string, string) {
	cacheMu.RLock()
	if cachedReady {
		p, s := cachedPath, cachedSrc
		cacheMu.RUnlock()
		return p, s
	}
	cacheMu.RUnlock()

	p, s := resolveBinary("ffmpeg")
	cacheMu.Lock()
	cachedReady = true
	cachedPath = p
	cachedSrc = s
	cacheMu.Unlock()
	return p, s
}

// FindFFprobe returns the resolved path to ffprobe, or "" if unavailable.
func FindFFprobe() string {
	probeMu.RLock()
	if probeReady {
		p := probeCached
		probeMu.RUnlock()
		return p
	}
	probeMu.RUnlock()

	p, _ := resolveBinary("ffprobe")
	probeMu.Lock()
	probeReady = true
	probeCached = p
	probeMu.Unlock()
	return p
}

func resolveBinary(base string) (string, string) {
	name := binaryName(base, runtime.GOOS)

	// Tier 1: Bundled right next to the Raglex executable or inside a macOS .app bundle.
	if exe, err := execPathFn(); err == nil && exe != "" {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
			exe = resolved
		}
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, name),
			filepath.Join(exeDir, "bin", name),
			filepath.Join(exeDir, "resources", name),
			// macOS Raglex.app/Contents/MacOS/Raglex -> ../Resources/ffmpeg
			filepath.Join(exeDir, "..", "Resources", name),
			filepath.Join(exeDir, "..", "Resources", "bin", name),
			filepath.Join(exeDir, "..", "Helpers", name),
			// Linux /usr/share or ~/.local/share layout next to bin/raglex
			filepath.Join(exeDir, "..", "lib", "raglex", name),
		}
		for _, cand := range candidates {
			if isExecutableFile(cand) {
				if abs, err := filepath.Abs(cand); err == nil {
					return abs, SourceBundled
				}
				return cand, SourceBundled
			}
		}
	}

	// Tier 2: App-managed local binary cache (<UserConfigDir>/Raglex/bin/).
	if dir, err := ManagedDir(); err == nil && dir != "" {
		cand := filepath.Join(dir, name)
		if isExecutableFile(cand) {
			return cand, SourceManaged
		}
	}

	// Tier 3: System PATH fallback.
	if p, err := lookPathFn(base); err == nil && p != "" {
		return p, SourceSystem
	}
	return "", SourceNone
}

// ManagedDir returns Raglex's per-user directory for auto-downloaded media tools.
func ManagedDir() (string, error) {
	cfg, err := userConfigDirFn()
	if err != nil || strings.TrimSpace(cfg) == "" {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(cfg, "Raglex", "bin"), nil
}

// ManagedPath returns the expected path for Raglex's auto-downloaded ffmpeg binary.
func ManagedPath() (string, error) {
	dir, err := ManagedDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, binaryName("ffmpeg", runtime.GOOS)), nil
}

func binaryName(base, goos string) string {
	if goos == "windows" {
		return base + ".exe"
	}
	return base
}

func isExecutableFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() < 16 {
		return false
	}
	if runtime.GOOS != "windows" && fi.Mode()&0111 == 0 {
		return false
	}
	return true
}

// DownloadSpec describes one static ffmpeg download source.
type DownloadSpec struct {
	URL    string
	Format string // "gz" or "zip"
}

// DownloadSpecs returns ordered static ffmpeg download candidates for goos/goarch.
func DownloadSpecs(goos, goarch string) ([]DownloadSpec, error) {
	base := "https://github.com/eugeneware/ffmpeg-static/releases/download/" + staticReleaseTag
	switch goos + "/" + goarch {
	case "windows/amd64":
		return []DownloadSpec{
			{URL: base + "/ffmpeg-win32-x64.gz", Format: "gz"},
		}, nil
	case "windows/arm64":
		return []DownloadSpec{
			{URL: "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-winarm64-gpl.zip", Format: "zip"},
			{URL: base + "/ffmpeg-win32-x64.gz", Format: "gz"},
		}, nil
	case "darwin/arm64":
		return []DownloadSpec{
			{URL: base + "/ffmpeg-darwin-arm64.gz", Format: "gz"},
		}, nil
	case "darwin/amd64":
		return []DownloadSpec{
			{URL: base + "/ffmpeg-darwin-x64.gz", Format: "gz"},
		}, nil
	case "linux/amd64":
		return []DownloadSpec{
			{URL: base + "/ffmpeg-linux-x64.gz", Format: "gz"},
		}, nil
	case "linux/arm64":
		return []DownloadSpec{
			{URL: base + "/ffmpeg-linux-arm64.gz", Format: "gz"},
		}, nil
	default:
		return nil, fmt.Errorf("no prebuilt static ffmpeg for %s/%s", goos, goarch)
	}
}

// InstallManaged downloads and installs a static ffmpeg binary into ManagedDir(),
// reporting download progress via onProgress(bytesRead, totalBytes) if non-nil.
func InstallManaged(ctx context.Context, onProgress func(done, total int64)) (string, error) {
	specs, err := DownloadSpecs(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	return installFromSpecs(ctx, specs, onProgress)
}

func installFromSpecs(ctx context.Context, specs []DownloadSpec, onProgress func(done, total int64)) (string, error) {
	dir, err := ManagedDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create ffmpeg dir: %w", err)
	}
	dest := filepath.Join(dir, binaryName("ffmpeg", runtime.GOOS))

	var lastErr error
	for _, spec := range specs {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err := downloadAndExtract(ctx, spec, dest, onProgress); err == nil {
			ResetCache()
			return dest, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no ffmpeg download sources available")
	}
	return "", lastErr
}

func downloadAndExtract(ctx context.Context, spec DownloadSpec, dest string, onProgress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Raglex/ffmpeg-installer")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download ffmpeg: HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	pr := &progressReader{r: resp.Body, total: total, onProgress: onProgress}

	tmpOut, err := os.CreateTemp(filepath.Dir(dest), ".ffmpeg-install-*")
	if err != nil {
		return err
	}
	tmpPath := tmpOut.Name()
	cleanup := true
	defer func() {
		_ = tmpOut.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	switch spec.Format {
	case "gz":
		gz, err := gzip.NewReader(pr)
		if err != nil {
			return fmt.Errorf("open gzip stream: %w", err)
		}
		_, copyErr := io.Copy(tmpOut, gz)
		closeErr := gz.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	case "zip":
		tmpZip, err := os.CreateTemp(filepath.Dir(dest), ".ffmpeg-archive-*.zip")
		if err != nil {
			return err
		}
		zipPath := tmpZip.Name()
		defer os.Remove(zipPath)
		_, copyErr := io.Copy(tmpZip, pr)
		_ = tmpZip.Close()
		if copyErr != nil {
			return copyErr
		}
		if err := extractBinaryFromZip(zipPath, binaryName("ffmpeg", runtime.GOOS), tmpOut); err != nil {
			return err
		}
	default:
		if _, err := io.Copy(tmpOut, pr); err != nil {
			return err
		}
	}

	if err := tmpOut.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		// On Windows os.Rename fails if dest already exists; replace cleanly.
		_ = os.Remove(dest)
		if err2 := os.Rename(tmpPath, dest); err2 != nil {
			return err2
		}
	}
	cleanup = false
	return nil
}

func extractBinaryFromZip(zipPath, targetBase string, dst io.Writer) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Base(f.Name), targetBase) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(dst, rc)
			_ = rc.Close()
			return copyErr
		}
	}
	return fmt.Errorf("archive does not contain %s", targetBase)
}

type progressReader struct {
	r          io.Reader
	done       int64
	total      int64
	lastEmit   time.Time
	onProgress func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		if p.onProgress != nil && (p.lastEmit.IsZero() || time.Since(p.lastEmit) >= 80*time.Millisecond || err == io.EOF) {
			p.lastEmit = time.Now()
			p.onProgress(p.done, p.total)
		}
	}
	return n, err
}
