package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

// ErrRenameConflict means one or more target filenames collide or are invalid.
var ErrRenameConflict = errors.New("raglex: rename conflict")

// RenameSpec defines a composable batch-rename pipeline.
type RenameSpec struct {
	// Template overrides the base filename stem when non-empty.
	// Supported tokens: {name}, {ext}, {index}, {index:03}, {date}, {w}x{h}, {tag}, {rating}.
	Template        string
	Find            string
	Replace         string
	UseRegex        bool
	CaseInsensitive bool
	Prefix          string
	Suffix          string
	CaseMode        string // "", "lower", "upper", "title"
	StartIndex      int    // defaults to 1 when <= 0
	RenameDiskFile  bool   // rename library-owned (copy/move) files on disk
	IncludeExternal bool   // also rename index/symlink files on disk when RenameDiskFile is true
}

// RenamePreview is one row of a live batch-rename dry run.
type RenamePreview struct {
	AssetID        string
	StorageMode    string
	OldName        string
	NewName        string
	OldPath        string
	NewPath        string
	WillRenameDisk bool
	Changed        bool
	Conflict       string
}

var indexTokenRe = regexp.MustCompile(`\{index(?::0?(\d+))?\}`)

// PreviewBatchRename computes target filenames and checks for conflicts without
// modifying the database or disk.
func (l *Library) PreviewBatchRename(assetIDs []string, spec RenameSpec) ([]RenamePreview, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}
	startIdx := spec.StartIndex
	if startIdx <= 0 {
		startIdx = 1
	}

	var findRe *regexp.Regexp
	if spec.Find != "" {
		if spec.UseRegex {
			pattern := spec.Find
			if spec.CaseInsensitive {
				pattern = "(?i)" + pattern
			}
			var err error
			findRe, err = regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex: %w", err)
			}
		} else if spec.CaseInsensitive {
			var err error
			findRe, err = regexp.Compile("(?i)" + regexp.QuoteMeta(spec.Find))
			if err != nil {
				return nil, err
			}
		}
	}

	assets := make([]model.Asset, 0, len(assetIDs))
	firstTag := make(map[string]string, len(assetIDs))
	inBatchPaths := make(map[string]bool, len(assetIDs))

	err := l.DB.View(func(tx *bbolt.Tx) error {
		tagNames := map[string]string{}
		if tags, err := db.ListTags(tx); err == nil {
			for _, tg := range tags {
				tagNames[tg.ID] = tg.Name
			}
		}
		for _, id := range assetIDs {
			a, err := db.GetAsset(tx, id)
			if err != nil {
				return err
			}
			assets = append(assets, a)
			if a.Path != "" {
				inBatchPaths[normPathKey(a.Path)] = true
			}
			if tids, err := db.TagsOfAsset(tx, id); err == nil && len(tids) > 0 {
				firstTag[id] = tagNames[tids[0]]
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]RenamePreview, 0, len(assets))
	seenDiskTargets := make(map[string]int, len(assets))

	for i, a := range assets {
		oldName := a.Label()
		ext := filepath.Ext(oldName)
		if ext == "" && a.Path != "" {
			ext = filepath.Ext(a.Path)
		}
		stem := strings.TrimSuffix(oldName, ext)
		extNoDot := strings.TrimPrefix(ext, ".")

		newStem := stem
		if strings.TrimSpace(spec.Template) != "" {
			newStem = expandRenameTemplate(spec.Template, a, stem, extNoDot, firstTag[a.ID], startIdx+i)
		}

		if spec.Find != "" {
			if findRe != nil {
				newStem = findRe.ReplaceAllString(newStem, spec.Replace)
			} else {
				newStem = strings.ReplaceAll(newStem, spec.Find, spec.Replace)
			}
		}

		if spec.Prefix != "" {
			newStem = spec.Prefix + newStem
		}
		if spec.Suffix != "" {
			newStem = newStem + spec.Suffix
		}

		switch spec.CaseMode {
		case "lower":
			newStem = strings.ToLower(newStem)
		case "upper":
			newStem = strings.ToUpper(newStem)
		case "title":
			newStem = titleWords(newStem)
		}

		newStem = sanitizeFileStem(newStem)
		newName := newStem
		if ext != "" && !strings.HasSuffix(strings.ToLower(newName), strings.ToLower(ext)) {
			newName = newStem + ext
		}

		canDisk := !a.Missing && a.Path != "" && spec.RenameDiskFile &&
			(a.StorageMode == model.StorageCopy || a.StorageMode == model.StorageMove || spec.IncludeExternal)

		newPath := a.Path
		if canDisk {
			newPath = filepath.Join(filepath.Dir(a.Path), newName)
		}

		changed := newName != oldName || (canDisk && newPath != a.Path)
		row := RenamePreview{
			AssetID:        a.ID,
			StorageMode:    a.StorageMode,
			OldName:        oldName,
			NewName:        newName,
			OldPath:        a.Path,
			NewPath:        newPath,
			WillRenameDisk: canDisk && newPath != a.Path,
			Changed:        changed,
		}

		if strings.TrimSpace(newStem) == "" {
			row.Conflict = "empty filename"
		} else if row.WillRenameDisk {
			key := normPathKey(newPath)
			if prevIdx, dup := seenDiskTargets[key]; dup {
				row.Conflict = "duplicate target filename"
				if out[prevIdx].Conflict == "" {
					out[prevIdx].Conflict = "duplicate target filename"
				}
			} else {
				seenDiskTargets[key] = i
				if !inBatchPaths[key] {
					if _, statErr := os.Stat(newPath); statErr == nil {
						row.Conflict = "file already exists on disk"
					}
				}
			}
		}

		out = append(out, row)
	}
	return out, nil
}

// ApplyBatchRename executes the rename preview atomically on disk (via two-phase
// temp names so circular/overlapping renames succeed safely) and in bbolt, and
// stores a RenameBatch for UndoLastBatchRename.
func (l *Library) ApplyBatchRename(assetIDs []string, spec RenameSpec) (model.RenameBatch, error) {
	preview, err := l.PreviewBatchRename(assetIDs, spec)
	if err != nil {
		return model.RenameBatch{}, err
	}
	var toApply []RenamePreview
	for _, row := range preview {
		if row.Conflict != "" {
			return model.RenameBatch{}, fmt.Errorf("%w: %s (%s)", ErrRenameConflict, row.NewName, row.Conflict)
		}
		if row.Changed {
			toApply = append(toApply, row)
		}
	}
	if len(toApply) == 0 {
		return model.RenameBatch{}, nil
	}

	// Phase 1 & 2 on disk for entries that move files.
	type diskMove struct {
		oldPath string
		tmpPath string
		newPath string
	}
	var moves []diskMove
	for _, row := range toApply {
		if !row.WillRenameDisk {
			continue
		}
		tmp := filepath.Join(filepath.Dir(row.OldPath), fmt.Sprintf(".raglex_ren_%s_%s", row.AssetID, filepath.Base(row.OldPath)))
		moves = append(moves, diskMove{oldPath: row.OldPath, tmpPath: tmp, newPath: row.NewPath})
	}

	if err := executeTwoPhaseDiskRename(moves); err != nil {
		return model.RenameBatch{}, err
	}

	batch := model.RenameBatch{
		ID:        model.NewID(),
		CreatedAt: time.Now().UTC(),
		Entries:   make([]model.RenameEntry, 0, len(toApply)),
	}

	err = l.DB.Update(func(tx *bbolt.Tx) error {
		for _, row := range toApply {
			cur, err := db.GetAsset(tx, row.AssetID)
			if err != nil {
				return err
			}
			entry := model.RenameEntry{
				AssetID:        cur.ID,
				OldDisplayName: cur.DisplayName,
				NewDisplayName: row.NewName,
				OldPath:        cur.Path,
				NewPath:        row.NewPath,
				RenamedDisk:    row.WillRenameDisk,
			}
			oldPath := cur.Path
			if row.WillRenameDisk {
				cur.Path = row.NewPath
				// When disk filename already matches NewName, keep DisplayName clean.
				cur.DisplayName = ""
				entry.NewDisplayName = ""
			} else {
				cur.DisplayName = row.NewName
			}
			if err := db.UpdateAssetPath(tx, cur, oldPath); err != nil {
				return err
			}
			batch.Entries = append(batch.Entries, entry)
		}
		return db.PutRenameBatch(tx, batch)
	})
	if err != nil {
		// Roll back disk moves if DB write failed.
		rev := make([]diskMove, 0, len(moves))
		for _, m := range moves {
			rev = append(rev, diskMove{oldPath: m.newPath, tmpPath: m.tmpPath, newPath: m.oldPath})
		}
		_ = executeTwoPhaseDiskRename(rev)
		return model.RenameBatch{}, err
	}
	return batch, nil
}

// HasRenameUndo reports whether there is a batch rename available to undo.
func (l *Library) HasRenameUndo() bool {
	var ok bool
	_ = l.DB.View(func(tx *bbolt.Tx) error {
		_, err := db.LastRenameBatch(tx)
		ok = err == nil
		return nil
	})
	return ok
}

// UndoLastBatchRename reverts the most recent batch rename on disk and in bbolt.
func (l *Library) UndoLastBatchRename() (int, error) {
	var batch model.RenameBatch
	err := l.DB.View(func(tx *bbolt.Tx) error {
		var err error
		batch, err = db.LastRenameBatch(tx)
		return err
	})
	if err != nil {
		return 0, err
	}

	type diskMove struct {
		oldPath string
		tmpPath string
		newPath string
	}
	var moves []diskMove
	for _, e := range batch.Entries {
		if !e.RenamedDisk || e.NewPath == e.OldPath {
			continue
		}
		if _, err := os.Stat(e.NewPath); err != nil {
			continue
		}
		tmp := filepath.Join(filepath.Dir(e.NewPath), fmt.Sprintf(".raglex_undo_%s_%s", e.AssetID, filepath.Base(e.NewPath)))
		moves = append(moves, diskMove{oldPath: e.NewPath, tmpPath: tmp, newPath: e.OldPath})
	}
	if err := executeTwoPhaseDiskRename(moves); err != nil {
		return 0, err
	}

	reverted := 0
	err = l.DB.Update(func(tx *bbolt.Tx) error {
		for _, e := range batch.Entries {
			cur, err := db.GetAsset(tx, e.AssetID)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					continue
				}
				return err
			}
			oldPath := cur.Path
			cur.DisplayName = e.OldDisplayName
			if e.RenamedDisk {
				cur.Path = e.OldPath
			}
			if err := db.UpdateAssetPath(tx, cur, oldPath); err != nil {
				return err
			}
			reverted++
		}
		return db.DeleteRenameBatch(tx, batch.ID)
	})
	return reverted, err
}

