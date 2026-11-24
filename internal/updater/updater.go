// Package updater checks GitHub Releases for newer versions of Raglex,
// verifies SHA-256 checksums when published, and performs an atomic
// self-replacement of the running executable.
package updater

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is the application's current semantic version.
// Override at build time with: -ldflags "-X raglex/internal/updater.Version=..."
var Version = "0.3.5"

// DefaultRepo is the upstream GitHub repository slug ("owner/repo").
const DefaultRepo = "Batkixni/Raglex"

// Release describes a downloadable GitHub Release matched to the current OS/arch.
type Release struct {
	TagName      string    `json:"tag_name"`
	Name         string    `json:"name"`
	Body         string    `json:"body"`
	HTMLURL      string    `json:"html_url"`
	Prerelease   bool      `json:"prerelease"`
	PublishedAt  time.Time `json:"published_at"`
	AssetName    string    `json:"asset_name"`
	DownloadURL  string    `json:"download_url"`
	AssetSize    int64     `json:"asset_size"`
	ChecksumsURL string    `json:"checksums_url"`
}

// Progress reports live download / verification / installation status.
type Progress struct {
	Stage   string  // "downloading", "verifying", "extracting", "installing", "done"
	Done    int64
	Total   int64
	Percent float64 // 0..100
}

// Client queries GitHub Releases and applies updates.
type Client struct {
	Repo              string
	CurrentVersion    string
	IncludePrerelease bool
	APIBase           string // defaults to "https://api.github.com"
	Token             string // optional GitHub token (for private beta repos)
	HTTPClient        *http.Client
}

