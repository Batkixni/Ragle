package watcher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"raglex/internal/library"
	"raglex/internal/model"
)

// ClipSidecarSuffix identifies metadata files written by the Raglex browser
// extension while the Raglex desktop app is closed (Offline Drop Mode).
const ClipSidecarSuffix = ".raglex-clip.json"

// ClipSidecar preserves tags, notes, source URL, and target routing for items
// saved to the offline drop folder by the Chrome/Firefox browser extension.
type ClipSidecar struct {
	Version         int      `json:"version,omitempty"`
	Kind            string   `json:"kind,omitempty"` // "image" or "source"
	Filename        string   `json:"filename,omitempty"`
	URL             string   `json:"url,omitempty"`
	SourceURL       string   `json:"source_url,omitempty"`
	SourceTitle     string   `json:"source_title,omitempty"`
	Notes           string   `json:"notes,omitempty"`
	TagIDs          []string `json:"tag_ids,omitempty"`
	TagNames        []string `json:"tag_names,omitempty"`
	ProjectID       string   `json:"project_id,omitempty"`
	BoardID         string   `json:"board_id,omitempty"`
	AutoPlatformTag bool     `json:"auto_platform_tag,omitempty"`
	AutoStack       bool     `json:"auto_stack,omitempty"`
}

// ClipRouteEvent reports an imported offline clip that requested project/board routing.
type ClipRouteEvent struct {
	AssetIDs  []string
	ProjectID string
	BoardID   string
}

func isClipSidecar(base string) bool {
	return strings.HasSuffix(strings.ToLower(base), ClipSidecarSuffix)
}

func (w *Watcher) resolveClipTags(clip ClipSidecar, fallbackTag string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range clip.TagIDs {
		add(id)
	}
	for _, name := range clip.TagNames {
		if tid := w.lib.EnsureTagID(name); tid != "" {
			add(tid)
		}
	}
	if fallbackTag != "" {
		if tid := w.lib.EnsureTagID(fallbackTag); tid != "" {
			add(tid)
		}
	}
	return out
}

// ingestClipSidecar imports a single .raglex-clip.json sidecar (either a paired
// downloaded image/screenshot or an offline webpage/YouTube/RSS source URL) and
// removes the sidecar once ingested so it is never processed twice.
func (w *Watcher) ingestClipSidecar(ctx context.Context, sidecarPath string, rule WatchRule) (int, bool) {
	if w == nil || w.lib == nil {
		return 0, false
	}
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return 0, false
	}
	var clip ClipSidecar
	if err := json.Unmarshal(raw, &clip); err != nil {
		return 0, false
	}
	mode := rule.StorageMode
	if mode == "" {
		mode = model.StorageIndex
	}
	tagIDs := w.resolveClipTags(clip, rule.TagName)

	// Case 1: Offline webpage / YouTube / RSS source clip (resolved when Raglex opens).
	if strings.EqualFold(clip.Kind, "source") && strings.TrimSpace(clip.URL) != "" {
		res, err := w.lib.ImportSourcesFromURLs(ctx, []string{strings.TrimSpace(clip.URL)}, library.SourceImportOptions{
			TagIDs:              tagIDs,
			AutoPlatformTag:     clip.AutoPlatformTag,
			AutoStackMultiImage: clip.AutoStack,
		})
		if err != nil || len(res.Assets) == 0 {
			return 0, false
		}
		var ids []string
		for _, a := range res.Assets {
			ids = append(ids, a.ID)
			if strings.TrimSpace(clip.Notes) != "" {
				_, _ = w.lib.UpdateAssetMeta(a.ID, strings.TrimSpace(clip.Notes), a.SourceURL, a.SourceTitle)
			}
		}
		_ = os.Remove(sidecarPath)
		w.emitClipRoute(ids, clip.ProjectID, clip.BoardID)
		return len(res.Assets), true
	}

	// Case 2: Paired downloaded image or screenshot file.
	mediaPath := strings.TrimSuffix(sidecarPath, ClipSidecarSuffix)
	if _, err := os.Stat(mediaPath); err != nil && strings.TrimSpace(clip.Filename) != "" {
		cand := filepath.Join(filepath.Dir(sidecarPath), filepath.Base(clip.Filename))
		if _, err2 := os.Stat(cand); err2 == nil {
			mediaPath = cand
		}
	}
	st, err := os.Stat(mediaPath)
	if err != nil || st.IsDir() || st.Size() == 0 {
		// Paired media is still downloading; leave sidecar in place for the next event/scan.
		return 0, false
	}
	res, err := w.lib.Import(ctx, []string{mediaPath}, mode, tagIDs, nil)
	if err != nil || len(res.Items) == 0 {
		return 0, false
	}
	var ids []string
	for _, it := range res.Items {
		if it.AssetID == "" {
			continue
		}
		ids = append(ids, it.AssetID)
		for _, tid := range tagIDs {
			_ = w.lib.SetAssetTag(it.AssetID, tid, true)
		}
		if clip.Notes != "" || clip.SourceURL != "" || clip.SourceTitle != "" {
			_, _ = w.lib.UpdateAssetMeta(it.AssetID, clip.Notes, clip.SourceURL, clip.SourceTitle)
		}
	}
	_ = os.Remove(sidecarPath)
	w.emitClipRoute(ids, clip.ProjectID, clip.BoardID)
	added := res.Added
	if added == 0 && len(ids) > 0 {
		return 0, true
	}
	return added, len(ids) > 0
}

func (w *Watcher) emitClipRoute(assetIDs []string, projectID, boardID string) {
	if len(assetIDs) == 0 || (strings.TrimSpace(projectID) == "" && strings.TrimSpace(boardID) == "") {
		return
	}
	w.mu.Lock()
	cb := w.onClipRoute
	w.mu.Unlock()
	if cb != nil {
		cb(ClipRouteEvent{
			AssetIDs:  assetIDs,
			ProjectID: strings.TrimSpace(projectID),
			BoardID:   strings.TrimSpace(boardID),
		})
	}
}
