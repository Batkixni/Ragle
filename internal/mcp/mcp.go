// Package mcp implements Raglex's Model Context Protocol (MCP) JSON-RPC 2.0
// server for AI agents (Claude, Antigravity, Cursor). It serves identical
// tools and resources over both Stdio (cmd/raglex-mcp) and HTTP (/mcp on
// internal/server).
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"raglex/internal/ai/laya"
	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

// Server handles MCP JSON-RPC 2.0 requests against an open Raglex library.
type Server struct {
	lib      *library.Library
	projects *project.Store
	layaURL  string
	layaMod  string
	onChange func()
}

// Options configures an MCP Server.
type Options struct {
	Library   *library.Library
	Projects  *project.Store
	LayaURL   string
	LayaModel string
	OnChange  func()
}

// New constructs an MCP Server.
func New(opts Options) *Server {
	return &Server{
		lib:      opts.Library,
		projects: opts.Projects,
		layaURL:  opts.LayaURL,
		layaMod:  opts.LayaModel,
		onChange: opts.OnChange,
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

// ServeStdio reads newline-delimited JSON-RPC 2.0 requests from r and writes
// responses to w until EOF or ctx cancellation.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(rpcResponse{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "parse error: " + err.Error()},
			})
			continue
		}
		if resp := s.handle(ctx, req); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// ServeHTTP exposes MCP over Streamable HTTP JSON-RPC (POST /mcp).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: err.Error()},
		})
		return
	}
	resp := s.handle(r.Context(), req)
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handle(ctx context.Context, req rpcRequest) *rpcResponse {
	// Notifications have nil ID and expect no response.
	isNotif := req.ID == nil && strings.HasPrefix(req.Method, "notifications/")
	switch req.Method {
	case "initialize":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]string{
					"name":    "raglex-mcp",
					"version": "0.2.0",
				},
				"capabilities": map[string]any{
					"tools":     map[string]any{},
					"resources": map[string]any{},
				},
			},
		}
	case "notifications/initialized", "notifications/cancelled":
		return nil
	case "ping":
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": toolSchemas()}}
	case "tools/call":
		res, err := s.callTool(ctx, req.Params)
		if err != nil {
			return &rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"isError": true,
					"content": []map[string]string{{"type": "text", "text": err.Error()}},
				},
			}
		}
		raw, _ := json.MarshalIndent(res, "", "  ")
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]string{{"type": "text", "text": string(raw)}},
			},
		}
	case "resources/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"resources": []map[string]string{
					{
						"uri":         "raglex://library/summary",
						"name":        "Raglex Library Summary",
						"description": "Counts of assets, tags, projects, and standalone boards in the open library",
						"mimeType":    "application/json",
					},
				},
			},
		}
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.URI == "raglex://library/summary" {
			summary := s.librarySummary()
			raw, _ := json.MarshalIndent(summary, "", "  ")
			return &rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"contents": []map[string]string{{
						"uri":      p.URI,
						"mimeType": "application/json",
						"text":     string(raw),
					}},
				},
			}
		}
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "unknown resource uri: " + p.URI},
		}
	default:
		if isNotif {
			return nil
		}
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) librarySummary() map[string]any {
	out := map[string]any{}
	if s.lib != nil {
		assets, _ := s.lib.List(library.Filter{})
		tags, _ := s.lib.Tags()
		out["asset_count"] = len(assets)
		out["tags"] = tags
	}
	if s.projects != nil {
		projs, _ := s.projects.List()
		sbs, _ := s.projects.StandaloneBoards()
		out["projects"] = projs
		out["standalone_boards"] = sbs
	}
	return out
}

