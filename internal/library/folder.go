package library

import (
	"sort"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

// CreateFolder adds a root folder. Sibling names under the same parent are unique ignoring case.
func (l *Library) CreateFolder(name string) (model.Folder, error) {
	return l.CreateFolderWithParent(name, "")
}

// CreateFolderWithParent adds a folder optionally nested under parentID.
func (l *Library) CreateFolderWithParent(name, parentID string) (model.Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Folder{}, ErrEmptyName
	}
	var created model.Folder
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		if parentID != "" {
			if _, err := db.GetFolder(tx, parentID); err != nil {
				return err
			}
		}
		folders, err := db.ListFolders(tx)
		if err != nil {
			return err
		}
		for _, f := range folders {
			if f.ParentID == parentID && strings.EqualFold(f.Name, name) {
				return ErrDuplicateName
			}
		}
		created = model.Folder{
			ID:        model.NewID(),
			Name:      name,
			ParentID:  parentID,
			CreatedAt: time.Now().UTC(),
		}
		return db.PutFolder(tx, created)
	})
	return created, err
}

// EnsureFolderWithParent returns the existing folder matching (name, parentID)
// or creates it (applying optional colorHex).
func (l *Library) EnsureFolderWithParent(name, parentID, colorHex string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var id string
	_ = l.DB.Update(func(tx *bbolt.Tx) error {
		folders, err := db.ListFolders(tx)
		if err != nil {
			return err
		}
		for _, f := range folders {
			if f.ParentID == parentID && strings.EqualFold(strings.TrimSpace(f.Name), name) {
				id = f.ID
				if colorHex != "" && f.Color == "" {
					f.Color = colorHex
					_ = db.PutFolder(tx, f)
				}
				return nil
			}
		}
		created := model.Folder{
			ID:        model.NewID(),
			Name:      name,
			ParentID:  parentID,
			Color:     strings.TrimSpace(colorHex),
			CreatedAt: time.Now().UTC(),
		}
		if err := db.PutFolder(tx, created); err != nil {
			return err
		}
		id = created.ID
		return nil
	})
	return id
}

// EnsureFolderPath creates or reuses a nested folder chain (e.g. ["UI", "Icons", "Dark"])
// starting at parentID and returns the leaf folder ID.
func (l *Library) EnsureFolderPath(parentID string, segments []string) string {
	cur := parentID
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		next := l.EnsureFolderWithParent(seg, cur, "")
		if next == "" {
			break
		}
		cur = next
	}
	return cur
}

// UpdateFolder renames a folder and/or reparents it, rejecting cycles and sibling duplicates.
func (l *Library) UpdateFolder(id, name, parentID string) (model.Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Folder{}, ErrEmptyName
	}
	if parentID == id {
		return model.Folder{}, ErrTagCycle
	}
	var updated model.Folder
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetFolder(tx, id)
		if err != nil {
			return err
		}
		folders, err := db.ListFolders(tx)
		if err != nil {
			return err
		}
		byID := make(map[string]model.Folder, len(folders))
		for _, f := range folders {
			byID[f.ID] = f
			if f.ID != id && f.ParentID == parentID && strings.EqualFold(f.Name, name) {
				return ErrDuplicateName
			}
		}
		if parentID != "" {
			if _, ok := byID[parentID]; !ok {
				return db.ErrNotFound
			}
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
		return db.PutFolder(tx, cur)
	})
	return updated, err
}

// SetFolderColor sets or clears the #RRGGBB display color on a folder.
func (l *Library) SetFolderColor(id, color string) error {
	color = strings.TrimSpace(color)
	return l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetFolder(tx, id)
		if err != nil {
			return err
		}
		cur.Color = color
		return db.PutFolder(tx, cur)
	})
}

// ReorderFolders persists the explicit sibling order for folders under parentID.
func (l *Library) ReorderFolders(parentID string, orderedIDs []string) error {
	return l.DB.Update(func(tx *bbolt.Tx) error {
		all, err := db.ListFolders(tx)
		if err != nil {
			return err
		}
		byID := make(map[string]model.Folder, len(all))
		for _, f := range all {
			byID[f.ID] = f
		}
		seen := make(map[string]bool, len(orderedIDs))
		seq := 10
		for _, id := range orderedIDs {
			f, ok := byID[id]
			if !ok || seen[id] {
				continue
			}
			if parentID != "" && parentID != f.ParentID {
				walk := parentID
				cyclic := false
				for walk != "" {
					if walk == id {
						cyclic = true
						break
					}
					walk = byID[walk].ParentID
				}
				if cyclic {
					continue
				}
				f.ParentID = parentID
			} else if parentID == "" && f.ParentID != "" {
				f.ParentID = ""
			}
			f.Order = seq
			seq += 10
			seen[id] = true
			if err := db.PutFolder(tx, f); err != nil {
				return err
			}
		}
		var rest []model.Folder
		for _, f := range all {
			if f.ParentID == parentID && !seen[f.ID] {
				rest = append(rest, f)
			}
		}
		sort.Slice(rest, func(i, j int) bool {
			return FolderLess(rest[i], rest[j])
		})
		for _, f := range rest {
			f.Order = seq
			seq += 10
			if err := db.PutFolder(tx, f); err != nil {
				return err
			}
		}
		return nil
	})
}

