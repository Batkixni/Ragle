package cloud

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/share"
	rsync "raglex/internal/sync"
)

// PublicShareRecord is a Cloud-hosted public share (folder or project) that any
// Raglex user—registered or unregistered—can download using its share code.
type PublicShareRecord struct {
	Code          string          `json:"code"`
	OwnerUserID   string          `json:"owner_user_id"`
	OwnerName     string          `json:"owner_name"`
	Kind          string          `json:"kind"` // "folder" or "project"
	Title         string          `json:"title"`
	AssetCount    int             `json:"asset_count"`
	FolderCount   int             `json:"folder_count"`
	FileCount     int             `json:"file_count"`
	TotalBytes    int64           `json:"total_bytes"`
	AllowedHashes map[string]bool `json:"allowed_hashes,omitempty"`
	Bundle        share.Bundle    `json:"bundle"`
	Downloads     int             `json:"downloads"`
	Revoked       bool            `json:"revoked,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	ExpiresAt     time.Time       `json:"expires_at,omitempty"`
}

// PublicShareSummary omits the heavy bundle & hash map when listing shares.
type PublicShareSummary struct {
	Code        string    `json:"code"`
	OwnerName   string    `json:"owner_name"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	AssetCount  int       `json:"asset_count"`
	FolderCount int       `json:"folder_count"`
	FileCount   int       `json:"file_count"`
	TotalBytes  int64     `json:"total_bytes"`
	Downloads   int       `json:"downloads"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

func (s *Server) publicShareURL(code string) string {
	return fmt.Sprintf("https://%s/s/%s", s.opts.PublicHost, code)
}

func (s *Server) toPublicShareSummary(rec PublicShareRecord) PublicShareSummary {
	return PublicShareSummary{
		Code:        rec.Code,
		OwnerName:   rec.OwnerName,
		Kind:        rec.Kind,
		Title:       rec.Title,
		AssetCount:  rec.AssetCount,
		FolderCount: rec.FolderCount,
		FileCount:   rec.FileCount,
		TotalBytes:  rec.TotalBytes,
		Downloads:   rec.Downloads,
		URL:         s.publicShareURL(rec.Code),
		CreatedAt:   rec.CreatedAt,
		ExpiresAt:   rec.ExpiresAt,
	}
}

// NormalizePublicShareCode normalizes user-typed codes or URLs like
// "https://cloud.raglex.sorai.tw/s/SHR-ABCD-1234" or "shr abcd 1234" -> "SHR-ABCD-1234".
func NormalizePublicShareCode(raw string) string {
	s := strings.TrimSpace(raw)
	if idx := strings.LastIndex(s, "/s/"); idx >= 0 {
		s = s[idx+3:]
	}
	if q := strings.IndexAny(s, "?#"); q >= 0 {
		s = s[:q]
	}
	s = strings.Trim(s, "/ \t\r\n")
	s = strings.ToUpper(s)
	return s
}

func generatePublicShareCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return fmt.Sprintf("SHR-%s-%s", string(out[:4]), string(out[4:])), nil
}

// handlePublicSharesCollection handles authenticated POST (publish) and GET (list own shares).
func (s *Server) handlePublicSharesCollection(w http.ResponseWriter, r *http.Request, user User) {
	switch r.Method {
	case http.MethodGet:
		var list []PublicShareSummary
		_ = s.db.View(func(tx *bbolt.Tx) error {
			return tx.Bucket(bucketPublicShares).ForEach(func(_, v []byte) error {
				var rec PublicShareRecord
				if json.Unmarshal(v, &rec) == nil && rec.OwnerUserID == user.ID && !rec.Revoked {
					list = append(list, s.toPublicShareSummary(rec))
				}
				return nil
			})
		})
		sort.Slice(list, func(i, j int) bool {
			return list[i].CreatedAt.After(list[j].CreatedAt)
		})
		if list == nil {
			list = []PublicShareSummary{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "shares": list})

	case http.MethodPost:
		var req struct {
			Bundle      share.Bundle `json:"bundle"`
			ExpiresDays int          `json:"expires_days"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON payload")
			return
		}
		kind := strings.ToLower(strings.TrimSpace(req.Bundle.Kind))
		if kind == "" {
			kind = "project"
		}
		if kind != "folder" && kind != "project" {
			writeErr(w, http.StatusBadRequest, "invalid share kind")
			return
		}
		req.Bundle.Kind = kind
		title := strings.TrimSpace(req.Bundle.DisplayName())
		if title == "" {
			title = "Shared Collection"
		}

		// Security check: every file hash included in the public share MUST be owned by this user
		// and present in our blob store so no one can expose another tenant's private blobs.
		allowed := make(map[string]bool, len(req.Bundle.Files))
		var totalBytes int64
		for i, f := range req.Bundle.Files {
			norm, err := rsync.ValidateSHA256(f.Hash)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid file sha256 in bundle")
				return
			}
			if !s.userOwnsBlob(user.ID, norm) || !s.blobs.Has(r.Context(), norm) {
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("missing blob %s — upload files before publishing", norm[:12]))
				return
			}
			req.Bundle.Files[i].Hash = norm
			allowed[norm] = true
			totalBytes += f.Size
		}

		ownerName := strings.TrimSpace(user.Name)
		if ownerName == "" {
			ownerName = user.Email
		}

		var code string
		for attempt := 0; attempt < 8; attempt++ {
			cand, err := generatePublicShareCode()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			exists := false
			_ = s.db.View(func(tx *bbolt.Tx) error {
				exists = tx.Bucket(bucketPublicShares).Get([]byte(cand)) != nil
				return nil
			})
			if !exists {
				code = cand
				break
			}
		}
		if code == "" {
			writeErr(w, http.StatusInternalServerError, "failed to allocate share code")
			return
		}

		now := time.Now().UTC()
		rec := PublicShareRecord{
			Code:          code,
			OwnerUserID:   user.ID,
			OwnerName:     ownerName,
			Kind:          kind,
			Title:         title,
			AssetCount:    len(req.Bundle.Assets),
			FolderCount:   len(req.Bundle.Folders),
			FileCount:     len(req.Bundle.Files),
			TotalBytes:    totalBytes,
			AllowedHashes: allowed,
			Bundle:        req.Bundle,
			CreatedAt:     now,
		}
		if req.ExpiresDays > 0 {
			rec.ExpiresAt = now.Add(time.Duration(req.ExpiresDays) * 24 * time.Hour)
		}

		err := s.db.Update(func(tx *bbolt.Tx) error {
			raw, err := json.Marshal(rec)
			if err != nil {
				return err
			}
			return tx.Bucket(bucketPublicShares).Put([]byte(code), raw)
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"ok":    true,
			"code":  code,
			"url":   s.publicShareURL(code),
			"share": s.toPublicShareSummary(rec),
		})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handlePublicShareByCode handles:
