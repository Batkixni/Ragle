package share

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

// sharedTagName is the tag a received project gets when it arrived with none.
const sharedTagName = "Shared"

// FileSnap is one file the sender is offering. The bytes follow only after
// the receiver accepts.
type FileSnap struct {
	Hash string `msgpack:"hash"`
	Name string `msgpack:"name"`
	Size int64  `msgpack:"size"`
}

// AssetSnap is an asset record plus the names of its tags.
type AssetSnap struct {
	Asset    model.Asset `msgpack:"asset"`
	TagNames []string    `msgpack:"tag_names"`
}

// Bundle is the copy of a project that crosses the network.
// Paths are file names only. Thumbnail paths are cleared.
type Bundle struct {
	Project    model.Project     `msgpack:"project"`
	Tags       []model.Tag       `msgpack:"tags"`
	Boards     []model.Board     `msgpack:"boards"`
	Placements []model.Placement `msgpack:"placements"`
	Assets     []AssetSnap       `msgpack:"assets"`
	References []string          `msgpack:"references"`
	Files      []FileSnap        `msgpack:"files"`
}

type pendingFile struct {
	hash string
	path string
	name string
}

// Export reads a project into a bundle. When includeFiles is set, Files lists
// the asset files that still match their stored hash, and local maps each
// hash to the path on this machine. local is not part of the bundle.
func Export(d *db.DB, projectID string, includeFiles bool) (Bundle, map[string]string, error) {
	var b Bundle
	var pending []pendingFile
	err := d.View(func(tx *bbolt.Tx) error {
		p, err := db.GetProject(tx, projectID)
		if err != nil {
			return err
		}
		b.Project = p
		tagIDs, err := db.TagsOfProject(tx, projectID)
		if err != nil {
			return err
		}
		for _, id := range tagIDs {
			tag, err := db.GetTag(tx, id)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					continue
				}
				return err
			}
			b.Tags = append(b.Tags, tag)
		}
		boardIDs, err := db.BoardsOfProject(tx, projectID)
		if err != nil {
			return err
		}
		for _, id := range boardIDs {
			board, err := db.GetBoard(tx, id)
			if err != nil {
				return err
			}
			b.Boards = append(b.Boards, board)
			items, err := db.LoadBoard(tx, id)
			if err != nil {
				return err
			}
			b.Placements = append(b.Placements, items...)
		}
		refs, err := db.AssetsOfProject(tx, projectID)
		if err != nil {
			return err
		}
		b.References = append([]string(nil), refs...)
		seen := map[string]bool{}
		ids := append([]string{}, refs...)
		for _, item := range b.Placements {
			if item.AssetID != "" {
				ids = append(ids, item.AssetID)
			}
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			asset, err := db.GetAsset(tx, id)
			if err != nil {
				return err
			}
			tagIDs, err := db.TagsOfAsset(tx, id)
			if err != nil {
				return err
			}
			var names []string
			for _, tagID := range tagIDs {
				tag, err := db.GetTag(tx, tagID)
				if err != nil {
					if errors.Is(err, db.ErrNotFound) {
						continue
					}
					return err
				}
				names = append(names, tag.Name)
			}
			if includeFiles && !asset.Missing && asset.Hash != "" && asset.Path != "" {
				pending = append(pending, pendingFile{
					hash: asset.Hash,
					path: asset.Path,
					name: fileBase(asset.Path),
				})
			}
			b.Assets = append(b.Assets, AssetSnap{Asset: scrubAsset(asset), TagNames: names})
		}
		return nil
	})
	if err != nil {
		return Bundle{}, nil, err
	}
	local := map[string]string{}
	for _, item := range pending {
		if _, ok := local[item.hash]; ok {
			continue
		}
		sum, err := hashFile(item.path)
		if err != nil || sum != item.hash {
			continue
		}
		fi, err := os.Stat(item.path)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		b.Files = append(b.Files, FileSnap{Hash: item.hash, Name: item.name, Size: fi.Size()})
		local[item.hash] = item.path
	}
	return b, local, nil
}

