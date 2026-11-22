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

	"raglex/internal/db"
	"raglex/internal/model"
	"raglex/internal/thumbnail"
)

// ErrSymlink means the operating system refused to create a link.
// The source file is left where it was.
var ErrSymlink = errors.New("raglex: symlink failed")

// ErrEmptyName means a tag name was blank.
var ErrEmptyName = errors.New("raglex: empty name")

// ErrDuplicateName means a tag with that name already exists.
var ErrDuplicateName = errors.New("raglex: tag already exists")

// ErrTagCycle means moving a tag under itself or one of its own descendants.
var ErrTagCycle = errors.New("raglex: tag cannot be its own ancestor")

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

// ImportProgress reports live status during a batched import.
type ImportProgress struct {
	Stage    string // "scanning", "importing"
	Done     int
	Total    int
	Added    int
	Existing int
	Failed   int
	Current  string
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

// CreateTag adds a root tag. Names are unique, ignoring case.
func (l *Library) CreateTag(name string) (model.Tag, error) {
	return l.CreateTagWithParent(name, "")
}

// CreateTagWithParent adds a tag optionally nested under parentID.
func (l *Library) CreateTagWithParent(name, parentID string) (model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Tag{}, ErrEmptyName
	}
	var created model.Tag
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		if parentID != "" {
			if _, err := db.GetTag(tx, parentID); err != nil {
				return err
			}
		}
		tags, err := db.ListTags(tx)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if strings.EqualFold(tag.Name, name) {
				return ErrDuplicateName
			}
		}
		created = model.Tag{
			ID:        model.NewID(),
			Name:      name,
			ParentID:  parentID,
			CreatedAt: time.Now().UTC(),
		}
		return db.PutTag(tx, created)
	})
	return created, err
}

// UpdateTag renames a tag and/or changes its parent tag, rejecting cycles.
func (l *Library) UpdateTag(id, name, parentID string) (model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Tag{}, ErrEmptyName
	}
	if parentID == id {
		return model.Tag{}, ErrTagCycle
	}
	var updated model.Tag
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetTag(tx, id)
		if err != nil {
			return err
		}
		tags, err := db.ListTags(tx)
		if err != nil {
			return err
		}
		byID := make(map[string]model.Tag, len(tags))
		for _, tag := range tags {
			byID[tag.ID] = tag
			if tag.ID != id && strings.EqualFold(tag.Name, name) {
				return ErrDuplicateName
			}
		}
		if parentID != "" {
			if _, ok := byID[parentID]; !ok {
				return db.ErrNotFound
			}
			// Walk ancestors of parentID to prevent cycles.
			walk := parentID
			seen := map[string]bool{id: true}
			for walk != "" {
				if seen[walk] {
					return ErrTagCycle
				}
				seen[walk] = true
				walk = byID[walk].ParentID
			}
		}
		cur.Name = name
		cur.ParentID = parentID
		updated = cur
		return db.PutTag(tx, cur)
	})
	return updated, err
}

// SetTagColor sets or clears the #RRGGBB display color on a tag.
func (l *Library) SetTagColor(id, color string) error {
	color = strings.TrimSpace(color)
	return l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetTag(tx, id)
		if err != nil {
			return err
		}
		cur.Color = color
		return db.PutTag(tx, cur)
	})
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

