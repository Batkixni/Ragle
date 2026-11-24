// Package server provides Raglex's unified HTTP server shared by both the
// desktop app's built-in server and the standalone headless VPS CLI
// (cmd/raglex-server). It serves:
//   - Local Companion REST API (/api/v1/...) for browser extensions, plugins, & MCP
//   - Web Gallery & Interactive Board Viewer (/, /project/..., /board/...)
//   - Embedded WebRTC Pairing Signaling (/healthz, /v1/rooms/...)
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
	"raglex/internal/share"
	rsync "raglex/internal/sync"
	"raglex/internal/thumbnail"
)

const (
	DefaultPort = 47595
	maxImportMB = 64 << 20
)

// Options configures a Raglex server instance.
type Options struct {
	Library     *library.Library // optional: when nil, runs signaling-only mode
	Projects    *project.Store   // optional
	Token       string           // optional bearer/header token for mutating API routes
	SignalTTL   time.Duration
	OnChange    func() // called after mutating API calls so live desktop UI reloads
	ExtraRoutes func(mux *http.ServeMux)
}

// Server hosts the unified HTTP API, Web/Mobile PWA Viewer, Sync API v1,
// Public Share Links, and WebRTC/E2E Relay Signaling service.
type Server struct {
	lib         *library.Library
	projects    *project.Store
	token       string
	signal      *share.SignalingServer
	syncStore   *rsync.Store
	syncHandler *rsync.Handler
	onChange    func()
	mux         *http.ServeMux

	mu       sync.Mutex
	srv      *http.Server
	listener net.Listener
	addr     string
}

// GenerateToken returns a random 24-hex-char pairing token.
func GenerateToken() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// New builds a configured Server ready to Start or be served via Handler().
func New(opts Options) *Server {
	ttl := opts.SignalTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	s := &Server{
		lib:      opts.Library,
		projects: opts.Projects,
		token:    strings.TrimSpace(opts.Token),
		signal:   share.NewSignalingServer(ttl),
		onChange: opts.OnChange,
		mux:      http.NewServeMux(),
	}
	if s.lib != nil {
		if st, err := rsync.OpenStore(s.lib.DB.Root()); err == nil {
			blobs, berr := rsync.NewLocalBlobStore(s.lib.DB.Root(), rsync.R2FromEnv())
			if berr == nil {
				s.syncStore = st
				s.syncHandler = &rsync.Handler{
					Store:    st,
					Blobs:    blobs,
					Lib:      s.lib,
					Projects: s.projects,
					Auth:     s.isAuthorized,
				}
			} else {
				_ = st.Close()
			}
		}
	}
	s.registerRoutes()
	if opts.ExtraRoutes != nil {
		opts.ExtraRoutes(s.mux)
	}
	return s
}

// CreatePublicShare creates a public or password-protected web share link directly
// on this server's share store and returns its full relative path ("/s/<id>").
func (s *Server) CreatePublicShare(req rsync.CreateShareRequest) (rsync.ShareLink, string, error) {
	if s == nil || s.syncStore == nil {
		return rsync.ShareLink{}, "", errors.New("sync/share store unavailable")
	}
	link, err := s.syncStore.CreateShare(req)
	if err != nil {
		return rsync.ShareLink{}, "", err
	}
	return link.Sanitized(), "/s/" + link.ID, nil
}

// Handler returns the root HTTP handler (with CORS middleware).
func (s *Server) Handler() http.Handler {
	return s.withCORS(s.mux)
}

// Start binds to addr (e.g. "127.0.0.1:41595" or ":8787") and begins serving.
func (s *Server) Start(addr string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		return s.addr, nil
	}
	if strings.TrimSpace(addr) == "" {
		addr = fmt.Sprintf("127.0.0.1:%d", DefaultPort)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.listener = ln
	s.srv = srv
	s.addr = ln.Addr().String()
	go func() {
		_ = srv.Serve(ln)
	}()
	return s.addr, nil
}

// Stop gracefully shuts down the HTTP server and signaling reaper.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.listener = nil
	s.addr = ""
	s.mu.Unlock()

	if s.signal != nil {
		s.signal.Close()
	}
	if s.syncStore != nil {
		_ = s.syncStore.Close()
		s.syncStore = nil
	}
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// Running reports whether the server is currently listening.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.srv != nil
}

