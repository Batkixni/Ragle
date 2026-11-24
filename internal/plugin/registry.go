package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// DefaultRegistryURL is the community GitHub PR plugin catalog fetched by Raglex.
// Plugin authors submit a PR adding plugins/<id>/plugin.json to Batkixni/raglex-plugins;
// CI validates the JSON & SHA-256 and builds registry.json automatically.
const DefaultRegistryURL = "https://raw.githubusercontent.com/Batkixni/raglex-plugins/main/registry.json"

const (
	maxRegistryBytes = 4 << 20   // 4 MiB
	maxPluginZipSize = 64 << 20  // 64 MiB
	maxExtractedSize = 128 << 20 // 128 MiB
)

var validPluginIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

// RegistryEntry describes a community or official plugin listed in registry.json.
type RegistryEntry struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Author           string   `json:"author"`
	Description      string   `json:"description"`
	Icon             string   `json:"icon,omitempty"`   // semantic icon name or icon URL/file
	Accent           string   `json:"accent,omitempty"` // optional #RRGGBB tint for icon badge
	Category         string   `json:"category,omitempty"` // preview, export, import, utility, ai
	Repository       string   `json:"repository,omitempty"`
	DownloadURL      string   `json:"download_url,omitempty"`
	SHA256           string   `json:"sha256,omitempty"`
	MinRaglexVersion string   `json:"min_raglex_version,omitempty"`
	Hooks            []string `json:"hooks,omitempty"`
	Permissions      []string `json:"permissions,omitempty"`
	Official         bool     `json:"official,omitempty"`
	Builtin          bool     `json:"builtin,omitempty"` // bundled starter template installable offline
}

// RegistryCatalog is the top-level JSON document published by Batkixni/raglex-plugins.
type RegistryCatalog struct {
	SchemaVersion int             `json:"schema_version"`
	UpdatedAt     string          `json:"updated_at,omitempty"`
	Plugins       []RegistryEntry `json:"plugins"`
}

// ValidateRegistryEntry enforces safe plugin IDs, required metadata, and HTTPS
// download URLs (also used by PR validation tools).
func ValidateRegistryEntry(e RegistryEntry) error {
	id := strings.TrimSpace(e.ID)
	if !validPluginIDRe.MatchString(id) {
		return fmt.Errorf("invalid plugin id %q (use lowercase a-z, 0-9, '.', '_', '-')", e.ID)
	}
	if strings.TrimSpace(e.Name) == "" {
		return fmt.Errorf("plugin %q missing name", id)
	}
	if strings.TrimSpace(e.Version) == "" {
		return fmt.Errorf("plugin %q missing version", id)
	}
	if !e.Builtin {
		u := strings.TrimSpace(e.DownloadURL)
		if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://127.0.0.1") && !strings.HasPrefix(u, "http://localhost") {
			return fmt.Errorf("plugin %q download_url must use https://", id)
		}
		if len(strings.TrimSpace(e.SHA256)) != 64 {
			return fmt.Errorf("plugin %q requires a 64-char sha256 checksum", id)
		}
	}
	return nil
}

