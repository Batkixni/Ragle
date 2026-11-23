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
	"raglex/internal/sources"
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

// SourceImportOptions controls multi-source URL / Feed / YouTube / oEmbed / JSON-LD importing.
type SourceImportOptions struct {
	TagIDs              []string // explicit tag IDs applied to every imported asset
	AutoPlatformTag     bool     // auto-create & attach platform/author tags (e.g. "YouTube", channel)
	AutoStackMultiImage bool     // fold 2+ images resolved from a single post into an Asset Stack
	Registry            *sources.Registry
	OnProgress          func(done, total int, label string)
}

// SourceImportResult summarizes a single or multi-URL source import.
type SourceImportResult struct {
	Assets     []model.Asset
	StackCount int
	Errors     []string
}

// EnsureTagID returns the ID of an existing tag matching name (case-insensitive)
// or creates a new root tag with that name.
func (l *Library) EnsureTagID(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || l == nil {
		return ""
	}
	if tags, err := l.Tags(); err == nil {
		for _, tg := range tags {
			if strings.EqualFold(strings.TrimSpace(tg.Name), name) {
				return tg.ID
			}
		}
	}
	if tg, err := l.CreateTag(name); err == nil {
		return tg.ID
	}
	return ""
}

// ImportFromURL fetches rawURL (direct image, YouTube, oEmbed, JSON-LD, RSS/Atom,
// or standard webpage), downloads its visual cover/media into the library, and
// records SourceURL, SourceTitle, DisplayName, IsBookmark, and Notes automatically.
func (l *Library) ImportFromURL(ctx context.Context, rawURL string, mode string) (model.Asset, error) {
	res, err := l.ImportSourcesFromURLs(ctx, []string{rawURL}, SourceImportOptions{
		AutoPlatformTag:     false,
		AutoStackMultiImage: true,
	})
	if err != nil {
		return model.Asset{}, err
	}
	if len(res.Assets) == 0 {
		if len(res.Errors) > 0 {
			return model.Asset{}, errors.New(res.Errors[0])
		}
		return model.Asset{}, errors.New("no visual items resolved from URL")
	}
	return res.Assets[0], nil
}

// ImportSourcesFromURLs resolves one or more URLs across all registered source
// adapters (YouTube, RSS/Atom/JSON Feed, oEmbed, JSON-LD galleries, OpenGraph,
// direct images) and imports all resulting media items cleanly into the library.
func (l *Library) ImportSourcesFromURLs(ctx context.Context, rawURLs []string, opts SourceImportOptions) (SourceImportResult, error) {
	var result SourceImportResult
	reg := opts.Registry
	if reg == nil {
		reg = sources.DefaultRegistry()
	}
	client := &http.Client{Timeout: 20 * time.Second}
	fetchImg := func(target string) ([]byte, string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Raglex/0.3")
		req.Header.Set("Accept", "image/avif,image/webp,image/*,*/*;q=0.8")
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, webImportMaxImageBytes))
		if err != nil {
			return nil, "", err
		}
		ct := resp.Header.Get("Content-Type")
		if !isImageContent(ct, data) {
			return nil, "", errors.New("not image content")
		}
		return data, ct, nil
	}

	var resolvedAll []sources.ResolvedItem
	for _, raw := range rawURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		items, err := reg.Resolve(ctx, raw)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", raw, err))
			continue
		}
		resolvedAll = append(resolvedAll, items...)
	}
	if len(resolvedAll) == 0 {
		if len(result.Errors) > 0 {
			return result, errors.New(result.Errors[0])
		}
		return result, errors.New("empty URL")
	}

	for idx, item := range resolvedAll {
		if opts.OnProgress != nil {
			opts.OnProgress(idx, len(resolvedAll), item.Title)
		}
		tagIDs := append([]string(nil), opts.TagIDs...)
		if opts.AutoPlatformTag {
			for _, tn := range item.Tags {
				if tid := l.EnsureTagID(tn); tid != "" {
					tagIDs = append(tagIDs, tid)
				}
			}
		}

		// Download all image URLs for this item (or primary + fallbacks, or placeholder).
		type dlEntry struct {
			url  string
			ct   string
			data []byte
		}
		var downloaded []dlEntry
		if len(item.ImageURLs) > 1 {
			// Multi-image gallery / JSON-LD post: download each unique image cleanly.
			for _, imgURL := range item.ImageURLs {
				if data, ct, err := fetchImg(imgURL); err == nil && len(data) > 0 {
					downloaded = append(downloaded, dlEntry{url: imgURL, ct: ct, data: data})
				}
			}
		} else {
			cands := append([]string(nil), item.ImageURLs...)
			cands = append(cands, item.FallbackImg...)
			for _, imgURL := range cands {
				if data, ct, err := fetchImg(imgURL); err == nil && len(data) > 0 {
					downloaded = append(downloaded, dlEntry{url: imgURL, ct: ct, data: data})
					break
				}
			}
		}
		if len(downloaded) == 0 {
			if !item.IsBookmark {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to download image", item.PageURL))
				continue
			}
			downloaded = []dlEntry{{url: item.PageURL, ct: "image/png", data: makeWebPlaceholderPNG()}}
		}

		parsedPage, _ := url.Parse(item.PageURL)
		var itemAssets []model.Asset
		for imgIdx, dl := range downloaded {
			ext := extFromContent(dl.ct, dl.url, dl.data)
			stem := sanitizeWebFilename(item.Title)
			if stem == "" {
				if u, err := url.Parse(dl.url); err == nil {
					base := filepath.Base(u.Path)
					base = strings.TrimSuffix(base, filepath.Ext(base))
					stem = sanitizeWebFilename(base)
				}
			}
			if stem == "" && parsedPage != nil {
				stem = sanitizeWebFilename(parsedPage.Host)
			}
			if stem == "" {
				stem = "web_bookmark"
			}
			if len(downloaded) > 1 {
				stem = fmt.Sprintf("%s_%02d", stem, imgIdx+1)
			}

			asset, _, err := l.ImportBytes(stem+ext, dl.data, tagIDs, item.PageURL, item.Title)
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
				continue
			}
			_ = l.DB.Update(func(tx *bolt.Tx) error {
				cur, err := db.GetAsset(tx, asset.ID)
				if err != nil {
					return err
				}
				cur.SourceURL = item.PageURL
				cur.SourceTitle = item.Title
				if item.Description != "" {
					cur.Notes = item.Description
				} else if item.Author != "" && cur.Notes == "" {
					cur.Notes = item.Author
				}
				if item.IsBookmark {
					cur.IsBookmark = true
					if strings.TrimSpace(item.Title) != "" {
						cur.DisplayName = strings.TrimSpace(item.Title)
					} else if parsedPage != nil && parsedPage.Host != "" {
						cur.DisplayName = parsedPage.Host
					}
				}
				asset = cur
				return db.PutAsset(tx, cur)
			})
			itemAssets = append(itemAssets, asset)
			result.Assets = append(result.Assets, asset)
		}

		// Fold multi-image posts cleanly into a single Version Stack when requested.
		if opts.AutoStackMultiImage && len(itemAssets) >= 2 {
			ids := make([]string, 0, len(itemAssets))
			for _, a := range itemAssets {
				ids = append(ids, a.ID)
			}
			stackName := strings.TrimSpace(item.Title)
			if stackName == "" {
				stackName = "Gallery"
			}
			if _, err := l.CreateStack(stackName, ids, ids[0]); err == nil {
				result.StackCount++
			}
		}
	}
	if opts.OnProgress != nil {
		opts.OnProgress(len(resolvedAll), len(resolvedAll), "")
	}
	return result, nil
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
