// Package library imports files into a library and keeps tags on them.
package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"ragle/internal/db"
	"ragle/internal/model"
	"ragle/internal/thumbnail"
)

// ErrSymlink means the operating system refused to create a link.
// The source file is left where it was.
var ErrSymlink = errors.New("ragle: symlink failed")

// ErrEmptyName means a tag name was blank.
var ErrEmptyName = errors.New("ragle: empty name")

// ErrDuplicateName means a tag with that name already exists.
var ErrDuplicateName = errors.New("ragle: tag already exists")

// Filter selects assets. An empty TagID lists the whole library.
// Kind is image, video, other, or empty. MinRating of 0 does not filter.
type Filter struct {
	TagID     string
	Kind      string
	MinRating int
}

// Item is one file the importer considered.
type Item struct {
	Path    string
	AssetID string
	Status  string // added, existing, failed
	Err     error
}

// Result counts an import.
type Result struct {
	Added    int
	Existing int
	Failed   int
	Items    []Item
}

// Library is the open asset catalog.
type Library struct {
	DB *db.DB
}

// Open creates or opens the library in root.
func Open(root string) (*Library, error) {
	d, err := db.Open(root)
	if err != nil {
		return nil, err
	}
	return &Library{DB: d}, nil
}

// Close closes the database.
func (l *Library) Close() error { return l.DB.Close() }

// CreateTag adds a tag. Names are unique, ignoring case.
func (l *Library) CreateTag(name string) (model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Tag{}, ErrEmptyName
	}
	var created model.Tag
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		tags, err := db.ListTags(tx)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if strings.EqualFold(tag.Name, name) {
				return ErrDuplicateName
			}
		}
		created = model.Tag{ID: model.NewID(), Name: name, CreatedAt: time.Now().UTC()}
		return db.PutTag(tx, created)
	})
	return created, err
}

// Tags lists every tag.
func (l *Library) Tags() ([]model.Tag, error) {
	var tags []model.Tag
	err := l.DB.View(func(tx *bbolt.Tx) error {
		var err error
		tags, err = db.ListTags(tx)
		return err
	})
	return tags, err
}

// DeleteTag removes the tag and its links. Assets and projects stay.
func (l *Library) DeleteTag(id string) error {
	if err := l.DB.Backup(); err != nil {
		return err
	}
	return l.DB.Update(func(tx *bbolt.Tx) error { return db.DeleteTag(tx, id) })
}

// SetAssetTag adds or removes one tag on an asset.
func (l *Library) SetAssetTag(assetID, tagID string, on bool) error {
	return l.DB.Update(func(tx *bbolt.Tx) error {
		if _, err := db.GetAsset(tx, assetID); err != nil {
			return err
		}
		if _, err := db.GetTag(tx, tagID); err != nil {
			return err
		}
		if on {
			return db.LinkAssetTag(tx, assetID, tagID)
		}
		return db.UnlinkAssetTag(tx, assetID, tagID)
	})
}

// AssetTags lists tags on an asset.
func (l *Library) AssetTags(assetID string) ([]model.Tag, error) {
	var tags []model.Tag
	err := l.DB.View(func(tx *bbolt.Tx) error {
		ids, err := db.TagsOfAsset(tx, assetID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			tag, err := db.GetTag(tx, id)
			if err != nil {
				return err
			}
			tags = append(tags, tag)
		}
		return nil
	})
	return tags, err
}

// SetRating stores a rating from 0 to 5.
func (l *Library) SetRating(assetID string, rating int) error {
	if rating < 0 {
		rating = 0
	}
	if rating > 5 {
		rating = 5
	}
	return l.DB.Update(func(tx *bbolt.Tx) error {
		a, err := db.GetAsset(tx, assetID)
		if err != nil {
			return err
		}
		a.Rating = rating
		return db.PutAsset(tx, a)
	})
}