// Addr returns the bound TCP address (host:port) when running.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// LocalURL returns http://127.0.0.1:<port> for the bound server.
func (s *Server) LocalURL() string {
	addr := s.Addr()
	if addr == "" {
		return ""
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	return "http://127.0.0.1:" + port
}

// LANURL returns http://<lan-ipv4>:<port> if a non-loopback IPv4 interface is found.
func (s *Server) LANURL() string {
	addr := s.Addr()
	if addr == "" {
		return ""
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	ip := detectLANIP()
	if ip == "" {
		return ""
	}
	return "http://" + ip + ":" + port
}

// Token returns the configured companion/API token.
func (s *Server) Token() string { return s.token }

func detectLANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipNet.IP.To4()
			if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() {
				continue
			}
			return v4.String()
		}
	}
	return ""
}

func (s *Server) notifyChanged() {
	if s.onChange != nil {
		s.onChange()
	}
}

func (s *Server) registerRoutes() {
	// Embedded WebRTC Signaling routes (drop-in compatible with cmd/raglex-signal).
	s.mux.Handle("/healthz", s.signal)
	s.mux.Handle("/v1/rooms", s.signal)
	s.mux.Handle("/v1/rooms/", s.signal)

	// Companion REST API (all private library reads & writes require token when configured).
	s.mux.HandleFunc("/api/v1/health", s.handleHealth)
	s.mux.HandleFunc("/api/v1/assets", s.requireToken(s.handleAssets))
	s.mux.HandleFunc("/api/v1/assets/", s.requireToken(s.handleAssetByID))
	s.mux.HandleFunc("/api/v1/similar/", s.requireToken(s.handleSimilar))
	s.mux.HandleFunc("/api/v1/tags", s.requireToken(s.handleTags))
	s.mux.HandleFunc("/api/v1/projects", s.requireToken(s.handleProjects))
	s.mux.HandleFunc("/api/v1/boards", s.requireToken(s.handleBoards))
	s.mux.HandleFunc("/api/v1/boards/", s.requireToken(s.handleBoardByID))
	s.mux.HandleFunc("/api/v1/import/base64", s.requireToken(s.handleImportBase64))
	s.mux.HandleFunc("/api/v1/import/url", s.requireToken(s.handleImportURL))
	s.mux.HandleFunc("/api/v1/import/source", s.requireToken(s.handleImportSource))
	s.mux.HandleFunc("/api/v1/stacks", s.requireToken(s.handleCreateStack))
	s.mux.HandleFunc("/api/v1/rename/preview", s.requireToken(s.handleRenamePreview))
	s.mux.HandleFunc("/api/v1/rename/apply", s.requireToken(s.handleRenameApply))

	// Sync API v1 (Oplog + SHA-256 Blobs + Public Share Links).
	if s.syncHandler != nil {
		s.syncHandler.RegisterRoutes(s.mux)
	}

	// Mobile-First PWA (/mobile, /manifest.webmanifest, /sw.js).
	s.registerPWARoutes()

	// Web Gallery & Interactive Board Viewer (protected by HttpOnly token cookie / unlock gate).
	s.mux.HandleFunc("/", s.requireWebAuth(s.handleWebIndex))
	s.mux.HandleFunc("/board/", s.requireWebAuth(s.handleWebBoard))
	s.mux.HandleFunc("/project/", s.requireWebAuth(s.handleWebProject))
}

func isAllowedCORSOrigin(origin, reqHost string) bool {
	if origin == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(origin))
	if strings.HasPrefix(lower, "chrome-extension://") ||
		strings.HasPrefix(lower, "moz-extension://") ||
		strings.HasPrefix(lower, "safari-web-extension://") {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	if reqHost != "" {
		rh := strings.ToLower(reqHost)
		if hostOnly, _, err := net.SplitHostPort(rh); err == nil {
			rh = hostOnly
		}
		if h == rh {
			return true
		}
	}
	return false
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := isAllowedCORSOrigin(origin, r.Host)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Raglex-Token, X-Raglex-Share-Password")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		if r.Method == http.MethodOptions {
			if !allowed && origin != "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const sessionCookieName = "raglex_token"

func (s *Server) tokenMatches(got string) bool {
	if s.token == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *Server) isAuthorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	got := strings.TrimSpace(r.Header.Get("X-Raglex-Token"))
	if got == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			got = strings.TrimSpace(auth[7:])
		}
	}
	if got == "" {
		got = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if got == "" {
		if ck, err := r.Cookie(sessionCookieName); err == nil {
			got = strings.TrimSpace(ck.Value)
		}
	}
	return s.tokenMatches(got)
}

