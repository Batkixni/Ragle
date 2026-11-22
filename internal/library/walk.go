package library

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func collectFiles(root string, inputs []string) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	seen := map[string]bool{}
	var out []string
	for _, input := range inputs {
		abs, err := cleanPath(input)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if path != abs && (skipLibraryDir(path, root) || strings.HasPrefix(d.Name(), ".")) {
						return fs.SkipDir
					}
					return nil
				}
				if strings.HasPrefix(d.Name(), ".") || skipLibraryFile(path, root) {
					return nil
				}
				if !d.Type().IsRegular() && d.Type()&os.ModeSymlink == 0 {
					return nil
				}
				if !seen[path] {
					seen[path] = true
					out = append(out, path)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		if skipLibraryFile(abs, root) {
			continue
		}
		if !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	return out, nil
}

func skipLibraryDir(path, root string) bool {
	for _, name := range []string{"files", "thumbs"} {
		dir := filepath.Join(root, name)
		if path == dir || strings.HasPrefix(path, dir+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func skipLibraryFile(path, root string) bool {
	if skipLibraryDir(path, root) {
		return true
	}
	base := filepath.Base(path)
	return base == "ragle.db" || base == "ragle.db.bak" || strings.HasPrefix(base, "ragle.db.")
}
