package ui

import "path/filepath"

func openLocalFile(path string) error {
	if path == "" {
		return nil
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		abs = filepath.Clean(path)
	}
	return openLocalFileOS(abs)
}

func revealLocalFile(path string) error {
	if path == "" {
		return nil
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		abs = filepath.Clean(path)
	}
	return revealLocalFileOS(abs)
}
