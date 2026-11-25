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

// ClipSidecar preserves full source URL, title, author/page metadata, tags,
// notes, and target routing for items saved to the offline drop folder by the
// Chrome/Firefox browser extension.
type ClipSidecar struct {
	Version         int      `json:"version,omitempty"`
	Kind            string   `json:"kind,omitempty"` // "image" or "source"
	Filename        string   `json:"filename,omitempty"`
	URL             string   `json:"url,omitempty"`
	SourceURL       string   `json:"source_url,omitempty"`
	SourceTitle     string   `json:"source_title,omitempty"`
	ImageURL        string   `json:"image_url,omitempty"`
	CanonicalURL    string   `json:"canonical_url,omitempty"`
	SiteName        string   `json:"site_name,omitempty"`
	Author          string   `json:"author,omitempty"`
	Description     string   `json:"description,omitempty"`
	Alt             string   `json:"alt,omitempty"`
	SelectedText    string   `json:"selected_text,omitempty"`
	ClippedAt       string   `json:"clipped_at,omitempty"`
	Notes           string   `json:"notes,omitempty"`
	TagIDs          []string `json:"tag_ids,omitempty"`
	TagNames        []string `json:"tag_names,omitempty"`
	ProjectID       string   `json:"project_id,omitempty"`
	BoardID         string   `json:"board_id,omitempty"`
	AutoPlatformTag bool     `json:"auto_platform_tag,omitempty"`
	AutoStack       bool     `json:"auto_stack,omitempty"`
}