// FolderLess defines canonical display order for sibling folders: custom Order
// first, then case-insensitive Name.
func FolderLess(a, b model.Folder) bool {
	if a.Order != b.Order {
		if a.Order > 0 && b.Order > 0 {
			return a.Order < b.Order
		}
		if a.Order > 0 {
			return true
		}
		if b.Order > 0 {
			return false
		}
	}
	la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name)
	if la != lb {
		return la < lb
	}
	return a.ID < b.ID
}

// Folders lists every folder sorted by custom Order then Name.
func (l *Library) Folders() ([]model.Folder, error) {
	var folders []model.Folder
	err := l.DB.View(func(tx *bbolt.Tx) error {
		var err error
		folders, err = db.ListFolders(tx)
		return err
	})
	sort.Slice(folders, func(i, j int) bool {
		return FolderLess(folders[i], folders[j])
	})
	return folders, err
}

// DeleteFolder removes the folder and its links. Direct child folders are
// promoted to the deleted folder's parent; assets remain in the library.
func (l *Library) DeleteFolder(id string) error {
	if err := l.DB.Backup(); err != nil {
		return err
	}
	return l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetFolder(tx, id)
		if err != nil {
			return err
		}
		folders, err := db.ListFolders(tx)
		if err != nil {
			return err
		}
		for _, child := range folders {
			if child.ParentID == id {
				child.ParentID = cur.ParentID
				if err := db.PutFolder(tx, child); err != nil {
					return err
				}
			}
		}
		return db.DeleteFolder(tx, id)
	})
}

// DeleteFoldersBatch deletes multiple folders in a single atomic transaction.
// When includeSubfolders is true, every descendant subfolder under each selected
// folder is deleted as well; when false, surviving children are promoted to their
// nearest non-deleted ancestor. Original asset files on disk are never deleted.
func (l *Library) DeleteFoldersBatch(ids []string, includeSubfolders bool) (int, error) {
	if l == nil || len(ids) == 0 {
		return 0, nil
	}
	if err := l.DB.Backup(); err != nil {
		return 0, err
	}
	var deleted int
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		folders, err := db.ListFolders(tx)
		if err != nil {
			return err
		}
		byID := make(map[string]model.Folder, len(folders))
		children := make(map[string][]string, len(folders))
		for _, f := range folders {
			byID[f.ID] = f
			children[f.ParentID] = append(children[f.ParentID], f.ID)
		}

		toDelete := make(map[string]bool, len(ids))
		var markSubtree func(fid string)
		markSubtree = func(fid string) {
			if toDelete[fid] {
				return
			}
			if _, ok := byID[fid]; !ok {
				return
			}
			toDelete[fid] = true
			for _, ch := range children[fid] {
				markSubtree(ch)
			}
		}
		for _, id := range ids {
			if _, ok := byID[id]; !ok {
				continue
			}
			if includeSubfolders {
				markSubtree(id)
			} else {
				toDelete[id] = true
			}
		}
		if len(toDelete) == 0 {
			return nil
		}

		// Promote surviving children whose parent is being deleted.
		if !includeSubfolders {
			for _, f := range folders {
				if toDelete[f.ID] || f.ParentID == "" || !toDelete[f.ParentID] {
					continue
				}
				walk := f.ParentID
				for walk != "" && toDelete[walk] {
					walk = byID[walk].ParentID
				}
				f.ParentID = walk
				if err := db.PutFolder(tx, f); err != nil {
					return err
				}
			}
		}

		for id := range toDelete {
			if err := db.DeleteFolder(tx, id); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})
	return deleted, err
}

// SetAssetFolder adds or removes one folder membership on an asset.
func (l *Library) SetAssetFolder(assetID, folderID string, on bool) error {
	return l.DB.Update(func(tx *bbolt.Tx) error {
		if _, err := db.GetAsset(tx, assetID); err != nil {
			return err
		}
		if _, err := db.GetFolder(tx, folderID); err != nil {
			return err
		}
		if on {
			return db.LinkAssetFolder(tx, assetID, folderID)
		}
		return db.UnlinkAssetFolder(tx, assetID, folderID)
	})
}

// AssetFolders lists folders containing assetID.
func (l *Library) AssetFolders(assetID string) ([]model.Folder, error) {
	var folders []model.Folder
	err := l.DB.View(func(tx *bbolt.Tx) error {
		ids, err := db.FoldersOfAsset(tx, assetID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			f, err := db.GetFolder(tx, id)
			if err != nil {
				return err
			}
			folders = append(folders, f)
		}
		return nil
	})
	return folders, err
}

func linkFolders(tx *bbolt.Tx, assetID string, folderIDs []string) error {
	for _, id := range folderIDs {
		if id == "" {
			continue
		}
		if _, err := db.GetFolder(tx, id); err != nil {
			return err
		}
		if err := db.LinkAssetFolder(tx, assetID, id); err != nil {
			return err
		}
	}
	return nil
}
