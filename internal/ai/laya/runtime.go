package laya

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultModel is CPU-friendly and supports Traditional Chinese + English.
	DefaultModelMultilingual = "laya:multilingual"

	ollayaReleasesAPI   = "https://api.github.com/repos/ollaya-dev/ollaya/releases/latest"
	fallbackReleaseTag  = "v0.18.0"
	fallbackDownloadFmt = "https://github.com/ollaya-dev/ollaya/releases/download/%s/%s"
)

// RuntimeStatus describes the current state of the managed in-app Ollaya/Laya runtime.
type RuntimeStatus struct {
	BinaryInstalled bool
	BinaryPath      string
	Running         bool
	ManagedByApp    bool
	InstalledModels []string
	LastError       string
}

// RuntimeManager downloads, starts, stops, and pulls models for the open-source
// Ollaya/Laya runtime directly inside Ragle with zero terminal commands.
type RuntimeManager struct {
	rootDir string
	baseURL string

	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

// NewRuntimeManager creates a manager storing binaries under rootDir (e.g. UserConfigDir/Ragle/runtime/ollaya).
func NewRuntimeManager(rootDir, baseURL string) *RuntimeManager {
	if strings.TrimSpace(rootDir) == "" {
		if cfgDir, err := os.UserConfigDir(); err == nil {
			rootDir = filepath.Join(cfgDir, "Ragle", "runtime", "ollaya")
		} else {
			rootDir = filepath.Join(os.TempDir(), "ragle-ollaya")
		}
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &RuntimeManager{
		rootDir: rootDir,
		baseURL: baseURL,
	}
}

// RootDir returns where the managed Ollaya binary is stored.
func (m *RuntimeManager) RootDir() string { return m.rootDir }

// SetBaseURL updates the target host/port (e.g. http://127.0.0.1:11435).
func (m *RuntimeManager) SetBaseURL(u string) {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u == "" {
		u = DefaultBaseURL
	}
	m.mu.Lock()
	m.baseURL = u
	m.mu.Unlock()
}

// BaseURL returns the configured runtime HTTP base URL.
func (m *RuntimeManager) BaseURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.baseURL
}

// BinaryPath returns the path to an installed ollaya binary (managed dir first, then PATH).
func (m *RuntimeManager) BinaryPath() (string, bool) {
	exeName := "ollaya"
	if runtime.GOOS == "windows" {
		exeName = "ollaya.exe"
	}
	managed := filepath.Join(m.rootDir, exeName)
	if info, err := os.Stat(managed); err == nil && !info.IsDir() {
		return managed, true
	}
	if p, err := exec.LookPath(exeName); err == nil {
		return p, true
	}
	return managed, false
}

// Status probes binary installation, daemon liveness, and pulled models.
func (m *RuntimeManager) Status(ctx context.Context) RuntimeStatus {
	bin, ok := m.BinaryPath()
	m.mu.Lock()
	managed := m.cmd != nil
	base := m.baseURL
	m.mu.Unlock()

	st := RuntimeStatus{
		BinaryInstalled: ok,
		BinaryPath:      bin,
		ManagedByApp:    managed,
	}
	c := NewClient(base, DefaultModelMultilingual)
	c.HTTPClient = &http.Client{Timeout: 1500 * time.Millisecond}
	if err := c.Health(ctx); err == nil {
		st.Running = true
		if models, err := c.ListModels(ctx); err == nil {
			st.InstalledModels = models
		}
	}
	return st
}

// Deploy performs one-click in-app deployment:
//  1. Downloads & extracts the official Ollaya binary if missing
//  2. Starts `ollaya serve` in the background (no console window)
//  3. Pulls modelName (e.g. "laya:multilingual") with live streaming progress
func (m *RuntimeManager) Deploy(ctx context.Context, modelName string, onProgress func(stage string, pct float64)) error {
	if strings.TrimSpace(modelName) == "" {
		modelName = DefaultModelMultilingual
	}
	report := func(stage string, pct float64) {
		if onProgress != nil {
			onProgress(stage, pct)
		}
	}

	if _, installed := m.BinaryPath(); !installed {
		report("download", 0.02)
		if err := m.DownloadBinary(ctx, func(pct float64) {
			report("download", pct)
		}); err != nil {
			return err
		}
	}

	report("start", 0.0)
	if err := m.Start(ctx); err != nil {
		return err
	}

	// Check if requested model is already pulled.
	c := NewClient(m.BaseURL(), modelName)
	if models, err := c.ListModels(ctx); err == nil {
		for _, existing := range models {
			if strings.EqualFold(existing, modelName) || strings.HasPrefix(strings.ToLower(existing), strings.ToLower(modelName)+":") {
				report("ready", 1.0)
				return nil
			}
		}
	}

	report("pull", 0.02)
	if err := c.PullModel(ctx, modelName, func(status string, pct float64) {
		report("pull:"+status, pct)
	}); err != nil {
		return err
	}
	report("ready", 1.0)
	return nil
}

// DownloadBinary fetches the official platform portable archive from
// github.com/ollaya-dev/ollaya releases and extracts the ollaya executable.
func (m *RuntimeManager) DownloadBinary(ctx context.Context, onProgress func(pct float64)) error {
	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return err
	}
	dlURL, assetName, err := resolveOllayaReleaseAsset(ctx)
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Ragle/0.2")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download ollaya failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("download ollaya %s returned HTTP %d", assetName, resp.StatusCode)
	}

	tmpArchive := filepath.Join(m.rootDir, assetName+".part")
	out, err := os.Create(tmpArchive)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	var copied int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				_ = out.Close()
				_ = os.Remove(tmpArchive)
				return werr
			}
			copied += int64(n)
			if onProgress != nil && total > 0 {
				onProgress(float64(copied) / float64(total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = out.Close()
			_ = os.Remove(tmpArchive)
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpArchive)
		return err
	}
	defer os.Remove(tmpArchive)

	switch {
	case strings.HasSuffix(assetName, ".zip"):
		return extractZipBinary(tmpArchive, m.rootDir)
	case strings.HasSuffix(assetName, ".tgz"), strings.HasSuffix(assetName, ".tar.gz"):
		return extractTgzBinary(tmpArchive, m.rootDir)
	default:
		return fmt.Errorf("unsupported archive format %s — install ollaya manually or use Windows/macOS zip/tgz", assetName)
	}
}

