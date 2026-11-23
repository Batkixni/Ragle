package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

const (
	webImportMaxHTMLBytes  = 4 << 20  // 4 MB HTML limit
	webImportMaxImageBytes = 40 << 20 // 40 MB image limit
)

// WebMeta holds extracted OpenGraph / Twitter Card / HTML metadata from a URL.
type WebMeta struct {
	PageURL     string
	ImageURL    string
	Title       string
	Description string
}

var (
	reMetaTag  = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	reLinkTag  = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reTitleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reAttr     = regexp.MustCompile(`(?is)([a-zA-Z0-9_:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+))`)
)

// ExtractWebMeta parses og:image, twitter:image, link rel=image_src, og:title,
// <title>, and og:description from an HTML document and resolves relative URLs.
func ExtractWebMeta(pageURL string, htmlBytes []byte) WebMeta {
	meta := WebMeta{PageURL: pageURL}
	base, _ := url.Parse(pageURL)
	resolve := func(raw string) string {
		raw = strings.TrimSpace(html.UnescapeString(raw))
		if raw == "" {
			return ""
		}
		u, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		if base != nil {
			return base.ResolveReference(u).String()
		}
		return u.String()
	}

	var ogImage, ogSecure, twImage, linkImg string
	for _, tag := range reMetaTag.FindAllString(string(htmlBytes), -1) {
		attrs := parseHTMLAttrs(tag)
		key := strings.ToLower(attrs["property"])
		if key == "" {
			key = strings.ToLower(attrs["name"])
		}
		val := strings.TrimSpace(html.UnescapeString(attrs["content"]))
		if val == "" {
			continue
		}
		switch key {
		case "og:image:secure_url":
			if ogSecure == "" {
				ogSecure = resolve(val)
			}
		case "og:image", "og:image:url":
			if ogImage == "" {
				ogImage = resolve(val)
			}
		case "twitter:image", "twitter:image:src":
			if twImage == "" {
				twImage = resolve(val)
			}
		case "og:title", "twitter:title":
			if meta.Title == "" {
				meta.Title = val
			}
		case "og:description", "twitter:description", "description":
			if meta.Description == "" {
				meta.Description = val
			}
		}
	}

	for _, tag := range reLinkTag.FindAllString(string(htmlBytes), -1) {
		attrs := parseHTMLAttrs(tag)
		if strings.EqualFold(attrs["rel"], "image_src") && attrs["href"] != "" {
			linkImg = resolve(attrs["href"])
			break
		}
	}

	if meta.Title == "" {
		if m := reTitleTag.FindSubmatch(htmlBytes); len(m) >= 2 {
			meta.Title = strings.TrimSpace(html.UnescapeString(string(m[1])))
		}
	}

	switch {
	case ogSecure != "":
		meta.ImageURL = ogSecure
	case ogImage != "":
		meta.ImageURL = ogImage
	case twImage != "":
		meta.ImageURL = twImage
	case linkImg != "":
		meta.ImageURL = linkImg
	}
	return meta
}

func parseHTMLAttrs(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range reAttr.FindAllStringSubmatch(tag, -1) {
		k := strings.ToLower(m[1])
		v := m[2]
		if v == "" {
			v = m[3]
		}
		if v == "" {
			v = m[4]
		}
		out[k] = v
	}
	return out
}