// NewClient constructs an updater client with sensible defaults.
func NewClient(includeBeta bool) *Client {
	token := strings.TrimSpace(os.Getenv("RAGLEX_GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	return &Client{
		Repo:              DefaultRepo,
		CurrentVersion:    Version,
		IncludePrerelease: includeBeta,
		APIBase:           "https://api.github.com",
		Token:             token,
		HTTPClient:        &http.Client{Timeout: 30 * time.Second},
	}
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) apiBase() string {
	base := strings.TrimRight(strings.TrimSpace(c.APIBase), "/")
	if base == "" {
		return "https://api.github.com"
	}
	return base
}

func (c *Client) repo() string {
	r := strings.TrimSpace(c.Repo)
	if r == "" {
		return DefaultRepo
	}
	return r
}

func (c *Client) currentVersion() string {
	v := strings.TrimSpace(c.CurrentVersion)
	if v == "" {
		return Version
	}
	return v
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("User-Agent", "Raglex/"+c.currentVersion())
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return req, nil
}

// Check queries GitHub Releases and returns the latest eligible release and
// whether it is strictly newer than CurrentVersion.
func (c *Client) Check(ctx context.Context) (*Release, bool, error) {
	var candidates []ghRelease
	if c.IncludePrerelease {
		endpoint := fmt.Sprintf("%s/repos/%s/releases?per_page=15", c.apiBase(), c.repo())
		req, err := c.newRequest(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
		if err != nil {
			return nil, false, err
		}
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, false, fmt.Errorf("connect github: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, false, errors.New("no public release found on GitHub")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, false, fmt.Errorf("github api HTTP %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&candidates); err != nil {
			return nil, false, fmt.Errorf("decode releases: %w", err)
		}
	} else {
		endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", c.apiBase(), c.repo())
		req, err := c.newRequest(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
		if err != nil {
			return nil, false, err
		}
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, false, fmt.Errorf("connect github: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, false, errors.New("no stable release published yet")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, false, fmt.Errorf("github api HTTP %d", resp.StatusCode)
		}
		var single ghRelease
		if err := json.NewDecoder(resp.Body).Decode(&single); err != nil {
			return nil, false, fmt.Errorf("decode release: %w", err)
		}
		candidates = append(candidates, single)
	}

	var best *ghRelease
	for i := range candidates {
		rel := &candidates[i]
		if rel.Draft || rel.TagName == "" {
			continue
		}
		if !c.IncludePrerelease && rel.Prerelease {
			continue
		}
		if best == nil || CompareVersion(rel.TagName, best.TagName) > 0 {
			best = rel
		}
	}
	if best == nil {
		return nil, false, errors.New("no eligible release found")
	}

	asset := matchPlatformAsset(best.Assets, runtime.GOOS, runtime.GOARCH)
	out := &Release{
		TagName:      best.TagName,
		Name:         best.Name,
		Body:         best.Body,
		HTMLURL:      best.HTMLURL,
		Prerelease:   best.Prerelease,
		PublishedAt:  best.PublishedAt,
		ChecksumsURL: findChecksumsURL(best.Assets, c.Token != ""),
	}
	if asset != nil {
		out.AssetName = asset.Name
		out.AssetSize = asset.Size
		if c.Token != "" && asset.URL != "" {
			out.DownloadURL = asset.URL
		} else {
			out.DownloadURL = asset.BrowserDownloadURL
		}
	}
	return out, IsNewer(out.TagName, c.currentVersion()), nil
}

// startExe captures the executable path at process startup before any in-place
// update renames the running binary to ".old" on Windows.
var startExe = resolveExecutablePath()

func resolveExecutablePath() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	return strings.TrimSuffix(exePath, ".old")
}

// MatchAsset picks the best desktop release asset for the given OS/arch.
func MatchAsset(names []string, goos, goarch string) string {
	assets := make([]ghAsset, len(names))
	for i, n := range names {
		assets[i] = ghAsset{Name: n}
	}
	if m := matchPlatformAsset(assets, goos, goarch); m != nil {
		return m.Name
	}
	return ""
}

func matchPlatformAsset(assets []ghAsset, goos, goarch string) *ghAsset {
	var best *ghAsset
	bestScore := -1
	osTokens := []string{goos}
	var foreignOS []string
	switch goos {
	case "darwin":
		osTokens = append(osTokens, "macos", "osx")
		foreignOS = []string{"windows", "linux"}
	case "windows":
		osTokens = append(osTokens, "win")
		foreignOS = []string{"darwin", "macos", "osx", "linux"}
	case "linux":
		foreignOS = []string{"darwin", "macos", "osx", "windows"}
	}

	archTokens := []string{goarch}
	var foreignArch []string
	switch goarch {
	case "amd64":
		archTokens = append(archTokens, "x86_64", "x64")
		foreignArch = []string{"arm64", "aarch64"}
	case "arm64":
		archTokens = append(archTokens, "aarch64")
		foreignArch = []string{"amd64", "x86_64", "x64"}
	}

	for i := range assets {
		a := &assets[i]
		low := strings.ToLower(a.Name)
		// Skip installers (NSIS setup, deb, rpm, dmg, msi), CLI bundles, browser
		// extensions, checksums, and signature files so self-update never overwrites
		// the app binary with an installer wizard or non-portable package.
		if strings.Contains(low, "cli") ||
			strings.Contains(low, "server") ||
			strings.Contains(low, "mcp") ||
			strings.Contains(low, "extension") ||
			strings.Contains(low, "checksum") ||
			strings.Contains(low, "sha256") ||
			strings.Contains(low, "setup") ||
			strings.Contains(low, "installer") ||
			strings.Contains(low, "uninstall") ||
			strings.HasSuffix(low, ".txt") ||
			strings.HasSuffix(low, ".sig") ||
			strings.HasSuffix(low, ".deb") ||
			strings.HasSuffix(low, ".rpm") ||
			strings.HasSuffix(low, ".dmg") ||
			strings.HasSuffix(low, ".msi") ||
			strings.HasSuffix(low, ".pkg") {
			continue
		}

		// Hard-reject assets belonging to another OS or CPU architecture.
		skipForeign := false
		for _, bad := range foreignOS {
			if strings.Contains(low, bad) {
				skipForeign = true
				break
			}
		}
		if skipForeign {
			continue
		}
		for _, bad := range foreignArch {
			if strings.Contains(low, bad) {
				skipForeign = true
				break
			}
		}
		if skipForeign {
			continue
		}

		score := 0
		hasOS := false
		for _, tok := range osTokens {
			if strings.Contains(low, tok) {
				hasOS = true
				score += 40
				break
			}
		}
		if !hasOS {
			continue
		}

		hasArch := false
		for _, tok := range archTokens {
			if strings.Contains(low, tok) {
				hasArch = true
				score += 30
				break
			}
		}
		if !hasArch {
			continue
		}

		switch goos {
		case "windows":
			if strings.HasSuffix(low, ".zip") {
				score += 20
			} else if strings.HasSuffix(low, ".exe") {
				score += 18
			} else {
				continue
			}
		case "darwin", "linux":
			if strings.HasSuffix(low, ".zip") {
				score += 20
			} else if strings.HasSuffix(low, ".tar.gz") || strings.HasSuffix(low, ".tgz") {
				score += 16
			} else {
				continue
			}
		}
		if strings.HasPrefix(low, "raglex") {
			score += 10
		}

		if score > bestScore {
			bestScore = score
			best = a
		}
	}
	return best
}

func findChecksumsURL(assets []ghAsset, useAPIURL bool) string {
	for _, a := range assets {
		low := strings.ToLower(a.Name)
		if strings.Contains(low, "sha256") || strings.Contains(low, "checksum") {
			if useAPIURL && a.URL != "" {
				return a.URL
			}
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// DownloadAndApply downloads rel, verifies its checksum (if available),
// extracts the executable when archived, and atomically replaces the currently
// running Raglex binary.
func (c *Client) DownloadAndApply(ctx context.Context, rel *Release, onProgress func(Progress)) error {
	exePath := startExe
	if exePath == "" {
		exePath = resolveExecutablePath()
	}
	if exePath == "" {
		return errors.New("locate current executable: empty executable path")
	}
	return c.DownloadAndApplyTo(ctx, rel, exePath, onProgress)
}

// DownloadAndApplyTo is the testable core of DownloadAndApply targeting targetExe.
func (c *Client) DownloadAndApplyTo(ctx context.Context, rel *Release, targetExe string, onProgress func(Progress)) error {
	if rel == nil || rel.DownloadURL == "" {
		return errors.New("no downloadable binary for this platform in release")
	}
	report := func(p Progress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	tmpDir, err := os.MkdirTemp("", "raglex-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	dlName := rel.AssetName
	if dlName == "" {
		dlName = "raglex-update"
	}
	dlPath := filepath.Join(tmpDir, filepath.Base(dlName))

	// 1. Download release package with progress streaming.
	accept := "application/octet-stream"
	req, err := c.newRequest(ctx, http.MethodGet, rel.DownloadURL, accept)
	if err != nil {
		return err
	}
	dlClient := &http.Client{Timeout: 10 * time.Minute}
	if c.HTTPClient != nil && c.HTTPClient.Transport != nil {
		dlClient.Transport = c.HTTPClient.Transport
	}
	resp, err := dlClient.Do(req)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	if total <= 0 {
		total = rel.AssetSize
	}
	outFile, err := os.Create(dlPath)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	var written int64
	buf := make([]byte, 64*1024)
	lastTick := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := outFile.Write(buf[:n]); werr != nil {
				outFile.Close()
				return werr
			}
			_, _ = hasher.Write(buf[:n])
			written += int64(n)
			if time.Since(lastTick) >= 80*time.Millisecond {
				pct := 0.0
				if total > 0 {
					pct = float64(written) * 100 / float64(total)
				}
				report(Progress{Stage: "downloading", Done: written, Total: total, Percent: pct})
				lastTick = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			outFile.Close()
			return fmt.Errorf("read update stream: %w", rerr)
		}
	}
	if err := outFile.Close(); err != nil {
		return err
	}
	report(Progress{Stage: "downloading", Done: written, Total: max(total, written), Percent: 100})

	// 2. Verify SHA-256 checksum if published alongside the release.
	if rel.ChecksumsURL != "" && rel.AssetName != "" {
		report(Progress{Stage: "verifying", Percent: 100})
		expected, err := c.fetchExpectedSHA256(ctx, rel.ChecksumsURL, rel.AssetName)
		if err != nil {
			return fmt.Errorf("fetch checksums: %w", err)
		}
		if expected != "" {
			actual := hex.EncodeToString(hasher.Sum(nil))
			if !strings.EqualFold(actual, expected) {
				return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
			}
		}
	}

	// 3. Extract executable if .zip, otherwise use downloaded binary directly.
	newBinPath := dlPath
	if strings.HasSuffix(strings.ToLower(dlPath), ".zip") {
		report(Progress{Stage: "extracting", Percent: 100})
		extracted, err := extractExecutableFromZip(dlPath, tmpDir, filepath.Base(targetExe))
		if err != nil {
			return fmt.Errorf("extract update archive: %w", err)
		}
		newBinPath = extracted
	}

	// 4. Atomically replace targetExe (safe even while running on Windows!).
	report(Progress{Stage: "installing", Percent: 100})
	if err := replaceBinary(targetExe, newBinPath); err != nil {
		return fmt.Errorf("install update: %w", err)
	}
	report(Progress{Stage: "done", Percent: 100})
	return nil
}

func (c *Client) fetchExpectedSHA256(ctx context.Context, rawURL, assetName string) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, rawURL, "application/octet-stream")
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("checksums HTTP %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	want := strings.ToLower(filepath.Base(assetName))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if strings.ToLower(filepath.Base(name)) == want {
			return strings.TrimSpace(fields[0]), nil
		}
	}
	return "", sc.Err()
}

func extractExecutableFromZip(zipPath, dstDir, targetBase string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	wantBase := strings.ToLower(targetBase)
	var chosen *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := strings.ToLower(filepath.Base(f.Name))
		if base == wantBase || base == "raglex.exe" || base == "raglex" {
			chosen = f
			break
		}
		if chosen == nil && runtime.GOOS == "windows" && strings.HasSuffix(base, ".exe") &&
			!strings.Contains(base, "server") && !strings.Contains(base, "mcp") &&
			!strings.Contains(base, "setup") && !strings.Contains(base, "installer") &&
			!strings.Contains(base, "uninstall") {
			chosen = f
		}
	}
	if chosen == nil {
		return "", errors.New("no Raglex executable found inside release zip")
	}

	rc, err := chosen.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	outPath := filepath.Join(dstDir, filepath.Base(chosen.Name))
	wf, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(wf, rc); err != nil {
		wf.Close()
		return "", err
	}
	if err := wf.Close(); err != nil {
		return "", err
	}
	return outPath, nil
}

func replaceBinary(targetExe, newBinPath string) error {
	st, err := os.Stat(newBinPath)
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		return errors.New("downloaded binary is empty")
	}
	targetExe = strings.TrimSuffix(targetExe, ".old")
	dir := filepath.Dir(targetExe)
	staged := targetExe + ".new"
	backup := targetExe + ".old"

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	_ = os.Remove(staged)

	if err := copyFile(newBinPath, staged, 0o755); err != nil {
		return err
	}

	// Preserve existing executable permissions on Unix if present.
	if st, err := os.Stat(targetExe); err == nil {
		_ = os.Chmod(staged, st.Mode().Perm()|0o111)
	}

	_ = os.Remove(backup)
	hadTarget := false
	if _, err := os.Stat(targetExe); err == nil {
		hadTarget = true
		if err := os.Rename(targetExe, backup); err != nil {
			_ = os.Remove(staged)
			return err
		}
	}

	if err := os.Rename(staged, targetExe); err != nil {
		if hadTarget {
			_ = os.Rename(backup, targetExe)
		}
		_ = os.Remove(staged)
		return err
	}

	// Best-effort immediate removal on Unix; on Windows the old running binary
	// stays locked until exit and is removed on next launch by CleanupOldBinary.
	_ = os.Remove(backup)
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// CleanupOldBinary removes leftover ".old" executables from a previous update.
func CleanupOldBinary() {
	exePath := startExe
	if exePath == "" {
		exePath = resolveExecutablePath()
	}
	if exePath == "" {
		return
	}
	_ = os.Remove(exePath + ".old")
	_ = os.Remove(exePath + ".new")
}

// Relaunch starts the newly installed executable.
func Relaunch() error {
	exePath := startExe
	if exePath == "" {
		exePath = resolveExecutablePath()
	}
	if exePath == "" {
		return errors.New("locate current executable: empty executable path")
	}
	cmd := exec.Command(exePath, os.Args[1:]...)
	detachCmd(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

// IsNewer returns true when candidate is strictly greater than current per SemVer 2.0.
func IsNewer(candidate, current string) bool {
	return CompareVersion(candidate, current) > 0
}

// CompareVersion compares two SemVer strings (with optional leading "v" and
// pre-release suffixes like "-beta.1"). Returns -1, 0, or +1.
func CompareVersion(a, b string) int {
	va := parseSemVer(a)
	vb := parseSemVer(b)
	if va.major != vb.major {
		return cmpInt(va.major, vb.major)
	}
	if va.minor != vb.minor {
		return cmpInt(va.minor, vb.minor)
	}
	if va.patch != vb.patch {
		return cmpInt(va.patch, vb.patch)
	}
	// Release without pre-release tag is strictly newer than pre-release.
	if va.pre == "" && vb.pre == "" {
		return 0
	}
	if va.pre == "" {
		return 1
	}
	if vb.pre == "" {
		return -1
	}
	pa := strings.Split(va.pre, ".")
	pb := strings.Split(vb.pre, ".")
	n := min(len(pa), len(pb))
	for i := 0; i < n; i++ {
		if pa[i] == pb[i] {
			continue
		}
		na, erra := strconv.Atoi(pa[i])
		nb, errb := strconv.Atoi(pb[i])
		if erra == nil && errb == nil {
			return cmpInt(na, nb)
		}
		if erra == nil {
			return -1
		}
		if errb == nil {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
		return 1
	}
	return cmpInt(len(pa), len(pb))
}

type semVer struct {
	major int
	minor int
	patch int
	pre   string
}

func parseSemVer(raw string) semVer {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}
	var pre string
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	var sv semVer
	sv.pre = pre
	if len(parts) > 0 {
		sv.major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) > 1 {
		sv.minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		sv.patch, _ = strconv.Atoi(parts[2])
	}
	return sv
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