func (s *Server) setSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 3600,
	})
}

func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthorized(r) {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized: missing or invalid token")
			return
		}
		next(w, r)
	}
}

// requireWebAuth protects HTML pages (/, /board/, /project/, /mobile) when a
// server token is configured. Opening /?token=<tok> or submitting the unlock
// form sets an HttpOnly SameSite=Strict cookie so assets & PWA calls succeed
// while cross-site pages and unauthenticated LAN peers are blocked.
func (s *Server) requireWebAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			next(w, r)
			return
		}
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			if tok := strings.TrimSpace(r.FormValue("token")); s.tokenMatches(tok) {
				s.setSessionCookie(w)
				http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
				return
			}
		}
		if qTok := strings.TrimSpace(r.URL.Query().Get("token")); qTok != "" && s.tokenMatches(qTok) {
			s.setSessionCookie(w)
			clean := *r.URL
			q := clean.Query()
			q.Del("token")
			clean.RawQuery = q.Encode()
			http.Redirect(w, r, clean.RequestURI(), http.StatusSeeOther)
			return
		}
		if s.isAuthorized(r) {
			next(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Raglex — Access Key</title><style>body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#0e0e11;color:#fafafa;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}form{width:340px;max-width:90vw;background:#18181d;border:1px solid rgba(255,255,255,.1);border-radius:14px;padding:24px;display:flex;flex-direction:column;gap:12px}h2{margin:0;font-size:16px}p{margin:0;font-size:12px;color:#a1a1aa;line-height:1.5}input{background:#111115;border:1px solid rgba(255,255,255,.12);border-radius:8px;padding:10px 12px;color:#fff;font-size:14px;outline:none}button{background:#FF5C33;color:#fff;border:0;border-radius:8px;padding:10px;font-weight:600;cursor:pointer}</style></head><body><form method="POST"><h2>Raglex Access Key</h2><p>Enter your Raglex Server Access Key (from Preferences &rarr; Sharing) to view this library.</p><input type="password" name="token" placeholder="Access Key" autofocus required><button type="submit">Unlock</button></form></body></html>`))
	}
}