func (c ClipSidecar) toClipMeta(fallbackURL, fallbackTitle string) library.ClipMeta {
	srcURL := strings.TrimSpace(c.SourceURL)
	if srcURL == "" {
		srcURL = strings.TrimSpace(fallbackURL)
	}
	if srcURL == "" {
		srcURL = strings.TrimSpace(c.URL)
	}
	srcTitle := strings.TrimSpace(c.SourceTitle)
	if srcTitle == "" {
		srcTitle = strings.TrimSpace(fallbackTitle)
	}
	imgURL := strings.TrimSpace(c.ImageURL)
	if imgURL == "" && strings.EqualFold(c.Kind, "image") {
		imgURL = strings.TrimSpace(c.URL)
	}
	return library.ClipMeta{
		SourceURL:    srcURL,
		SourceTitle:  srcTitle,
		ImageURL:     imgURL,
		CanonicalURL: strings.TrimSpace(c.CanonicalURL),
		SiteName:     strings.TrimSpace(c.SiteName),
		Author:       strings.TrimSpace(c.Author),
		Description:  strings.TrimSpace(c.Description),
		Alt:          strings.TrimSpace(c.Alt),
		SelectedText: strings.TrimSpace(c.SelectedText),
		ClippedAt:    strings.TrimSpace(c.ClippedAt),
		Notes:        strings.TrimSpace(c.Notes),
	}
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

func stripExtLower(base string) string {
	ext := filepath.Ext(base)
	return strings.ToLower(strings.TrimSuffix(base, ext))
}

func stripCollisionSuffix(stem string) string {
	// Handles browser conflictAction:"uniquify" names like "name_1234 (1)".
	if idx := strings.LastIndex(stem, " ("); idx > 0 && strings.HasSuffix(stem, ")") {
		inner := stem[idx+2 : len(stem)-1]
		if inner != "" {
			allDigits := true
			for _, r := range inner {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return stem[:idx]
			}
		}
	}
	return stem
}

// findSidecarMediaPath resolves the downloaded media file paired with sidecarPath,
// gracefully tolerating browser extension rewrites (e.g. .jpg -> .webp/.png) or
// conflictAction:"uniquify" suffixes like " (1)".
func findSidecarMediaPath(sidecarPath string, clip ClipSidecar) string {
	dir := filepath.Dir(sidecarPath)
	exact := strings.TrimSuffix(sidecarPath, ClipSidecarSuffix)
	if st, err := os.Stat(exact); err == nil && !st.IsDir() && st.Size() > 0 {
		return exact
	}
	if fn := strings.TrimSpace(clip.Filename); fn != "" {
		cand := filepath.Join(dir, filepath.Base(fn))
		if st, err := os.Stat(cand); err == nil && !st.IsDir() && st.Size() > 0 {
			return cand
		}
	}

	targetStem := stripCollisionSuffix(stripExtLower(filepath.Base(exact)))
	if targetStem == "" && strings.TrimSpace(clip.Filename) != "" {
		targetStem = stripCollisionSuffix(stripExtLower(filepath.Base(clip.Filename)))
	}
	if targetStem == "" {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if isIgnoredWatchFile(name) {
			continue
		}
		candStem := stripCollisionSuffix(stripExtLower(name))
		if candStem == targetStem {
			full := filepath.Join(dir, name)
			if st, err := os.Stat(full); err == nil && !st.IsDir() && st.Size() > 0 {
				return full
			}
		}
	}
	return ""
}

// findMatchingSidecar finds the .raglex-clip.json sidecar belonging to mediaPath,
// checking exact match first and then stem-based match for browser-renamed files.
func findMatchingSidecar(mediaPath string) string {
	exact := mediaPath + ClipSidecarSuffix
	if _, err := os.Stat(exact); err == nil {
		return exact
	}
	dir := filepath.Dir(mediaPath)
	targetStem := stripCollisionSuffix(stripExtLower(filepath.Base(mediaPath)))
	if targetStem == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !isClipSidecar(name) {
			continue
		}
		trimmed := strings.TrimSuffix(name, name[len(name)-len(ClipSidecarSuffix):])
		if stripCollisionSuffix(stripExtLower(trimmed)) == targetStem {
			return filepath.Join(dir, name)
		}
	}
	return ""
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
func (w *Watcher) ingestClipSidecar(ctx context.Context, sidecarPath string, rule WatchRule) (int, bool, string) {
	if w == nil || w.lib == nil {
		return 0, false, ""
	}
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return 0, false, ""
	}
	var clip ClipSidecar
	if err := json.Unmarshal(raw, &clip); err != nil {
		return 0, false, ""
	}
	mode := rule.StorageMode
	if mode == "" {
		mode = model.StorageIndex
	}
	tagIDs := w.resolveClipTags(clip, rule.TagName)

	// Case 1: Offline webpage / YouTube / RSS source clip (resolved when Raglex opens).
	if strings.EqualFold(clip.Kind, "source") {
		targetURL := strings.TrimSpace(clip.URL)
		if targetURL == "" {
			targetURL = strings.TrimSpace(clip.SourceURL)
		}
		if targetURL == "" {
			return 0, false, ""
		}
		res, err := w.lib.ImportSourcesFromURLs(ctx, []string{targetURL}, library.SourceImportOptions{
			TagIDs:              tagIDs,
			AutoPlatformTag:     clip.AutoPlatformTag,
			AutoStackMultiImage: clip.AutoStack,
		})
		if err != nil || len(res.Assets) == 0 {
			// Fallback to browser-captured metadata so offline/login-gated pages
			// are still saved with their full URL, title, preview, and notes.
			fb, ferr := w.lib.ImportFallbackBookmark(ctx, clip.toClipMeta(targetURL, ""), tagIDs)
			if ferr != nil {
				return 0, false, ""
			}
			res.Assets = []model.Asset{fb}
		}
		var ids []string
		for _, a := range res.Assets {
			ids = append(ids, a.ID)
			meta := clip.toClipMeta(a.SourceURL, a.SourceTitle)
			// Prefer the full browser-captured URL if present; keep resolved title if sidecar lacked one.
			if strings.TrimSpace(clip.SourceURL) != "" {
				meta.SourceURL = strings.TrimSpace(clip.SourceURL)
			} else if strings.TrimSpace(targetURL) != "" {
				meta.SourceURL = targetURL
			}
			if strings.TrimSpace(a.SourceTitle) != "" && strings.TrimSpace(clip.SourceTitle) == "" {
				meta.SourceTitle = a.SourceTitle
			}
			_, _ = w.lib.ApplyClipMeta(a.ID, meta)
			_, _ = w.lib.EnsureVisualFeatures(a.ID)
		}
		_ = os.Remove(sidecarPath)
		w.emitClipRoute(ids, clip.ProjectID, clip.BoardID)
		return len(res.Assets), true, ""
	}

	// Case 2: Paired downloaded image or screenshot file.
	mediaPath := findSidecarMediaPath(sidecarPath, clip)
	if mediaPath == "" {
		// Paired media is still downloading; leave sidecar in place for the next event/scan.
		return 0, false, ""
	}
	res, err := w.lib.Import(ctx, []string{mediaPath}, mode, tagIDs, nil)
	if err != nil || len(res.Items) == 0 {
		return 0, false, ""
	}
	meta := clip.toClipMeta("", "")
	var ids []string
	for _, it := range res.Items {
		if it.AssetID == "" {
			continue
		}
		ids = append(ids, it.AssetID)
		for _, tid := range tagIDs {
			_ = w.lib.SetAssetTag(it.AssetID, tid, true)
		}
		_, _ = w.lib.ApplyClipMeta(it.AssetID, meta)
		_, _ = w.lib.EnsureVisualFeatures(it.AssetID)
	}
	_ = os.Remove(sidecarPath)
	w.emitClipRoute(ids, clip.ProjectID, clip.BoardID)
	added := res.Added
	if added == 0 && len(ids) > 0 {
		return 0, true, mediaPath
	}
	return added, len(ids) > 0, mediaPath
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
