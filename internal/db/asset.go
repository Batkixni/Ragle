package db

import (
	"go.etcd.io/bbolt"

	"ragle/internal/model"
)

// PutAsset writes the asset and its hash and path indexes. The path index
// is omitted while the asset is missing, so a moved file can take the key.
func PutAsset(tx *bbolt.Tx, a model.Asset) error {
	raw, err := encode(a)
	if err != nil {
		return err
	}
	if err := tx.Bucket(BucketAssets).Put([]byte(a.ID), raw); err != nil {
		return err
	}
	if err := tx.Bucket(BucketAssetsByHash).Put([]byte(a.Hash), []byte(a.ID)); err != nil {
		return err
	}
	pathBucket := tx.Bucket(BucketAssetsByPath)
	if a.Missing || a.Path == "" {
		return pathBucket.Delete([]byte(a.Path))
	}
	return pathBucket.Put([]byte(a.Path), []byte(a.ID))
}

// GetAsset reads one asset.
func GetAsset(tx *bbolt.Tx, id string) (model.Asset, error) {
	var a model.Asset
	raw := tx.Bucket(BucketAssets).Get([]byte(id))
	if raw == nil {
		return a, ErrNotFound
	}
	err := decode(raw, &a)
	return a, err
}

// AssetIDByHash returns the asset stored for hash.
func AssetIDByHash(tx *bbolt.Tx, hash string) (string, bool) {
	v := tx.Bucket(BucketAssetsByHash).Get([]byte(hash))
	if v == nil {
		return "", false
	}
	return string(v), true
}

// AssetIDByPath returns the asset stored at path.
func AssetIDByPath(tx *bbolt.Tx, path string) (string, bool) {
	v := tx.Bucket(BucketAssetsByPath).Get([]byte(path))
	if v == nil {
		return "", false
	}
	return string(v), true
}

// ListAssets scans the assets bucket.
func ListAssets(tx *bbolt.Tx) ([]model.Asset, error) {
	var out []model.Asset
	err := tx.Bucket(BucketAssets).ForEach(func(_, v []byte) error {
		var a model.Asset
		if err := decode(v, &a); err != nil {
			return err
		}
		out = append(out, a)
		return nil
	})
	return out, err
}

// ListMissing returns assets whose file could not be found.
func ListMissing(tx *bbolt.Tx) ([]model.Asset, error) {
	all, err := ListAssets(tx)
	if err != nil {
		return nil, err
	}
	var out []model.Asset
	for _, a := range all {
		if a.Missing {
			out = append(out, a)
		}
	}
	return out, nil
}

// DeleteAssetRecords removes the asset, its indexes, tag links, project
// links, and placements that point at it. It does not touch the file.
func DeleteAssetRecords(tx *bbolt.Tx, id string) error {
	a, err := GetAsset(tx, id)
	if err != nil {
		return err
	}
	tagIDs, err := TagsOfAsset(tx, id)
	if err != nil {
		return err
	}
	for _, tagID := range tagIDs {
		if err := UnlinkAssetTag(tx, id, tagID); err != nil {
			return err
		}
	}
	projectIDs, err := ProjectsOfAsset(tx, id)
	if err != nil {
		return err
	}
	for _, projectID := range projectIDs {
		if err := UnlinkProjectAsset(tx, projectID, id); err != nil {
			return err
		}
	}
	placementIDs, err := PlacementsOfAsset(tx, id)
	if err != nil {
		return err
	}
	for _, placementID := range placementIDs {
		if err := DeletePlacement(tx, placementID); err != nil {
			return err
		}
	}
	if err := tx.Bucket(BucketAssetsByHash).Delete([]byte(a.Hash)); err != nil {
		return err
	}
	if a.Path != "" {
		if err := tx.Bucket(BucketAssetsByPath).Delete([]byte(a.Path)); err != nil {
			return err
		}
	}
	return tx.Bucket(BucketAssets).Delete([]byte(id))
}

// UpdateAssetPath atomically updates an asset's record and swaps its path index
// entry when oldPath != a.Path.
func UpdateAssetPath(tx *bbolt.Tx, a model.Asset, oldPath string) error {
	if oldPath != "" && oldPath != a.Path {
		if err := tx.Bucket(BucketAssetsByPath).Delete([]byte(oldPath)); err != nil {
			return err
		}
	}
	return PutAsset(tx, a)
}

// PutRenameBatch appends a batch rename entry to rename_history.
func PutRenameBatch(tx *bbolt.Tx, batch model.RenameBatch) error {
	raw, err := encode(batch)
	if err != nil {
		return err
	}
	return tx.Bucket(BucketRenameHistory).Put([]byte(batch.ID), raw)
}

// LastRenameBatch returns the most recent batch rename (ULID keys sort by time).
func LastRenameBatch(tx *bbolt.Tx) (model.RenameBatch, error) {
	var b model.RenameBatch
	c := tx.Bucket(BucketRenameHistory).Cursor()
	k, v := c.Last()
	if k == nil {
		return b, ErrNotFound
	}
	err := decode(v, &b)
	return b, err
}

// DeleteRenameBatch removes a batch rename entry after undo.
func DeleteRenameBatch(tx *bbolt.Tx, id string) error {
	return tx.Bucket(BucketRenameHistory).Delete([]byte(id))
}