func (s *Server) callTool(ctx context.Context, rawParams json.RawMessage) (any, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &call); err != nil {
		return nil, err
	}
	if s.lib == nil {
		return nil, fmt.Errorf("raglex library is not open")
	}

	switch call.Name {
	case "search_assets":
		var args struct {
			Query     string `json:"query"`
			Tag       string `json:"tag"`
			Color     string `json:"color"`
			MinRating int    `json:"min_rating"`
			Limit     int    `json:"limit"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		if args.Limit <= 0 {
			args.Limit = 50
		}
		var assets []model.Asset
		var err error
		if args.Color != "" {
			hits, cerr := s.lib.SearchByColor(args.Color, 0.25)
			err = cerr
			for _, h := range hits {
				assets = append(assets, h.Asset)
			}
		} else {
			tagID := ""
			if args.Tag != "" {
				if tags, _ := s.lib.Tags(); len(tags) > 0 {
					for _, tg := range tags {
						if strings.EqualFold(tg.Name, args.Tag) || tg.ID == args.Tag {
							tagID = tg.ID
							break
						}
					}
				}
			}
			assets, err = s.lib.List(library.Filter{TagID: tagID, MinRating: args.MinRating})
		}
		if err != nil {
			return nil, err
		}
		q := strings.ToLower(strings.TrimSpace(args.Query))
		var out []map[string]any
		for _, a := range assets {
			if q != "" && !strings.Contains(strings.ToLower(a.Label()+" "+a.Notes+" "+a.SourceTitle), q) {
				continue
			}
			out = append(out, map[string]any{
				"id":      a.ID,
				"name":    a.Label(),
				"path":    a.Path,
				"mime":    a.Mime,
				"width":   a.Width,
				"height":  a.Height,
				"rating":  a.Rating,
				"palette": a.Palette,
				"notes":   a.Notes,
			})
			if len(out) >= args.Limit {
				break
			}
		}
		return map[string]any{"count": len(out), "assets": out}, nil

	case "find_similar_assets":
		var args struct {
			AssetID   string  `json:"asset_id"`
			ImagePath string  `json:"image_path"`
			MinScore  float64 `json:"min_score"`
			Limit     int     `json:"limit"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		if args.Limit <= 0 {
			args.Limit = 20
		}
		var hits []library.VisualMatch
		var err error
		if args.AssetID != "" {
			hits, err = s.lib.FindSimilarByAsset(args.AssetID, args.MinScore, args.Limit)
		} else if args.ImagePath != "" {
			hits, err = s.lib.FindSimilarByImage(args.ImagePath, args.MinScore, args.Limit)
		} else {
			return nil, fmt.Errorf("provide asset_id or image_path")
		}
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(hits))
		for _, h := range hits {
			out = append(out, map[string]any{
				"id":         h.Asset.ID,
				"name":       h.Asset.Label(),
				"similarity": h.Similarity,
			})
		}
		return map[string]any{"matches": out}, nil

	case "list_tags":
		tags, err := s.lib.Tags()
		if err != nil {
			return nil, err
		}
		return map[string]any{"tags": tags}, nil

	case "tag_assets":
		var args struct {
			AssetIDs []string `json:"asset_ids"`
			TagName  string   `json:"tag_name"`
			Add      *bool    `json:"add"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		add := true
		if args.Add != nil {
			add = *args.Add
		}
		tags, _ := s.lib.Tags()
		var targetID string
		for _, tg := range tags {
			if strings.EqualFold(tg.Name, args.TagName) || tg.ID == args.TagName {
				targetID = tg.ID
				break
			}
		}
		if targetID == "" && add {
			tg, err := s.lib.CreateTag(args.TagName)
			if err != nil {
				return nil, err
			}
			targetID = tg.ID
		}
		if targetID == "" {
			return nil, fmt.Errorf("tag %q not found", args.TagName)
		}
		updated := 0
		for _, id := range args.AssetIDs {
			if err := s.lib.SetAssetTag(id, targetID, add); err == nil {
				updated++
			}
		}
		if s.onChange != nil {
			s.onChange()
		}
		return map[string]any{"tag_id": targetID, "updated": updated}, nil

	case "batch_rename_assets":
		var args struct {
			AssetIDs       []string `json:"asset_ids"`
			Template       string   `json:"template"`
			Find           string   `json:"find"`
			Replace        string   `json:"replace"`
			Prefix         string   `json:"prefix"`
			Suffix         string   `json:"suffix"`
			CaseMode       string   `json:"case_mode"`
			RenameDiskFile bool     `json:"rename_disk_file"`
			DryRun         bool     `json:"dry_run"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		spec := library.RenameSpec{
			Template:       args.Template,
			Find:           args.Find,
			Replace:        args.Replace,
			Prefix:         args.Prefix,
			Suffix:         args.Suffix,
			CaseMode:       args.CaseMode,
			RenameDiskFile: args.RenameDiskFile,
		}
		if args.DryRun {
			rows, err := s.lib.PreviewBatchRename(args.AssetIDs, spec)
			if err != nil {
				return nil, err
			}
			return map[string]any{"dry_run": true, "preview": rows}, nil
		}
		batch, err := s.lib.ApplyBatchRename(args.AssetIDs, spec)
		if err != nil {
			return nil, err
		}
		if s.onChange != nil {
			s.onChange()
		}
		return map[string]any{"dry_run": false, "batch_id": batch.ID, "renamed": len(batch.Entries)}, nil

	case "list_boards":
		if s.projects == nil {
			return nil, fmt.Errorf("projects store unavailable")
		}
		sbs, err := s.projects.StandaloneBoards()
		if err != nil {
			return nil, err
		}
		projs, _ := s.projects.List()
		return map[string]any{"standalone_boards": sbs, "projects": projs}, nil

	case "create_board":
		if s.projects == nil {
			return nil, fmt.Errorf("projects store unavailable")
		}
		var args struct {
			Name      string   `json:"name"`
			ProjectID string   `json:"project_id"`
			AssetIDs  []string `json:"asset_ids"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		if strings.TrimSpace(args.Name) == "" {
			args.Name = "AI Moodboard"
		}
		var b model.Board
		var err error
		if args.ProjectID != "" {
			b, err = s.projects.CreateBoard(args.ProjectID, args.Name)
		} else {
			b, err = s.projects.CreateStandaloneBoard(args.Name)
		}
		if err != nil {
			return nil, err
		}
		placed := 0
		if len(args.AssetIDs) > 0 {
			placed, _ = s.projects.AddAssetsToBoard(b.ID, args.AssetIDs)
		}
		if s.onChange != nil {
			s.onChange()
		}
		return map[string]any{"board": b, "placed": placed}, nil

	case "add_assets_to_board":
		if s.projects == nil {
			return nil, fmt.Errorf("projects store unavailable")
		}
		var args struct {
			BoardID  string   `json:"board_id"`
			AssetIDs []string `json:"asset_ids"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		n, err := s.projects.AddAssetsToBoard(args.BoardID, args.AssetIDs)
		if err != nil {
			return nil, err
		}
		if s.onChange != nil {
			s.onChange()
		}
		return map[string]any{"board_id": args.BoardID, "added": n}, nil

	case "classify_assets_with_laya":
		var args struct {
			AssetIDs  []string `json:"asset_ids"`
			OllayaURL string   `json:"ollaya_url"`
			Model     string   `json:"model"`
			AutoApply bool     `json:"auto_apply"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		url := args.OllayaURL
		if url == "" {
			url = s.layaURL
		}
		mod := args.Model
		if mod == "" {
			mod = s.layaMod
		}
		client := laya.NewClient(url, mod)
		tags, err := s.lib.Tags()
		if err != nil {
			return nil, err
		}
		results := map[string]any{}
		for _, id := range args.AssetIDs {
			a, err := s.lib.Get(id)
			if err != nil {
				continue
			}
			cls, err := client.ClassifyAssetTags(ctx, a, tags, 0.78, 0.35)
			if err != nil {
				return nil, err
			}
			if args.AutoApply {
				for _, hit := range cls.AutoApply {
					_ = s.lib.SetAssetTag(id, hit.Tag.ID, true)
				}
			}
			results[id] = cls
		}
		if args.AutoApply && s.onChange != nil {
			s.onChange()
		}
		return map[string]any{"classifications": results}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func toolSchemas() []map[string]any {
	return []map[string]any{
		{
			"name":        "search_assets",
			"description": "Search Raglex library assets by keyword, tag name/ID, dominant hex color (#RRGGBB), or minimum star rating.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]string{"type": "string"},
					"tag":        map[string]string{"type": "string"},
					"color":      map[string]string{"type": "string", "description": "Hex color like #FF5C33"},
					"min_rating": map[string]string{"type": "integer"},
					"limit":      map[string]string{"type": "integer"},
				},
			},
		},
		{
			"name":        "find_similar_assets",
			"description": "Reverse image search using pHash/dHash perceptual similarity from an existing asset_id or external image_path.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"asset_id":   map[string]string{"type": "string"},
					"image_path": map[string]string{"type": "string"},
					"min_score":  map[string]string{"type": "number"},
					"limit":      map[string]string{"type": "integer"},
				},
			},
		},
		{
			"name":        "list_tags",
			"description": "List all hierarchical tags in the Raglex library.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "tag_assets",
			"description": "Add or remove a tag across one or more assets (creating the tag if needed when add=true).",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"asset_ids", "tag_name"},
				"properties": map[string]any{
					"asset_ids": map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
					"tag_name":  map[string]string{"type": "string"},
					"add":       map[string]string{"type": "boolean"},
				},
			},
		},
		{
			"name":        "batch_rename_assets",
			"description": "Preview (dry_run=true) or apply a reversible batch rename across assets using templates like {tag}_{index:03}.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"asset_ids"},
				"properties": map[string]any{
					"asset_ids":        map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
					"template":         map[string]string{"type": "string"},
					"find":             map[string]string{"type": "string"},
					"replace":          map[string]string{"type": "string"},
					"prefix":           map[string]string{"type": "string"},
					"suffix":           map[string]string{"type": "string"},
					"case_mode":        map[string]string{"type": "string"},
					"rename_disk_file": map[string]string{"type": "boolean"},
					"dry_run":          map[string]string{"type": "boolean"},
				},
			},
		},
		{
			"name":        "list_boards",
			"description": "List standalone moodboards and projects in Raglex.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "create_board",
			"description": "Create a standalone moodboard (or project board) and arrange optional asset_ids cleanly on its canvas.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]any{
					"name":       map[string]string{"type": "string"},
					"project_id": map[string]string{"type": "string"},
					"asset_ids":  map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
				},
			},
		},
		{
			"name":        "add_assets_to_board",
			"description": "Place one or more library assets onto an existing moodboard.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"board_id", "asset_ids"},
				"properties": map[string]any{
					"board_id":  map[string]string{"type": "string"},
					"asset_ids": map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
				},
			},
		},
		{
			"name":        "classify_assets_with_laya",
			"description": "Score assets against existing library tags using the local open-source Ollaya/Laya decision model.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"asset_ids"},
				"properties": map[string]any{
					"asset_ids":  map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
					"ollaya_url": map[string]string{"type": "string"},
					"model":      map[string]string{"type": "string"},
					"auto_apply": map[string]string{"type": "boolean"},
				},
			},
		},
	}
}
