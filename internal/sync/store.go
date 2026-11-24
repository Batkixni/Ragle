// Package sync implements Raglex's open-source Sync API v1 (append-only delta
// oplog, content-addressed SHA-256 blob deduplication with optional Cloudflare R2/S3
// presigned URLs, public share links, and multi-device desktop sync client).
// Both self-hosted servers and Raglex Cloud run this exact same open core.
package sync

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

var (
	bucketOps         = []byte("sync_ops")
	bucketOpIDs       = []byte("sync_op_ids")
	bucketShares      = []byte("share_links")
	bucketMeta        = []byte("sync_meta")
	bucketRemoteKnown = []byte("remote_known")
)

// SyncOp is one idempotent delta operation in the append-only sync oplog.
type SyncOp struct {
	Seq       uint64          `json:"seq"`
	OpID      string          `json:"op_id"`
	DeviceID  string          `json:"device_id"`
	Timestamp int64           `json:"timestamp"` // unix millis
	Kind      string          `json:"kind"`      // asset.upsert, asset.meta, asset.delete, project.upsert, board.patch
	EntityID  string          `json:"entity_id"`
	Payload   json.RawMessage `json:"payload"`
}

// ShareLink represents a public or password-protected web share link for a
// single asset, a project, or a whiteboard.
type ShareLink struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"` // "asset", "project", "board"
	TargetID      string    `json:"target_id"`
	Title         string    `json:"title"`
	PasswordSalt  string    `json:"password_salt,omitempty"`
	PasswordHash  string    `json:"password_hash,omitempty"`
	HasPassword   bool      `json:"has_password"`
	AllowDownload bool      `json:"allow_download"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	Views         int64     `json:"views"`
}

// Expired reports whether the share link has passed its optional expiration.
func (s ShareLink) Expired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt)
}

// CheckPassword verifies a candidate plaintext password against the salted hash.
func (s ShareLink) CheckPassword(pw string) bool {
	if !s.HasPassword || s.PasswordHash == "" {
		return true
	}
	return hashPassword(s.PasswordSalt, pw) == s.PasswordHash
}

func hashPassword(salt, pw string) string {
	h := sha256.Sum256([]byte(salt + ":" + pw))
	return hex.EncodeToString(h[:])
}

type sharedSyncDB struct {
	db   *bbolt.DB
	refs int
}

var (
	openSyncDBMu sync.Mutex
	openSyncDBs  = map[string]*sharedSyncDB{}
)

func syncDBKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

// Store persists sync oplog entries and public share links in a dedicated bbolt file.
type Store struct {
	key    string
	db     *bbolt.DB
	mu     sync.Mutex
	closed bool
}

// OpenStore opens (or creates) <root>/sync.db. Multiple callers in the same
// process (such as the built-in Companion Server and desktop SyncNow) safely
// share one reference-counted bbolt handle without flock timeouts.
func OpenStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(root, "sync.db")
	key := syncDBKey(dbPath)

	openSyncDBMu.Lock()
	defer openSyncDBMu.Unlock()

	if existing, ok := openSyncDBs[key]; ok && existing.refs > 0 {
		existing.refs++
		return &Store{key: key, db: existing.db}, nil
	}

	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, b := range [][]byte{bucketOps, bucketOpIDs, bucketShares, bucketMeta, bucketRemoteKnown} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	openSyncDBs[key] = &sharedSyncDB{db: db, refs: 1}
	return &Store{key: key, db: db}, nil
}

// Close releases this Store reference and closes the underlying bbolt database
// once no callers in this process still hold it open.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed || s.db == nil {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	key := s.key
	db := s.db
	s.mu.Unlock()

	openSyncDBMu.Lock()
	defer openSyncDBMu.Unlock()
	if sh, ok := openSyncDBs[key]; ok {
		sh.refs--
		if sh.refs <= 0 {
			delete(openSyncDBs, key)
			return db.Close()
		}
		return nil
	}
	return db.Close()
}

func seqKey(seq uint64) []byte {
	var k [8]byte
	binary.BigEndian.PutUint64(k[:], seq)
	return k[:]
}

// PushOps appends ops idempotently (skipping any OpID already stored) and
// returns the new latest sequence number and accepted op count.
func (s *Store) PushOps(ops []SyncOp) (uint64, int, error) {
	var latest uint64
	accepted := 0
	err := s.db.Update(func(tx *bbolt.Tx) error {
		bOps := tx.Bucket(bucketOps)
		bIDs := tx.Bucket(bucketOpIDs)
		latest = bOps.Sequence()
		for _, op := range ops {
			op.OpID = strings.TrimSpace(op.OpID)
			if op.OpID == "" || op.Kind == "" || op.EntityID == "" {
				continue
			}
			if bIDs.Get([]byte(op.OpID)) != nil {
				continue // already applied — idempotent retry
			}
			seq, err := bOps.NextSequence()
			if err != nil {
				return err
			}
			op.Seq = seq
			if op.Timestamp == 0 {
				op.Timestamp = time.Now().UnixMilli()
			}
			raw, err := json.Marshal(op)
			if err != nil {
				return err
			}
			sk := seqKey(seq)
			if err := bOps.Put(sk, raw); err != nil {
				return err
			}
			if err := bIDs.Put([]byte(op.OpID), sk); err != nil {
				return err
			}
			latest = seq
			accepted++
		}
		return nil
	})
	return latest, accepted, err
}

// PullOps returns up to limit ops with Seq > since in strictly ascending order.
func (s *Store) PullOps(since uint64, limit int) ([]SyncOp, uint64, bool, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	var ops []SyncOp
	var latest uint64
	hasMore := false
	err := s.db.View(func(tx *bbolt.Tx) error {
		bOps := tx.Bucket(bucketOps)
		latest = bOps.Sequence()
		c := bOps.Cursor()
		k, v := c.Seek(seqKey(since + 1))
		for ; k != nil; k, v = c.Next() {
			if len(ops) >= limit {
				hasMore = true
				break
			}
			var op SyncOp
			if err := json.Unmarshal(v, &op); err == nil {
				ops = append(ops, op)
			}
		}
		return nil
	})
	return ops, latest, hasMore, err
}

// LatestSeq returns the highest committed sequence number.
func (s *Store) LatestSeq() uint64 {
	var latest uint64
	_ = s.db.View(func(tx *bbolt.Tx) error {
		latest = tx.Bucket(bucketOps).Sequence()
		return nil
	})
	return latest
}

// CreateShareRequest configures a new public share link.
type CreateShareRequest struct {
	Kind          string `json:"kind"` // "asset", "project", "board"
	TargetID      string `json:"target_id"`
	Title         string `json:"title"`
	Password      string `json:"password,omitempty"`
	AllowDownload bool   `json:"allow_download"`
	TTLHours      int    `json:"ttl_hours,omitempty"` // 0 = never expires
}

// CreateShare creates and persists a new ShareLink.
func (s *Store) CreateShare(req CreateShareRequest) (ShareLink, error) {
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	if req.Kind != "asset" && req.Kind != "project" && req.Kind != "board" {
		return ShareLink{}, errors.New("sync: share kind must be asset, project, or board")
	}
	if strings.TrimSpace(req.TargetID) == "" {
		return ShareLink{}, errors.New("sync: target_id is required")
	}
	var idBytes [9]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return ShareLink{}, err
	}
	id := hex.EncodeToString(idBytes[:])
	now := time.Now().UTC()
	link := ShareLink{
		ID:            id,
		Kind:          req.Kind,
		TargetID:      strings.TrimSpace(req.TargetID),
		Title:         strings.TrimSpace(req.Title),
		AllowDownload: req.AllowDownload,
		CreatedAt:     now,
	}
	if req.TTLHours > 0 {
		link.ExpiresAt = now.Add(time.Duration(req.TTLHours) * time.Hour)
	}
	if strings.TrimSpace(req.Password) != "" {
		var salt [8]byte
		_, _ = rand.Read(salt[:])
		link.PasswordSalt = hex.EncodeToString(salt[:])
		link.PasswordHash = hashPassword(link.PasswordSalt, req.Password)
		link.HasPassword = true
	}
	err := s.db.Update(func(tx *bbolt.Tx) error {
		raw, err := json.Marshal(link)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketShares).Put([]byte(link.ID), raw)
	})
	return link, err
}

// GetShare loads a ShareLink by ID and optionally increments its view counter.
func (s *Store) GetShare(id string, countView bool) (ShareLink, bool) {
	var link ShareLink
	found := false
	if countView {
		_ = s.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketShares)
			raw := b.Get([]byte(id))
			if raw == nil {
				return nil
			}
			if err := json.Unmarshal(raw, &link); err != nil {
				return nil
			}
			found = true
			link.Views++
			next, _ := json.Marshal(link)
			return b.Put([]byte(id), next)
		})
	} else {
		_ = s.db.View(func(tx *bbolt.Tx) error {
			raw := tx.Bucket(bucketShares).Get([]byte(id))
			if raw == nil {
				return nil
			}
			if err := json.Unmarshal(raw, &link); err == nil {
				found = true
			}
			return nil
		})
	}
	return link, found
}

// ListShares returns all saved share links (with password hashes redacted).
func (s *Store) ListShares() []ShareLink {
	var out []ShareLink
	_ = s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketShares).ForEach(func(_, v []byte) error {
			var l ShareLink
			if err := json.Unmarshal(v, &l); err == nil {
				l.PasswordHash = ""
				l.PasswordSalt = ""
				out = append(out, l)
			}
			return nil
		})
	})
	return out
}

// DeleteShare revokes a public share link immediately.
func (s *Store) DeleteShare(id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketShares).Delete([]byte(id))
	})
}

// GetCursor / SetCursor persist a client's last-synced sequence number inside sync.db.
func (s *Store) GetCursor(key string) uint64 {
	var seq uint64
	_ = s.db.View(func(tx *bbolt.Tx) error {
		if v := tx.Bucket(bucketMeta).Get([]byte("cursor:" + key)); len(v) == 8 {
			seq = binary.BigEndian.Uint64(v)
		}
		return nil
	})
	return seq
}

func (s *Store) SetCursor(key string, seq uint64) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketMeta).Put([]byte("cursor:"+key), seqKey(seq))
	})
}

// EnsureDeviceID returns a stable, unique random device ID persisted inside
// this library's sync.db so two separate devices (e.g. Windows + macOS) signed
// into the same account never collide on a shared default device name.
func (s *Store) EnsureDeviceID() string {
	var id string
	_ = s.db.View(func(tx *bbolt.Tx) error {
		if v := tx.Bucket(bucketMeta).Get([]byte("device_id")); len(v) > 0 {
			id = string(v)
		}
		return nil
	})
	if id != "" {
		return id
	}
	var rnd [12]byte
	_, _ = rand.Read(rnd[:])
	id = "dev-" + hex.EncodeToString(rnd[:])
	_ = s.db.Update(func(tx *bbolt.Tx) error {
		if cur := tx.Bucket(bucketMeta).Get([]byte("device_id")); len(cur) > 0 {
			id = string(cur)
			return nil
		}
		return tx.Bucket(bucketMeta).Put([]byte("device_id"), []byte(id))
	})
	return id
}

// LoadRemoteKnown returns the sets of OpIDs and SHA-256 blob hashes already
// confirmed to exist on the remote server identified by scope.
func (s *Store) LoadRemoteKnown(scope string) (ops map[string]bool, hashes map[string]bool) {
	ops = map[string]bool{}
	hashes = map[string]bool{}
	if s == nil || s.db == nil || scope == "" {
		return ops, hashes
	}
	opPrefix := []byte(scope + ":op:")
	hashPrefix := []byte(scope + ":h:")
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketRemoteKnown)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, _ := c.Seek(opPrefix); k != nil && bytes.HasPrefix(k, opPrefix); k, _ = c.Next() {
			ops[string(k[len(opPrefix):])] = true
		}
		for k, _ := c.Seek(hashPrefix); k != nil && bytes.HasPrefix(k, hashPrefix); k, _ = c.Next() {
			hashes[string(k[len(hashPrefix):])] = true
		}
		return nil
	})
	return ops, hashes
}

// MarkRemoteKnown records OpIDs and SHA-256 hashes confirmed on the remote
// server so subsequent syncs skip redundant /v1/blobs/check & /v1/sync/push
// payloads for unchanged assets.
func (s *Store) MarkRemoteKnown(scope string, opIDs []string, hashes []string) error {
	if s == nil || s.db == nil || scope == "" || (len(opIDs) == 0 && len(hashes) == 0) {
		return nil
	}
	opPrefix := scope + ":op:"
	hashPrefix := scope + ":h:"
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketRemoteKnown)
		if err != nil {
			return err
		}
		for _, id := range opIDs {
			if id != "" {
				if err := b.Put([]byte(opPrefix+id), []byte{1}); err != nil {
					return err
				}
			}
		}
		for _, h := range hashes {
			if h != "" {
				if err := b.Put([]byte(hashPrefix+h), []byte{1}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Sanitized returns a copy of ShareLink safe to return in public API responses.
func (s ShareLink) Sanitized() ShareLink {
	s.PasswordHash = ""
	s.PasswordSalt = ""
	return s
}

func ValidateSHA256(hash string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(hash))
	if len(h) != 64 {
		return "", fmt.Errorf("invalid sha256 length: %d", len(h))
	}
	if _, err := hex.DecodeString(h); err != nil {
		return "", fmt.Errorf("invalid sha256 hex: %w", err)
	}
	return h, nil
}