//   - GET /api/v1/public-shares/{code} (PUBLIC, no auth needed)
//   - GET /api/v1/public-shares/{code}/blobs/{sha256} (PUBLIC, scoped to AllowedHashes)
//   - DELETE /api/v1/public-shares/{code} (OWNER auth required)
func (s *Server) handlePublicShareByCode(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/public-shares/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, "missing share code")
		return
	}
	code := NormalizePublicShareCode(parts[0])

	// Blob download subroute: /api/v1/public-shares/{code}/blobs/{sha256}
	if len(parts) == 3 && parts[1] == "blobs" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		rec, ok := s.loadActivePublicShare(code)
		if !ok {
			writeErr(w, http.StatusNotFound, "分享碼不存在或已失效")
			return
		}
		norm, err := rsync.ValidateSHA256(parts[2])
		if err != nil || !rec.AllowedHashes[norm] {
			writeErr(w, http.StatusNotFound, "blob not in this share")
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
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, rc)
		}
		return
	}

	if len(parts) != 1 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		rec, ok := s.loadActivePublicShare(code)
		if !ok {
			writeErr(w, http.StatusNotFound, "分享碼不存在、已過期或已被撤銷")
			return
		}
		// Increment download counter asynchronously-safe inside bbolt.
		_ = s.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketPublicShares)
			raw := b.Get([]byte(code))
			if raw == nil {
				return nil
			}
			var cur PublicShareRecord
			if json.Unmarshal(raw, &cur) != nil {
				return nil
			}
			cur.Downloads++
			rec.Downloads = cur.Downloads
			upd, err := json.Marshal(cur)
			if err != nil {
				return err
			}
			return b.Put([]byte(code), upd)
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"share":  s.toPublicShareSummary(rec),
			"bundle": rec.Bundle,
		})

	case http.MethodDelete:
		user, ok := s.authenticate(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var notFound, forbidden bool
		err := s.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketPublicShares)
			raw := b.Get([]byte(code))
			if raw == nil {
				notFound = true
				return nil
			}
			var rec PublicShareRecord
			if err := json.Unmarshal(raw, &rec); err != nil {
				return b.Delete([]byte(code))
			}
			if rec.OwnerUserID != user.ID {
				forbidden = true
				return nil
			}
			rec.Revoked = true
			upd, err := json.Marshal(rec)
			if err != nil {
				return err
			}
			return b.Put([]byte(code), upd)
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if notFound {
			writeErr(w, http.StatusNotFound, "share not found")
			return
		}
		if forbidden {
			writeErr(w, http.StatusForbidden, "only the owner can revoke this share")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": code})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) loadActivePublicShare(code string) (PublicShareRecord, bool) {
	var rec PublicShareRecord
	var ok bool
	_ = s.db.View(func(tx *bbolt.Tx) error {
		raw := tx.Bucket(bucketPublicShares).Get([]byte(code))
		if raw == nil {
			return nil
		}
		if json.Unmarshal(raw, &rec) != nil {
			return nil
		}
		if rec.Revoked {
			return nil
		}
		if !rec.ExpiresAt.IsZero() && time.Now().UTC().After(rec.ExpiresAt) {
			return nil
		}
		ok = true
		return nil
	})
	return rec, ok
}