func (s *Server) requireLibrary(w http.ResponseWriter) bool {
	if s.lib == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "library not mounted (signaling-only mode)")
		return false
	}
	return true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"ok":        true,
		"app":       "raglex",
		"signaling": true,
		"library":   s.lib != nil,
	}
	if s.isAuthorized(r) {
		if s.lib != nil {
			if assets, err := s.lib.List(library.Filter{}); err == nil {
				resp["assets"] = len(assets)
			}
			if tags, err := s.lib.Tags(); err == nil {
				resp["tags"] = len(tags)
			}
		}
		if s.projects != nil {
			if projs, err := s.projects.List(); err == nil {
				resp["projects"] = len(projs)
			}
			if sbs, err := s.projects.StandaloneBoards(); err == nil {
				resp["standalone_boards"] = len(sbs)
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type apiAsset struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Path        string               `json:"path"`
	StorageMode string               `json:"storage_mode"`
	Mime        string               `json:"mime"`
	Width       int                  `json:"width"`
	Height      int                  `json:"height"`
	Duration    float64              `json:"duration,omitempty"`
	Rating      int                  `json:"rating"`
	Missing     bool                 `json:"missing"`
	SourceURL   string               `json:"source_url,omitempty"`
	SourceTitle string               `json:"source_title,omitempty"`
	Notes       string               `json:"notes,omitempty"`
	Palette     []model.PaletteColor `json:"palette,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	ThumbURL    string               `json:"thumb_url"`
	FileURL     string               `json:"file_url"`
	CreatedAt   time.Time            `json:"created_at"`
}

func toAPIAsset(a model.Asset, tagNames []string) apiAsset {
	return apiAsset{
		ID:          a.ID,
		Name:        a.Label(),
		Path:        a.Path,
		StorageMode: a.StorageMode,
		Mime:        a.Mime,
		Width:       a.Width,
		Height:      a.Height,
		Duration:    a.Duration,
		Rating:      a.Rating,
		Missing:     a.Missing,
		SourceURL:   a.SourceURL,
		SourceTitle: a.SourceTitle,
		Notes:       a.Notes,
		Palette:     a.Palette,
		Tags:        tagNames,
		ThumbURL:    "/api/v1/assets/" + a.ID + "/thumb",
		FileURL:     "/api/v1/assets/" + a.ID + "/file",
		CreatedAt:   a.CreatedAt,
	}
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	tagID := strings.TrimSpace(r.URL.Query().Get("tag"))
	colorHex := strings.TrimSpace(r.URL.Query().Get("color"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	var assets []model.Asset
	var err error
	if colorHex != "" {
		hits, cerr := s.lib.SearchByColor(colorHex, 0.25)
		err = cerr
		for _, h := range hits {
			assets = append(assets, h.Asset)
		}
	} else {
		assets, err = s.lib.List(library.Filter{TagID: tagID})
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]apiAsset, 0, len(assets))
	for _, a := range assets {
		if q != "" {
			hay := strings.ToLower(a.Label() + " " + a.Notes + " " + a.SourceTitle + " " + a.SourceURL)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		var tnames []string
		if tgs, err := s.lib.AssetTags(a.ID); err == nil {
			for _, tg := range tgs {
				tnames = append(tnames, tg.Name)
			}
		}
		out = append(out, toAPIAsset(a, tnames))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": out, "count": len(out)})
}

func (s *Server) handleAssetByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/assets/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONError(w, http.StatusNotFound, "missing asset id")
		return
	}
	id := parts[0]
	asset, err := s.lib.Get(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "asset not found")
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "file":
			if asset.Missing || asset.Path == "" {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, asset.Path)
			return
		case "thumb":
			p := asset.ThumbnailPath
			if p == "" {
				p = asset.Path
			}
			if p == "" {
				http.NotFound(w, r)
				return
			}
			if _, err := os.Stat(p); err != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, p)
			return
		}
	}
	var tnames []string
	if tgs, err := s.lib.AssetTags(asset.ID); err == nil {
		for _, tg := range tgs {
			tnames = append(tnames, tg.Name)
		}
	}
	writeJSON(w, http.StatusOK, toAPIAsset(asset, tnames))
}

func (s *Server) handleSimilar(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/similar/"), "/")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing asset id")
		return
	}
	hits, err := s.lib.FindSimilarByAsset(id, 0.5, 40)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	type hitResp struct {
		Asset      apiAsset `json:"asset"`
		Similarity float64  `json:"similarity"`
	}
	out := make([]hitResp, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitResp{Asset: toAPIAsset(h.Asset, nil), Similarity: h.Similarity})
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": out})
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		tags, err := s.lib.Tags()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
	case http.MethodPost:
		s.requireToken(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Name     string `json:"name"`
				ParentID string `json:"parent_id"`
				Color    string `json:"color"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			tg, err := s.lib.CreateTagWithParent(req.Name, req.ParentID)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			if req.Color != "" {
				_ = s.lib.SetTagColor(tg.ID, req.Color)
				tg.Color = req.Color
			}
			s.notifyChanged()
			writeJSON(w, http.StatusCreated, tg)
		})(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects store unavailable")
		return
	}
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	projs, err := s.projects.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projs})
}