// RenameAsset renames a single asset (optionally also renaming the file on disk).
func (l *Library) RenameAsset(id, newName string, renameDisk bool) (model.Asset, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return model.Asset{}, ErrEmptyName
	}
	ext := filepath.Ext(newName)
	stem := strings.TrimSuffix(newName, ext)
	spec := RenameSpec{
		Template:        stem,
		RenameDiskFile:  renameDisk,
		IncludeExternal: renameDisk,
	}
	if _, err := l.ApplyBatchRename([]string{id}, spec); err != nil {
		return model.Asset{}, err
	}
	return l.Get(id)
}

// UpdateAssetMeta updates an asset's editable notes and source URL/title.
func (l *Library) UpdateAssetMeta(id, notes, sourceURL, sourceTitle string) (model.Asset, error) {
	var updated model.Asset
	err := l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetAsset(tx, id)
		if err != nil {
			return err
		}
		cur.Notes = strings.TrimSpace(notes)
		cur.SourceURL = strings.TrimSpace(sourceURL)
		cur.SourceTitle = strings.TrimSpace(sourceTitle)
		updated = cur
		return db.PutAsset(tx, cur)
	})
	return updated, err
}

// ImportBytes imports raw bytes (e.g. from clipboard paste or browser extension)
// as a library-owned copy, deduplicating by SHA-256 and optionally attaching
// source metadata and tags.
func (l *Library) ImportBytes(filename string, data []byte, tagIDs []string, sourceURL, sourceTitle string) (model.Asset, bool, error) {
	if len(data) == 0 {
		return model.Asset{}, false, errors.New("raglex: empty payload")
	}
	name := sanitizeFileStem(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	ext := filepath.Ext(filename)
	if name == "" {
		name = "clip_" + time.Now().Format("20060102_150405")
	}
	if ext == "" {
		ext = ".png"
	}
	tmpDir, err := os.MkdirTemp("", "raglex-clip-*")
	if err != nil {
		return model.Asset{}, false, err
	}
	defer os.RemoveAll(tmpDir)
	tmpFile := filepath.Join(tmpDir, name+ext)
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return model.Asset{}, false, err
	}
	item := l.importOne(tmpFile, model.StorageCopy, tagIDs)
	if item.Err != nil || item.AssetID == "" {
		if item.Err != nil {
			return model.Asset{}, false, item.Err
		}
		return model.Asset{}, false, errors.New("raglex: import failed")
	}
	if sourceURL != "" || sourceTitle != "" {
		_, _ = l.UpdateAssetMeta(item.AssetID, "", sourceURL, sourceTitle)
	}
	asset, err := l.Get(item.AssetID)
	return asset, item.Status == "added", err
}

