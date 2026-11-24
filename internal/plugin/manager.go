// Package plugin discovers, validates, and executes Raglex external plugins.
// Each plugin lives in <library>/plugins/<id>/manifest.json and communicates
// via structured JSON over stdin/stdout (language-agnostic: Go, Python, Node,
// Rust, or shell scripts).
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"raglex/internal/model"
)

// Manifest describes an installed Raglex plugin.
type Manifest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Author      string   `json:"author,omitempty"`
	Icon        string   `json:"icon,omitempty"`        // semantic icon key (import, palette, markdown, duplicate, crop, layers) or relative PNG file
	Accent      string   `json:"accent,omitempty"`      // optional #RRGGBB icon badge accent
	Command     []string `json:"command"`               // executable + args (resolved relative to plugin dir if relative)
	Hooks       []string `json:"hooks,omitempty"`       // context_menu, batch_action, preview_renderer, inspector_card, exporter, importer, library_action
	Extensions  []string `json:"extensions,omitempty"`  // optional file extensions handled by preview_renderer (e.g. [".md", ".markdown"])
	Permissions []string `json:"permissions,omitempty"` // read_library, write_tags, disk, network
}

// Installed wraps a discovered plugin manifest with its directory & state.
type Installed struct {
	Manifest Manifest
	Dir      string
	Enabled  bool
}

// AssetPayload is the JSON-friendly asset snapshot passed to a plugin on stdin.
type AssetPayload struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Path      string               `json:"path"`
	Mime      string               `json:"mime"`
	Width     int                  `json:"width"`
	Height    int                  `json:"height"`
	Rating    int                  `json:"rating"`
	Notes     string               `json:"notes,omitempty"`
	SourceURL string               `json:"source_url,omitempty"`
	Palette   []model.PaletteColor `json:"palette,omitempty"`
}

// Request is written as JSON to the plugin's stdin.
type Request struct {
	Hook        string         `json:"hook"`
	LibraryPath string         `json:"library_path"`
	ServerURL   string         `json:"server_url,omitempty"`
	Token       string         `json:"token,omitempty"`
	Assets      []AssetPayload `json:"assets"`
}

// Response is read as JSON from the plugin's stdout.
type Response struct {
	OK           bool                `json:"ok"`
	Toast        string              `json:"toast,omitempty"`
	Error        string              `json:"error,omitempty"`
	Reload       bool                `json:"reload,omitempty"`
	AddTags      map[string][]string `json:"add_tags,omitempty"`      // assetID -> []tagName
	PreviewTitle string              `json:"preview_title,omitempty"` // returned by preview_renderer hook
	PreviewText  string              `json:"preview_text,omitempty"`  // returned by preview_renderer hook
	PreviewKV    map[string]string   `json:"preview_kv,omitempty"`    // optional metadata key/values
}

// Manager tracks installed plugins for a library.
type Manager struct {
	dir string

	mu      sync.RWMutex
	plugins map[string]*Installed
	order   []string
}

// NewManager creates a Manager rooted at <libraryRoot>/plugins.
func NewManager(libraryRoot string) *Manager {
	dir := filepath.Join(libraryRoot, "plugins")
	_ = os.MkdirAll(dir, 0o755)
	m := &Manager{
		dir:     dir,
		plugins: map[string]*Installed{},
	}
	_ = m.Reload()
	return m
}

// Dir returns the library's plugins folder.
func (m *Manager) Dir() string { return m.dir }

// Reload scans the plugins directory for valid manifest.json files.
func (m *Manager) Reload() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	prevEnabled := map[string]bool{}
	for id, p := range m.plugins {
		prevEnabled[id] = p.Enabled
	}

	next := map[string]*Installed{}
	var order []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pdir := filepath.Join(m.dir, entry.Name())
		raw, err := os.ReadFile(filepath.Join(pdir, "manifest.json"))
		if err != nil {
			continue
		}
		var mf Manifest
		if err := json.Unmarshal(raw, &mf); err != nil {
			continue
		}
		mf.ID = strings.TrimSpace(mf.ID)
		mf.Name = strings.TrimSpace(mf.Name)
		if mf.ID == "" {
			mf.ID = entry.Name()
		}
		if mf.Name == "" {
			mf.Name = mf.ID
		}
		if len(mf.Command) == 0 {
			continue
		}
		enabled := true
		if prev, ok := prevEnabled[mf.ID]; ok {
			enabled = prev
		}
		next[mf.ID] = &Installed{
			Manifest: mf,
			Dir:      pdir,
			Enabled:  enabled,
		}
		order = append(order, mf.ID)
	}
	sort.Strings(order)
	m.plugins = next
	m.order = order
	return nil
}

