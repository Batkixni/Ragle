package db

import (
	"go.etcd.io/bbolt"

	"raglex/internal/model"
)

// PutProject writes a project record.
func PutProject(tx *bbolt.Tx, p model.Project) error {
	raw, err := encode(p)
	if err != nil {
		return err
	}
	return tx.Bucket(BucketProjects).Put([]byte(p.ID), raw)
}

// GetProject reads one project.
func GetProject(tx *bbolt.Tx, id string) (model.Project, error) {
	var p model.Project
	raw := tx.Bucket(BucketProjects).Get([]byte(id))
	if raw == nil {
		return p, ErrNotFound
	}
	err := decode(raw, &p)
	return p, err
}

// ListProjects scans the projects bucket.
func ListProjects(tx *bbolt.Tx) ([]model.Project, error) {
	var out []model.Project
	err := tx.Bucket(BucketProjects).ForEach(func(_, v []byte) error {
		var p model.Project
		if err := decode(v, &p); err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

// LinkProjectTag records that projectID belongs to tagID.
func LinkProjectTag(tx *bbolt.Tx, projectID, tagID string) error {
	if err := putEmpty(tx.Bucket(BucketProjectTags), indexKey(projectID, tagID)); err != nil {
		return err
	}
	return putEmpty(tx.Bucket(BucketTagProjects), indexKey(tagID, projectID))
}

// UnlinkProjectTag removes both directions.
func UnlinkProjectTag(tx *bbolt.Tx, projectID, tagID string) error {
	if err := tx.Bucket(BucketProjectTags).Delete(indexKey(projectID, tagID)); err != nil {
		return err
	}
	return tx.Bucket(BucketTagProjects).Delete(indexKey(tagID, projectID))
}

// TagsOfProject lists tag IDs on a project.
func TagsOfProject(tx *bbolt.Tx, projectID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketProjectTags), projectID+"/")
}

// ProjectsOfTag lists project IDs in a tag.
func ProjectsOfTag(tx *bbolt.Tx, tagID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketTagProjects), tagID+"/")
}

// LinkProjectAsset records a library reference on a project.
func LinkProjectAsset(tx *bbolt.Tx, projectID, assetID string) error {
	if err := putEmpty(tx.Bucket(BucketProjectAssets), indexKey(projectID, assetID)); err != nil {
		return err
	}
	return putEmpty(tx.Bucket(BucketAssetProjects), indexKey(assetID, projectID))
}

// UnlinkProjectAsset removes both directions. Placements are the caller's concern.
func UnlinkProjectAsset(tx *bbolt.Tx, projectID, assetID string) error {
	if err := tx.Bucket(BucketProjectAssets).Delete(indexKey(projectID, assetID)); err != nil {
		return err
	}
	return tx.Bucket(BucketAssetProjects).Delete(indexKey(assetID, projectID))
}

// AssetsOfProject lists asset IDs referenced by a project.
func AssetsOfProject(tx *bbolt.Tx, projectID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketProjectAssets), projectID+"/")
}

// ProjectsOfAsset lists projects that reference an asset.
func ProjectsOfAsset(tx *bbolt.Tx, assetID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketAssetProjects), assetID+"/")
}

// DeleteProject removes the project, its tag links, asset references,
// boards, and placements. Library assets stay.
func DeleteProject(tx *bbolt.Tx, projectID string) error {
	tagIDs, err := TagsOfProject(tx, projectID)
	if err != nil {
		return err
	}
	for _, tagID := range tagIDs {
		if err := UnlinkProjectTag(tx, projectID, tagID); err != nil {
			return err
		}
	}
	assetIDs, err := AssetsOfProject(tx, projectID)
	if err != nil {
		return err
	}
	for _, assetID := range assetIDs {
		if err := UnlinkProjectAsset(tx, projectID, assetID); err != nil {
			return err
		}
	}
	boardIDs, err := BoardsOfProject(tx, projectID)
	if err != nil {
		return err
	}
	for _, boardID := range boardIDs {
		if err := DeleteBoard(tx, boardID); err != nil {
			return err
		}
	}
	return tx.Bucket(BucketProjects).Delete([]byte(projectID))
}
