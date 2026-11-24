package library

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type collectedFile struct {
	Path       string
	RelFolders []string
}

func collectFiles(root string, inputs []string) ([]string, error) {
	items, err := collectFilesWithFolders(root, inputs)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Path
	}
	return out, nil
}

func collectFilesWithFolders(root string, inputs []string) ([]collectedFile, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	seen := map[string]bool{}
	var out []collectedFile
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
			baseFolder := filepath.Base(abs)
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
					var relSegs []string
					if baseFolder != "" && baseFolder != "." && baseFolder != string(os.PathSeparator) {
						relSegs = append(relSegs, baseFolder)
					}
					if rel, rerr := filepath.Rel(abs, filepath.Dir(path)); rerr == nil && rel != "." && rel != "" {
						for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
							if seg != "" && seg != "." && seg != ".." {
								relSegs = append(relSegs, seg)
							}
						}
					}
					out = append(out, collectedFile{Path: path, RelFolders: relSegs})
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
			out = append(out, collectedFile{Path: abs})
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
	if base == "raglex.db" || base == "raglex.db.bak" || strings.HasPrefix(base, "raglex.db.") {
		return true
	}
	return IsAuxiliarySidecarFile(base)
}

// IsAuxiliarySidecarFile reports whether base is an external or OS auxiliary sidecar
// file (such as metadata.json, mtime.json, tags.json, or *_thumbnail.png) that
// should not be imported as a user creative asset.
func IsAuxiliarySidecarFile(base string) bool {
	low := strings.ToLower(strings.TrimSpace(base))
	switch low {
	case "metadata.json", "mtime.json", "tags.json", "saved-filters.json", "actions.json", "thumbs.db", "desktop.ini", ".ds_store":
		return true
	}
	if strings.HasSuffix(low, "_thumbnail.png") || strings.HasSuffix(low, "_thumbnail.jpg") || strings.HasSuffix(low, "_thumbnail.webp") {
		return true
	}
	return false
}