func (s *Server) handleBoards(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects store unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		standalone, err := s.projects.StandaloneBoards()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		all := append([]model.Board(nil), standalone...)
		if projs, err := s.projects.List(); err == nil {
			for _, p := range projs {
				if bs, err := s.projects.Boards(p.ID); err == nil {
					all = append(all, bs...)
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"boards": all, "standalone": standalone})
	case http.MethodPost:
		s.requireToken(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Name      string   `json:"name"`
				ProjectID string   `json:"project_id"`
				AssetIDs  []string `json:"asset_ids"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			if strings.TrimSpace(req.Name) == "" {
				req.Name = "Untitled Board"
			}
			var b model.Board
			var err error
			if req.ProjectID != "" {
				b, err = s.projects.CreateBoard(req.ProjectID, req.Name)
			} else {
				b, err = s.projects.CreateStandaloneBoard(req.Name)
			}
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			if len(req.AssetIDs) > 0 {
				_, _ = s.projects.AddAssetsToBoard(b.ID, req.AssetIDs)
			}
			s.notifyChanged()
			writeJSON(w, http.StatusCreated, b)
		})(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleBoardByID(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects store unavailable")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/boards/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONError(w, http.StatusNotFound, "missing board id")
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "assets" && r.Method == http.MethodPost {
		s.requireToken(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				AssetIDs []string `json:"asset_ids"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			n, err := s.projects.AddAssetsToBoard(id, req.AssetIDs)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.notifyChanged()
			writeJSON(w, http.StatusOK, map[string]any{"added": n})
		})(w, r)
		return
	}
	b, err := s.projects.GetBoard(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "board not found")
		return
	}
	placements, _ := s.projects.LoadPlacements(id)
	writeJSON(w, http.StatusOK, map[string]any{"board": b, "placements": placements})
}

func (s *Server) handleImportBase64(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Filename    string   `json:"filename"`
		DataBase64  string   `json:"data_base64"`
		SourceURL   string   `json:"source_url"`
		SourceTitle string   `json:"source_title"`
		Notes       string   `json:"notes"`
		TagIDs      []string `json:"tag_ids"`
		ProjectID   string   `json:"project_id"`
		BoardID     string   `json:"board_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxImportMB)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	rawStr := strings.TrimSpace(req.DataBase64)
	if idx := strings.Index(rawStr, ","); idx != -1 && strings.HasPrefix(rawStr, "data:") {
		rawStr = rawStr[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(rawStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid base64 payload")
		return
	}
	asset, added, err := s.lib.ImportBytes(req.Filename, data, req.TagIDs, req.SourceURL, req.SourceTitle)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Notes != "" {
		asset, _ = s.lib.UpdateAssetMeta(asset.ID, req.Notes, req.SourceURL, req.SourceTitle)
	}
	s.attachImportedDestinations([]string{asset.ID}, req.ProjectID, req.BoardID)
	if _, err := s.lib.EnsureVisualFeatures(asset.ID); err == nil {
		asset, _ = s.lib.Get(asset.ID)
	}
	s.notifyChanged()
	writeJSON(w, http.StatusCreated, map[string]any{
		"added": added,
		"asset": toAPIAsset(asset, nil),
	})
}

func (s *Server) attachImportedDestinations(assetIDs []string, projectID, boardID string) {
	if s.projects == nil || len(assetIDs) == 0 {
		return
	}
	if pid := strings.TrimSpace(projectID); pid != "" {
		for _, id := range assetIDs {
			_ = s.projects.Reference(pid, id)
		}
	}
	if bid := strings.TrimSpace(boardID); bid != "" {
		_, _ = s.projects.AddAssetsToBoard(bid, assetIDs)
	}
}

func (s *Server) handleImportURL(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		URL         string   `json:"url"`
		Filename    string   `json:"filename"`
		SourceURL   string   `json:"source_url"`
		SourceTitle string   `json:"source_title"`
		Notes       string   `json:"notes"`
		TagIDs      []string `json:"tag_ids"`
		ProjectID   string   `json:"project_id"`
		BoardID     string   `json:"board_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		writeJSONError(w, http.StatusBadRequest, "url must start with http:// or https://")
		return
	}
	httpClient := &http.Client{Timeout: 20 * time.Second}
	fetchReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, req.URL, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.SourceURL != "" {
		fetchReq.Header.Set("Referer", req.SourceURL)
	}
	resp, err := httpClient.Do(fetchReq)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("upstream returned %d", resp.StatusCode))
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImportMB))
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	fname := strings.TrimSpace(req.Filename)
	if fname == "" {
		fname = filepath.Base(strings.SplitN(req.URL, "?", 2)[0])
	}
	if !strings.Contains(fname, ".") {
		ct := resp.Header.Get("Content-Type")
		switch {
		case strings.Contains(ct, "jpeg"):
			fname += ".jpg"
		case strings.Contains(ct, "webp"):
			fname += ".webp"
		case strings.Contains(ct, "gif"):
			fname += ".gif"
		default:
			fname += ".png"
		}
	}
	srcURL := req.SourceURL
	if srcURL == "" {
		srcURL = req.URL
	}
	asset, added, err := s.lib.ImportBytes(fname, data, req.TagIDs, srcURL, req.SourceTitle)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Notes != "" {
		asset, _ = s.lib.UpdateAssetMeta(asset.ID, req.Notes, srcURL, req.SourceTitle)
	}
	s.attachImportedDestinations([]string{asset.ID}, req.ProjectID, req.BoardID)
	if _, err := s.lib.EnsureVisualFeatures(asset.ID); err == nil {
		asset, _ = s.lib.Get(asset.ID)
	}
	s.notifyChanged()
	writeJSON(w, http.StatusCreated, map[string]any{
		"added": added,
		"asset": toAPIAsset(asset, nil),
	})
}

