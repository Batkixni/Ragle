// Package sources implements Raglex's modular multi-platform source resolver
// architecture. Each platform or format (YouTube, RSS/Atom/JSON Feed,
// oEmbed/JSON-LD/OpenGraph web pages, direct media URLs) is an independent
// Provider registered in a Registry.
package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	MaxHTMLBytes  = 4 << 20  // 4 MB HTML/XML/JSON limit
	MaxImageBytes = 40 << 20 // 40 MB image limit
	userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Raglex/0.3"
)

// ResolvedItem is a normalized visual/media entry returned by any source Provider.
type ResolvedItem struct {
	Source      string   // "youtube", "feed", "oembed", "jsonld", "web", "image"
	SourceLabel string   // human-friendly badge e.g. "YouTube", "RSS", "Vimeo", "Web"
	PageURL     string   // canonical clickable source URL
	Title       string   // display title
	Author      string   // creator / channel / site author
	Description string   // summary or caption
	ImageURLs   []string // ordered candidate/gallery image URLs (first = cover; 2+ can stack)
	FallbackImg []string // lower-res fallback URLs if primary ImageURLs[0] returns 404
	Tags        []string // suggested tags (platform, author, keywords)
	IsBookmark  bool     // true for webpages/videos/posts; false for raw direct image URLs
}

// Provider resolves a specific platform or media format into one or more items.
type Provider interface {
	Name() string
	Match(u *url.URL) bool
	Resolve(ctx context.Context, client *http.Client, rawURL string) ([]ResolvedItem, error)
}

// Registry runs matching providers in priority order and falls back to the
// universal oEmbed / JSON-LD / OpenGraph / direct-image resolver.
type Registry struct {
	providers []Provider
	fallback  Provider
	client    *http.Client
}

// DefaultRegistry returns the standard multi-source resolver chain.
func DefaultRegistry() *Registry {
	feed := &FeedProvider{}
	return &Registry{
		providers: []Provider{
			&YouTubeProvider{Feed: feed},
			feed,
		},
		fallback: &UniversalMediaProvider{Feed: feed},
		client:   &http.Client{Timeout: 20 * time.Second},
	}
}

// WithClient overrides the HTTP client (useful for tests or custom proxies).
func (r *Registry) WithClient(c *http.Client) *Registry {
	if c != nil {
		r.client = c
	}
	return r
}

// NormalizeURL trims whitespace and prepends https:// when no scheme is present.
func NormalizeURL(raw string) (string, *url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, errors.New("empty URL")
	}
	low := strings.ToLower(raw)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", nil, fmt.Errorf("invalid URL: %s", raw)
	}
	return raw, u, nil
}

// ParseURLList splits a multi-line (or whitespace-separated) paste into
// deduplicated valid HTTP(S) URLs.
func ParseURLList(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t'
	}) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Allow pasting multiple space-separated URLs on one line.
		parts := strings.Fields(line)
		for _, p := range parts {
			norm, _, err := NormalizeURL(p)
			if err != nil {
				continue
			}
			if !seen[norm] {
				seen[norm] = true
				out = append(out, norm)
			}
		}
	}
	return out
}

// DetectSourceKind returns a fast, zero-network source kind identifier for UI badges.
func DetectSourceKind(rawURL string) string {
	_, u, err := NormalizeURL(rawURL)
	if err != nil {
		return ""
	}
	if isYouTubeHost(u.Host) {
		if strings.Contains(strings.ToLower(u.Path), "/feeds/videos.xml") {
			return "feed"
		}
		return "youtube"
	}
	if looksLikeFeedURL(u) {
		return "feed"
	}
	if looksLikeImageURL(u) {
		return "image"
	}
	return "web"
}

// Resolve selects the matching provider and returns one or more normalized items.
func (r *Registry) Resolve(ctx context.Context, rawURL string) ([]ResolvedItem, error) {
	norm, u, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	client := r.client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	for _, p := range r.providers {
		if p.Match(u) {
			items, err := p.Resolve(ctx, client, norm)
			if err == nil && len(items) > 0 {
				return items, nil
			}
			// If a specialized provider fails (e.g. non-video YouTube page), fall through cleanly.
		}
	}
	return r.fallback.Resolve(ctx, client, norm)
}

func fetchBytes(ctx context.Context, client *http.Client, target string, accept string, maxBytes int64) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	if accept == "" {
		accept = "text/html,application/xhtml+xml,application/xml,application/json,image/*,*/*;q=0.8"
	}
	req.Header.Set("Accept", accept)
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

func isImageBytes(ct string, data []byte) bool {
	low := strings.ToLower(ct)
	if strings.HasPrefix(low, "image/") {
		return true
	}
	if len(data) >= 12 {
		det := strings.ToLower(http.DetectContentType(data[:min(512, len(data))]))
		if strings.HasPrefix(det, "image/") {
			return true
		}
		if bytes.HasPrefix(bytes.TrimSpace(data[:min(256, len(data))]), []byte("<svg")) {
			return true
		}
	}
	return false
}

func looksLikeImageURL(u *url.URL) bool {
	ext := strings.ToLower(path.Ext(u.Path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".svg", ".avif", ".tiff":
		return true
	}
	return false
}

func resolveRef(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") && base != nil && base.Scheme != "" {
		raw = base.Scheme + ":" + raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if base != nil {
		return base.ResolveReference(ref).String()
	}
	return ref.String()
}

func dedupStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
