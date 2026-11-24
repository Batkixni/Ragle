// Package cloud implements the multi-tenant Raglex Cloud server:
//   - Account Auth (/api/v1/auth/register, /login, /me, /logout)
//   - Per-user isolated Open Sync API v1 (/v1/sync/*, /v1/blobs/*, /v1/shares*)
//   - Content-addressed SHA-256 blob storage (Hetzner Storage Box / local / R2)
//     with per-user ACL isolation and storage quota accounting
//   - Embedded WebRTC pairing signaling (/signal/*)
package cloud

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/model"
	"raglex/internal/share"
	rsync "raglex/internal/sync"
	"raglex/internal/updater"
)

var (
	bucketUsersByID    = []byte("users_by_id")
	bucketUsersByEmail = []byte("users_by_email")
	bucketTokens       = []byte("tokens")
	bucketUserBlobs    = []byte("user_blobs") // key: userID + ":" + sha256 -> uint64 size
	bucketUserUsage    = []byte("user_usage") // key: userID -> uint64 total bytes
	bucketInvites      = []byte("invites")    // key: uppercase invite code -> InviteCode JSON
)

const (
	pbkdf2Iterations = 210_000
	pbkdf2KeyLen     = 32
	tokenTTL         = 90 * 24 * time.Hour
	maxUploadBytes   = 512 << 20 // 512 MiB per single blob
)

// Options configures a Raglex Cloud server instance.
type Options struct {
	DataDir       string        // Local fast NVMe directory for accounts.db and per-user sync.db
	BlobDir       string        // Large storage volume (e.g. /mnt/storagebox/raglex-cloud) for blobs
	PublicHost    string        // e.g. "cloud.raglex.sorai.tw"
	QuotaBytes    int64         // Per-user storage quota (default 10 GiB; <=0 means unlimited)
	AllowRegister bool          // Allow account registration (when false, completely closed)
	InviteOnly    bool          // Require a valid invite code for registration
	AdminToken    string        // Bearer token for /api/v1/admin/* invite & user management
	SignalTTL     time.Duration // WebRTC pairing room TTL (default 5m)
	R2            *rsync.R2Presigner
}

