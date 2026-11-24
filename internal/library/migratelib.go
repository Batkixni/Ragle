package library

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

// MigrateLibraryResult summarizes an external .library or .eaglepack migration.
type MigrateLibraryResult struct {
	AssetsImported int
	AssetsExisting int
	FoldersMapped  int
	TagsMapped     int
	Skipped        int
}

type extLibRootMeta struct {
	Folder  *extLibFolder    `json:"folder"`
	Folders []extLibFolder   `json:"folders"`
	Images  []extLibItemMeta `json:"images"`
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

// ImportExternalPackage automatically imports either a .library directory or a
// .eaglepack / .zip archive package, preserving nested folders in Raglex's
// Folder hierarchy and item tags in Raglex's Tag system.
func (l *Library) ImportExternalPackage(ctx context.Context, srcPath string, storageMode string) (MigrateLibraryResult, error) {
	return l.ImportExternalPackageWithProgress(ctx, srcPath, storageMode, nil)
}

// ImportExternalPackageWithProgress imports a .library folder or .eaglepack archive
// with live progress callbacks for both archive extraction and item import.
func (l *Library) ImportExternalPackageWithProgress(ctx context.Context, srcPath string, storageMode string, progress func(ImportProgress)) (MigrateLibraryResult, error) {
	srcPath = filepath.Clean(strings.TrimSpace(srcPath))
	fi, err := os.Stat(srcPath)
	if err != nil {
		return MigrateLibraryResult{}, err
	}
	if !fi.IsDir() {
		low := strings.ToLower(srcPath)
		if strings.HasSuffix(low, ".eaglepack") || strings.HasSuffix(low, ".zip") {
			return l.ImportExternalPackWithProgress(ctx, srcPath, progress)
		}
		return MigrateLibraryResult{}, errors.New("unsupported package file: expected .library folder or .eaglepack archive")
	}
	return l.ImportExternalLibraryWithProgress(ctx, srcPath, storageMode, progress)
}

// ImportExternalPack extracts and imports a .eaglepack ZIP archive into the
// library (copying media files into library files/ so extracted assets stay
// intact permanently), recreating the nested Folder hierarchy and Tags.
func (l *Library) ImportExternalPack(ctx context.Context, packPath string) (MigrateLibraryResult, error) {
	return l.ImportExternalPackWithProgress(ctx, packPath, nil)
}

// ImportExternalPackWithProgress extracts and imports a .eaglepack ZIP archive with progress updates.
func (l *Library) ImportExternalPackWithProgress(ctx context.Context, packPath string, progress func(ImportProgress)) (MigrateLibraryResult, error) {
	var out MigrateLibraryResult
	if l == nil {
		return out, errors.New("library is nil")
	}
	zr, err := zip.OpenReader(packPath)
	if err != nil {
		return out, fmt.Errorf("open pack archive: %w", err)
	}
	defer zr.Close()

	tmpDir, err := os.MkdirTemp("", "raglex-pack-*")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(tmpDir)

	// Pre-count actual .info item directories inside the archive so progress shows
	// the true total asset count from the very beginning instead of raw zip entries.
	infoSet := map[string]bool{}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if idx := strings.Index(strings.ToLower(name), ".info/"); idx > 0 {
			infoSet[name[:idx+5]] = true
		}
	}
	estItems := len(infoSet)
	if progress != nil && estItems > 0 {
		progress(ImportProgress{
			Done:    0,
			Total:   estItems,
			Current: filepath.Base(packPath),
		})
	}

	cleanTmp := filepath.Clean(tmpDir) + string(os.PathSeparator)
	for _, f := range zr.File {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if name == "" || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		dst := filepath.Join(tmpDir, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(dst), cleanTmp) && filepath.Clean(dst) != filepath.Clean(tmpDir) {
			continue // zip-slip protection
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return out, err
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		w, err := os.Create(dst)
		if err != nil {
			_ = rc.Close()
			continue
		}
		_, _ = io.Copy(w, rc)
		_ = w.Close()
		_ = rc.Close()
	}

	// Locate effective package root (some packs wrap content inside a single top-level dir).
	packRoot := locatePackRoot(tmpDir)
	// Always store extracted pack files inside the library via copy mode so they remain valid.
	return l.importExternalDir(ctx, packRoot, model.StorageCopy, progress)
}

func locatePackRoot(dir string) string {
	for _, marker := range []string{"pack.json", "metadata.json", "images"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return dir
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !strings.HasPrefix(e.Name(), "__") {
			sub := filepath.Join(dir, e.Name())
			for _, marker := range []string{"pack.json", "metadata.json", "images"} {
				if _, err := os.Stat(filepath.Join(sub, marker)); err == nil {
					return sub
				}
			}
		}
	}
	return dir
}

// ImportExternalLibrary migrates a standard .library package directory
// (root metadata.json + images/<id>.info/metadata.json + media files) into this
// Raglex library, preserving nested folder hierarchies as Raglex Folders,
// item tags as Raglex Tags, star ratings, notes/annotations, and source URLs.
func (l *Library) ImportExternalLibrary(ctx context.Context, libDir string, storageMode string) (MigrateLibraryResult, error) {
	return l.importExternalDir(ctx, libDir, storageMode, nil)
}

// ImportExternalLibraryWithProgress migrates a .library directory with progress updates.
func (l *Library) ImportExternalLibraryWithProgress(ctx context.Context, libDir string, storageMode string, progress func(ImportProgress)) (MigrateLibraryResult, error) {
	return l.importExternalDir(ctx, libDir, storageMode, progress)
}

func (l *Library) importExternalDir(ctx context.Context, libDir string, storageMode string, progress func(ImportProgress)) (MigrateLibraryResult, error) {
	var out MigrateLibraryResult
	if l == nil {
		return out, errors.New("library is nil")
	}
	libDir = filepath.Clean(strings.TrimSpace(libDir))
	if storageMode == "" {
		storageMode = model.StorageIndex
	}

	// 1. Parse root metadata.json or pack.json folder hierarchy -> nested Raglex Folders.
	// Note: .library uses plural "folders": [...], while .eaglepack uses singular "folder": {...} (or "folders").
	folderToID := map[string]string{}
	inlineMetaByID := map[string]extLibItemMeta{}
	for _, metaFile := range []string{"pack.json", "metadata.json"} {
		rawRoot, err := os.ReadFile(filepath.Join(libDir, metaFile))
		if err != nil {
			continue
		}
		var rootMeta extLibRootMeta
		if json.Unmarshal(rawRoot, &rootMeta) == nil {
			var walkFolders func(list []extLibFolder, parentFolderID string)
			walkFolders = func(list []extLibFolder, parentFolderID string) {
				for _, f := range list {
					name := strings.TrimSpace(f.Name)
					if name == "" {
						continue
					}
					fid := l.EnsureFolderWithParent(name, parentFolderID, resolveExtColor(f.IconColor))
					if fid != "" {
						if f.ID != "" {
							folderToID[f.ID] = fid
						}
						out.FoldersMapped++
					}
					if len(f.Children) > 0 {
						walkFolders(f.Children, fid)
					}
				}
			}
			if rootMeta.Folder != nil {
				walkFolders([]extLibFolder{*rootMeta.Folder}, "")
			}
			if len(rootMeta.Folders) > 0 {
				walkFolders(rootMeta.Folders, "")
			}
			for _, img := range rootMeta.Images {
				if img.ID != "" {
					inlineMetaByID[img.ID] = img
				}
			}
		}
	}

	// 2. Discover .info item directories under images/ (or directly under pack root).
	var infoDirs []string
	imagesDir := filepath.Join(libDir, "images")
	if st, err := os.Stat(imagesDir); err == nil && st.IsDir() {
		if entries, err := os.ReadDir(imagesDir); err == nil {
			for _, ent := range entries {
				if ent.IsDir() && strings.HasSuffix(strings.ToLower(ent.Name()), ".info") {
					infoDirs = append(infoDirs, filepath.Join(imagesDir, ent.Name()))
				}
			}
		}
	}
	if len(infoDirs) == 0 {
		if entries, err := os.ReadDir(libDir); err == nil {
			for _, ent := range entries {
				if ent.IsDir() && strings.HasSuffix(strings.ToLower(ent.Name()), ".info") {
					infoDirs = append(infoDirs, filepath.Join(libDir, ent.Name()))
				}
			}
		}
	}
	if len(infoDirs) == 0 {
		return out, errors.New("selected package does not contain an images/ or *.info directory")
	}

	totalItems := len(infoDirs)
	if progress != nil {
		progress(ImportProgress{Done: 0, Total: totalItems})
	}

	mappedTagsGlobal := map[string]bool{}
	tagNameCache := map[string]string{}
	for idx, infoDir := range infoDirs {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		var meta extLibItemMeta
		hasMeta := false
		if rawMeta, err := os.ReadFile(filepath.Join(infoDir, "metadata.json")); err == nil {
			if json.Unmarshal(rawMeta, &meta) == nil {
				hasMeta = true
			}
		}
		if !hasMeta {
			baseID := strings.TrimSuffix(filepath.Base(infoDir), ".info")
			if m, ok := inlineMetaByID[baseID]; ok {
				meta = m
				hasMeta = true
			}
		}
		if !hasMeta || meta.IsDeleted {
			out.Skipped++
			if progress != nil {
				progress(ImportProgress{Done: idx + 1, Total: totalItems, Added: out.AssetsImported, Existing: out.AssetsExisting, Failed: out.Skipped})
			}
			continue
		}

		// Detect Web Bookmark items (ext == "url" / "webloc" / "website"):
		// Use the bundled *_thumbnail.png/.jpg cover art as the visual asset instead of
		// importing the raw 72-byte .url shortcut file as a blank URL icon!
		isBookmark := isExtBookmarkMeta(meta)
		mediaPath := ""
		if isBookmark {
			mediaPath = findExtLibThumbnailFile(infoDir)
			if mediaPath == "" {
				// Fallback to any image in infoDir, or a clean placeholder PNG.
				cand := findExtLibMediaFile(infoDir, meta)
				if cand != "" && !isURLShortcutFile(cand) {
					mediaPath = cand
				} else {
					if meta.URL == "" && cand != "" {
						meta.URL = parseURLShortcutFile(cand)
					}
					ph := filepath.Join(infoDir, "_bookmark_cover.png")
					if err := os.WriteFile(ph, makeWebPlaceholderPNG(), 0o644); err == nil {
						mediaPath = ph
					}
				}
			} else if meta.URL == "" {
				if urlFile := findExtLibMediaFile(infoDir, meta); isURLShortcutFile(urlFile) {
					meta.URL = parseURLShortcutFile(urlFile)
				}
			}
		} else {
			mediaPath = findExtLibMediaFile(infoDir, meta)
			if isURLShortcutFile(mediaPath) {
				isBookmark = true
				if meta.URL == "" {
					meta.URL = parseURLShortcutFile(mediaPath)
				}
				if thumb := findExtLibThumbnailFile(infoDir); thumb != "" {
					mediaPath = thumb
				}
			}
		}
		if mediaPath == "" {
			out.Skipped++
			if progress != nil {
				progress(ImportProgress{Done: idx + 1, Total: totalItems, Added: out.AssetsImported, Existing: out.AssetsExisting, Failed: out.Skipped})
			}
			continue
		}

		// Resolve folder IDs -> Raglex Folders, and item tags -> Raglex Tags.
		folderSeen := map[string]bool{}
		var folderIDs []string
		for _, fid := range meta.Folders {
			if mapped := folderToID[fid]; mapped != "" && !folderSeen[mapped] {
				folderSeen[mapped] = true
				folderIDs = append(folderIDs, mapped)
			}
		}

		tagSeen := map[string]bool{}
		var tagIDs []string
		for _, tname := range meta.Tags {
			key := strings.ToLower(strings.TrimSpace(tname))
			if key == "" {
				continue
			}
			tid, ok := tagNameCache[key]
			if !ok {
				tid = l.EnsureTagID(tname)
				tagNameCache[key] = tid
			}
			if tid != "" && !tagSeen[tid] {
				tagSeen[tid] = true
				tagIDs = append(tagIDs, tid)
				if !mappedTagsGlobal[tid] {
					mappedTagsGlobal[tid] = true
					out.TagsMapped++
				}
			}
		}

		importMode := storageMode
		if isBookmark {
			// Bookmark cover images extracted from packages must always be copied permanently
			// AND renamed cleanly (without "_thumbnail." in the filename, which walk.go skips as sidecar).
			importMode = model.StorageCopy
			if strings.Contains(strings.ToLower(filepath.Base(mediaPath)), "_thumbnail.") {
				ext := filepath.Ext(mediaPath)
				if ext == "" {
					ext = ".png"
				}
				cleanStem := sanitizeBookmarkFilename(meta.Name)
				if cleanStem == "" {
					cleanStem = meta.ID
				}
				if cleanStem == "" {
					cleanStem = "bookmark"
				}
				cleanPath := filepath.Join(infoDir, cleanStem+ext)
				if rawImg, rerr := os.ReadFile(mediaPath); rerr == nil && len(rawImg) > 0 {
					if werr := os.WriteFile(cleanPath, rawImg, 0o644); werr == nil {
						mediaPath = cleanPath
					}
				}
			}
		}
		res, err := l.ImportWithFolders(ctx, []string{mediaPath}, importMode, tagIDs, folderIDs, false, nil)
		if err != nil || len(res.Items) == 0 {
			out.Skipped++
			if progress != nil {
				progress(ImportProgress{Done: idx + 1, Total: totalItems, Added: out.AssetsImported, Existing: out.AssetsExisting, Failed: out.Skipped, Current: meta.Name})
			}
			continue
		}
		if res.Added > 0 {
			out.AssetsImported += res.Added
		} else if res.Existing > 0 {
			out.AssetsExisting += res.Existing
		}
		assetID := res.Items[0].AssetID
		if assetID != "" {
			// Apply tags, folders, bookmark flag, display name, rating, notes & source URL in ONE bbolt tx.
			_ = l.DB.Update(func(tx *bbolt.Tx) error {
				a, err := db.GetAsset(tx, assetID)
				if err != nil {
					return err
				}
				changed := false
				if isBookmark {
					a.IsBookmark = true
					if strings.TrimSpace(meta.Name) != "" {
						a.DisplayName = strings.TrimSpace(meta.Name)
					}
					changed = true
				}
				if meta.Star >= 1 && meta.Star <= 5 && a.Rating != meta.Star {
					a.Rating = meta.Star
					changed = true
				}
				if note := strings.TrimSpace(meta.Annotation); note != "" && a.Notes != note {
					a.Notes = note
					changed = true
				}
				if u := strings.TrimSpace(meta.URL); u != "" && a.SourceURL != u {
					a.SourceURL = u
					changed = true
				}
				if title := strings.TrimSpace(meta.Name); title != "" && a.SourceTitle != title {
					a.SourceTitle = title
					changed = true
				}
				if changed {
					if err := db.PutAsset(tx, a); err != nil {
						return err
					}
				}
				for _, tid := range tagIDs {
					_ = db.LinkAssetTag(tx, assetID, tid)
				}
				for _, fid := range folderIDs {
					_ = db.LinkAssetFolder(tx, assetID, fid)
				}
				return nil
			})
		}

		if progress != nil {
			progress(ImportProgress{
				Done:     idx + 1,
				Total:    totalItems,
				Added:    out.AssetsImported,
				Existing: out.AssetsExisting,
				Failed:   out.Skipped,
				Current:  meta.Name,
			})
		}
	}
	return out, nil
}

func isExtBookmarkMeta(meta extLibItemMeta) bool {
	ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(meta.Ext), "."))
	return ext == "url" || ext == "webloc" || ext == "website"
}

func isURLShortcutFile(path string) bool {
	low := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(low, ".url") || strings.HasSuffix(low, ".webloc")
}

func parseURLShortcutFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if len(t) > 4 && strings.EqualFold(t[:4], "URL=") {
			return strings.TrimSpace(t[4:])
		}
	}
	return ""
}

func findExtLibThumbnailFile(infoDir string) string {
	files, err := os.ReadDir(infoDir)
	if err != nil {
		return ""
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		low := strings.ToLower(f.Name())
		if strings.Contains(low, "_thumbnail.") && (strings.HasSuffix(low, ".png") || strings.HasSuffix(low, ".jpg") || strings.HasSuffix(low, ".jpeg") || strings.HasSuffix(low, ".webp")) {
			return filepath.Join(infoDir, f.Name())
		}
	}
	return ""
}

func sanitizeBookmarkFilename(name string) string {
	s := strings.TrimSpace(name)
	repl := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", "\r", " ", "\n", " ",
	)
	s = strings.TrimSpace(repl.Replace(s))
	s = strings.Trim(s, ". ")
	if len([]rune(s)) > 120 {
		s = string([]rune(s)[:120])
	}
	return s
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
