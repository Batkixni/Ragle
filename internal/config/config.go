// Package config stores app preferences outside the library folder.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is the app preference file. It is not part of a library.
type Config struct {
	LibraryPath  string  `json:"library_path"`
	Language     string  `json:"language"`
	SignalingURL string  `json:"signaling_url,omitempty"`
	ThemeMode    string  `json:"theme_mode,omitempty"` // "auto", "light", "dark"
	CardSize     float64 `json:"card_size,omitempty"`

	// Custom theme colors (optional overrides applied on top of light/dark mode).
	ThemePreset   string `json:"theme_preset,omitempty"`   // "", "graphite", "warm", "midnight", "custom"
	CustomCanvas  string `json:"custom_canvas,omitempty"`  // #RRGGBB
	CustomSurface string `json:"custom_surface,omitempty"` // #RRGGBB
	CustomAccent  string `json:"custom_accent,omitempty"`  // #RRGGBB
	CustomText    string `json:"custom_text,omitempty"`    // #RRGGBB

	// Onboarding state (shown automatically on first launch).
	OnboardingDone bool `json:"onboarding_done,omitempty"`

	// Workspace & appearance preferences.
	ShowSidebarCounts *bool  `json:"show_sidebar_counts,omitempty"`
	ShowTagColors     *bool  `json:"show_tag_colors,omitempty"`
	ShowCardCaption   *bool  `json:"show_card_caption,omitempty"`
	DefaultSort       int    `json:"default_sort,omitempty"`
	LayoutMode        string `json:"layout_mode,omitempty"` // "bento", "masonry", "grid"

	// Import preferences.
	DefaultStorageMode   string `json:"default_storage_mode,omitempty"` // "index", "symlink", "move", "copy"
	AutoSelectImported   *bool  `json:"auto_select_imported,omitempty"`
	AutoTagOnImport      *bool  `json:"auto_tag_on_import,omitempty"`
	IgnoreHiddenOnImport *bool  `json:"ignore_hidden_on_import,omitempty"`
	ImportRecursive      *bool  `json:"import_recursive,omitempty"`
	NotifyDuplicates     *bool  `json:"notify_duplicates,omitempty"`

	// Database & safety preferences.
	BackupBeforeDelete *bool `json:"backup_before_delete,omitempty"`

	// Sharing preferences.
	DeviceName        string `json:"device_name,omitempty"`
	ShareIncludeFiles *bool  `json:"share_include_files,omitempty"`

	// Stage 14: Built-in Visual Embedding & Laya / Ollaya AI decision classifier preferences.
	EmbedEnabled      *bool   `json:"embed_enabled,omitempty"`
	LayaEnabled       *bool   `json:"laya_enabled,omitempty"`
	LayaURL           string  `json:"laya_url,omitempty"`
	LayaModel         string  `json:"laya_model,omitempty"`
	LayaAutoThreshold float64 `json:"laya_auto_threshold,omitempty"`

	// Stage 15: Built-in local/LAN server preferences.
	ServerPort      int    `json:"server_port,omitempty"`
	ServerBindAll   bool   `json:"server_bind_all,omitempty"`
	ServerAutoStart bool   `json:"server_auto_start,omitempty"`
	ServerToken     string `json:"server_token,omitempty"`

	// Auto-updater preferences (GitHub Releases).
	AutoCheckUpdates  *bool `json:"auto_check_updates,omitempty"`
	UpdateBetaChannel *bool `json:"update_beta_channel,omitempty"`
}

// BoolOr returns *p when non-nil, or def otherwise.
func BoolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// BoolPtr returns a pointer to v for optional JSON fields.
func BoolPtr(v bool) *bool { return &v }

// Path is os.UserConfigDir()/Ragle/config.json.
func Path() (string, error) {
	if pathOverride != "" {
		return pathOverride, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Ragle", "config.json"), nil
}

// pathOverride is set by tests.
var pathOverride string

// Load reads the preference file. A missing file is an empty config.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes the preference file, creating the folder when needed.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}