func (s *Server) handleImportSource(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		URL             string   `json:"url"`
		URLs            []string `json:"urls"`
		Notes           string   `json:"notes"`
		TagIDs          []string `json:"tag_ids"`
		ProjectID       string   `json:"project_id"`
		BoardID         string   `json:"board_id"`
		AutoPlatformTag *bool    `json:"auto_platform_tag"`
		AutoStack       *bool    `json:"auto_stack"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	urls := append([]string(nil), req.URLs...)
	if u := strings.TrimSpace(req.URL); u != "" {
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		writeJSONError(w, http.StatusBadRequest, "missing url")
		return
	}
	autoTag := true
	if req.AutoPlatformTag != nil {
		autoTag = *req.AutoPlatformTag
	}
	autoStack := true
	if req.AutoStack != nil {
		autoStack = *req.AutoStack
	}
	res, err := s.lib.ImportSourcesFromURLs(r.Context(), urls, library.SourceImportOptions{
		TagIDs:              req.TagIDs,
		AutoPlatformTag:     autoTag,
		AutoStackMultiImage: autoStack,
	})
	if err != nil && len(res.Assets) == 0 {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	ids := make([]string, 0, len(res.Assets))
	apiAssets := make([]apiAsset, 0, len(res.Assets))
	for _, a := range res.Assets {
		if strings.TrimSpace(req.Notes) != "" {
			a, _ = s.lib.UpdateAssetMeta(a.ID, strings.TrimSpace(req.Notes), a.SourceURL, a.SourceTitle)
		}
		if _, err := s.lib.EnsureVisualFeatures(a.ID); err == nil {
			a, _ = s.lib.Get(a.ID)
		}
		ids = append(ids, a.ID)
		apiAssets = append(apiAssets, toAPIAsset(a, nil))
	}
	s.attachImportedDestinations(ids, req.ProjectID, req.BoardID)
	s.notifyChanged()
	writeJSON(w, http.StatusCreated, map[string]any{
		"added":  len(apiAssets),
		"stacks": res.StackCount,
		"assets": apiAssets,
	})
}

func (s *Server) handleCreateStack(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Name     string   `json:"name"`
		AssetIDs []string `json:"asset_ids"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.AssetIDs) < 2 {
		writeJSONError(w, http.StatusBadRequest, "need at least 2 asset_ids to create a stack")
		return
	}
	st, err := s.lib.CreateStack(req.Name, req.AssetIDs, req.AssetIDs[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.notifyChanged()
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) handleRenamePreview(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AssetIDs []string           `json:"asset_ids"`
		Spec     library.RenameSpec `json:"spec"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := s.lib.PreviewBatchRename(req.AssetIDs, req.Spec)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": rows})
}

func (s *Server) handleRenameApply(w http.ResponseWriter, r *http.Request) {
	if !s.requireLibrary(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AssetIDs []string           `json:"asset_ids"`
		Spec     library.RenameSpec `json:"spec"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	batch, err := s.lib.ApplyBatchRename(req.AssetIDs, req.Spec)
	if err != nil {
		if errors.Is(err, library.ErrRenameConflict) {
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.notifyChanged()
	writeJSON(w, http.StatusOK, map[string]any{"batch": batch})
}

// unused import guard for thumbnail package (used in web/color helpers if needed)
var _ = thumbnail.ParseHexRGB

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
