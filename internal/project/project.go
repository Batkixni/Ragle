// Package project stores projects, the assets they reference, boards, and placements.
package project

import (
	"errors"
	"time"

	"go.etcd.io/bbolt"

	"ragle/internal/db"
	"ragle/internal/model"
)

// ErrNeedTag means a new project was given no tag.
var ErrNeedTag = errors.New("ragle: a project needs a tag")

// Store uses the library database.
type Store struct {
	DB *db.DB
}

// New wraps an open database.
func New(d *db.DB) *Store { return &Store{DB: d} }

// Create adds a project under at least one existing tag.
func (s *Store) Create(name, description string, tagIDs []string) (model.Project, error) {
	if len(tagIDs) == 0 {
		return model.Project{}, ErrNeedTag
	}
	p := model.Project{
		ID:          model.NewID(),
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().UTC(),
	}
	err := s.DB.Update(func(tx *bbolt.Tx) error {
		for _, id := range tagIDs {
			if _, err := db.GetTag(tx, id); err != nil {
				return err
			}
		}
		if err := db.PutProject(tx, p); err != nil {
			return err
		}
		for _, id := range tagIDs {
			if err := db.LinkProjectTag(tx, p.ID, id); err != nil {
				return err
			}
		}
		return nil
	})
	return p, err
}

// List returns every project.
func (s *Store) List() ([]model.Project, error) {
	var out []model.Project
	err := s.DB.View(func(tx *bbolt.Tx) error {
		var err error
		out, err = db.ListProjects(tx)
		return err
	})
	return out, err
}

// Update edits a project's name and description.
func (s *Store) Update(projectID, name, description string) (model.Project, error) {
	var updated model.Project
	err := s.DB.Update(func(tx *bbolt.Tx) error {
		p, err := db.GetProject(tx, projectID)
		if err != nil {
			return err
		}
		if name != "" {
			p.Name = name
		}
		p.Description = description
		updated = p
		return db.PutProject(tx, p)
	})
	return updated, err
}

// UsageOfAsset returns the projects referencing assetID and total board placement count.
func (s *Store) UsageOfAsset(assetID string) ([]model.Project, int, error) {
	var projs []model.Project
	var placements int
	err := s.DB.View(func(tx *bbolt.Tx) error {
		pids, err := db.ProjectsOfAsset(tx, assetID)
		if err != nil {
			return err
		}
		for _, id := range pids {
			p, err := db.GetProject(tx, id)
			if err != nil {
				continue
			}
			projs = append(projs, p)
		}
		pl, err := db.PlacementsOfAsset(tx, assetID)
		if err != nil {
			return err
		}
		placements = len(pl)
		return nil
	})
	return projs, placements, err
}