// BuiltinStarterCatalog returns official reference plugins bundled with Raglex
// so users can try 1-click plugin installation immediately (even offline) and
// developers have clean reference implementations.
func BuiltinStarterCatalog() []RegistryEntry {
	return []RegistryEntry{
		{
			ID:               "raglex.library-migrator",
			Name:             "Eagle Import",
			Version:          "1.1.1",
			Author:           "Raglex Official",
			Description:      "Import Eagle .library folders or .eaglepack archives into Raglex, restoring nested folders and colors while preserving tags, star ratings, notes, and source URLs.",
			Icon:             "import",
			Accent:           "#FF5C33",
			Category:         "import",
			Repository:       "https://github.com/Batkixni/Raglex/blob/main/docs/PLUGIN_SDK.md",
			MinRaglexVersion: "0.3.3",
			Hooks:            []string{"importer", "library_action"},
			Permissions:      []string{"read_library", "write_tags", "disk"},
			Official:         true,
			Builtin:          true,
		},
		{
			ID:               "raglex.duplicate-finder",
			Name:             "Smart Duplicate Finder",
			Version:          "1.0.1",
			Author:           "Raglex Official",
			Description:      "Scan your library for potential duplicate assets by filename and dimensions, automatically tagging matches with #Duplicate-Review for quick cleanup.",
			Icon:             "duplicate",
			Accent:           "#A855F7",
			Category:         "utility",
			Repository:       "https://github.com/Batkixni/Raglex/blob/main/docs/PLUGIN_SDK.md",
			MinRaglexVersion: "0.3.3",
			Hooks:            []string{"library_action"},
			Permissions:      []string{"read_library", "write_tags"},
			Official:         true,
			Builtin:          true,
		},
		{
			ID:               "raglex.markdown-notes",
			Name:             "Markdown Notes & Moodboard Brief",
			Version:          "0.3.1",
			Author:           "Raglex Official",
			Description:      "Preview and edit .md notes natively inside Raglex with a borderless writing desk, and export selected reference assets into a structured Markdown brief.",
			Icon:             "markdown",
			Accent:           "#38BDF8",
			Category:         "preview",
			Repository:       "https://github.com/Batkixni/Raglex/blob/main/docs/PLUGIN_SDK.md",
			MinRaglexVersion: "0.2.1",
			Hooks:            []string{"preview_renderer", "context_menu", "batch_action"},
			Permissions:      []string{"read_library", "disk"},
			Official:         true,
			Builtin:          true,
		},
		{
			ID:               "raglex.palette-export",
			Name:             "Color Palette Exporter (CSS / JSON)",
			Version:          "0.2.1",
			Author:           "Raglex Official",
			Description:      "Export dominant color palettes from selected assets as CSS custom properties (:root) and design tokens ready to paste into frontend projects.",
			Icon:             "palette",
			Accent:           "#10B981",
			Category:         "export",
			Repository:       "https://github.com/Batkixni/Raglex/blob/main/docs/PLUGIN_SDK.md",
			MinRaglexVersion: "0.2.1",
			Hooks:            []string{"context_menu", "batch_action"},
			Permissions:      []string{"read_library", "disk"},
			Official:         true,
			Builtin:          true,
		},
		{
			ID:               "raglex.auto-aspect-tagger",
			Name:             "Auto Aspect Ratio & Resolution Tagger",
			Version:          "0.2.1",
			Author:           "Raglex Official",
			Description:      "Automatically tag images by orientation and resolution (Portrait, Landscape, Square, Ultrawide, 4K+).",
			Icon:             "crop",
			Accent:           "#F59E0B",
			Category:         "utility",
			Repository:       "https://github.com/Batkixni/Raglex/blob/main/docs/PLUGIN_SDK.md",
			MinRaglexVersion: "0.2.1",
			Hooks:            []string{"context_menu", "batch_action"},
			Permissions:      []string{"read_library", "write_tags"},
			Official:         true,
			Builtin:          true,
		},
	}
}

// FetchRegistry downloads registry.json from registryURL (defaulting to
// DefaultRegistryURL) and merges it with BuiltinStarterCatalog. Remote entries
// take precedence over built-in starters with the same ID. If the remote fetch
// fails (offline or repo not yet published), the built-in catalog is still
// returned alongside the fetch error so the UI can show an informative banner.
func FetchRegistry(ctx context.Context, registryURL string) ([]RegistryEntry, error) {
	url := strings.TrimSpace(registryURL)
	if url == "" {
		url = DefaultRegistryURL
	}

	merged := map[string]RegistryEntry{}
	for _, b := range BuiltinStarterCatalog() {
		merged[b.ID] = b
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return sortRegistry(merged), err
	}
	req.Header.Set("User-Agent", "Raglex-PluginRegistry/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sortRegistry(merged), err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return sortRegistry(merged), fmt.Errorf("registry HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryBytes))
	if err != nil {
		return sortRegistry(merged), err
	}

	var cat RegistryCatalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		// Also accept bare JSON array of RegistryEntry for convenience.
		var arr []RegistryEntry
		if err2 := json.Unmarshal(raw, &arr); err2 != nil {
			return sortRegistry(merged), fmt.Errorf("parse registry.json: %w", err)
		}
		cat.Plugins = arr
	}

	for _, e := range cat.Plugins {
		if err := ValidateRegistryEntry(e); err != nil {
			continue
		}
		merged[e.ID] = e
	}
	return sortRegistry(merged), nil
}

