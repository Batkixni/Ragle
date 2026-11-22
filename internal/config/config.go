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
	LibraryPath string `json:"library_path"`
	Language    string `json:"language"`
}

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
