package db

import (
	"go.etcd.io/bbolt"

	"raglex/internal/model"
)

// PutBoard writes a board and updates its project or standalone index.
func PutBoard(tx *bbolt.Tx, b model.Board) error {
	raw, err := encode(b)
	if err != nil {
		return err
	}
	if err := tx.Bucket(BucketBoards).Put([]byte(b.ID), raw); err != nil {
		return err
	}
	if b.ProjectID == "" {
		return putEmpty(tx.Bucket(BucketBoardsStandalone), []byte(b.ID))
	}
	return putEmpty(tx.Bucket(BucketBoardsByProject), indexKey(b.ProjectID, b.ID))
}

// StandaloneBoards lists board IDs not attached to any project.
func StandaloneBoards(tx *bbolt.Tx) ([]string, error) {
	b := tx.Bucket(BucketBoardsStandalone)
	if b == nil {
		return nil, nil
	}
	var ids []string
	err := b.ForEach(func(k, _ []byte) error {
		ids = append(ids, string(k))
		return nil
	})
	return ids, err
}

// MoveBoardProject re-assigns a board's ProjectID (empty string makes it standalone).
func MoveBoardProject(tx *bbolt.Tx, boardID, newProjectID string) error {
	b, err := GetBoard(tx, boardID)
	if err != nil {
		return err
	}
	if b.ProjectID == newProjectID {
		return nil
	}
	if b.ProjectID == "" {
		if err := tx.Bucket(BucketBoardsStandalone).Delete([]byte(b.ID)); err != nil {
			return err
		}
	} else {
		if err := tx.Bucket(BucketBoardsByProject).Delete(indexKey(b.ProjectID, b.ID)); err != nil {
			return err
		}
	}
	b.ProjectID = newProjectID
	return PutBoard(tx, b)
}

// GetBoard reads one board.
func GetBoard(tx *bbolt.Tx, id string) (model.Board, error) {
	var b model.Board
	raw := tx.Bucket(BucketBoards).Get([]byte(id))
	if raw == nil {
		return b, ErrNotFound
	}
	err := decode(raw, &b)
	return b, err
}

// BoardsOfProject lists board IDs in a project.
func BoardsOfProject(tx *bbolt.Tx, projectID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketBoardsByProject), projectID+"/")
}

// PutPlacement writes a placement and its board and asset indexes.
func PutPlacement(tx *bbolt.Tx, p model.Placement) error {
	raw, err := encode(p)
	if err != nil {
		return err
	}
	if err := tx.Bucket(BucketPlacements).Put([]byte(p.ID), raw); err != nil {
		return err
	}
	if err := putEmpty(tx.Bucket(BucketPlacementsByBoard), indexKey(p.BoardID, p.ID)); err != nil {
		return err
	}
	if p.AssetID == "" {
		return nil
	}
	return putEmpty(tx.Bucket(BucketPlacementsByAsset), indexKey(p.AssetID, p.ID))
}

// GetPlacement reads one placement.
func GetPlacement(tx *bbolt.Tx, id string) (model.Placement, error) {
	var p model.Placement
	raw := tx.Bucket(BucketPlacements).Get([]byte(id))
	if raw == nil {
		return p, ErrNotFound
	}
	err := decode(raw, &p)
	return p, err
}

// PlacementsOfBoard lists placement IDs on a board.
func PlacementsOfBoard(tx *bbolt.Tx, boardID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketPlacementsByBoard), boardID+"/")
}

// PlacementsOfAsset lists placements that reference an asset.
func PlacementsOfAsset(tx *bbolt.Tx, assetID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketPlacementsByAsset), assetID+"/")
}

// LoadBoard reads every placement on a board.
func LoadBoard(tx *bbolt.Tx, boardID string) ([]model.Placement, error) {
	ids, err := PlacementsOfBoard(tx, boardID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Placement, 0, len(ids))
	for _, id := range ids {
		p, err := GetPlacement(tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// DeletePlacement removes one placement and its indexes. It does not
// remove children; DeletePlacementTree does.
func DeletePlacement(tx *bbolt.Tx, id string) error {
	p, err := GetPlacement(tx, id)
	if err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}
	if err := tx.Bucket(BucketPlacementsByBoard).Delete(indexKey(p.BoardID, p.ID)); err != nil {
		return err
	}
	if p.AssetID != "" {
		if err := tx.Bucket(BucketPlacementsByAsset).Delete(indexKey(p.AssetID, p.ID)); err != nil {
			return err
		}
	}
	return tx.Bucket(BucketPlacements).Delete([]byte(id))
}

// DeletePlacementTree removes a placement and every descendant on its board.
func DeletePlacementTree(tx *bbolt.Tx, id string) error {
	root, err := GetPlacement(tx, id)
	if err != nil {
		return err
	}
	all, err := LoadBoard(tx, root.BoardID)
	if err != nil {
		return err
	}
	drop := map[string]bool{id: true}
	changed := true
	for changed {
		changed = false
		for _, p := range all {
			if p.ParentID != "" && drop[p.ParentID] && !drop[p.ID] {
				drop[p.ID] = true
				changed = true
			}
		}
	}
	for dropID := range drop {
		if err := DeletePlacement(tx, dropID); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceBoardPlacements rewrites every placement of a board in one
// transaction. Placements of other boards are left alone.
func ReplaceBoardPlacements(tx *bbolt.Tx, boardID string, items []model.Placement) error {
	ids, err := PlacementsOfBoard(tx, boardID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := DeletePlacement(tx, id); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		item.BoardID = boardID
		if err := PutPlacement(tx, item); err != nil {
			return err
		}
	}
	return nil
}

// DeleteBoard removes a board and its placements.
func DeleteBoard(tx *bbolt.Tx, boardID string) error {
	b, err := GetBoard(tx, boardID)
	if err != nil {
		return err
	}
	ids, err := PlacementsOfBoard(tx, boardID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := DeletePlacement(tx, id); err != nil {
			return err
		}
	}
	if b.ProjectID == "" {
		if err := tx.Bucket(BucketBoardsStandalone).Delete([]byte(b.ID)); err != nil {
			return err
		}
	} else {
		if err := tx.Bucket(BucketBoardsByProject).Delete(indexKey(b.ProjectID, b.ID)); err != nil {
			return err
		}
	}
	return tx.Bucket(BucketBoards).Delete([]byte(boardID))
}