// Start launches `ollaya serve` managed by Ragle and waits until the HTTP daemon
// responds (up to 15s). If an external Ollaya daemon is already listening on
// BaseURL, Start succeeds without spawning a duplicate.
func (m *RuntimeManager) Start(ctx context.Context) error {
	base := m.BaseURL()
	c := NewClient(base, DefaultModelMultilingual)
	c.HTTPClient = &http.Client{Timeout: 1200 * time.Millisecond}
	if err := c.Health(ctx); err == nil {
		return nil
	}

	bin, ok := m.BinaryPath()
	if !ok {
		return errors.New("ollaya binary not installed yet")
	}

	hostPort := "127.0.0.1:11435"
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		hostPort = u.Host
	}

	m.mu.Lock()
	if m.cmd != nil {
		m.mu.Unlock()
		return nil
	}
	procCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, bin, "serve")
	cmd.Dir = m.rootDir
	cmd.Env = append(os.Environ(), "OLLAYA_HOST="+hostPort)
	hideConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		cancel()
		m.mu.Unlock()
		return fmt.Errorf("start ollaya serve: %w", err)
	}
	m.cmd = cmd
	m.cancel = cancel
	m.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
			m.cancel = nil
		}
		m.mu.Unlock()
	}()

	// Poll until daemon is live.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.Health(ctx); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("ollaya daemon did not become ready at %s within 15s", base)
}

// Stop terminates the app-managed `ollaya serve` child process, if running.
func (m *RuntimeManager) Stop() {
	m.mu.Lock()
	cmd := m.cmd
	cancel := m.cancel
	m.cmd = nil
	m.cancel = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func desiredAssetName() string {
	switch runtime.GOOS {
	case "windows":
		return "ollaya-windows-amd64.zip"
	case "darwin":
		return "ollaya-darwin-arm64.tgz"
	default:
		return "ollaya-linux-amd64.tar.zst"
	}
}

func resolveOllayaReleaseAsset(ctx context.Context) (string, string, error) {
	want := desiredAssetName()
	httpClient := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ollayaReleasesAPI, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Ragle/0.2")
		req.Header.Set("Accept", "application/vnd.github+json")
		if resp, err := httpClient.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var rel struct {
					Assets []struct {
						Name string `json:"name"`
						URL  string `json:"browser_download_url"`
					} `json:"assets"`
				}
				if json.NewDecoder(resp.Body).Decode(&rel) == nil {
					for _, a := range rel.Assets {
						if a.Name == want && a.URL != "" {
							return a.URL, a.Name, nil
						}
					}
				}
			}
		}
	}
	// Reliable fallback directly to pinned release.
	return fmt.Sprintf(fallbackDownloadFmt, fallbackReleaseTag, want), want, nil
}

func extractZipBinary(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	extracted := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		low := strings.ToLower(base)
		// Extract the ollaya binary and any companion DLLs bundled in the archive.
		if low != "ollaya.exe" && low != "ollaya" && !strings.HasSuffix(low, ".dll") && !strings.HasSuffix(low, ".so") && !strings.HasSuffix(low, ".dylib") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		dst := filepath.Join(destDir, base)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if copyErr != nil {
			return copyErr
		}
		if low == "ollaya.exe" || low == "ollaya" {
			extracted = true
		}
	}
	if !extracted {
		return errors.New("ollaya binary not found inside downloaded zip")
	}
	return nil
}

func extractTgzBinary(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	extracted := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(hdr.Name)
		low := strings.ToLower(base)
		if low != "ollaya" && !strings.HasSuffix(low, ".dylib") && !strings.HasSuffix(low, ".so") {
			continue
		}
		dst := filepath.Join(destDir, base)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, tr)
		_ = out.Close()
		if copyErr != nil {
			return copyErr
		}
		if low == "ollaya" {
			extracted = true
		}
	}
	if !extracted {
		return errors.New("ollaya binary not found inside downloaded archive")
	}
	return nil
}

// ListModels queries Ollaya's GET /api/tags endpoint for locally pulled models.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET /api/tags status %d", resp.StatusCode)
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out, nil
}

// PullModel calls Ollaya's POST /api/pull with NDJSON streaming progress.
func (c *Client) PullModel(ctx context.Context, modelName string, onProgress func(status string, pct float64)) error {
	payload, _ := json.Marshal(map[string]any{
		"model":  modelName,
		"name":   modelName,
		"stream": true,
	})
	httpClient := &http.Client{Timeout: 20 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/pull", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollaya pull failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ollaya pull status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 16*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var evt struct {
			Status    string `json:"status"`
			Error     string `json:"error"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
		}
		if json.Unmarshal([]byte(line), &evt) != nil {
			continue
		}
		if evt.Error != "" {
			return errors.New(evt.Error)
		}
		pct := 0.0
		if evt.Total > 0 {
			pct = float64(evt.Completed) / float64(evt.Total)
		}
		if onProgress != nil {
			onProgress(evt.Status, pct)
		}
	}
	return sc.Err()
}