// InviteCode grants registration permission when InviteOnly is enabled.
type InviteCode struct {
	Code      string    `json:"code"`
	MaxUses   int       `json:"max_uses"` // 0 = unlimited
	UsedCount int       `json:"used_count"`
	Note      string    `json:"note,omitempty"`
	Revoked   bool      `json:"revoked,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// User is a registered Raglex Cloud account stored in accounts.db.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordSalt string    `json:"password_salt"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

type tokenRecord struct {
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Server serves the full Raglex Cloud HTTP API.
type Server struct {
	opts       Options
	db         *bbolt.DB
	blobs      *rsync.LocalBlobStore
	signal     *share.SignalingServer
	mux        *http.ServeMux

	storesMu   sync.RWMutex
	userStores map[string]*rsync.Store
}

// Open initializes Raglex Cloud storage and HTTP routes.
func Open(opts Options) (*Server, error) {
	if strings.TrimSpace(opts.DataDir) == "" {
		return nil, errors.New("cloud: DataDir is required")
	}
	if strings.TrimSpace(opts.BlobDir) == "" {
		opts.BlobDir = opts.DataDir
	}
	if opts.PublicHost == "" {
		opts.PublicHost = "cloud.raglex.sorai.tw"
	}
	if opts.SignalTTL <= 0 {
		opts.SignalTTL = 5 * time.Minute
	}
	if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(opts.DataDir, "users"), 0o755); err != nil {
		return nil, fmt.Errorf("create users dir: %w", err)
	}
	blobs, err := rsync.NewLocalBlobStore(opts.BlobDir, opts.R2)
	if err != nil {
		return nil, fmt.Errorf("open blob store: %w", err)
	}
	dbPath := filepath.Join(opts.DataDir, "accounts.db")
	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open accounts db: %w", err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, b := range [][]byte{bucketUsersByID, bucketUsersByEmail, bucketTokens, bucketUserBlobs, bucketUserUsage, bucketInvites} {
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

	s := &Server{
		opts:       opts,
		db:         db,
		blobs:      blobs,
		signal:     share.NewSignalingServer(opts.SignalTTL),
		mux:        http.NewServeMux(),
		userStores: map[string]*rsync.Store{},
	}
	s.registerRoutes()
	return s, nil
}

// Close shuts down the signaling server and closes all open bbolt databases.
func (s *Server) Close() error {
	if s.signal != nil {
		s.signal.Close()
	}
	s.storesMu.Lock()
	for id, st := range s.userStores {
		_ = st.Close()
		delete(s.userStores, id)
	}
	s.storesMu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// ServeHTTP handles all HTTP requests with clean security headers.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/", s.handleLanding)
	s.mux.HandleFunc("/healthz", s.handleHealth)

	// Account Auth API v1.
	s.mux.HandleFunc("/api/v1/auth/register", s.handleRegister)
	s.mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/v1/auth/me", s.handleMe)
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)

	// Admin Invite & User Management API v1.
	s.mux.HandleFunc("/api/v1/admin/invites", s.withAdmin(s.handleAdminInvites))
	s.mux.HandleFunc("/api/v1/admin/invites/", s.withAdmin(s.handleAdminInviteByCode))
	s.mux.HandleFunc("/api/v1/admin/users", s.withAdmin(s.handleAdminUsers))

	// Multi-tenant Open Sync API v1.
	s.mux.HandleFunc("/v1/sync/status", s.withUser(s.handleSyncStatus))
	s.mux.HandleFunc("/v1/sync/pull", s.withUser(s.handleSyncPull))
	s.mux.HandleFunc("/v1/sync/push", s.withUser(s.handleSyncPush))
	s.mux.HandleFunc("/v1/blobs/check", s.withUser(s.handleBlobCheck))
	s.mux.HandleFunc("/v1/blobs/presign", s.withUser(s.handleBlobPresign))
	s.mux.HandleFunc("/v1/blobs/", s.withUser(s.handleBlobObject))

	// Embedded WebRTC Pairing Signaling.
	s.mux.Handle("/signal/", http.StripPrefix("/signal", s.signal))
	s.mux.Handle("/pair/", s.signal)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"service":      "raglex-cloud",
		"host":         s.opts.PublicHost,
		"version":      updater.Version,
		"blob_backend": s.blobs.BackendName(),
		"register":     s.opts.AllowRegister,
		"invite_only":  s.opts.InviteOnly,
	})
}

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-Hant">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Raglex Cloud</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #0c0c0f; color: #f4f4f5; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
  .card { width: min(440px, 90vw); padding: 32px; border-radius: 18px; background: #141419; border: 1px solid rgba(255,255,255,.09); box-shadow: 0 24px 60px rgba(0,0,0,.5); }
  .badge { display: inline-flex; align-items: center; gap: 7px; font-size: 12px; color: #34d399; background: rgba(16,185,129,.12); padding: 4px 10px; border-radius: 999px; margin-bottom: 14px; }
  .dot { width: 7px; height: 7px; border-radius: 999px; background: #10b981; }
  h1 { margin: 0 0 8px; font-size: 22px; letter-spacing: -.02em; }
  p { margin: 0 0 18px; font-size: 13.5px; line-height: 1.6; color: #a1a1aa; }
  .meta { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; color: #d4d4d8; background: #0f0f13; border: 1px solid rgba(255,255,255,.07); border-radius: 10px; padding: 12px 14px; line-height: 1.7; }
</style>
</head>
<body>
  <div class="card">
    <div class="badge"><span class="dot"></span>Raglex Cloud Online</div>
    <h1>Raglex Cloud</h1>
    <p>本地優先設計靈感庫的雲端帳號驗證、跨裝置同步與配對中繼服務。請在 Raglex 桌面版「偏好設定 → 帳號與雲端」中登入使用。</p>
    <div class="meta">Endpoint: https://%s<br>Protocol: raglex-sync-v1 · v%s</div>
  </div>
</body>
</html>`, s.opts.PublicHost, updater.Version)
}

// --- Auth Handlers ---

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.opts.AllowRegister {
		writeErr(w, http.StatusForbidden, "目前未開放註冊新帳號")
		return
	}
	var req struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		Name       string `json:"name"`
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	invCode := normalizeInviteCode(req.InviteCode)
	if s.opts.InviteOnly && invCode == "" {
		writeErr(w, http.StatusForbidden, "此伺服器僅限邀請碼註冊，請輸入有效邀請碼")
		return
	}
	email := normalizeEmail(req.Email)
	if !isValidEmail(email) {
		writeErr(w, http.StatusBadRequest, "請輸入有效的電子信箱格式")
		return
	}
	if len(req.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "密碼長度至少需要 6 個字元")
		return
	}
	if len(req.Password) > 128 {
		writeErr(w, http.StatusBadRequest, "密碼長度不可超過 128 個字元")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			name = email[:at]
		}
	}

	saltHex, hashHex, err := hashUserPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "password hashing failed")
		return
	}

	user := User{
		ID:           model.NewID(),
		Email:        email,
		Name:         name,
		PasswordSalt: saltHex,
		PasswordHash: hashHex,
		CreatedAt:    time.Now().UTC(),
	}
	rawToken, tokHash, err := generateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	var dup bool
	var invErr string
	err = s.db.Update(func(tx *bbolt.Tx) error {
		byEmail := tx.Bucket(bucketUsersByEmail)
		if byEmail.Get([]byte(email)) != nil {
			dup = true
			return nil
		}
		if s.opts.InviteOnly {
			invB := tx.Bucket(bucketInvites)
			rawInv := invB.Get([]byte(invCode))
			if rawInv == nil {
				invErr = "邀請碼無效或不存在"
				return nil
			}
			var inv InviteCode
			if err := json.Unmarshal(rawInv, &inv); err != nil {
				invErr = "邀請碼資料異常"
				return nil
			}
			if inv.Revoked {
				invErr = "此邀請碼已停用"
				return nil
			}
			if !inv.ExpiresAt.IsZero() && time.Now().UTC().After(inv.ExpiresAt) {
				invErr = "此邀請碼已過期"
				return nil
			}
			if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
				invErr = "此邀請碼已達使用上限"
				return nil
			}
			inv.UsedCount++
			updatedInv, err := json.Marshal(inv)
			if err != nil {
				return err
			}
			if err := invB.Put([]byte(invCode), updatedInv); err != nil {
				return err
			}
		}
		rawUser, err := json.Marshal(user)
		if err != nil {
			return err
		}
		if err := tx.Bucket(bucketUsersByID).Put([]byte(user.ID), rawUser); err != nil {
			return err
		}
		if err := byEmail.Put([]byte(email), []byte(user.ID)); err != nil {
			return err
		}
		now := time.Now().UTC()
		rawTok, _ := json.Marshal(tokenRecord{
			UserID:    user.ID,
			CreatedAt: now,
			ExpiresAt: now.Add(tokenTTL),
		})
		return tx.Bucket(bucketTokens).Put([]byte(tokHash), rawTok)
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dup {
		writeErr(w, http.StatusConflict, "此電子信箱已註冊，請直接登入")
		return
	}
	if invErr != "" {
		writeErr(w, http.StatusForbidden, invErr)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":      true,
		"user_id": user.ID,
		"email":   user.Email,
		"name":    user.Name,
		"token":   rawToken,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "請輸入電子信箱與密碼")
		return
	}

	var user User
	var found bool
	_ = s.db.View(func(tx *bbolt.Tx) error {
		uid := tx.Bucket(bucketUsersByEmail).Get([]byte(email))
		if uid == nil {
			return nil
		}
		raw := tx.Bucket(bucketUsersByID).Get(uid)
		if raw == nil {
			return nil
		}
		if json.Unmarshal(raw, &user) == nil {
			found = true
		}
		return nil
	})
	if !found || !verifyUserPassword(req.Password, user.PasswordSalt, user.PasswordHash) {
		writeErr(w, http.StatusUnauthorized, "電子信箱或密碼錯誤")
		return
	}

	rawToken, tokHash, err := generateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	now := time.Now().UTC()
	err = s.db.Update(func(tx *bbolt.Tx) error {
		rawTok, _ := json.Marshal(tokenRecord{
			UserID:    user.ID,
			CreatedAt: now,
			ExpiresAt: now.Add(tokenTTL),
		})
		return tx.Bucket(bucketTokens).Put([]byte(tokHash), rawTok)
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"user_id": user.ID,
		"email":   user.Email,
		"name":    user.Name,
		"token":   rawToken,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, ok := s.authenticate(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	used := s.userUsedBytes(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"user_id":     user.ID,
		"email":       user.Email,
		"name":        user.Name,
		"used_bytes":  used,
		"quota_bytes": s.opts.QuotaBytes,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tok := extractBearer(r)
	if tok != "" {
		tokHash := sha256Hex(tok)
		_ = s.db.Update(func(tx *bbolt.Tx) error {
			return tx.Bucket(bucketTokens).Delete([]byte(tokHash))
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Admin Invite & User Management ---

func (s *Server) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminTok := strings.TrimSpace(s.opts.AdminToken)
		if adminTok == "" {
			writeErr(w, http.StatusForbidden, "admin API is disabled (RAGLEX_CLOUD_ADMIN_TOKEN not set)")
			return
		}
		got := extractBearer(r)
		if got == "" {
			got = strings.TrimSpace(r.Header.Get("X-Admin-Token"))
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(adminTok)) != 1 {
			writeErr(w, http.StatusUnauthorized, "invalid admin token")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleAdminInvites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var list []InviteCode
		_ = s.db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketInvites)
			return b.ForEach(func(_, v []byte) error {
				var inv InviteCode
				if json.Unmarshal(v, &inv) == nil {
					list = append(list, inv)
				}
				return nil
			})
		})
		if list == nil {
			list = []InviteCode{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "invites": list})

	case http.MethodPost:
		var req struct {
			Code         string `json:"code"`
			MaxUses      int    `json:"max_uses"`
			Note         string `json:"note"`
			ExpiresHours int    `json:"expires_hours"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req)
		code := normalizeInviteCode(req.Code)
		if code == "" {
			var err error
			code, err = generateInviteCode()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "failed to generate invite code")
				return
			}
		}
		maxUses := req.MaxUses
		if maxUses < 0 {
			maxUses = 1
		}
		now := time.Now().UTC()
		inv := InviteCode{
			Code:      code,
			MaxUses:   maxUses,
			Note:      strings.TrimSpace(req.Note),
			CreatedAt: now,
		}
		if req.ExpiresHours > 0 {
			inv.ExpiresAt = now.Add(time.Duration(req.ExpiresHours) * time.Hour)
		}
		var dup bool
		err := s.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketInvites)
			if b.Get([]byte(code)) != nil {
				dup = true
				return nil
			}
			raw, err := json.Marshal(inv)
			if err != nil {
				return err
			}
			return b.Put([]byte(code), raw)
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if dup {
			writeErr(w, http.StatusConflict, "invite code already exists")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "invite": inv})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminInviteByCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	code := normalizeInviteCode(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/invites/"))
	if code == "" {
		writeErr(w, http.StatusBadRequest, "missing invite code")
		return
	}
	var found bool
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketInvites)
		raw := b.Get([]byte(code))
		if raw == nil {
			return nil
		}
		found = true
		var inv InviteCode
		if err := json.Unmarshal(raw, &inv); err != nil {
			return b.Delete([]byte(code))
		}
		inv.Revoked = true
		updated, err := json.Marshal(inv)
		if err != nil {
			return err
		}
		return b.Put([]byte(code), updated)
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "invite code not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": code})
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	type userSummary struct {
		ID        string    `json:"id"`
		Email     string    `json:"email"`
		Name      string    `json:"name"`
		UsedBytes uint64    `json:"used_bytes"`
		CreatedAt time.Time `json:"created_at"`
	}
	var users []userSummary
	_ = s.db.View(func(tx *bbolt.Tx) error {
		usageB := tx.Bucket(bucketUserUsage)
		return tx.Bucket(bucketUsersByID).ForEach(func(_, v []byte) error {
			var u User
			if json.Unmarshal(v, &u) == nil {
				var used uint64
				if raw := usageB.Get([]byte(u.ID)); len(raw) == 8 {
					used = binary.BigEndian.Uint64(raw)
				}
				users = append(users, userSummary{
					ID:        u.ID,
					Email:     u.Email,
					Name:      u.Name,
					UsedBytes: used,
					CreatedAt: u.CreatedAt,
				})
			}
			return nil
		})
	})
	if users == nil {
		users = []userSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "users": users})
}