// Apply writes a received bundle into d. paths maps a hash to a file this
// receive already wrote. Assets whose hash is already in the library are
// reused, and the extra file is removed. New assets without a file are
// stored as missing index records so a later delete does not remove a file
// the library does not own. The returned id is the new project.
func Apply(d *db.DB, b Bundle, paths map[string]string) (string, error) {
	if paths == nil {
		paths = map[string]string{}
	}
	for hash, path := range paths {
		sum, err := hashFile(path)
		if err != nil || sum != hash {
			return "", errors.New("raglex: file hash mismatch")
		}
	}
	var projectID string
	var drop []string
	err := d.Update(func(tx *bbolt.Tx) error {
		known, err := tagIndex(tx)
		if err != nil {
			return err
		}
		ensure := func(name string) (string, error) {
			name = strings.TrimSpace(name)
			if name == "" {
				return "", nil
			}
			key := strings.ToLower(name)
			if id, ok := known[key]; ok {
				return id, nil
			}
			tag := model.Tag{ID: model.NewID(), Name: name, CreatedAt: time.Now()}
			if err := db.PutTag(tx, tag); err != nil {
				return "", err
			}
			known[key] = tag.ID
			return tag.ID, nil
		}
		var projectTags []string
		for _, tag := range b.Tags {
			id, err := ensure(tag.Name)
			if err != nil {
				return err
			}
			if id != "" {
				projectTags = append(projectTags, id)
			}
		}
		if len(projectTags) == 0 {
			id, err := ensure(sharedTagName)
			if err != nil {
				return err
			}
			projectTags = []string{id}
		}
		assetMap := map[string]string{}
		for _, snap := range b.Assets {
			if snap.Asset.Hash == "" {
				return errors.New("raglex: asset hash required")
			}
			if existing, ok := db.AssetIDByHash(tx, snap.Asset.Hash); ok {
				assetMap[snap.Asset.ID] = existing
				if path := paths[snap.Asset.Hash]; path != "" {
					drop = append(drop, path)
				}
				if err := linkNames(tx, ensure, existing, snap.TagNames); err != nil {
					return err
				}
				continue
			}
			asset := snap.Asset
			asset.ID = model.NewID()
			asset.ThumbnailPath = ""
			asset.Path = fileBase(asset.Path)
			if path := paths[asset.Hash]; path != "" {
				asset.Path = path
				asset.StorageMode = model.StorageCopy
				asset.Missing = false
			} else {
				asset.StorageMode = model.StorageIndex
				asset.Missing = true
			}
			if asset.CreatedAt.IsZero() {
				asset.CreatedAt = time.Now()
			}
			if err := db.PutAsset(tx, asset); err != nil {
				return err
			}
			assetMap[snap.Asset.ID] = asset.ID
			if err := linkNames(tx, ensure, asset.ID, snap.TagNames); err != nil {
				return err
			}
		}
		project := b.Project
		project.ID = model.NewID()
		if project.CreatedAt.IsZero() {
			project.CreatedAt = time.Now()
		}
		if err := db.PutProject(tx, project); err != nil {
			return err
		}
		for _, tagID := range projectTags {
			if err := db.LinkProjectTag(tx, project.ID, tagID); err != nil {
				return err
			}
		}
		boardMap := map[string]string{}
		for _, board := range b.Boards {
			nb := board
			nb.ID = model.NewID()
			nb.ProjectID = project.ID
			if nb.CreatedAt.IsZero() {
				nb.CreatedAt = time.Now()
			}
			if err := db.PutBoard(tx, nb); err != nil {
				return err
			}
			boardMap[board.ID] = nb.ID
		}
		placeMap := map[string]string{}
		for _, item := range b.Placements {
			placeMap[item.ID] = model.NewID()
		}
		linked := map[string]bool{}
		linkAsset := func(id string) error {
			if id == "" || linked[id] {
				return nil
			}
			linked[id] = true
			return db.LinkProjectAsset(tx, project.ID, id)
		}
		for _, old := range b.References {
			if err := linkAsset(assetMap[old]); err != nil {
				return err
			}
		}
		for _, src := range b.Placements {
			boardID := boardMap[src.BoardID]
			if boardID == "" {
				continue
			}
			item := src
			item.ID = placeMap[src.ID]
			item.BoardID = boardID
			if src.ParentID != "" {
				item.ParentID = placeMap[src.ParentID]
			}
			if src.AssetID != "" {
				item.AssetID = assetMap[src.AssetID]
				if err := linkAsset(item.AssetID); err != nil {
					return err
				}
			}
			if err := db.PutPlacement(tx, item); err != nil {
				return err
			}
		}
		projectID = project.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	for _, path := range drop {
		_ = os.Remove(path)
	}
	return projectID, nil
}

// SaveFile writes r into dir under name, adding " (2)" when the name is taken.
// It consumes r to completion.
func SaveFile(dir, name string, r io.Reader) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		_, _ = io.Copy(io.Discard, r)
		return "", err
	}
	name = fileBase(name)
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		dest := uniqueDest(dir, name)
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			last = err
			continue
		}
		if err != nil {
			_, _ = io.Copy(io.Discard, r)
			return "", err
		}
		_, copyErr := io.Copy(out, r)
		closeErr := out.Close()
		if copyErr != nil {
			_ = os.Remove(dest)
			return "", copyErr
		}
		if closeErr != nil {
			_ = os.Remove(dest)
			return "", closeErr
		}
		return dest, nil
	}
	_, _ = io.Copy(io.Discard, r)
	if last == nil {
		last = errors.New("raglex: could not name file")
	}
	return "", last
}

func linkNames(tx *bbolt.Tx, ensure func(string) (string, error), assetID string, names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		id, err := ensure(name)
		if err != nil {
			return err
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if err := db.LinkAssetTag(tx, assetID, id); err != nil {
			return err
		}
	}
	return nil
}

func tagIndex(tx *bbolt.Tx) (map[string]string, error) {
	tags, err := db.ListTags(tx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]string, len(tags))
	for _, tag := range tags {
		known[strings.ToLower(strings.TrimSpace(tag.Name))] = tag.ID
	}
	return known, nil
}

func scrubAsset(asset model.Asset) model.Asset {
	asset.ThumbnailPath = ""
	asset.Path = fileBase(asset.Path)
	return asset
}

func fileBase(path string) string {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "file"
	}
	return name
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

func uniqueDest(dir, name string) string {
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "file"
	}
	candidate := filepath.Join(dir, name)
	if _, err := os.Lstat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(name)
	stem := name
	if ext != "" {
		stem = name[:len(name)-len(ext)]
	}
	for i := 2; i < 10000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, os.Getpid(), ext))
}
