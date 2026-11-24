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
	Hash string `msgpack:"hash" json:"hash"`
	Name string `msgpack:"name" json:"name"`
	Size int64  `msgpack:"size" json:"size"`
}

// FolderSnap is one folder node in a shared folder tree.
type FolderSnap struct {
	ID       string `msgpack:"id" json:"id"`
	Name     string `msgpack:"name" json:"name"`
	ParentID string `msgpack:"parent_id,omitempty" json:"parent_id,omitempty"`
	Color    string `msgpack:"color,omitempty" json:"color,omitempty"`
	Order    int    `msgpack:"order,omitempty" json:"order,omitempty"`
}

// AssetSnap is an asset record plus the names of its tags and folder IDs.
type AssetSnap struct {
	Asset     model.Asset `msgpack:"asset" json:"asset"`
	TagNames  []string    `msgpack:"tag_names" json:"tag_names"`
	FolderIDs []string    `msgpack:"folder_ids,omitempty" json:"folder_ids,omitempty"`
}

// Bundle is the copy of a project or folder tree that crosses the network.
// Paths are file names only. Thumbnail paths are cleared.
type Bundle struct {
	Kind         string            `msgpack:"kind,omitempty" json:"kind,omitempty"` // "", "project", or "folder"
	RootFolderID string            `msgpack:"root_folder_id,omitempty" json:"root_folder_id,omitempty"`
	Folders      []FolderSnap      `msgpack:"folders,omitempty" json:"folders,omitempty"`
	Project      model.Project     `msgpack:"project" json:"project"`
	Tags         []model.Tag       `msgpack:"tags" json:"tags"`
	Boards       []model.Board     `msgpack:"boards" json:"boards"`
	Placements   []model.Placement `msgpack:"placements" json:"placements"`
	Assets       []AssetSnap       `msgpack:"assets" json:"assets"`
	References   []string          `msgpack:"references" json:"references"`
	Files        []FileSnap        `msgpack:"files" json:"files"`
}