// DeleteTag removes the tag and its links. Direct child tags are promoted to
// the deleted tag's parent so no nested tags become orphaned. Assets and projects stay.
func (l *Library) DeleteTag(id string) error {
	if err := l.DB.Backup(); err != nil {
		return err
	}
	return l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetTag(tx, id)
		if err != nil {
			return err
		}
		tags, err := db.ListTags(tx)
		if err != nil {
			return err
		}
		for _, child := range tags {
			if child.ParentID == id {
				child.ParentID = cur.ParentID
				if err := db.PutTag(tx, child); err != nil {
					return err
				}
			}
		}
		return db.DeleteTag(tx, id)
	})
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
		if a.StackID != "" {
			if err := detachAssetFromStackTx(tx, a.ID, a.StackID); err != nil {
				return err
			}
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
				return fmt.Errorf("raglex: path already belongs to another asset")
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

// RelinkPaths scans files or folders and relinks any missing assets whose
// SHA-256 matches. Returns the count of relinked assets.
func (l *Library) RelinkPaths(ctx context.Context, paths []string) (int, error) {
	files, err := collectFiles(l.DB.Root(), paths)
	if err != nil {
		return 0, err
	}
	relinked := 0
	for _, p := range files {
		if ctx.Err() != nil {
			return relinked, ctx.Err()
		}
		ok, err := l.TryRelink(p)
		if err != nil {
			continue
		}
		if ok {
			relinked++
		}
	}
	return relinked, nil
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

// CatalogSnapshot holds the entire library catalog loaded in one bbolt read transaction.
type CatalogSnapshot struct {
	Tags      []model.Tag
	Assets    []model.Asset
	Stacks    []model.Stack
	AssetTags map[string][]model.Tag
}

// LoadCatalogSnapshot loads all tags, assets, stacks, and asset-tag mappings in
// a single bbolt read transaction (avoiding N+1 transactions on large libraries).
func (l *Library) LoadCatalogSnapshot() (CatalogSnapshot, error) {
	var snap CatalogSnapshot
	err := l.DB.View(func(tx *bbolt.Tx) error {
		tags, err := db.ListTags(tx)
		if err != nil {
			return err
		}
		assets, err := db.ListAssets(tx)
		if err != nil {
			return err
		}
		stacks, err := db.ListStacks(tx)
		if err != nil {
			return err
		}
		rawLinks, err := db.AllAssetTagIDs(tx)
		if err != nil {
			return err
		}
		tagByID := make(map[string]model.Tag, len(tags))
		for _, tg := range tags {
			tagByID[tg.ID] = tg
		}
		assetTags := make(map[string][]model.Tag, len(rawLinks))
		for assetID, tids := range rawLinks {
			list := make([]model.Tag, 0, len(tids))
			for _, tid := range tids {
				if tg, ok := tagByID[tid]; ok {
					list = append(list, tg)
				}
			}
			if len(list) > 0 {
				assetTags[assetID] = list
			}
		}
		snap = CatalogSnapshot{
			Tags:      tags,
			Assets:    assets,
			Stacks:    stacks,
			AssetTags: assetTags,
		}
		return nil
	})
	return snap, err
}

// PurgeAuxiliarySidecars removes any already-imported Eagle/system auxiliary files
// (such as metadata.json and *_thumbnail.png) from the catalog in one batch transaction.
func (l *Library) PurgeAuxiliarySidecars() (int, error) {
	var toDelete []model.Asset
	err := l.DB.View(func(tx *bbolt.Tx) error {
		all, err := db.ListAssets(tx)
		if err != nil {
			return err
		}
		for _, a := range all {
			if IsAuxiliarySidecarFile(filepath.Base(a.Path)) {
				toDelete = append(toDelete, a)
			}
		}
		return nil
	})
	if err != nil || len(toDelete) == 0 {
		return 0, err
	}
	err = l.DB.Update(func(tx *bbolt.Tx) error {
		for _, a := range toDelete {
			if a.StackID != "" {
				_ = detachAssetFromStackTx(tx, a.ID, a.StackID)
			}
			if err := db.DeleteAssetRecords(tx, a.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, a := range toDelete {
		if a.ThumbnailPath != "" {
			_ = os.Remove(a.ThumbnailPath)
		}
		if a.StorageMode == model.StorageCopy || a.StorageMode == model.StorageMove {
			_ = os.Remove(a.Path)
		} else if a.StorageMode == model.StorageSymlink {
			if fi, statErr := os.Lstat(a.Path); statErr == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(a.Path)
			}
		}
	}
	return len(toDelete), nil
}

// Import walks paths, applies mode, and commits new files in batched transactions.
// progress may be nil. A symlink failure is reported on that file and does
// not copy it instead.
func (l *Library) Import(ctx context.Context, paths []string, mode string, tagIDs []string, progress func(done, total int)) (Result, error) {
	var fn func(ImportProgress)
	if progress != nil {
		fn = func(p ImportProgress) {
			if p.Total > 0 {
				progress(p.Done, p.Total)
			}
		}
	}
	return l.ImportWithProgress(ctx, paths, mode, tagIDs, fn)
}

// ImportWithProgress imports paths in fast 128-file transactions while
// reporting live file-level progress and respecting ctx cancellation.
func (l *Library) ImportWithProgress(ctx context.Context, paths []string, mode string, tagIDs []string, progress func(ImportProgress)) (Result, error) {
	files, err := collectFiles(l.DB.Root(), paths)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if len(files) == 0 {
		return result, nil
	}

	total := len(files)
	lastEmit := time.Time{}
	emit := func(done int, current string, force bool) {
		if progress == nil {
			return
		}
		if !force && !lastEmit.IsZero() && time.Since(lastEmit) < 60*time.Millisecond {
			return
		}
		lastEmit = time.Now()
		progress(ImportProgress{
			Stage:    "importing",
			Done:     done,
			Total:    total,
			Added:    result.Added,
			Existing: result.Existing,
			Failed:   result.Failed,
			Current:  filepath.Base(current),
		})
	}
	emit(0, files[0], true)

	const batchSize = 128
	type preparedItem struct {
		abs      string
		stored   string
		sum      string
		asset    model.Asset
		existing string
		err      error
	}

	for start := 0; start < len(files); start += batchSize {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		end := min(start+batchSize, len(files))
		chunk := files[start:end]
		prep := make([]preparedItem, len(chunk))

		// Phase 1: hash & check existing in a single read transaction.
		for i, rawPath := range chunk {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			emit(start+i, rawPath, false)
			abs, err := cleanPath(rawPath)
			if err != nil {
				prep[i] = preparedItem{abs: rawPath, err: err}
				continue
			}
			sum, err := hashFile(abs)
			if err != nil {
				prep[i] = preparedItem{abs: abs, err: err}
				continue
			}
			prep[i] = preparedItem{abs: abs, sum: sum}
		}

		_ = l.DB.View(func(tx *bbolt.Tx) error {
			for i := range prep {
				if prep[i].err != nil {
					continue
				}
				if id, ok := db.AssetIDByHash(tx, prep[i].sum); ok {
					prep[i].existing = id
					continue
				}
				if id, ok := db.AssetIDByPath(tx, prep[i].abs); ok {
					prep[i].existing = id
				}
			}
			return nil
		})

		// Phase 2: place new files & inspect dimensions outside the DB lock.
		seenHashInBatch := map[string]string{}
		for i := range prep {
			if ctx.Err() != nil {
				// Roll back any un-committed files placed earlier in this chunk.
				for j := 0; j < i; j++ {
					if prep[j].stored != "" {
						undoPlace(mode, prep[j].abs, prep[j].stored)
					}
				}
				return result, ctx.Err()
			}
			p := &prep[i]
			emit(start+i, p.abs, false)
			if p.err != nil || p.existing != "" {
				continue
			}
			if dupID, ok := seenHashInBatch[p.sum]; ok {
				p.existing = dupID
				continue
			}
			stored, err := placeFile(l.DB.FilesDir(), p.abs, mode)
			if err != nil {
				p.err = err
				continue
			}
			p.stored = stored
			info := inspect(stored)
			p.asset = model.Asset{
				ID:          model.NewID(),
				Path:        stored,
				StorageMode: mode,
				Hash:        p.sum,
				Mime:        info.mime,
				Width:       info.width,
				Height:      info.height,
				Duration:    info.duration,
				CreatedAt:   time.Now().UTC(),
			}
			seenHashInBatch[p.sum] = p.asset.ID
		}

		// Phase 3: commit the entire chunk in a SINGLE bbolt write transaction!
		_ = l.DB.Update(func(tx *bbolt.Tx) error {
			for i := range prep {
				p := &prep[i]
				if p.err != nil {
					continue
				}
				if p.existing != "" {
					if err := linkTags(tx, p.existing, tagIDs); err != nil {
						p.err = err
					}
					continue
				}
				if id, ok := db.AssetIDByHash(tx, p.sum); ok {
					p.existing = id
					if err := linkTags(tx, id, tagIDs); err != nil {
						p.err = err
					}
					continue
				}
				if err := db.PutAsset(tx, p.asset); err != nil {
					p.err = err
					continue
				}
				if err := linkTags(tx, p.asset.ID, tagIDs); err != nil {
					p.err = err
				}
			}
			return nil
		})

		// Phase 4: tally results and undo any failed placements.
		for i := range prep {
			p := &prep[i]
			var item Item
			switch {
			case p.err != nil:
				if p.stored != "" {
					undoPlace(mode, p.abs, p.stored)
				}
				item = Item{Path: p.abs, AssetID: p.existing, Status: "failed", Err: p.err}
				result.Failed++
			case p.existing != "":
				if p.stored != "" {
					undoPlace(mode, p.abs, p.stored)
				}
				item = Item{Path: p.abs, AssetID: p.existing, Status: "existing"}
				result.Existing++
			default:
				item = Item{Path: p.stored, AssetID: p.asset.ID, Status: "added"}
				result.Added++
			}
			result.Items = append(result.Items, item)
		}
		emit(end, chunk[len(chunk)-1], true)
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
	case ".svg":
		return "image/svg+xml"
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
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".opus":
		return "audio/opus"
	case ".wma":
		return "audio/x-ms-wma"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".pdf":
		return "application/pdf"
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