// List returns assets, optionally limited to one tag, then filtered in memory.
func (l *Library) List(f Filter) ([]model.Asset, error) {
	var assets []model.Asset
	err := l.DB.View(func(tx *bbolt.Tx) error {
		if f.TagID == "" {
			var err error
			assets, err = db.ListAssets(tx)
			return err
		}
		ids, err := db.AssetsOfTag(tx, f.TagID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			a, err := db.GetAsset(tx, id)
			if err != nil {
				return err
			}
			assets = append(assets, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if f.Kind == "" && f.MinRating <= 0 {
		return assets, nil
	}
	out := assets[:0]
	for _, a := range assets {
		if f.MinRating > 0 && a.Rating < f.MinRating {
			continue
		}
		if f.Kind != "" && model.MediaKind(a.Mime) != f.Kind {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// Get reads one asset.
func (l *Library) Get(id string) (model.Asset, error) {
	var a model.Asset
	err := l.DB.View(func(tx *bbolt.Tx) error {
		var err error
		a, err = db.GetAsset(tx, id)
		return err
	})
	return a, err
}

// SetThumbnail records a generated preview path.
func (l *Library) SetThumbnail(id, path string) error {
	return l.DB.Update(func(tx *bbolt.Tx) error {
		a, err := db.GetAsset(tx, id)
		if err != nil {
			return err
		}
		a.ThumbnailPath = path
		return db.PutAsset(tx, a)
	})
}

// Delete removes the asset, its links, and placements. Index mode keeps
// the user's file. Symlink mode removes only the link. Copy and move
// remove the file the library owns.
func (l *Library) Delete(id string) error {
	if err := l.DB.Backup(); err != nil {
		return err
	}
	var owned, thumb, mode string
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		a, err := db.GetAsset(tx, id)
		if err != nil {
			return err
		}
		mode = a.StorageMode
		thumb = a.ThumbnailPath
		if mode == model.StorageCopy || mode == model.StorageMove || mode == model.StorageSymlink {
			owned = a.Path
		}
		return db.DeleteAssetRecords(tx, id)
	})
	if err != nil {
		return err
	}
	if mode == model.StorageSymlink && owned != "" {
		if fi, statErr := os.Lstat(owned); statErr == nil && fi.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(owned)
		}
	} else if owned != "" {
		_ = os.Remove(owned)
	}
	if thumb != "" {
		_ = os.Remove(thumb)
	}
	return nil
}

// MarkMissing flags the asset stored at path and drops the path index.
// The path stays on the record so the interface can show where it was.
// It reports whether an asset changed.
func (l *Library) MarkMissing(path string) (bool, error) {
	path, err := cleanPath(path)
	if err != nil {
		return false, err
	}
	var changed bool
	err = l.DB.Update(func(tx *bbolt.Tx) error {
		id, ok := db.AssetIDByPath(tx, path)
		if !ok {
			return nil
		}
		a, err := db.GetAsset(tx, id)
		if err != nil {
			return err
		}
		a.Missing = true
		changed = true
		return db.PutAsset(tx, a)
	})
	return changed, err
}

// TryRelink attaches path to a missing asset with the same hash.
// It hashes nothing when every asset is still linked.
func (l *Library) TryRelink(path string) (bool, error) {
	path, err := cleanPath(path)
	if err != nil {
		return false, err
	}
	var missing []model.Asset
	err = l.DB.View(func(tx *bbolt.Tx) error {
		var err error
		missing, err = db.ListMissing(tx)
		return err
	})
	if err != nil || len(missing) == 0 {
		return false, err
	}
	sum, err := hashFile(path)
	if err != nil {
		return false, err
	}
	base := filepath.Base(path)
	ordered := make([]model.Asset, 0, len(missing))
	var rest []model.Asset
	for _, a := range missing {
		if filepath.Base(a.Path) == base {
			ordered = append(ordered, a)
		} else {
			rest = append(rest, a)
		}
	}
	ordered = append(ordered, rest...)
	for _, a := range ordered {
		if a.Hash != sum {
			continue
		}
		err := l.DB.Update(func(tx *bbolt.Tx) error {
			if other, ok := db.AssetIDByPath(tx, path); ok && other != a.ID {
				return fmt.Errorf("ragle: path already belongs to another asset")
			}
			cur, err := db.GetAsset(tx, a.ID)
			if err != nil {
				return err
			}
			cur.Path = path
			cur.Missing = false
			return db.PutAsset(tx, cur)
		})
		return err == nil, err
	}
	return false, nil
}

// WatchDirs are the directories the file watcher should observe:
// the library's files folder, and the parent of each indexed asset.
func (l *Library) WatchDirs() ([]string, error) {
	assets, err := l.List(Filter{})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{l.DB.FilesDir(): true}
	for _, a := range assets {
		if a.StorageMode != model.StorageIndex || a.Missing || a.Path == "" {
			continue
		}
		seen[filepath.Dir(a.Path)] = true
	}
	out := make([]string, 0, len(seen))
	for dir := range seen {
		out = append(out, dir)
	}
	return out, nil
}

// Import walks paths, applies mode, and writes one transaction per new file.
// progress may be nil. A symlink failure is reported on that file and does
// not copy it instead.
func (l *Library) Import(ctx context.Context, paths []string, mode string, tagIDs []string, progress func(done, total int)) (Result, error) {
	files, err := collectFiles(l.DB.Root(), paths)
	if err != nil {
		return Result{}, err
	}
	var result Result
	for i, path := range files {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		item := l.importOne(path, mode, tagIDs)
		result.Items = append(result.Items, item)
		switch item.Status {
		case "added":
			result.Added++
		case "existing":
			result.Existing++
		default:
			result.Failed++
		}
		if progress != nil {
			progress(i+1, len(files))
		}
	}
	return result, nil
}

func (l *Library) importOne(path, mode string, tagIDs []string) Item {
	abs, err := cleanPath(path)
	if err != nil {
		return Item{Path: path, Status: "failed", Err: err}
	}
	sum, err := hashFile(abs)
	if err != nil {
		return Item{Path: abs, Status: "failed", Err: err}
	}
	var existingID string
	err = l.DB.View(func(tx *bbolt.Tx) error {
		if id, ok := db.AssetIDByHash(tx, sum); ok {
			existingID = id
			return nil
		}
		if id, ok := db.AssetIDByPath(tx, abs); ok {
			existingID = id
		}
		return nil
	})
	if err != nil {
		return Item{Path: abs, Status: "failed", Err: err}
	}
	if existingID != "" {
		err = l.DB.Update(func(tx *bbolt.Tx) error { return linkTags(tx, existingID, tagIDs) })
		if err != nil {
			return Item{Path: abs, Status: "failed", Err: err, AssetID: existingID}
		}
		return Item{Path: abs, AssetID: existingID, Status: "existing"}
	}

	stored, err := placeFile(l.DB.FilesDir(), abs, mode)
	if err != nil {
		return Item{Path: abs, Status: "failed", Err: err}
	}
	info := inspect(stored)
	asset := model.Asset{
		ID:          model.NewID(),
		Path:        stored,
		StorageMode: mode,
		Hash:        sum,
		Mime:        info.mime,
		Width:       info.width,
		Height:      info.height,
		Duration:    info.duration,
		CreatedAt:   time.Now().UTC(),
	}
	err = l.DB.Update(func(tx *bbolt.Tx) error {
		if id, ok := db.AssetIDByHash(tx, sum); ok {
			existingID = id
			return linkTags(tx, id, tagIDs)
		}
		if err := db.PutAsset(tx, asset); err != nil {
			return err
		}
		return linkTags(tx, asset.ID, tagIDs)
	})
	if existingID != "" && err == nil {
		undoPlace(mode, abs, stored)
		return Item{Path: abs, AssetID: existingID, Status: "existing"}
	}
	if err != nil {
		undoPlace(mode, abs, stored)
		return Item{Path: abs, Status: "failed", Err: err}
	}
	return Item{Path: stored, AssetID: asset.ID, Status: "added"}
}

func linkTags(tx *bbolt.Tx, assetID string, tagIDs []string) error {
	for _, id := range tagIDs {
		if id == "" {
			continue
		}
		if _, err := db.GetTag(tx, id); err != nil {
			return err
		}
		if err := db.LinkAssetTag(tx, assetID, id); err != nil {
			return err
		}
	}
	return nil
}

type inspected struct {
	mime     string
	width    int
	height   int
	duration float64
}

func inspect(path string) inspected {
	mime := mimeOf(path)
	out := inspected{mime: mime}
	if model.MediaKind(mime) == model.MediaImage {
		out.width, out.height = imageSize(path)
	}
	if model.MediaKind(mime) == model.MediaVideo {
		if w, h, dur, err := thumbnail.Probe(path); err == nil {
			out.width, out.height, out.duration = w, h, dur
		}
	}
	return out
}

func mimeOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := io.ReadFull(f, buf)
	if n > 0 {
		return http.DetectContentType(buf[:n])
	}
	return "application/octet-stream"
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func cleanPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