// DisplayName returns the user-facing title of this shared bundle.
func (b Bundle) DisplayName() string {
	if b.Kind == "folder" {
		for _, f := range b.Folders {
			if f.ID == b.RootFolderID && strings.TrimSpace(f.Name) != "" {
				return f.Name
			}
		}
		if len(b.Folders) > 0 && strings.TrimSpace(b.Folders[0].Name) != "" {
			return b.Folders[0].Name
		}
	}
	return b.Project.Name
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

// ExportFolder reads a folder and all of its nested subfolders plus every asset
// linked to any folder in that subtree into a Bundle.
func ExportFolder(d *db.DB, folderID string, includeFiles bool) (Bundle, map[string]string, error) {
	b := Bundle{
		Kind:         "folder",
		RootFolderID: folderID,
	}
	var pending []pendingFile
	err := d.View(func(tx *bbolt.Tx) error {
		root, err := db.GetFolder(tx, folderID)
		if err != nil {
			return err
		}
		allFolders, err := db.ListFolders(tx)
		if err != nil {
			return err
		}
		children := map[string][]model.Folder{}
		for _, f := range allFolders {
			children[f.ParentID] = append(children[f.ParentID], f)
		}
		subtreeIDs := map[string]bool{}
		var ordered []model.Folder
		var walk func(f model.Folder)
		walk = func(f model.Folder) {
			if subtreeIDs[f.ID] {
				return
			}
			subtreeIDs[f.ID] = true
			ordered = append(ordered, f)
			for _, ch := range children[f.ID] {
				walk(ch)
			}
		}
		walk(root)

		for _, f := range ordered {
			pid := f.ParentID
			if f.ID == root.ID || !subtreeIDs[pid] {
				pid = ""
			}
			b.Folders = append(b.Folders, FolderSnap{
				ID:       f.ID,
				Name:     f.Name,
				ParentID: pid,
				Color:    f.Color,
				Order:    f.Order,
			})
		}

		seenAsset := map[string]bool{}
		var assetIDs []string
		for _, f := range ordered {
			ids, err := db.AssetsOfFolder(tx, f.ID)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if !seenAsset[id] {
					seenAsset[id] = true
					assetIDs = append(assetIDs, id)
				}
			}
		}

		for _, id := range assetIDs {
			asset, err := db.GetAsset(tx, id)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					continue
				}
				return err
			}
			tagIDs, err := db.TagsOfAsset(tx, id)
			if err != nil {
				return err
			}
			var tagNames []string
			for _, tid := range tagIDs {
				tg, err := db.GetTag(tx, tid)
				if err != nil {
					if errors.Is(err, db.ErrNotFound) {
						continue
					}
					return err
				}
				tagNames = append(tagNames, tg.Name)
			}
			fids, err := db.FoldersOfAsset(tx, id)
			if err != nil {
				return err
			}
			var subFids []string
			for _, fid := range fids {
				if subtreeIDs[fid] {
					subFids = append(subFids, fid)
				}
			}
			if includeFiles && !asset.Missing && asset.Hash != "" && asset.Path != "" {
				pending = append(pending, pendingFile{
					hash: asset.Hash,
					path: asset.Path,
					name: fileBase(asset.Path),
				})
			}
			b.Assets = append(b.Assets, AssetSnap{
				Asset:     scrubAsset(asset),
				TagNames:  tagNames,
				FolderIDs: subFids,
			})
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
// the library does not own. The returned id is the new project or folder.
func Apply(d *db.DB, b Bundle, paths map[string]string) (string, error) {
	return ApplyInto(d, b, paths, "")
}

// ApplyInto applies a bundle into d, optionally updating an existing local
// projectID in-place (merging boards, placements, and references without creating
// duplicate projects — ideal for live multi-user collaboration sync!).
func ApplyInto(d *db.DB, b Bundle, paths map[string]string, existingProjectID string) (string, error) {
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
		if b.Kind != "folder" && len(projectTags) == 0 {
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

		// Folder-kind share: reconstruct the nested Folder tree and link all assets.
		if b.Kind == "folder" {
			existingFolders, err := db.ListFolders(tx)
			if err != nil {
				return err
			}
			// Index existing folders by (parentID + "\x00" + lowercase name).
			folderKey := func(pid, name string) string {
				return pid + "\x00" + strings.ToLower(strings.TrimSpace(name))
			}
			knownFolders := make(map[string]model.Folder, len(existingFolders))
			for _, ef := range existingFolders {
				knownFolders[folderKey(ef.ParentID, ef.Name)] = ef
			}
			folderMap := map[string]string{}
			// Create/resolve folders in topological order (parents before children).
			remaining := append([]FolderSnap(nil), b.Folders...)
			for pass := 0; len(remaining) > 0 && pass < 32; pass++ {
				var next []FolderSnap
				progressed := false
				for _, fs := range remaining {
					mappedParent := ""
					if fs.ParentID != "" {
						mp, ok := folderMap[fs.ParentID]
						if !ok {
							next = append(next, fs)
							continue
						}
						mappedParent = mp
					}
					name := strings.TrimSpace(fs.Name)
					if name == "" {
						name = "Shared Folder"
					}
					k := folderKey(mappedParent, name)
					if ex, ok := knownFolders[k]; ok {
						folderMap[fs.ID] = ex.ID
					} else {
						nf := model.Folder{
							ID:        model.NewID(),
							Name:      name,
							ParentID:  mappedParent,
							Color:     fs.Color,
							Order:     fs.Order,
							CreatedAt: time.Now().UTC(),
						}
						if err := db.PutFolder(tx, nf); err != nil {
							return err
						}
						knownFolders[k] = nf
						folderMap[fs.ID] = nf.ID
					}
					progressed = true
				}
				remaining = next
				if !progressed {
					break
				}
			}
			rootMapped := folderMap[b.RootFolderID]
			if rootMapped == "" && len(b.Folders) > 0 {
				rootMapped = folderMap[b.Folders[0].ID]
			}
			for _, snap := range b.Assets {
				aid := assetMap[snap.Asset.ID]
				if aid == "" {
					continue
				}
				linkedAny := false
				for _, oldFID := range snap.FolderIDs {
					if mappedFID := folderMap[oldFID]; mappedFID != "" {
						if err := db.LinkAssetFolder(tx, aid, mappedFID); err != nil {
							return err
						}
						linkedAny = true
					}
				}
				if !linkedAny && rootMapped != "" {
					if err := db.LinkAssetFolder(tx, aid, rootMapped); err != nil {
						return err
					}
				}
			}
			projectID = rootMapped
			return nil
		}

		project := b.Project
		reusingProject := false
		if existingProjectID != "" {
			if curP, err := db.GetProject(tx, existingProjectID); err == nil {
				project = curP
				if strings.TrimSpace(b.Project.Description) != "" {
					project.Description = b.Project.Description
				}
				reusingProject = true
			}
		}
		if !reusingProject {
			project.ID = model.NewID()
			if project.CreatedAt.IsZero() {
				project.CreatedAt = time.Now()
			}
		}
		if err := db.PutProject(tx, project); err != nil {
			return err
		}
		for _, tagID := range projectTags {
			if err := db.LinkProjectTag(tx, project.ID, tagID); err != nil {
				return err
			}
		}
		existingBoardsByName := map[string]model.Board{}
		if reusingProject {
			if bids, err := db.BoardsOfProject(tx, project.ID); err == nil {
				for _, bid := range bids {
					if eb, err := db.GetBoard(tx, bid); err == nil {
						existingBoardsByName[strings.ToLower(strings.TrimSpace(eb.Name))] = eb
					}
				}
			}
		}
		boardMap := map[string]string{}
		for _, board := range b.Boards {
			bk := strings.ToLower(strings.TrimSpace(board.Name))
			if eb, ok := existingBoardsByName[bk]; ok {
				boardMap[board.ID] = eb.ID
				// Replace placements cleanly on reused collab board so repeated syncs never duplicate items.
				if oldItems, err := db.LoadBoard(tx, eb.ID); err == nil {
					for _, oi := range oldItems {
						_ = db.DeletePlacement(tx, oi.ID)
					}
				}
				continue
			}
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