// List returns all discovered plugins in deterministic order.
func (m *Manager) List() []Installed {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Installed, 0, len(m.order))
	for _, id := range m.order {
		if p, ok := m.plugins[id]; ok {
			out = append(out, *p)
		}
	}
	return out
}

// ForHook returns enabled plugins declaring hook (or all enabled if hook == "").
func (m *Manager) ForHook(hook string) []Installed {
	all := m.List()
	var out []Installed
	for _, p := range all {
		if !p.Enabled {
			continue
		}
		if hook == "" || len(p.Manifest.Hooks) == 0 {
			out = append(out, p)
			continue
		}
		for _, h := range p.Manifest.Hooks {
			if strings.EqualFold(h, hook) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// PreviewProviderForExt returns the first enabled plugin declaring hook
// "preview_renderer" that handles file extension ext (e.g. ".md").
func (m *Manager) PreviewProviderForExt(ext string) (Installed, bool) {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if m == nil || ext == "" {
		return Installed{}, false
	}
	for _, p := range m.ForHook("preview_renderer") {
		for _, cand := range p.Manifest.Extensions {
			if strings.EqualFold(strings.TrimSpace(cand), ext) {
				return p, true
			}
		}
	}
	return Installed{}, false
}

// SetEnabled toggles whether a plugin is enabled.
func (m *Manager) SetEnabled(id string, on bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.plugins[id]; ok {
		p.Enabled = on
	}
}

// Execute runs the plugin command with req serialized as JSON on stdin and
// decodes its JSON response from stdout.
func (m *Manager) Execute(ctx context.Context, pluginID string, req Request) (Response, error) {
	if m == nil {
		return Response{}, errors.New("plugin manager unavailable")
	}
	m.mu.RLock()
	p, ok := m.plugins[pluginID]
	var inst Installed
	if ok {
		inst = *p
	}
	m.mu.RUnlock()
	if !ok {
		return Response{}, fmt.Errorf("plugin %q not found", pluginID)
	}
	if !inst.Enabled {
		return Response{}, fmt.Errorf("plugin %q is disabled", pluginID)
	}
	if len(inst.Manifest.Command) == 0 {
		return Response{}, fmt.Errorf("plugin %q has empty command", pluginID)
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	exe := inst.Manifest.Command[0]
	if strings.HasPrefix(exe, ".") || strings.ContainsAny(exe, `/\`) {
		if !filepath.IsAbs(exe) {
			exe = filepath.Join(inst.Dir, exe)
		}
	}
	args := make([]string, len(inst.Manifest.Command)-1)
	for i, a := range inst.Manifest.Command[1:] {
		if strings.HasPrefix(a, "./") || strings.HasPrefix(a, `.\\`) {
			args[i] = filepath.Join(inst.Dir, a)
		} else {
			args[i] = a
		}
	}

	rawIn, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	hideCmd(cmd)
	cmd.Dir = inst.Dir
	cmd.Stdin = bytes.NewReader(rawIn)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Response{}, fmt.Errorf("plugin %s failed: %s", inst.Manifest.Name, msg)
	}

	var resp Response
	outBytes := bytes.TrimSpace(stdout.Bytes())
	if len(outBytes) == 0 {
		return Response{OK: true}, nil
	}
	if err := json.Unmarshal(outBytes, &resp); err != nil {
		// Allow plain text stdout as a quick toast message for simple scripts.
		return Response{OK: true, Toast: string(outBytes)}, nil
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
