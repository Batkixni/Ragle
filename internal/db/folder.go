package db

import (
	"go.etcd.io/bbolt"

	"raglex/internal/model"
)

// PutFolder writes a folder record. Sibling name uniqueness is enforced by the caller.
func PutFolder(tx *bbolt.Tx, f model.Folder) error {
	raw, err := encode(f)
	if err != nil {
		return err
	}
	return tx.Bucket(BucketFolders).Put([]byte(f.ID), raw)
}

// GetFolder reads one folder by ID.
func GetFolder(tx *bbolt.Tx, id string) (model.Folder, error) {
	var f model.Folder
	b := tx.Bucket(BucketFolders)
	if b == nil {
		return f, ErrNotFound
	}
	raw := b.Get([]byte(id))
	if raw == nil {
		return f, ErrNotFound
	}
	err := decode(raw, &f)
	return f, err
}

// ListFolders scans the folders bucket.
func ListFolders(tx *bbolt.Tx) ([]model.Folder, error) {
	var out []model.Folder
	b := tx.Bucket(BucketFolders)
	if b == nil {
		return out, nil
	}
	err := b.ForEach(func(_, v []byte) error {
		var f model.Folder
		if err := decode(v, &f); err != nil {
			return err
		}
		out = append(out, f)
		return nil
	})
	return out, err
}

// LinkAssetFolder records that assetID belongs to folderID, in both directions.
func LinkAssetFolder(tx *bbolt.Tx, assetID, folderID string) error {
	if err := putEmpty(tx.Bucket(BucketAssetFolders), indexKey(assetID, folderID)); err != nil {
		return err
	}
	return putEmpty(tx.Bucket(BucketFolderAssets), indexKey(folderID, assetID))
}

// UnlinkAssetFolder removes both directions.
func UnlinkAssetFolder(tx *bbolt.Tx, assetID, folderID string) error {
	if b := tx.Bucket(BucketAssetFolders); b != nil {
		if err := b.Delete(indexKey(assetID, folderID)); err != nil {
			return err
		}
	}
	if b := tx.Bucket(BucketFolderAssets); b != nil {
		return b.Delete(indexKey(folderID, assetID))
	}
	return nil
}

// FoldersOfAsset lists folder IDs linked to an asset.
func FoldersOfAsset(tx *bbolt.Tx, assetID string) ([]string, error) {
	b := tx.Bucket(BucketAssetFolders)
	if b == nil {
		return nil, nil
	}
	return collectIDs(b, assetID+"/")
}

// AllAssetFolderIDs scans the asset_folders bucket once and returns map[assetID][]folderID.
func AllAssetFolderIDs(tx *bbolt.Tx) (map[string][]string, error) {
	out := make(map[string][]string)
	b := tx.Bucket(BucketAssetFolders)
	if b == nil {
		return out, nil
	}
	err := b.ForEach(func(k, _ []byte) error {
		s := string(k)
		for i := 0; i < len(s); i++ {
			if s[i] == '/' {
				assetID := s[:i]
				folderID := s[i+1:]
				if assetID != "" && folderID != "" {
					out[assetID] = append(out[assetID], folderID)
				}
				break
			}
		}
		return nil
	})
	return out, err
}

// AssetsOfFolder lists asset IDs directly inside folderID.
func AssetsOfFolder(tx *bbolt.Tx, folderID string) ([]string, error) {
	b := tx.Bucket(BucketFolderAssets)
	if b == nil {
		return nil, nil
	}
	return collectIDs(b, folderID+"/")
}

// DeleteFolder removes the folder and its asset links. Assets themselves stay.
func DeleteFolder(tx *bbolt.Tx, folderID string) error {
	assetIDs, err := AssetsOfFolder(tx, folderID)
	if err != nil {
		return err
	}
	for _, assetID := range assetIDs {
		if err := UnlinkAssetFolder(tx, assetID, folderID); err != nil {
			return err
		}
	}
	if b := tx.Bucket(BucketFolders); b != nil {
		return b.Delete([]byte(folderID))
	}
	return nil
}
