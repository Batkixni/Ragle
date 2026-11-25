package library

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

// ClipMeta carries rich page & image context captured by the Raglex browser
// extension (both during live online clips and offline drop-folder clips).
type ClipMeta struct {
	SourceURL    string `json:"source_url,omitempty"`
	SourceTitle  string `json:"source_title,omitempty"`
	ImageURL     string `json:"image_url,omitempty"`
	CanonicalURL string `json:"canonical_url,omitempty"`
	SiteName     string `json:"site_name,omitempty"`
	Author       string `json:"author,omitempty"`
	Description  string `json:"description,omitempty"`
	Alt          string `json:"alt,omitempty"`
	SelectedText string `json:"selected_text,omitempty"`
	ClippedAt    string `json:"clipped_at,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// FormatClipNotes combines the user's explicit notes with any non-redundant
// rich metadata captured from the browser page (author, site, description,
// selected quote, image alt text, direct image URL, and capture time).
func FormatClipNotes(m ClipMeta) string {
	userNotes := strings.TrimSpace(m.Notes)
	titleLow := strings.ToLower(strings.TrimSpace(m.SourceTitle))
	notesLow := strings.ToLower(userNotes)

	var metaLines []string
	appendLine := func(label, val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		if notesLow != "" && strings.Contains(notesLow, strings.ToLower(val)) {
			return
		}
		metaLines = append(metaLines, fmt.Sprintf("%s: %s", label, val))
	}

	if author := strings.TrimSpace(m.Author); author != "" {
		appendLine("Author", author)
	}
	if site := strings.TrimSpace(m.SiteName); site != "" {
		appendLine("Site", site)
	}
	if desc := collapseWhitespace(m.Description); desc != "" && strings.ToLower(desc) != titleLow {
		appendLine("Description", truncateRunes(desc, 360))
	}
	if sel := collapseWhitespace(m.SelectedText); sel != "" {
		appendLine("Quote", truncateRunes(sel, 360))
	}
	if alt := collapseWhitespace(m.Alt); alt != "" && strings.ToLower(alt) != titleLow && !strings.EqualFold(alt, strings.TrimSpace(m.Description)) {
		appendLine("Alt", truncateRunes(alt, 220))
	}
	srcURL := strings.TrimSpace(m.SourceURL)
	if canon := strings.TrimSpace(m.CanonicalURL); canon != "" && canon != srcURL {
		appendLine("Canonical", canon)
	}
	if imgURL := strings.TrimSpace(m.ImageURL); imgURL != "" && imgURL != srcURL &&
		(strings.HasPrefix(imgURL, "http://") || strings.HasPrefix(imgURL, "https://")) {
		appendLine("Image", imgURL)
	}
	if clipped := formatClipTimestamp(m.ClippedAt); clipped != "" {
		appendLine("Clipped", clipped)
	}

	if len(metaLines) == 0 {
		return userNotes
	}
	metaBlock := strings.Join(metaLines, "\n")
	if userNotes == "" {
		return metaBlock
	}
	return userNotes + "\n\n" + metaBlock
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func formatClipTimestamp(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Local().Format("2006-01-02 15:04")
		}
	}
	return raw
}

// ApplyClipMeta persists full source URL, source title, human-readable display
// name (when not yet set), and formatted clip notes onto an imported asset.
func (l *Library) ApplyClipMeta(id string, m ClipMeta) (model.Asset, error) {
	var updated model.Asset
	err := l.DB.Update(func(tx *bolt.Tx) error {
		cur, err := db.GetAsset(tx, id)
		if err != nil {
			return err
		}
		if srcURL := strings.TrimSpace(m.SourceURL); srcURL != "" {
			cur.SourceURL = srcURL
		}
		if srcTitle := strings.TrimSpace(m.SourceTitle); srcTitle != "" {
			cur.SourceTitle = srcTitle
			if strings.TrimSpace(cur.DisplayName) == "" {
				cur.DisplayName = srcTitle
			}
		}
		formatted := FormatClipNotes(m)
		if formatted != "" {
			if cur.Notes == "" {
				cur.Notes = formatted
			} else if !strings.Contains(cur.Notes, formatted) {
				// Prefer user's explicit notes first, keeping any resolver description intact.
				if strings.TrimSpace(m.Notes) != "" && !strings.Contains(cur.Notes, strings.TrimSpace(m.Notes)) {
					cur.Notes = strings.TrimSpace(m.Notes) + "\n\n" + cur.Notes
				}
				// Append structured fields not already present.
				for _, line := range strings.Split(FormatClipNotes(ClipMeta{
					SourceURL:    m.SourceURL,
					SourceTitle:  m.SourceTitle,
					ImageURL:     m.ImageURL,
					CanonicalURL: m.CanonicalURL,
					SiteName:     m.SiteName,
					Author:       m.Author,
					Description:  m.Description,
					Alt:          m.Alt,
					SelectedText: m.SelectedText,
					ClippedAt:    m.ClippedAt,
				}), "\n") {
					line = strings.TrimSpace(line)
					if line != "" && !strings.Contains(cur.Notes, line) {
						cur.Notes += "\n" + line
					}
				}
			}
		}
		updated = cur
		return db.PutAsset(tx, cur)
	})
	return updated, err
}

// ImportFallbackBookmark creates a bookmark asset directly from browser-captured
// metadata when server-side URL resolution fails (for example on login-walled
// pages or when Raglex imports an offline clip without network access).
func (l *Library) ImportFallbackBookmark(ctx context.Context, m ClipMeta, tagIDs []string) (model.Asset, error) {
	srcURL := strings.TrimSpace(m.SourceURL)
	if srcURL == "" {
		srcURL = strings.TrimSpace(m.CanonicalURL)
	}
	title := strings.TrimSpace(m.SourceTitle)
	parsed, _ := url.Parse(srcURL)
	if title == "" && parsed != nil {
		title = parsed.Host
	}
	if title == "" {
		title = "Web Clip"
	}

	var imgData []byte
	var ct string
	if imgURL := strings.TrimSpace(m.ImageURL); strings.HasPrefix(imgURL, "http://") || strings.HasPrefix(imgURL, "https://") {
		client := &http.Client{Timeout: 12 * time.Second}
		if req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil); err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Raglex/0.5")
			if srcURL != "" {
				req.Header.Set("Referer", srcURL)
			}
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					if data, rerr := io.ReadAll(io.LimitReader(resp.Body, webImportMaxImageBytes)); rerr == nil && isImageContent(resp.Header.Get("Content-Type"), data) {
						imgData = data
						ct = resp.Header.Get("Content-Type")
					}
				}
			}
		}
	}
	ext := ".png"
	if len(imgData) > 0 {
		ext = extFromContent(ct, m.ImageURL, imgData)
	} else {
		imgData = makeWebPlaceholderPNG()
	}

	stem := sanitizeWebFilename(title)
	if stem == "" && parsed != nil {
		stem = sanitizeWebFilename(parsed.Host)
	}
	if stem == "" {
		stem = "web_bookmark"
	}

	asset, _, err := l.ImportBytes(stem+ext, imgData, tagIDs, srcURL, title)
	if err != nil {
		return model.Asset{}, err
	}
	_ = l.DB.Update(func(tx *bolt.Tx) error {
		cur, err := db.GetAsset(tx, asset.ID)
		if err != nil {
			return err
		}
		cur.IsBookmark = true
		cur.DisplayName = title
		cur.SourceURL = srcURL
		cur.SourceTitle = title
		asset = cur
		return db.PutAsset(tx, cur)
	})
	m.SourceURL = srcURL
	m.SourceTitle = title
	return l.ApplyClipMeta(asset.ID, m)
}