// ListByTag returns projects linked to tagID.
func (s *Store) ListByTag(tagID string) ([]model.Project, error) {
	var out []model.Project
	err := s.DB.View(func(tx *bbolt.Tx) error {
		ids, err := db.ProjectsOfTag(tx, tagID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			p, err := db.GetProject(tx, id)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

// TagIDs lists a project's tags.
func (s *Store) TagIDs(projectID string) ([]string, error) {
	var ids []string
	err := s.DB.View(func(tx *bbolt.Tx) error {
		var err error
		ids, err = db.TagsOfProject(tx, projectID)
		return err
	})
	return ids, err
}

// SetTag adds or removes one tag link. Removing the last tag is allowed;
// the project stays reachable from the full list.
func (s *Store) SetTag(projectID, tagID string, on bool) error {
	return s.DB.Update(func(tx *bbolt.Tx) error {
		if _, err := db.GetProject(tx, projectID); err != nil {
			return err
		}
		if on {
			if _, err := db.GetTag(tx, tagID); err != nil {
				return err
			}
			return db.LinkProjectTag(tx, projectID, tagID)
		}
		return db.UnlinkProjectTag(tx, projectID, tagID)
	})
}

// Delete removes the project, its boards, and its placements.
// Library files stay.
func (s *Store) Delete(projectID string) error {
	if err := s.DB.Backup(); err != nil {
		return err
	}
	return s.DB.Update(func(tx *bbolt.Tx) error { return db.DeleteProject(tx, projectID) })
}

// Reference records that a project uses an asset. Repeating it changes nothing.
func (s *Store) Reference(projectID, assetID string) error {
	return s.DB.Update(func(tx *bbolt.Tx) error {
		if _, err := db.GetProject(tx, projectID); err != nil {
			return err
		}
		if _, err := db.GetAsset(tx, assetID); err != nil {
			return err
		}
		return db.LinkProjectAsset(tx, projectID, assetID)
	})
}

// Unreference removes the reference and every placement of that asset on
// this project's boards. The library file stays, as do other projects.
func (s *Store) Unreference(projectID, assetID string) error {
	return s.DB.Update(func(tx *bbolt.Tx) error {
		boards, err := db.BoardsOfProject(tx, projectID)
		if err != nil {
			return err
		}
		for _, boardID := range boards {
			items, err := db.LoadBoard(tx, boardID)
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Kind == model.KindImage && item.AssetID == assetID {
					if err := db.DeletePlacement(tx, item.ID); err != nil {
						return err
					}
				}
			}
		}
		return db.UnlinkProjectAsset(tx, projectID, assetID)
	})
}

// Assets lists assets referenced by a project.
func (s *Store) Assets(projectID string) ([]model.Asset, error) {
	var out []model.Asset
	err := s.DB.View(func(tx *bbolt.Tx) error {
		ids, err := db.AssetsOfProject(tx, projectID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			a, err := db.GetAsset(tx, id)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return nil
	})
	return out, err
}

// CreateBoard adds a board to a project.
func (s *Store) CreateBoard(projectID, name string) (model.Board, error) {
	b := model.Board{
		ID:        model.NewID(),
		ProjectID: projectID,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	err := s.DB.Update(func(tx *bbolt.Tx) error {
		if _, err := db.GetProject(tx, projectID); err != nil {
			return err
		}
		return db.PutBoard(tx, b)
	})
	return b, err
}

// CreateStandaloneBoard creates a board outside of any project (ProjectID == "").
func (s *Store) CreateStandaloneBoard(name string) (model.Board, error) {
	b := model.Board{
		ID:        model.NewID(),
		ProjectID: "",
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	err := s.DB.Update(func(tx *bbolt.Tx) error {
		return db.PutBoard(tx, b)
	})
	return b, err
}

// StandaloneBoards lists every board not attached to a project.
func (s *Store) StandaloneBoards() ([]model.Board, error) {
	var out []model.Board
	err := s.DB.View(func(tx *bbolt.Tx) error {
		ids, err := db.StandaloneBoards(tx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			b, err := db.GetBoard(tx, id)
			if err != nil {
				return err
			}
			out = append(out, b)
		}
		return nil
	})
	return out, err
}

// GetBoard fetches one board by ID.
func (s *Store) GetBoard(boardID string) (model.Board, error) {
	var b model.Board
	err := s.DB.View(func(tx *bbolt.Tx) error {
		var err error
		b, err = db.GetBoard(tx, boardID)
		return err
	})
	return b, err
}

// RenameBoard renames a standalone or project board.
func (s *Store) RenameBoard(boardID, name string) (model.Board, error) {
	var updated model.Board
	err := s.DB.Update(func(tx *bbolt.Tx) error {
		b, err := db.GetBoard(tx, boardID)
		if err != nil {
			return err
		}
		if name != "" {
			b.Name = name
		}
		updated = b
		return db.PutBoard(tx, b)
	})
	return updated, err
}

// MoveBoardToProject attaches a standalone board (or moves an existing board)
// to projectID. Passing empty projectID detaches it into a standalone board.
// When attaching to a project, any image assets placed on the board are also
// referenced in that project.
func (s *Store) MoveBoardToProject(boardID, projectID string) error {
	return s.DB.Update(func(tx *bbolt.Tx) error {
		if projectID != "" {
			if _, err := db.GetProject(tx, projectID); err != nil {
				return err
			}
		}
		if err := db.MoveBoardProject(tx, boardID, projectID); err != nil {
			return err
		}
		if projectID != "" {
			items, err := db.LoadBoard(tx, boardID)
			if err != nil {
				return err
			}
			for _, it := range items {
				if it.Kind == model.KindImage && it.AssetID != "" {
					if _, err := db.GetAsset(tx, it.AssetID); err == nil {
						_ = db.LinkProjectAsset(tx, projectID, it.AssetID)
					}
				}
			}
		}
		return nil
	})
}

// AddAssetsToBoard appends image placements for assetIDs in a neat grid on boardID.
func (s *Store) AddAssetsToBoard(boardID string, assetIDs []string) (int, error) {
	if len(assetIDs) == 0 {
		return 0, nil
	}
	added := 0
	err := s.DB.Update(func(tx *bbolt.Tx) error {
		b, err := db.GetBoard(tx, boardID)
		if err != nil {
			return err
		}
		existing, err := db.LoadBoard(tx, boardID)
		if err != nil {
			return err
		}
		maxZ := 0
		baseX := 220.0
		baseY := 200.0
		if len(existing) > 0 {
			maxY := 0.0
			for _, it := range existing {
				if it.ZIndex > maxZ {
					maxZ = it.ZIndex
				}
				bottom := it.Y + it.Height*it.ScaleOrOne()/2
				if bottom > maxY {
					maxY = bottom
				}
			}
			baseY = maxY + 180
		}
		cols := 4
		const cellW = 340.0
		const cellH = 260.0
		for i, aid := range assetIDs {
			asset, err := db.GetAsset(tx, aid)
			if err != nil {
				continue
			}
			w, h := 300.0, 220.0
			if asset.Width > 0 && asset.Height > 0 {
				if asset.Width >= asset.Height {
					w = 300
					h = 300 * float64(asset.Height) / float64(asset.Width)
				} else {
					h = 300
					w = 300 * float64(asset.Width) / float64(asset.Height)
				}
			}
			maxZ++
			col := i % cols
			row := i / cols
			pl := model.Placement{
				ID:      model.NewID(),
				BoardID: boardID,
				Kind:    model.KindImage,
				AssetID: asset.ID,
				X:       baseX + float64(col)*cellW,
				Y:       baseY + float64(row)*cellH,
				Width:   w,
				Height:  h,
				Scale:   1,
				ZIndex:  maxZ,
			}
			if err := db.PutPlacement(tx, pl); err != nil {
				return err
			}
			if b.ProjectID != "" {
				_ = db.LinkProjectAsset(tx, b.ProjectID, asset.ID)
			}
			added++
		}
		return nil
	})
	return added, err
}

// Boards lists a project's boards.
func (s *Store) Boards(projectID string) ([]model.Board, error) {
	var out []model.Board
	err := s.DB.View(func(tx *bbolt.Tx) error {
		ids, err := db.BoardsOfProject(tx, projectID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			b, err := db.GetBoard(tx, id)
			if err != nil {
				return err
			}
			out = append(out, b)
		}
		return nil
	})
	return out, err
}

// DeleteBoard removes a board and its placements.
func (s *Store) DeleteBoard(boardID string) error {
	if err := s.DB.Backup(); err != nil {
		return err
	}
	return s.DB.Update(func(tx *bbolt.Tx) error { return db.DeleteBoard(tx, boardID) })
}

// LoadPlacements reads a board.
func (s *Store) LoadPlacements(boardID string) ([]model.Placement, error) {
	var items []model.Placement
	err := s.DB.View(func(tx *bbolt.Tx) error {
		var err error
		items, err = db.LoadBoard(tx, boardID)
		return err
	})
	return items, err
}

// SavePlacements replaces a board's placements in one transaction.
func (s *Store) SavePlacements(boardID string, items []model.Placement) error {
	items = normalize(items)
	return s.DB.Update(func(tx *bbolt.Tx) error {
		if _, err := db.GetBoard(tx, boardID); err != nil {
			return err
		}
		return db.ReplaceBoardPlacements(tx, boardID, items)
	})
}

func normalize(items []model.Placement) []model.Placement {
	ids := make(map[string]bool, len(items))
	for _, item := range items {
		if item.ID != "" {
			ids[item.ID] = true
		}
	}
	out := make([]model.Placement, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if item.ParentID != "" && !ids[item.ParentID] {
			item.ParentID = ""
		}
		if item.Scale == 0 {
			item.Scale = 1
		}
		out = append(out, item)
	}
	return out
}
