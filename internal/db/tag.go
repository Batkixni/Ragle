package db

import (
	"go.etcd.io/bbolt"

	"raglex/internal/model"
)

// PutTag writes a tag. Name uniqueness is enforced by the caller.
func PutTag(tx *bbolt.Tx, t model.Tag) error {
	raw, err := encode(t)
	if err != nil {
		return err
	}
	return tx.Bucket(BucketTags).Put([]byte(t.ID), raw)
}

// GetTag reads one tag.
func GetTag(tx *bbolt.Tx, id string) (model.Tag, error) {
	var t model.Tag
	raw := tx.Bucket(BucketTags).Get([]byte(id))
	if raw == nil {
		return t, ErrNotFound
	}
	err := decode(raw, &t)
	return t, err
}

// ListTags scans the tags bucket.
func ListTags(tx *bbolt.Tx) ([]model.Tag, error) {
	var out []model.Tag
	err := tx.Bucket(BucketTags).ForEach(func(_, v []byte) error {
		var t model.Tag
		if err := decode(v, &t); err != nil {
			return err
		}
		out = append(out, t)
		return nil
	})
	return out, err
}

// LinkAssetTag records that assetID has tagID, in both directions.
func LinkAssetTag(tx *bbolt.Tx, assetID, tagID string) error {
	if err := putEmpty(tx.Bucket(BucketAssetTags), indexKey(assetID, tagID)); err != nil {
		return err
	}
	return putEmpty(tx.Bucket(BucketTagAssets), indexKey(tagID, assetID))
}

// UnlinkAssetTag removes both directions.
func UnlinkAssetTag(tx *bbolt.Tx, assetID, tagID string) error {
	if err := tx.Bucket(BucketAssetTags).Delete(indexKey(assetID, tagID)); err != nil {
		return err
	}
	return tx.Bucket(BucketTagAssets).Delete(indexKey(tagID, assetID))
}

// TagsOfAsset lists tag IDs on an asset.
func TagsOfAsset(tx *bbolt.Tx, assetID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketAssetTags), assetID+"/")
}

// AssetsOfTag lists asset IDs in a tag.
func AssetsOfTag(tx *bbolt.Tx, tagID string) ([]string, error) {
	return collectIDs(tx.Bucket(BucketTagAssets), tagID+"/")
}

// DeleteTag removes the tag and its links to assets and projects.
// Projects and assets themselves stay.
func DeleteTag(tx *bbolt.Tx, tagID string) error {
	assetIDs, err := AssetsOfTag(tx, tagID)
	if err != nil {
		return err
	}
	for _, assetID := range assetIDs {
		if err := UnlinkAssetTag(tx, assetID, tagID); err != nil {
			return err
		}
	}
	projectIDs, err := ProjectsOfTag(tx, tagID)
	if err != nil {
		return err
	}
	for _, projectID := range projectIDs {
		if err := UnlinkProjectTag(tx, projectID, tagID); err != nil {
			return err
		}
	}
	return tx.Bucket(BucketTags).Delete([]byte(tagID))
}
