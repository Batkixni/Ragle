package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"raglex/internal/model"
)

// MigrateLibraryResult summarizes an external .library package migration.
type MigrateLibraryResult struct {
	AssetsImported int
	AssetsExisting int
	FoldersMapped  int
	Skipped        int
}

type extLibRootMeta struct {
	Folders []extLibFolder `json:"folders"`
}

type extLibFolder struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	IconColor string         `json:"iconColor"`
	Children  []extLibFolder `json:"children"`
}

type extLibItemMeta struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Ext        string   `json:"ext"`
	Tags       []string `json:"tags"`
	Folders    []string `json:"folders"`
	IsDeleted  bool     `json:"isDeleted"`
	URL        string   `json:"url"`
	Annotation string   `json:"annotation"`
	Star       int      `json:"star"`
}

var extColorHex = map[string]string{
	"red":    "#EF4444",
	"orange": "#F97316",
	"yellow": "#EAB308",
	"green":  "#22C55E",
	"aqua":   "#06B6D4",
	"blue":   "#3B82F6",
	"purple": "#A855F7",
	"pink":   "#EC4899",
}

// ImportExternalLibrary migrates a standard .library package directory
// (root metadata.json + images/<id>.info/metadata.json + media files) into this
// Raglex library, preserving nested folder hierarchies as nested Raglex tags,
// item tags, star ratings, notes/annotations, and source URLs.
func (l *Library) ImportExternalLibrary(ctx context.Context, libDir string, storageMode string) (MigrateLibraryResult, error) {
	var out MigrateLibraryResult
	if l == nil {
		return out, errors.New("library is nil")
	}
	libDir = filepath.Clean(strings.TrimSpace(libDir))
	imagesDir := filepath.Join(libDir, "images")
	st, err := os.Stat(imagesDir)
	if err != nil || !st.IsDir() {
		return out, errors.New("selected folder does not contain an images/ directory")
	}
	if storageMode == "" {
		storageMode = model.StorageIndex
	}

	// 1. Parse root metadata.json folder hierarchy -> nested Raglex Tags.
	folderToTagID := map[string]string{}
	if rawRoot, err := os.ReadFile(filepath.Join(libDir, "metadata.json")); err == nil {
		var rootMeta extLibRootMeta
		if json.Unmarshal(rawRoot, &rootMeta) == nil {
			var walkFolders func(list []extLibFolder, parentTagID string)
			walkFolders = func(list []extLibFolder, parentTagID string) {
				for _, f := range list {
					name := strings.TrimSpace(f.Name)
					if name == "" {
						continue
					}
					tagID := l.ensureTagWithParent(name, parentTagID, f.IconColor)
					if tagID != "" {
						if f.ID != "" {
							folderToTagID[f.ID] = tagID
						}
						out.FoldersMapped++
					}
					if len(f.Children) > 0 {
						walkFolders(f.Children, tagID)
					}
				}
			}
			walkFolders(rootMeta.Folders, "")
		}
	}

	// 2. Walk images/<id>.info/ directories.
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return out, err
	}
	for _, ent := range entries {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if !ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ".info") {
			continue
		}
		infoDir := filepath.Join(imagesDir, ent.Name())
		rawMeta, err := os.ReadFile(filepath.Join(infoDir, "metadata.json"))
		if err != nil {
			out.Skipped++
			continue
		}
		var meta extLibItemMeta
		if err := json.Unmarshal(rawMeta, &meta); err != nil || meta.IsDeleted {
			out.Skipped++
			continue
		}
		mediaPath := findExtLibMediaFile(infoDir, meta)
		if mediaPath == "" {
			out.Skipped++
			continue
		}

		// Resolve all tag IDs (both explicit item tags and mapped folder tags).
		tagSeen := map[string]bool{}
		var tagIDs []string
		addTag := func(id string) {
			if id != "" && !tagSeen[id] {
				tagSeen[id] = true
				tagIDs = append(tagIDs, id)
			}
		}
		for _, fid := range meta.Folders {
			addTag(folderToTagID[fid])
		}
		for _, tname := range meta.Tags {
			addTag(l.EnsureTagID(tname))
		}

		res, err := l.Import(ctx, []string{mediaPath}, storageMode, tagIDs, nil)
		if err != nil || len(res.Items) == 0 {
			out.Skipped++
			continue
		}
		if res.Added > 0 {
			out.AssetsImported += res.Added
		} else if res.Existing > 0 {
			out.AssetsExisting += res.Existing
		}
		assetID := res.Items[0].AssetID
		if assetID == "" {
			continue
		}
		for _, tid := range tagIDs {
			_ = l.SetAssetTag(assetID, tid, true)
		}
		if meta.Star >= 1 && meta.Star <= 5 {
			_ = l.SetRating(assetID, meta.Star)
		}
		if meta.Annotation != "" || meta.URL != "" || meta.Name != "" {
			_, _ = l.UpdateAssetMeta(assetID, strings.TrimSpace(meta.Annotation), strings.TrimSpace(meta.URL), strings.TrimSpace(meta.Name))
		}
	}
	return out, nil
}

func (l *Library) ensureTagWithParent(name, parentID, colorName string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if tags, err := l.Tags(); err == nil {
		for _, tg := range tags {
			if strings.EqualFold(strings.TrimSpace(tg.Name), name) && tg.ParentID == parentID {
				return tg.ID
			}
		}
	}
	tg, err := l.CreateTagWithParent(name, parentID)
	if err != nil {
		return l.EnsureTagID(name)
	}
	if hex := resolveExtColor(colorName); hex != "" {
		_ = l.SetTagColor(tg.ID, hex)
	}
	return tg.ID
}

func resolveExtColor(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		return s
	}
	return extColorHex[strings.ToLower(s)]
}

func findExtLibMediaFile(infoDir string, meta extLibItemMeta) string {
	if meta.Name != "" && meta.Ext != "" {
		cand := filepath.Join(infoDir, meta.Name+"."+strings.TrimPrefix(meta.Ext, "."))
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	files, err := os.ReadDir(infoDir)
	if err != nil {
		return ""
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		n := f.Name()
		low := strings.ToLower(n)
		if low == "metadata.json" || low == "mtime.json" || strings.Contains(low, "_thumbnail.") || strings.HasPrefix(low, ".") {
			continue
		}
		return filepath.Join(infoDir, n)
	}
	return ""
}
