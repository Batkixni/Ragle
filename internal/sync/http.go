package sync

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

// Handler exposes Raglex Sync API v1 (/v1/sync/*, /v1/blobs/*, /v1/shares*) and
// the public share viewer (/s/{id}) over standard net/http.
type Handler struct {
	Store    *Store
	Blobs    BlobStore
	Lib      *library.Library
	Projects *project.Store
	Auth     func(r *http.Request) bool // returns true if authorized for private API routes
}

// RegisterRoutes registers all Sync API v1 + Public Share routes onto mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/sync/status", h.requireAuth(h.handleStatus))
	mux.HandleFunc("/v1/sync/pull", h.requireAuth(h.handlePull))
	mux.HandleFunc("/v1/sync/push", h.requireAuth(h.handlePush))
	mux.HandleFunc("/v1/sync/reset", h.requireAuth(h.handleReset))
	mux.HandleFunc("/v1/blobs/check", h.requireAuth(h.handleBlobCheck))
	mux.HandleFunc("/v1/blobs/presign", h.requireAuth(h.handleBlobPresign))
	mux.HandleFunc("/v1/blobs/", h.requireAuth(h.handleBlobObject))
	mux.HandleFunc("/v1/shares", h.requireAuth(h.handleShares))
	mux.HandleFunc("/v1/shares/", h.requireAuth(h.handleShareByID))
	// Public share endpoints (no bearer token required; enforces share password + expiry).
	mux.HandleFunc("/s/", h.handlePublicShare)
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.Auth != nil && !h.Auth(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (h *Handler) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeSyncJSON(w, http.StatusOK, map[string]any{
		"protocol":     "raglex-sync-v1",
		"latest_seq":   h.Store.LatestSeq(),
		"blob_backend": h.Blobs.BackendName(),
		"share_count":  len(h.Store.ListShares()),
	})
}

func (h *Handler) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.Store.ResetOps(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeSyncJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"latest_seq": 0,
	})
}

func (h *Handler) handlePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ops, latest, hasMore, err := h.Store.PullOps(since, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ops == nil {
		ops = []SyncOp{}
	}
	writeSyncJSON(w, http.StatusOK, map[string]any{
		"ops":        ops,
		"latest_seq": latest,
		"has_more":   hasMore,
	})
}

func (h *Handler) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Ops []SyncOp `json:"ops"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	latest, accepted, err := h.Store.PushOps(req.Ops)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeSyncJSON(w, http.StatusOK, map[string]any{
		"latest_seq": latest,
		"accepted":   accepted,
	})
}

func (h *Handler) handleBlobCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Hashes []string `json:"hashes"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	present := []string{}
	missing := []string{}
	for _, raw := range req.Hashes {
		norm, err := ValidateSHA256(raw)
		if err != nil {
			continue
		}
		if h.Blobs.Has(r.Context(), norm) {
			present = append(present, norm)
		} else {
			missing = append(missing, norm)
		}
	}
	writeSyncJSON(w, http.StatusOK, map[string]any{
		"present": present,
		"missing": missing,
	})
}

func (h *Handler) handleBlobPresign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Hash   string `json:"hash"`
		Method string `json:"method"` // GET or PUT
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	m := strings.ToUpper(strings.TrimSpace(req.Method))
	if m != "GET" && m != "PUT" {
		m = "GET"
	}
	u, ok := h.Blobs.PresignURL(m, req.Hash, 15*time.Minute)
	writeSyncJSON(w, http.StatusOK, map[string]any{
		"presigned": ok,
		"url":       u,
	})
}