func normalizeInviteCode(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

func generateInviteCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // unambiguous charset
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return fmt.Sprintf("RGLX-%s-%s", string(out[:4]), string(out[4:])), nil
}

// --- Multi-Tenant Sync API v1 ---

func (s *Server) withUser(fn func(w http.ResponseWriter, r *http.Request, user User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authenticate(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		fn(w, r, user)
	}
}

func (s *Server) userSyncStore(userID string) (*rsync.Store, error) {
	s.storesMu.RLock()
	st, ok := s.userStores[userID]
	s.storesMu.RUnlock()
	if ok {
		return st, nil
	}
	s.storesMu.Lock()
	defer s.storesMu.Unlock()
	if st, ok := s.userStores[userID]; ok {
		return st, nil
	}
	st, err := rsync.OpenStore(filepath.Join(s.opts.DataDir, "users", userID))
	if err != nil {
		return nil, err
	}
	s.userStores[userID] = st
	return st, nil
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, _ *http.Request, user User) {
	st, err := s.userSyncStore(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"protocol":     "raglex-sync-v1",
		"user_id":      user.ID,
		"latest_seq":   st.LatestSeq(),
		"blob_backend": s.blobs.BackendName(),
		"used_bytes":   s.userUsedBytes(user.ID),
		"quota_bytes":  s.opts.QuotaBytes,
	})
}

