// Package db opens the library's bbolt file and keeps the buckets and
// indexes described in AGENT.md, plus the project-reference and meta
// buckets added for this version.
package db

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"go.etcd.io/bbolt"

	"raglex/internal/model"
)

const (
	fileName = "raglex.db"
	bakName  = "raglex.db.bak"
	filesDir = "files"
	thumbDir = "thumbs"

	schemaKey = "schema"
)

// Bucket names.
var (
	BucketAssets            = []byte("assets")
	BucketAssetsByHash      = []byte("assets_by_hash")
	BucketAssetsByPath      = []byte("assets_by_path")
	BucketTags              = []byte("tags")
	BucketAssetTags         = []byte("asset_tags")
	BucketTagAssets         = []byte("tag_assets")
	BucketFolders           = []byte("folders")
	BucketAssetFolders      = []byte("asset_folders")
	BucketFolderAssets      = []byte("folder_assets")
	BucketProjects          = []byte("projects")
	BucketProjectTags       = []byte("project_tags")
	BucketTagProjects       = []byte("tag_projects")
	BucketProjectAssets     = []byte("project_assets")
	BucketAssetProjects     = []byte("asset_projects")
	BucketBoards            = []byte("boards")
	BucketBoardsByProject   = []byte("boards_by_project")
	BucketPlacements        = []byte("placements")
	BucketPlacementsByBoard = []byte("placements_by_board")
	BucketPlacementsByAsset = []byte("placements_by_asset")
	BucketRenameHistory     = []byte("rename_history")
	BucketBoardsStandalone  = []byte("boards_standalone")
	BucketAssetVectors      = []byte("asset_vectors")
	BucketStacks            = []byte("stacks")
	BucketMeta              = []byte("meta")
)

var allBuckets = [][]byte{
	BucketAssets,
	BucketAssetsByHash,
	BucketAssetsByPath,
	BucketTags,
	BucketAssetTags,
	BucketTagAssets,
	BucketFolders,
	BucketAssetFolders,
	BucketFolderAssets,
	BucketProjects,
	BucketProjectTags,
	BucketTagProjects,
	BucketProjectAssets,
	BucketAssetProjects,
	BucketBoards,
	BucketBoardsByProject,
	BucketBoardsStandalone,
	BucketPlacements,
	BucketPlacementsByBoard,
	BucketPlacementsByAsset,
	BucketRenameHistory,
	BucketAssetVectors,
	BucketStacks,
	BucketMeta,
}

// ErrCorrupt means the database file could not be opened. A sibling
// raglex.db.bak may still be restorable.
var ErrCorrupt = errors.New("raglex: database cannot be opened")

// ErrBusy means another process holds the database.
var ErrBusy = errors.New("raglex: database is in use")

// ErrNotFound means the record or index entry is absent.
var ErrNotFound = errors.New("raglex: not found")

// DB is one open library.
type DB struct {
	bolt *bbolt.DB
	root string
	path string
}

// Root is the library folder the user chose.
func (d *DB) Root() string { return d.root }

// Path is the bbolt file.
func (d *DB) Path() string { return d.path }

// FilesDir is where copy, move, and symlink imports land.
func (d *DB) FilesDir() string { return filepath.Join(d.root, filesDir) }

// ThumbsDir holds generated previews.
func (d *DB) ThumbsDir() string { return filepath.Join(d.root, thumbDir) }

// BackupPath is the sibling automatic backup file (raglex.db.bak).
func (d *DB) BackupPath() string { return filepath.Join(d.root, bakName) }

// Open opens or creates a library in root. A successful open writes a
// snapshot to raglex.db.bak so a later failed open can restore it.
func Open(root string) (*DB, error) {
	if err := os.MkdirAll(filepath.Join(root, filesDir), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, thumbDir), 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(root, fileName)
	bolt, err := bbolt.Open(path, 0o644, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		if errors.Is(err, bbolt.ErrTimeout) {
			return nil, fmt.Errorf("%w: %s", ErrBusy, path)
		}
		return nil, fmt.Errorf("%w: %s: %v", ErrCorrupt, path, err)
	}
	d := &DB{bolt: bolt, root: root, path: path}
	if err := d.init(); err != nil {
		_ = bolt.Close()
		return nil, err
	}
	// The snapshot is the last known good file at the start of this session.
	// A failure here does not reject the open: the library is still usable.
	_ = d.Backup()
	return d, nil
}

func (d *DB) init() error {
	return d.bolt.Update(func(tx *bbolt.Tx) error {
		for _, name := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		meta := tx.Bucket(BucketMeta)
		cur := meta.Get([]byte(schemaKey))
		if cur == nil {
			return meta.Put([]byte(schemaKey), []byte(model.SchemaVersion))
		}
		if string(cur) != model.SchemaVersion {
			return fmt.Errorf("raglex: library schema %s is newer than this build", cur)
		}
		return nil
	})
}

// Close closes the file.
func (d *DB) Close() error {
	if d == nil || d.bolt == nil {
		return nil
	}
	return d.bolt.Close()
}

// Update runs fn in one write transaction. Callers update a record and its
// indexes inside fn.
func (d *DB) Update(fn func(tx *bbolt.Tx) error) error {
	return d.bolt.Update(fn)
}

// View runs fn in a read transaction.
func (d *DB) View(fn func(tx *bbolt.Tx) error) error {
	return d.bolt.View(fn)
}

// Backup writes a consistent copy to raglex.db.bak. bbolt keeps the file
// open, so the copy comes from the transaction rather than from copying
// the live file.
func (d *DB) Backup() error {
	return d.BackupTo(d.BackupPath())
}

// BackupTo writes a consistent snapshot of the live database to dst.
func (d *DB) BackupTo(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = d.bolt.View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(f)
		return err
	})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return replaceFile(tmp, dst)
}

// HasBackup reports whether raglex.db.bak exists beside the library.
func HasBackup(root string) bool {
	_, err := os.Stat(filepath.Join(root, bakName))
	return err == nil
}

// Restore replaces raglex.db with raglex.db.bak. The database must be closed.
func Restore(root string) error {
	src := filepath.Join(root, bakName)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := filepath.Join(root, fileName+".restore")
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return replaceFile(tmp, filepath.Join(root, fileName))
}

func replaceFile(src, dst string) error {
	_ = os.Remove(dst)
	return os.Rename(src, dst)
}

func encode(v any) ([]byte, error) {
	return msgpack.Marshal(v)
}

func decode(b []byte, v any) error {
	return msgpack.Unmarshal(b, v)
}

func indexKey(a, b string) []byte {
	return []byte(a + "/" + b)
}

func putEmpty(bucket *bbolt.Bucket, key []byte) error {
	return bucket.Put(key, []byte{})
}

func idAfter(key []byte, prefix string) string {
	return string(key[len(prefix):])
}

// forPrefix calls fn for every key that starts with prefix. prefix should
// include the trailing slash when the key is parent/child.
func forPrefix(bucket *bbolt.Bucket, prefix string, fn func(k, v []byte) error) error {
	if bucket == nil {
		return nil
	}
	p := []byte(prefix)
	c := bucket.Cursor()
	for k, v := c.Seek(p); k != nil && bytes.HasPrefix(k, p); k, v = c.Next() {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

func collectIDs(bucket *bbolt.Bucket, prefix string) ([]string, error) {
	var ids []string
	err := forPrefix(bucket, prefix, func(k, _ []byte) error {
		ids = append(ids, idAfter(k, prefix))
		return nil
	})
	return ids, err
}