func sortRegistry(m map[string]RegistryEntry) []RegistryEntry {
	out := make([]RegistryEntry, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Official != out[j].Official {
			return out[i].Official
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Install installs (or updates) a plugin from a RegistryEntry into the library's
// plugins folder, verifying SHA-256 and zip-slip safety for remote archives.
func (m *Manager) Install(ctx context.Context, entry RegistryEntry) error {
	if m == nil {
		return errors.New("plugin manager unavailable")
	}
	if err := ValidateRegistryEntry(entry); err != nil {
		return err
	}
	if strings.TrimSpace(entry.DownloadURL) != "" {
		return m.installRemoteZip(ctx, entry)
	}
	if entry.Builtin {
		return m.installBuiltinStarter(entry)
	}
	return fmt.Errorf("plugin %q has no download_url", entry.ID)
}

func (m *Manager) installRemoteZip(ctx context.Context, entry RegistryEntry) error {
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, entry.DownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Raglex-PluginInstaller/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", entry.ID, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginZipSize+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxPluginZipSize {
		return fmt.Errorf("plugin archive exceeds %d MB limit", maxPluginZipSize>>20)
	}

	// Strictly verify SHA-256 before extracting a single byte.
	sum := sha256.Sum256(data)
	gotHex := hex.EncodeToString(sum[:])
	wantHex := strings.ToLower(strings.TrimSpace(entry.SHA256))
	if wantHex != "" && gotHex != wantHex {
		return fmt.Errorf("sha256 mismatch for %s: got %s, want %s", entry.ID, gotHex, wantHex)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid plugin zip: %w", err)
	}

	// Extract to a temp directory first, verify manifest.json, then swap atomically.
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	stageDir, err := os.MkdirTemp(m.dir, ".stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)

	var totalExtracted int64
	prefix := commonZipRootDir(zr.File)
	for _, f := range zr.File {
		rel := strings.TrimPrefix(filepath.ToSlash(f.Name), prefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		cleanRel := filepath.Clean(filepath.FromSlash(rel))
		if cleanRel == "." || strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
			return fmt.Errorf("unsafe zip entry path: %q", f.Name)
		}
		dst := filepath.Join(stageDir, cleanRel)
		if !isWithinDir(stageDir, dst) {
			return fmt.Errorf("zip entry escapes target dir: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		outFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()&0o755|0o600)
		if err != nil {
			_ = rc.Close()
			return err
		}
		n, copyErr := io.Copy(outFile, io.LimitReader(rc, maxExtractedSize-totalExtracted+1))
		_ = outFile.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
		totalExtracted += n
		if totalExtracted > maxExtractedSize {
			return errors.New("extracted plugin exceeds size limit")
		}
	}

	rawMf, err := os.ReadFile(filepath.Join(stageDir, "manifest.json"))
	if err != nil {
		return errors.New("plugin archive missing manifest.json")
	}
	var mf Manifest
	if err := json.Unmarshal(rawMf, &mf); err != nil || len(mf.Command) == 0 {
		return errors.New("plugin manifest.json is invalid or missing command")
	}

	targetDir := filepath.Join(m.dir, entry.ID)
	if !isWithinDir(m.dir, targetDir) {
		return fmt.Errorf("unsafe plugin target dir for %q", entry.ID)
	}
	_ = os.RemoveAll(targetDir)
	if err := os.Rename(stageDir, targetDir); err != nil {
		return err
	}
	return m.Reload()
}

// Uninstall removes an installed plugin directory and reloads the catalog.
func (m *Manager) Uninstall(pluginID string) error {
	if m == nil {
		return errors.New("plugin manager unavailable")
	}
	m.mu.RLock()
	p, ok := m.plugins[pluginID]
	var dir string
	if ok {
		dir = p.Dir
	}
	m.mu.RUnlock()
	if !ok || dir == "" {
		return fmt.Errorf("plugin %q is not installed", pluginID)
	}
	if !isWithinDir(m.dir, dir) {
		return fmt.Errorf("refusing to delete path outside plugins dir: %s", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return m.Reload()
}

// IsInstalled returns the installed version and whether pluginID is present.
func (m *Manager) IsInstalled(pluginID string) (string, bool) {
	if m == nil {
		return "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.plugins[pluginID]; ok {
		return p.Manifest.Version, true
	}
	return "", false
}

// installBuiltinStarter writes a ready-to-run reference plugin script and
// manifest.json into <library>/plugins/<id>/ using cross-platform Python3 or
// PowerShell/sh fallback so users can try official starter plugins immediately.
func (m *Manager) installBuiltinStarter(entry RegistryEntry) error {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	targetDir := filepath.Join(m.dir, entry.ID)
	if !isWithinDir(m.dir, targetDir) {
		return fmt.Errorf("unsafe plugin id %q", entry.ID)
	}
	_ = os.RemoveAll(targetDir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	var scriptName, scriptBody string
	var cmd []string
	if runtime.GOOS == "windows" {
		scriptName = "run.ps1"
		cmd = []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "./run.ps1"}
		scriptBody = builtinStarterPowerShell(entry.ID)
	} else {
		scriptName = "run.py"
		cmd = []string{"python3", "./run.py"}
		scriptBody = builtinStarterPython(entry.ID)
	}

	var exts []string
	if entry.ID == "raglex.markdown-notes" {
		exts = []string{".md", ".markdown", ".txt"}
	}
	mf := Manifest{
		ID:          entry.ID,
		Name:        entry.Name,
		Version:     entry.Version,
		Description: entry.Description,
		Author:      entry.Author,
		Icon:        entry.Icon,
		Accent:      entry.Accent,
		Command:     cmd,
		Hooks:       entry.Hooks,
		Extensions:  exts,
		Permissions: entry.Permissions,
	}
	rawMf, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "manifest.json"), rawMf, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(targetDir, scriptName), []byte(scriptBody), 0o755); err != nil {
		return err
	}
	return m.Reload()
}

func builtinStarterPowerShell(id string) string {
	switch id {
	case "raglex.markdown-notes":
		return `$raw = [Console]::In.ReadToEnd()
$req = $raw | ConvertFrom-Json
if ($req.hook -eq "preview_renderer" -and $req.assets.Count -gt 0) {
    $a = $req.assets[0]
    $txt = ""
    if ($a.path -and (Test-Path -LiteralPath $a.path)) {
        $txt = Get-Content -LiteralPath $a.path -Raw -Encoding UTF8
        if ($txt.Length -gt 24000) { $txt = $txt.Substring(0, 24000) + [Environment]::NewLine + "... (truncated)" }
    }
    @{ ok = $true; preview_title = $a.name; preview_text = $txt } | ConvertTo-Json -Compress
    exit 0
}
$outPath = Join-Path $req.library_path "raglex-moodboard-notes.md"
$lines = @("# Raglex Moodboard Notes", "")
foreach ($a in $req.assets) {
    $lines += "## $($a.name)"
    if ($a.width -gt 0) { $lines += "- Dimensions: $($a.width) x $($a.height)" }
    if ($a.source_url) { $lines += "- Source: $($a.source_url)" }
    if ($a.notes) { $lines += "- Notes: $($a.notes)" }
    if ($a.palette -and $a.palette.Count -gt 0) {
        $hexes = ($a.palette | ForEach-Object { $_.Hex }) -join ", "
        $lines += "- Palette: $hexes"
    }
    $lines += ""
}
Set-Content -Path $outPath -Value ($lines -join [Environment]::NewLine) -Encoding UTF8
@{ ok = $true; toast = "Exported Markdown to raglex-moodboard-notes.md ($($req.assets.Count) assets)" } | ConvertTo-Json -Compress
`
	case "raglex.palette-export":
		return `$raw = [Console]::In.ReadToEnd()
$req = $raw | ConvertFrom-Json
$outPath = Join-Path $req.library_path "raglex-palette.css"
$lines = @(":root {")
$idx = 1
foreach ($a in $req.assets) {
    foreach ($p in $a.palette) {
        if ($p.Hex) {
            $lines += "  --raglex-color-${idx}: $($p.Hex);"
            $idx++
        }
    }
}
$lines += "}"
Set-Content -Path $outPath -Value ($lines -join [Environment]::NewLine) -Encoding UTF8
@{ ok = $true; toast = "Exported $($idx - 1) CSS palette tokens to raglex-palette.css" } | ConvertTo-Json -Compress
`
	default: // raglex.auto-aspect-tagger
		return `$raw = [Console]::In.ReadToEnd()
$req = $raw | ConvertFrom-Json
$add = @{}
foreach ($a in $req.assets) {
    $tags = @()
    if ($a.width -gt 0 -and $a.height -gt 0) {
        $ratio = [double]$a.width / [double]$a.height
        if ($ratio -ge 1.9) { $tags += "Ultrawide" }
        elseif ($ratio -gt 1.1) { $tags += "Landscape" }
        elseif ($ratio -lt 0.9) { $tags += "Portrait" }
        else { $tags += "Square" }
        if ($a.width -ge 3840 -or $a.height -ge 2160) { $tags += "4K+" }
    }
    if ($tags.Count -gt 0) { $add[$a.id] = $tags }
}
@{ ok = $true; reload = $true; add_tags = $add; toast = "Auto-tagged $($add.Count) assets by aspect ratio" } | ConvertTo-Json -Depth 4 -Compress
`
	}
}

func builtinStarterPython(id string) string {
	switch id {
	case "raglex.markdown-notes":
		return `#!/usr/bin/env python3
import json, os, sys
req = json.load(sys.stdin)
if req.get("hook") == "preview_renderer" and req.get("assets"):
    a = req["assets"][0]
    txt = ""
    p = a.get("path", "")
    if p and os.path.isfile(p):
        with open(p, "r", encoding="utf-8", errors="replace") as f:
            txt = f.read(24000)
    print(json.dumps({"ok": True, "preview_title": a.get("name", "Markdown"), "preview_text": txt}))
    sys.exit(0)
out_path = os.path.join(req["library_path"], "raglex-moodboard-notes.md")
lines = ["# Raglex Moodboard Notes", ""]
for a in req.get("assets", []):
    lines.append(f"## {a.get('name', 'Asset')}")
    if a.get("width", 0) > 0:
        lines.append(f"- Dimensions: {a['width']} x {a['height']}")
    if a.get("source_url"):
        lines.append(f"- Source: {a['source_url']}")
    if a.get("notes"):
        lines.append(f"- Notes: {a['notes']}")
    pal = [p.get("Hex") or p.get("hex") for p in a.get("palette", []) if p.get("Hex") or p.get("hex")]
    if pal:
        lines.append(f"- Palette: {', '.join(pal)}")
    lines.append("")
with open(out_path, "w", encoding="utf-8") as f:
    f.write("\n".join(lines))
print(json.dumps({"ok": True, "toast": f"Exported Markdown to raglex-moodboard-notes.md ({len(req.get('assets', []))} assets)"}))
`
	case "raglex.palette-export":
		return `#!/usr/bin/env python3
import json, os, sys
req = json.load(sys.stdin)
out_path = os.path.join(req["library_path"], "raglex-palette.css")
lines = [":root {"]
idx = 1
for a in req.get("assets", []):
    for p in a.get("palette", []):
        hx = p.get("Hex") or p.get("hex")
        if hx:
            lines.append(f"  --raglex-color-{idx}: {hx};")
            idx += 1
lines.append("}")
with open(out_path, "w", encoding="utf-8") as f:
    f.write("\n".join(lines))
print(json.dumps({"ok": True, "toast": f"Exported {idx - 1} CSS palette tokens to raglex-palette.css"}))
`
	default: // raglex.auto-aspect-tagger
		return `#!/usr/bin/env python3
import json, sys
req = json.load(sys.stdin)
add = {}
for a in req.get("assets", []):
    w, h = a.get("width", 0), a.get("height", 0)
    tags = []
    if w > 0 and h > 0:
        r = w / h
        if r >= 1.9: tags.append("Ultrawide")
        elif r > 1.1: tags.append("Landscape")
        elif r < 0.9: tags.append("Portrait")
        else: tags.append("Square")
        if w >= 3840 or h >= 2160: tags.append("4K+")
    if tags:
        add[a["id"]] = tags
print(json.dumps({"ok": True, "reload": True, "add_tags": add, "toast": f"Auto-tagged {len(add)} assets by aspect ratio"}))
`
	}
}

func commonZipRootDir(files []*zip.File) string {
	var top string
	for _, f := range files {
		p := strings.TrimPrefix(filepath.ToSlash(f.Name), "/")
		if p == "" {
			continue
		}
		idx := strings.IndexByte(p, '/')
		if idx <= 0 {
			return "" // file at root -> no common wrapper folder
		}
		dir := p[:idx+1]
		if top == "" {
			top = dir
		} else if top != dir {
			return ""
		}
	}
	return top
}

func isWithinDir(base, target string) bool {
	absBase, err1 := filepath.Abs(base)
	absTarget, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