func (s *Server) handleSyncPull(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	st, err := s.userSyncStore(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ops, latest, hasMore, err := st.PullOps(since, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ops == nil {
		ops = []rsync.SyncOp{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ops":        ops,
		"latest_seq": latest,
		"has_more":   hasMore,
	})
}

func (s *Server) handleSyncPush(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	st, err := s.userSyncStore(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req struct {
		Ops []rsync.SyncOp `json:"ops"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	latest, accepted, err := st.PushOps(req.Ops)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"latest_seq": latest,
		"accepted":   accepted,
	})
}

func (s *Server) handleBlobCheck(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Hashes []string `json:"hashes"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	present := []string{}
	missing := []string{}
	for _, raw := range req.Hashes {
		norm, err := rsync.ValidateSHA256(raw)
		if err != nil {
			continue
		}
		// Require per-user ownership link so users cannot probe other tenants' hashes.
		if s.userOwnsBlob(user.ID, norm) && s.blobs.Has(r.Context(), norm) {
			present = append(present, norm)
		} else {
			missing = append(missing, norm)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"present": present,
		"missing": missing,
	})
}

func (s *Server) handleBlobPresign(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Hash   string `json:"hash"`
		Method string `json:"method"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	m := strings.ToUpper(strings.TrimSpace(req.Method))
	if m != "GET" && m != "PUT" {
		m = "GET"
	}
	if m == "GET" && !s.userOwnsBlob(user.ID, req.Hash) {
		writeErr(w, http.StatusNotFound, "blob not found")
		return
	}
	u, ok := s.blobs.PresignURL(m, req.Hash, 15*time.Minute)
	writeJSON(w, http.StatusOK, map[string]any{
		"presigned": ok,
		"url":       u,
	})
}

func (s *Server) handleBlobObject(w http.ResponseWriter, r *http.Request, user User) {
	hash := strings.TrimPrefix(r.URL.Path, "/v1/blobs/")
	norm, err := rsync.ValidateSHA256(hash)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid sha256")
		return
	}
	switch r.Method {
	case http.MethodPut:
		if s.opts.QuotaBytes > 0 && !s.userOwnsBlob(user.ID, norm) {
			used := s.userUsedBytes(user.ID)
			if used >= uint64(s.opts.QuotaBytes) {
				writeErr(w, http.StatusRequestEntityTooLarge, "storage quota exceeded")
				return
			}
			if r.ContentLength > 0 && used+uint64(r.ContentLength) > uint64(s.opts.QuotaBytes) {
				writeErr(w, http.StatusRequestEntityTooLarge, "storage quota exceeded")
				return
			}
		}
		n, err := s.blobs.Put(r.Context(), norm, io.LimitReader(r.Body, maxUploadBytes))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.linkUserBlob(user.ID, norm, uint64(n)); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hash": norm, "size": n})
	case http.MethodGet, http.MethodHead:
		if !s.userOwnsBlob(user.ID, norm) {
			writeErr(w, http.StatusNotFound, "blob not found")
			return
		}
		rc, size, err := s.blobs.Open(r.Context(), norm)
		if err != nil {
			writeErr(w, http.StatusNotFound, "blob not found")
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, rc)
		}
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// --- Helpers ---

func (s *Server) authenticate(r *http.Request) (User, bool) {
	tok := extractBearer(r)
	if tok == "" {
		return User{}, false
	}
	tokHash := sha256Hex(tok)
	var user User
	var ok bool
	now := time.Now().UTC()
	_ = s.db.View(func(tx *bbolt.Tx) error {
		rawTok := tx.Bucket(bucketTokens).Get([]byte(tokHash))
		if rawTok == nil {
			return nil
		}
		var rec tokenRecord
		if err := json.Unmarshal(rawTok, &rec); err != nil {
			return nil
		}
		if !rec.ExpiresAt.IsZero() && now.After(rec.ExpiresAt) {
			return nil
		}
		rawUser := tx.Bucket(bucketUsersByID).Get([]byte(rec.UserID))
		if rawUser == nil {
			return nil
		}
		if json.Unmarshal(rawUser, &user) == nil {
			ok = true
		}
		return nil
	})
	return user, ok
}

func (s *Server) userOwnsBlob(userID, hash string) bool {
	norm, err := rsync.ValidateSHA256(hash)
	if err != nil {
		return false
	}
	var owned bool
	_ = s.db.View(func(tx *bbolt.Tx) error {
		owned = tx.Bucket(bucketUserBlobs).Get([]byte(userID+":"+norm)) != nil
		return nil
	})
	return owned
}

func (s *Server) linkUserBlob(userID, hash string, size uint64) error {
	key := []byte(userID + ":" + hash)
	return s.db.Update(func(tx *bbolt.Tx) error {
		ub := tx.Bucket(bucketUserBlobs)
		if ub.Get(key) != nil {
			return nil
		}
		var szBuf [8]byte
		binary.BigEndian.PutUint64(szBuf[:], size)
		if err := ub.Put(key, szBuf[:]); err != nil {
			return err
		}
		usageB := tx.Bucket(bucketUserUsage)
		var cur uint64
		if raw := usageB.Get([]byte(userID)); len(raw) == 8 {
			cur = binary.BigEndian.Uint64(raw)
		}
		binary.BigEndian.PutUint64(szBuf[:], cur+size)
		return usageB.Put([]byte(userID), szBuf[:])
	})
}

func (s *Server) userUsedBytes(userID string) uint64 {
	var cur uint64
	_ = s.db.View(func(tx *bbolt.Tx) error {
		if raw := tx.Bucket(bucketUserUsage).Get([]byte(userID)); len(raw) == 8 {
			cur = binary.BigEndian.Uint64(raw)
		}
		return nil
	})
	return cur
}

func extractBearer(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-Raglex-Token"))
}

func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func isValidEmail(email string) bool {
	at := strings.IndexByte(email, '@')
	if at <= 0 || at >= len(email)-3 {
		return false
	}
	domain := email[at+1:]
	return strings.Contains(domain, ".") && !strings.ContainsAny(email, " \t\r\n")
}

func hashUserPassword(password string) (saltHex, hashHex string, err error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}
	dk, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLen)
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(salt), hex.EncodeToString(dk), nil
}

func verifyUserPassword(password, saltHex, hashHex string) bool {
	salt, err := hex.DecodeString(saltHex)
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := hex.DecodeString(hashHex)
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func generateToken() (rawToken, tokenHash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := "rglx_" + hex.EncodeToString(b)
	return raw, sha256Hex(raw), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": msg})
}