func executeTwoPhaseDiskRename[T interface{ ~struct{ oldPath, tmpPath, newPath string } }](moves []T) error {
	type mv struct{ oldPath, tmpPath, newPath string }
	list := make([]mv, len(moves))
	for i, m := range moves {
		list[i] = mv(m)
	}
	staged := make([]mv, 0, len(list))
	for _, m := range list {
		if err := os.Rename(m.oldPath, m.tmpPath); err != nil {
			for j := len(staged) - 1; j >= 0; j-- {
				_ = os.Rename(staged[j].tmpPath, staged[j].oldPath)
			}
			return err
		}
		staged = append(staged, m)
	}
	completed := make([]mv, 0, len(staged))
	for _, m := range staged {
		if err := os.Rename(m.tmpPath, m.newPath); err != nil {
			for j := len(completed) - 1; j >= 0; j-- {
				_ = os.Rename(completed[j].newPath, completed[j].oldPath)
			}
			for j := len(completed); j < len(staged); j++ {
				_ = os.Rename(staged[j].tmpPath, staged[j].oldPath)
			}
			return err
		}
		completed = append(completed, m)
	}
	return nil
}

func expandRenameTemplate(tmpl string, a model.Asset, stem, extNoDot, tag string, idx int) string {
	out := indexTokenRe.ReplaceAllStringFunc(tmpl, func(tok string) string {
		m := indexTokenRe.FindStringSubmatch(tok)
		pad := 0
		if len(m) > 1 && m[1] != "" {
			pad, _ = strconv.Atoi(m[1])
		}
		if pad > 0 {
			return fmt.Sprintf("%0*d", pad, idx)
		}
		return strconv.Itoa(idx)
	})
	date := a.CreatedAt
	if date.IsZero() {
		date = time.Now()
	}
	dims := ""
	if a.Width > 0 && a.Height > 0 {
		dims = fmt.Sprintf("%dx%d", a.Width, a.Height)
	}
	repl := strings.NewReplacer(
		"{name}", stem,
		"{ext}", extNoDot,
		"{date}", date.Format("20060102"),
		"{w}x{h}", dims,
		"{w}", strconv.Itoa(a.Width),
		"{h}", strconv.Itoa(a.Height),
		"{tag}", tag,
		"{rating}", strconv.Itoa(a.Rating),
	)
	return repl.Replace(out)
}

func sanitizeFileStem(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			b.WriteRune('_')
		default:
			if r >= 0x20 {
				b.WriteRune(r)
			}
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.TrimRight(out, ".")
	return out
}

func titleWords(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	capNext := true
	for _, r := range s {
		if unicode.IsSpace(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			capNext = true
			continue
		}
		if capNext {
			b.WriteRune(unicode.ToTitle(r))
			capNext = false
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func normPathKey(p string) string {
	return strings.ToLower(filepath.Clean(p))
}
