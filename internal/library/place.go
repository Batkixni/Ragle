package library

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"raglex/internal/model"
)

func placeFile(filesDir, src, mode string) (string, error) {
	switch mode {
	case model.StorageIndex:
		return src, nil
	case model.StorageCopy:
		dest := uniqueDest(filesDir, filepath.Base(src))
		if err := copyFile(src, dest); err != nil {
			return "", err
		}
		return dest, nil
	case model.StorageMove:
		dest := uniqueDest(filesDir, filepath.Base(src))
		if err := os.Rename(src, dest); err != nil {
			if copyErr := copyFile(src, dest); copyErr != nil {
				return "", copyErr
			}
			if rmErr := os.Remove(src); rmErr != nil {
				_ = os.Remove(dest)
				return "", rmErr
			}
		}
		return dest, nil
	case model.StorageSymlink:
		dest := uniqueDest(filesDir, filepath.Base(src))
		if err := os.Symlink(src, dest); err != nil {
			return "", fmt.Errorf("%w: %v", ErrSymlink, err)
		}
		return dest, nil
	default:
		return "", fmt.Errorf("raglex: unknown storage mode %q", mode)
	}
}

func undoPlace(mode, src, stored string) {
	switch mode {
	case model.StorageCopy, model.StorageSymlink:
		_ = os.Remove(stored)
	case model.StorageMove:
		if src != stored {
			if err := os.Rename(stored, src); err != nil {
				_ = copyFile(stored, src)
				_ = os.Remove(stored)
			}
		}
	}
}

func uniqueDest(dir, name string) string {
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "file"
	}
	candidate := filepath.Join(dir, name)
	if _, err := os.Lstat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(name)
	stem := stringsTrimExt(name, ext)
	for i := 2; i < 10000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, os.Getpid(), ext))
}

func stringsTrimExt(name, ext string) string {
	if ext == "" {
		return name
	}
	return name[:len(name)-len(ext)]
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}