// ImportFromURL fetches rawURL (either a direct image URL or a webpage with
// og:image / twitter:image metadata), downloads the image into the library, and
// records SourceURL, SourceTitle, and Notes automatically.
func (l *Library) ImportFromURL(ctx context.Context, rawURL string, mode string) (model.Asset, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return model.Asset{}, errors.New("empty URL")
	}
	if !strings.HasPrefix(strings.ToLower(rawURL), "http://") && !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
		rawURL = "https://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return model.Asset{}, fmt.Errorf("invalid URL: %s", rawURL)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	fetch := func(target string, maxBytes int64) ([]byte, string, string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, "", "", err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Raglex/0.2")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,image/avif,image/webp,image/*,*/*;q=0.8")
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
		if err != nil {
			return nil, "", "", err
		}
		finalURL := target
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
		}
		return data, resp.Header.Get("Content-Type"), finalURL, nil
	}

	body, ct, finalURL, err := fetch(rawURL, webImportMaxImageBytes)
	if err != nil {
		return model.Asset{}, err
	}

	var imgBytes []byte
	var imgCT string
	isWebPage := false
	meta := WebMeta{PageURL: finalURL}

	if isImageContent(ct, body) {
		imgBytes = body
		imgCT = ct
		meta.ImageURL = finalURL
	} else {
		isWebPage = true
		htmlSlice := body
		if len(htmlSlice) > webImportMaxHTMLBytes {
			htmlSlice = htmlSlice[:webImportMaxHTMLBytes]
		}
		meta = ExtractWebMeta(finalURL, htmlSlice)
		if meta.ImageURL != "" {
			if dl, dlCT, _, dlErr := fetch(meta.ImageURL, webImportMaxImageBytes); dlErr == nil && len(dl) > 0 && isImageContent(dlCT, dl) {
				imgBytes = dl
				imgCT = dlCT
			}
		}
		// If the page has no og:image or download failed, synthesize a clean
		// minimalist cover placeholder so the web bookmark still imports cleanly!
		if len(imgBytes) == 0 {
			imgBytes = makeWebPlaceholderPNG()
			imgCT = "image/png"
		}
	}

	ext := extFromContent(imgCT, meta.ImageURL, imgBytes)
	stem := sanitizeWebFilename(meta.Title)
	if stem == "" {
		if u, err := url.Parse(meta.ImageURL); err == nil {
			base := filepath.Base(u.Path)
			base = strings.TrimSuffix(base, filepath.Ext(base))
			stem = sanitizeWebFilename(base)
		}
	}
	if stem == "" {
		stem = sanitizeWebFilename(parsed.Host)
	}
	if stem == "" {
		stem = "web_bookmark"
	}

	asset, _, err := l.ImportBytes(stem+ext, imgBytes, nil, meta.PageURL, meta.Title)
	if err != nil {
		return model.Asset{}, err
	}
	// Persist bookmark flag, human-readable webpage title, source URL, and description.
	_ = l.DB.Update(func(tx *bolt.Tx) error {
		cur, err := db.GetAsset(tx, asset.ID)
		if err != nil {
			return err
		}
		cur.SourceURL = meta.PageURL
		cur.SourceTitle = meta.Title
		if meta.Description != "" {
			cur.Notes = meta.Description
		}
		if isWebPage {
			cur.IsBookmark = true
			if strings.TrimSpace(meta.Title) != "" {
				cur.DisplayName = strings.TrimSpace(meta.Title)
			} else if parsed.Host != "" {
				cur.DisplayName = parsed.Host
			}
		}
		asset = cur
		return db.PutAsset(tx, cur)
	})
	return asset, nil
}

func makeWebPlaceholderPNG() []byte {
	const w, h = 640, 400
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x+y) / float64(w+h)
			r := uint8(24 + t*18)
			g := uint8(24 + t*20)
			b := uint8(30 + t*26)
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func isImageContent(ct string, data []byte) bool {
	low := strings.ToLower(ct)
	if strings.HasPrefix(low, "image/") {
		return true
	}
	if len(data) >= 12 {
		detected := strings.ToLower(http.DetectContentType(data[:min(512, len(data))]))
		if strings.HasPrefix(detected, "image/") {
			return true
		}
		if bytes.HasPrefix(bytes.TrimSpace(data[:min(256, len(data))]), []byte("<svg")) {
			return true
		}
	}
	return false
}

func extFromContent(ct, rawURL string, data []byte) string {
	mt, _, _ := mime.ParseMediaType(ct)
	switch strings.ToLower(mt) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	}
	if u, err := url.Parse(rawURL); err == nil {
		ext := strings.ToLower(filepath.Ext(u.Path))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", ".bmp", ".tiff", ".ico":
			return ext
		}
	}
	det := strings.ToLower(http.DetectContentType(data[:min(512, len(data))]))
	switch {
	case strings.Contains(det, "png"):
		return ".png"
	case strings.Contains(det, "webp"):
		return ".webp"
	case strings.Contains(det, "gif"):
		return ".gif"
	default:
		return ".jpg"
	}
}

func sanitizeWebFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*', '\r', '\n', '\t':
			b.WriteRune('_')
		default:
			if r >= 32 {
				b.WriteRune(r)
			}
		}
		if b.Len() >= 64 {
			break
		}
	}
	return strings.Trim(strings.TrimSpace(b.String()), "._ ")
}