func (h *Handler) handleBlobObject(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimPrefix(r.URL.Path, "/v1/blobs/")
	norm, err := ValidateSHA256(hash)
	if err != nil {
		http.Error(w, "invalid sha256", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		n, err := h.Blobs.Put(r.Context(), norm, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeSyncJSON(w, http.StatusOK, map[string]any{"hash": norm, "size": n})
	case http.MethodGet, http.MethodHead:
		rc, size, err := h.Blobs.Open(r.Context(), norm)
		if err != nil {
			http.Error(w, "blob not found", http.StatusNotFound)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, rc)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleShares(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeSyncJSON(w, http.StatusOK, map[string]any{"shares": h.Store.ListShares()})
	case http.MethodPost:
		var req CreateShareRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		link, err := h.Store.CreateShare(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeSyncJSON(w, http.StatusCreated, map[string]any{
			"share": link.Sanitized(),
			"path":  "/s/" + link.ID,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleShareByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/shares/")
	if r.Method == http.MethodDelete {
		_ = h.Store.DeleteShare(id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (h *Handler) handlePublicShare(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	isAPI := len(parts) >= 2 && parts[1] == "api"
	isMedia := len(parts) >= 3 && (parts[1] == "thumb" || parts[1] == "file")

	link, ok := h.Store.GetShare(id, !isAPI && !isMedia)
	if !ok {
		http.Error(w, "Share link not found", http.StatusNotFound)
		return
	}
	if link.Expired(time.Now().UTC()) {
		http.Error(w, "Share link expired", http.StatusGone)
		return
	}
	pw := r.URL.Query().Get("p")
	if pw == "" {
		pw = r.Header.Get("X-Raglex-Share-Password")
	}
	if !link.CheckPassword(pw) {
		if isAPI || isMedia {
			http.Error(w, "password required", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Raglex Share</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#0e0e11;color:#fafafa;font-family:system-ui,sans-serif}.card{background:#18181d;border:1px solid rgba(255,255,255,.1);padding:28px;border-radius:16px;width:320px;box-sizing:border-box}input{width:100%%;padding:10px 12px;border-radius:10px;border:1px solid rgba(255,255,255,.15);background:#111115;color:#fff;margin:12px 0;box-sizing:border-box}button{width:100%%;padding:10px;border-radius:10px;border:0;background:#ededed;color:#111;font-weight:600;cursor:pointer}</style></head><body><form class="card" method="get"><h3 style="margin:0 0 6px">%s</h3><p style="margin:0;color:#a1a1aa;font-size:13px">此分享連結受密碼保護</p><input type="password" name="p" placeholder="輸入存取密碼…" autofocus required><button type="submit">解鎖檢視</button></form></body></html>`, html.EscapeString(link.Title))
		return
	}

	assets := h.resolveShareAssets(link)
	if isMedia {
		assetID := parts[2]
		for _, a := range assets {
			if a.ID != assetID {
				continue
			}
			if parts[1] == "thumb" && a.ThumbnailPath != "" {
				http.ServeFile(w, r, a.ThumbnailPath)
				return
			}
			if parts[1] == "file" && link.AllowDownload && a.Path != "" {
				http.ServeFile(w, r, a.Path)
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	if isAPI {
		writeSyncJSON(w, http.StatusOK, map[string]any{
			"share":  link.Sanitized(),
			"assets": assets,
		})
		return
	}

	// Render sleek public read-only gallery viewer HTML.
	var itemsHTML strings.Builder
	qSuffix := ""
	if pw != "" {
		qSuffix = "?p=" + html.EscapeString(pw)
	}
	for _, a := range assets {
		thumbURL := fmt.Sprintf("/s/%s/thumb/%s%s", link.ID, a.ID, qSuffix)
		dlBtn := ""
		if link.AllowDownload {
			dlBtn = fmt.Sprintf(`<a class="dl" href="/s/%s/file/%s%s" download>下載原檔</a>`, link.ID, a.ID, qSuffix)
		}
		itemsHTML.WriteString(fmt.Sprintf(`<div class="card"><img loading="lazy" src="%s" alt="%s"><div class="meta"><span>%s</span>%s</div></div>`,
			thumbURL, html.EscapeString(a.Label()), html.EscapeString(a.Label()), dlBtn))
	}
	title := link.Title
	if title == "" {
		title = "Raglex Shared Collection"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s — Raglex</title><style>body{margin:0;background:#0e0e11;color:#fafafa;font-family:system-ui,-apple-system,sans-serif}header{padding:20px 28px;border-bottom:1px solid rgba(255,255,255,.08);display:flex;align-items:center;justify-content:space-between}main{padding:24px 28px;column-count:4;column-gap:16px}@media(max-width:960px){main{column-count:2}}@media(max-width:540px){main{column-count:1}}.card{break-inside:avoid;margin-bottom:16px;background:#18181d;border-radius:12px;overflow:hidden;border:1px solid rgba(255,255,255,.08)}.card img{width:100%%;display:block}.meta{padding:10px 12px;display:flex;justify-content:space-between;align-items:center;font-size:12px;color:#d4d4d8}.dl{color:#fafafa;text-decoration:none;background:rgba(255,255,255,.1);padding:4px 8px;border-radius:6px}</style></head><body><header><div><strong>%s</strong> <span style="color:#71717a;font-size:12px;margin-left:8px">%d 個素材</span></div><span style="color:#71717a;font-size:12px">Shared via Raglex</span></header><main>%s</main></body></html>`,
		html.EscapeString(title), html.EscapeString(title), len(assets), itemsHTML.String())
}

func (h *Handler) resolveShareAssets(link ShareLink) []model.Asset {
	if h.Lib == nil {
		return nil
	}
	switch link.Kind {
	case "asset":
		if a, err := h.Lib.Get(link.TargetID); err == nil {
			return []model.Asset{a}
		}
	case "project":
		if h.Projects != nil {
			if assets, err := h.Projects.Assets(link.TargetID); err == nil {
				return assets
			}
		}
	case "board":
		if h.Projects != nil {
			if items, err := h.Projects.LoadPlacements(link.TargetID); err == nil {
				seen := map[string]bool{}
				var out []model.Asset
				for _, it := range items {
					if it.AssetID == "" || seen[it.AssetID] {
						continue
					}
					seen[it.AssetID] = true
					if a, err := h.Lib.Get(it.AssetID); err == nil {
						out = append(out, a)
					}
				}
				return out
			}
		}
	}
	return nil
}

func writeSyncJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