// handlePublicShareLanding serves a clean browser page at /s/{code} so users
// who click a shared link see the title, item count, and a 1-click copyable code.
func (s *Server) handlePublicShareLanding(w http.ResponseWriter, r *http.Request) {
	code := NormalizePublicShareCode(strings.TrimPrefix(r.URL.Path, "/s/"))
	rec, ok := s.loadActivePublicShare(code)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, renderEmailShell("分享碼不存在或已失效", "此公開分享連結可能已被原作者撤銷或已過期。", "", "", "", "Raglex Cloud"))
		return
	}
	kindLabel := "專案"
	if rec.Kind == "folder" {
		kindLabel = "資料夾"
	}
	summary := fmt.Sprintf("由 %s 透過 Raglex Cloud 公開分享 · 包含 %d 個素材（無需註冊帳號即可在 Raglex 匯入使用）", rec.OwnerName, rec.AssetCount)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-Hant">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s · Raglex 公開分享</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #0c0c0f; color: #f4f4f5; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
  .card { width: min(460px, 92vw); padding: 32px; border-radius: 18px; background: #141419; border: 1px solid rgba(255,255,255,.09); box-shadow: 0 24px 60px rgba(0,0,0,.5); }
  .pill { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 600; color: #ff7854; background: rgba(255,92,51,.14); padding: 4px 11px; border-radius: 999px; margin-bottom: 14px; }
  h1 { margin: 0 0 8px; font-size: 22px; letter-spacing: -.02em; }
  p { margin: 0 0 20px; font-size: 13.5px; line-height: 1.65; color: #a1a1aa; }
  .code-box { display: flex; align-items: center; justify-content: space-between; gap: 12px; background: #0f0f13; border: 1px solid rgba(255,255,255,.1); border-radius: 12px; padding: 14px 16px; margin-bottom: 18px; }
  .code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 20px; font-weight: 700; letter-spacing: .08em; color: #ffffff; }
  button { cursor: pointer; border: 0; border-radius: 9px; padding: 9px 14px; font-size: 13px; font-weight: 600; background: #ff5c33; color: #fff; }
  button:hover { background: #ff6e4a; }
  .steps { font-size: 12.5px; color: #a1a1aa; line-height: 1.75; border-top: 1px solid rgba(255,255,255,.08); padding-top: 14px; }
</style>
</head>
<body>
  <div class="card">
    <div class="pill">Raglex 公開%s分享</div>
    <h1>%s</h1>
    <p>%s</p>
    <div class="code-box">
      <span class="code" id="code">%s</span>
      <button onclick="navigator.clipboard.writeText('%s');this.textContent='已複製';">複製分享碼</button>
    </div>
    <div class="steps">
      <strong>如何在 Raglex 中下載使用：</strong><br>
      1. 開啟 Raglex 桌面版（不需登入或註冊帳號）<br>
      2. 點擊右上角「+ 匯入」→「輸入分享碼 / 接收分享…」<br>
      3. 貼上上方分享碼即可自動下載所有素材與資料夾結構
    </div>
  </div>
</body>
</html>`,
		html.EscapeString(rec.Title),
		html.EscapeString(kindLabel),
		html.EscapeString(rec.Title),
		html.EscapeString(summary),
		html.EscapeString(rec.Code),
		html.EscapeString(rec.Code),
	)
}
