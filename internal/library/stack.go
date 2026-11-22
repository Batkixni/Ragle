package library

import (
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"ragle/internal/db"
	"ragle/internal/model"
)

// ErrStackTooSmall means fewer than 2 valid assets were supplied to form a stack.
var ErrStackTooSmall = errors.New("ragle: at least 2 assets are required to create a stack")

// ListStacks returns all asset stacks sorted newest first.
func (l *Library) ListStacks() ([]model.Stack, error) {
	var out []model.Stack
	err := l.DB.View(func(tx *bbolt.Tx) error {
		list, err := db.ListStacks(tx)
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// CreateStack groups assetIDs into a single collapsible version stack.
// If any asset already belongs to another stack, its existing members are
// cleanly detached (dissolving any source stack left with < 2 members).
func (l *Library) CreateStack(name string, assetIDs []string, coverID string) (model.Stack, error) {
	seen := make(map[string]bool, len(assetIDs))
	unique := make([]string, 0, len(assetIDs))
	for _, id := range assetIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if len(unique) < 2 {
		return model.Stack{}, ErrStackTooSmall
	}

	var created model.Stack
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		valid := make([]model.Asset, 0, len(unique))
		for _, id := range unique {
			a, err := db.GetAsset(tx, id)
			if err != nil {
				continue
			}
			valid = append(valid, a)
		}
		if len(valid) < 2 {
			return ErrStackTooSmall
		}

		// Detach from any previous stacks first.
		for _, a := range valid {
			if a.StackID != "" {
				if err := detachAssetFromStackTx(tx, a.ID, a.StackID); err != nil {
					return err
				}
			}
		}

		ids := make([]string, 0, len(valid))
		coverOK := false
		for _, a := range valid {
			ids = append(ids, a.ID)
			if a.ID == coverID {
				coverOK = true
			}
		}
		if !coverOK {
			coverID = ids[0]
		}
		stackName := strings.TrimSpace(name)
		if stackName == "" {
			stackName = versionStem(valid[0].Label())
			if stackName == "" {
				stackName = strings.TrimSuffix(valid[0].Label(), filepath.Ext(valid[0].Label()))
			}
		}

		created = model.Stack{
			ID:        model.NewID(),
			Name:      stackName,
			CoverID:   coverID,
			AssetIDs:  ids,
			CreatedAt: time.Now().UTC(),
		}
		if err := db.PutStack(tx, created); err != nil {
			return err
		}
		for _, a := range valid {
			a.StackID = created.ID
			if err := db.PutAsset(tx, a); err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

// AddToStack appends assets into an existing stack.
func (l *Library) AddToStack(stackID string, assetIDs []string) (model.Stack, error) {
	var updated model.Stack
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		st, err := db.GetStack(tx, stackID)
		if err != nil {
			return err
		}
		existing := make(map[string]bool, len(st.AssetIDs))
		for _, id := range st.AssetIDs {
			existing[id] = true
		}
		for _, id := range assetIDs {
			if id == "" || existing[id] {
				continue
			}
			a, err := db.GetAsset(tx, id)
			if err != nil {
				continue
			}
			if a.StackID != "" && a.StackID != stackID {
				if err := detachAssetFromStackTx(tx, a.ID, a.StackID); err != nil {
					return err
				}
			}
			a.StackID = stackID
			if err := db.PutAsset(tx, a); err != nil {
				return err
			}
			st.AssetIDs = append(st.AssetIDs, id)
			existing[id] = true
		}
		if err := db.PutStack(tx, st); err != nil {
			return err
		}
		updated = st
		return nil
	})
	return updated, err
}

// RemoveFromStack detaches assets from their stack(s). Any stack that drops
// below 2 assets is automatically dissolved so singletons stay unstacked.
func (l *Library) RemoveFromStack(assetIDs []string) error {
	return l.DB.Update(func(tx *bbolt.Tx) error {
		for _, id := range assetIDs {
			a, err := db.GetAsset(tx, id)
			if err != nil || a.StackID == "" {
				continue
			}
			oldStackID := a.StackID
			a.StackID = ""
			if err := db.PutAsset(tx, a); err != nil {
				return err
			}
			if err := detachAssetFromStackTx(tx, a.ID, oldStackID); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetStackCover changes which asset appears as the cover card of a stack.
func (l *Library) SetStackCover(stackID, assetID string) (model.Stack, error) {
	var updated model.Stack
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		st, err := db.GetStack(tx, stackID)
		if err != nil {
			return err
		}
		found := false
		for _, id := range st.AssetIDs {
			if id == assetID {
				found = true
				break
			}
		}
		if !found {
			return db.ErrNotFound
		}
		st.CoverID = assetID
		if err := db.PutStack(tx, st); err != nil {
			return err
		}
		updated = st
		return nil
	})
	return updated, err
}

// RenameStack updates a stack's display name.
func (l *Library) RenameStack(stackID, name string) (model.Stack, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Stack{}, ErrEmptyName
	}
	var updated model.Stack
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		st, err := db.GetStack(tx, stackID)
		if err != nil {
			return err
		}
		st.Name = name
		if err := db.PutStack(tx, st); err != nil {
			return err
		}
		updated = st
		return nil
	})
	return updated, err
}

// UngroupStack dissolves a stack and clears StackID on all its members.
func (l *Library) UngroupStack(stackID string) error {
	return l.DB.Update(func(tx *bbolt.Tx) error {
		st, err := db.GetStack(tx, stackID)
		if err != nil {
			return err
		}
		for _, id := range st.AssetIDs {
			if a, err := db.GetAsset(tx, id); err == nil && a.StackID == stackID {
				a.StackID = ""
				if err := db.PutAsset(tx, a); err != nil {
					return err
				}
			}
		}
		return db.DeleteStack(tx, stackID)
	})
}

func detachAssetFromStackTx(tx *bbolt.Tx, assetID, stackID string) error {
	st, err := db.GetStack(tx, stackID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	next := make([]string, 0, len(st.AssetIDs))
	for _, id := range st.AssetIDs {
		if id != assetID {
			next = append(next, id)
		}
	}
	if len(next) < 2 {
		// Dissolve remaining singleton so no 1-item stack lingers.
		for _, remID := range next {
			if a, err := db.GetAsset(tx, remID); err == nil && a.StackID == stackID {
				a.StackID = ""
				if err := db.PutAsset(tx, a); err != nil {
					return err
				}
			}
		}
		return db.DeleteStack(tx, stackID)
	}
	st.AssetIDs = next
	if st.CoverID == assetID {
		st.CoverID = next[0]
	}
	return db.PutStack(tx, st)
}

var (
	reVerSuffix   = regexp.MustCompile(`(?i)[\s._-]+(?:v\d+(?:\.\d+)?|ver\d+|rev\d+|r\d+|final\d*|draft\d*|wip\d*|edit\d*|copy\d*|alt\d*|angle\d*|view\d*|front|back|side|left|right|top|\d{1,3})$`)
	reParenSuffix = regexp.MustCompile(`\s*\(\d+\)$`)
)

// VersionStem extracts the canonical artwork stem by stripping trailing version
// suffixes like "_v1", "_v2", "-final", " (2)", "_01", "_front", etc.
func VersionStem(filename string) string {
	return versionStem(filename)
}

func versionStem(filename string) string {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if base == "" {
		return ""
	}
	prev := ""
	for prev != base {
		prev = base
		base = strings.TrimSpace(reParenSuffix.ReplaceAllString(base, ""))
		base = strings.TrimSpace(reVerSuffix.ReplaceAllString(base, ""))
	}
	return strings.Trim(base, " _-.")
}

// AutoStackByVersionName scans unstacked assets whose filenames share the same
// version stem (e.g. poster_v1.png, poster_v2.png, poster_final.png) and folds
// each group of >= 2 assets into a version stack. Returns count of stacks created.
func (l *Library) AutoStackByVersionName() (int, error) {
	assets, err := l.List(Filter{})
	if err != nil {
		return 0, err
	}
	groups := map[string][]model.Asset{}
	order := []string{}
	for _, a := range assets {
		if a.StackID != "" {
			continue
		}
		label := a.Label()
		stem := versionStem(label)
		rawBase := strings.TrimSpace(strings.TrimSuffix(filepath.Base(label), filepath.Ext(label)))
		// Only group items that actually had a version/angle suffix or match another item with one.
		if stem == "" {
			continue
		}
		key := strings.ToLower(stem)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], a)
		_ = rawBase
	}
	created := 0
	for _, key := range order {
		items := groups[key]
		if len(items) < 2 {
			continue
		}
		// Sort oldest -> newest / alphabetical so the latest or "final" version becomes cover!
		sort.Slice(items, func(i, j int) bool {
			return strings.ToLower(items[i].Label()) < strings.ToLower(items[j].Label())
		})
		ids := make([]string, len(items))
		coverID := items[len(items)-1].ID
		for i, it := range items {
			ids[i] = it.ID
			if strings.Contains(strings.ToLower(it.Label()), "final") {
				coverID = it.ID
			}
		}
		stem := versionStem(items[0].Label())
		if _, err := l.CreateStack(stem, ids, coverID); err == nil {
			created++
		}
	}
	return created, nil
}
