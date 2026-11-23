package sources

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// UniversalMediaProvider resolves direct images, RSS/Atom/JSON feeds discovered
// by Content-Type, oEmbed discovery links (<link type="application/json+oembed">),
// Schema.org JSON-LD (<script type="application/ld+json">), and OpenGraph / Twitter
// metadata (including multi-image galleries).
type UniversalMediaProvider struct {
	Feed *FeedProvider
}

var (
	reMetaTag    = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	reLinkTag    = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reTitleTag   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reJSONLDTag  = regexp.MustCompile(`(?is)<script\b[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)
	reTagAttr    = regexp.MustCompile(`(?is)([a-zA-Z0-9_:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+))`)
)

func (p *UniversalMediaProvider) Name() string { return "universal" }

func (p *UniversalMediaProvider) Match(_ *url.URL) bool { return true }

func (p *UniversalMediaProvider) Resolve(ctx context.Context, client *http.Client, rawURL string) ([]ResolvedItem, error) {
	body, ct, finalURL, err := fetchBytes(ctx, client, rawURL, "", MaxImageBytes)
	if err != nil {
		return nil, err
	}
	if isImageBytes(ct, body) {
		return []ResolvedItem{{
			Source:      "image",
			SourceLabel: "Image",
			PageURL:     finalURL,
			ImageURLs:   []string{finalURL},
			IsBookmark:  false,
		}}, nil
	}

	// Auto-detect RSS/Atom/JSON feeds even when URL doesn't end in .xml / /feed.
	htmlBytes := body
	if len(htmlBytes) > MaxHTMLBytes {
		htmlBytes = htmlBytes[:MaxHTMLBytes]
	}
	lowCT := strings.ToLower(ct)
	if p.Feed != nil && (strings.Contains(lowCT, "xml") || strings.Contains(lowCT, "rss") || strings.Contains(lowCT, "atom") || strings.Contains(lowCT, "feed+json")) {
		if items, err := p.Feed.Parse(finalURL, ct, htmlBytes); err == nil && len(items) > 0 {
			return items, nil
		}
	}

	parsed := ExtractStructuredHTML(finalURL, htmlBytes)

	// If the page advertises an official JSON oEmbed endpoint, query it to enrich
	// title, author, provider badge, and high-res thumbnail/media URL!
	if parsed.OEmbedURL != "" {
		if oeBytes, _, _, err := fetchBytes(ctx, client, parsed.OEmbedURL, "application/json", 512<<10); err == nil {
			var oe struct {
				Type         string `json:"type"`
				Title        string `json:"title"`
				AuthorName   string `json:"author_name"`
				ProviderName string `json:"provider_name"`
				ThumbnailURL string `json:"thumbnail_url"`
				URL          string `json:"url"`
			}
			if json.Unmarshal(oeBytes, &oe) == nil {
				base, _ := url.Parse(finalURL)
				if parsed.Title == "" && strings.TrimSpace(oe.Title) != "" {
					parsed.Title = strings.TrimSpace(oe.Title)
				}
				if parsed.Author == "" && strings.TrimSpace(oe.AuthorName) != "" {
					parsed.Author = strings.TrimSpace(oe.AuthorName)
				}
				if strings.TrimSpace(oe.ProviderName) != "" {
					parsed.Source = "oembed"
					parsed.SourceLabel = strings.TrimSpace(oe.ProviderName)
					parsed.Tags = append(parsed.Tags, strings.TrimSpace(oe.ProviderName))
				}
				var oeImgs []string
				if strings.EqualFold(oe.Type, "photo") && strings.TrimSpace(oe.URL) != "" {
					oeImgs = append(oeImgs, resolveRef(base, oe.URL))
				}
				if strings.TrimSpace(oe.ThumbnailURL) != "" {
					oeImgs = append(oeImgs, resolveRef(base, oe.ThumbnailURL))
				}
				if len(oeImgs) > 0 {
					parsed.ImageURLs = dedupStrings(append(oeImgs, parsed.ImageURLs...))
				}
			}
		}
	}

	parsed.Tags = dedupStrings(parsed.Tags)
	parsed.ImageURLs = dedupStrings(parsed.ImageURLs)
	return []ResolvedItem{parsed.ResolvedItem}, nil
}

// StructuredHTML extends ResolvedItem with discovered oEmbed endpoint URL.
type StructuredHTML struct {
	ResolvedItem
	OEmbedURL string
}

// ExtractStructuredHTML parses OpenGraph, Twitter Card, Schema.org JSON-LD
// (including multi-image galleries), and oEmbed discovery links from HTML.
func ExtractStructuredHTML(pageURL string, htmlBytes []byte) StructuredHTML {
	base, _ := url.Parse(pageURL)
	res := StructuredHTML{
		ResolvedItem: ResolvedItem{
			Source:      "web",
			SourceLabel: "Web",
			PageURL:     pageURL,
			IsBookmark:  true,
		},
	}
	if base != nil && base.Host != "" {
		res.SourceLabel = strings.TrimPrefix(strings.ToLower(base.Host), "www.")
	}

	text := string(htmlBytes)
	var ogSecure, ogImgs, twImgs, linkImgs []string

	for _, tag := range reMetaTag.FindAllString(text, -1) {
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
			ogSecure = append(ogSecure, resolveRef(base, val))
		case "og:image", "og:image:url":
			ogImgs = append(ogImgs, resolveRef(base, val))
		case "twitter:image", "twitter:image:src":
			twImgs = append(twImgs, resolveRef(base, val))
		case "og:title", "twitter:title":
			if res.Title == "" {
				res.Title = val
			}
		case "og:description", "twitter:description", "description":
			if res.Description == "" {
				res.Description = val
			}
		case "og:site_name":
			if res.SourceLabel == "" || strings.Contains(res.SourceLabel, ".") {
				res.SourceLabel = val
			}
		case "author":
			if res.Author == "" {
				res.Author = val
			}
		}
	}

	for _, tag := range reLinkTag.FindAllString(text, -1) {
		attrs := parseHTMLAttrs(tag)
		rel := strings.ToLower(attrs["rel"])
		typ := strings.ToLower(attrs["type"])
		href := strings.TrimSpace(html.UnescapeString(attrs["href"]))
		if href == "" {
			continue
		}
		if rel == "image_src" {
			linkImgs = append(linkImgs, resolveRef(base, href))
		}
		if strings.Contains(rel, "alternate") && strings.Contains(typ, "json+oembed") && res.OEmbedURL == "" {
			res.OEmbedURL = resolveRef(base, href)
		}
	}

	// Parse all JSON-LD blocks (supports single object, array, and @graph).
	var ldImgs []string
	var ldTags []string
	for _, m := range reJSONLDTag.FindAllStringSubmatch(text, -1) {
		rawJSON := strings.TrimSpace(m[1])
		if rawJSON == "" {
			continue
		}
		var doc any
		if err := json.Unmarshal([]byte(rawJSON), &doc); err != nil {
			continue
		}
		foundLD := false
		walkJSONLD(doc, func(obj map[string]any) {
			if res.Title == "" {
				if s := firstJSONString(obj["headline"], obj["name"]); s != "" {
					res.Title = s
					foundLD = true
				}
			}
			if res.Description == "" {
				if s := firstJSONString(obj["description"]); s != "" {
					res.Description = trimSummary(s)
					foundLD = true
				}
			}
			if res.Author == "" {
				if s := extractJSONLDAuthor(obj["author"]); s != "" {
					res.Author = s
					foundLD = true
				}
			}
			for _, u := range extractJSONLDImages(base, obj["image"]) {
				ldImgs = append(ldImgs, u)
				foundLD = true
			}
			for _, u := range extractJSONLDImages(base, obj["thumbnailUrl"]) {
				ldImgs = append(ldImgs, u)
				foundLD = true
			}
			if kw, ok := obj["keywords"]; ok {
				for _, k := range extractJSONLDKeywords(kw) {
					ldTags = append(ldTags, k)
				}
			}
		})
		if foundLD && res.Source == "web" {
			res.Source = "jsonld"
		}
	}

	if res.Title == "" {
		if m := reTitleTag.FindSubmatch(htmlBytes); len(m) >= 2 {
			res.Title = strings.TrimSpace(html.UnescapeString(string(m[1])))
		}
	}

	// Combine images in priority order (preserving multi-image galleries up to 12 images!).
	var allImgs []string
	allImgs = append(allImgs, ogSecure...)
	allImgs = append(allImgs, ogImgs...)
	allImgs = append(allImgs, ldImgs...)
	allImgs = append(allImgs, twImgs...)
	allImgs = append(allImgs, linkImgs...)
	allImgs = dedupStrings(allImgs)
	if len(allImgs) > 12 {
		allImgs = allImgs[:12]
	}
	res.ImageURLs = allImgs
	if res.Author != "" {
		res.Tags = append(res.Tags, res.Author)
	}
	if len(ldTags) > 6 {
		ldTags = ldTags[:6]
	}
	res.Tags = dedupStrings(append(res.Tags, ldTags...))
	return res
}

func walkJSONLD(v any, visit func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		visit(x)
		if g, ok := x["@graph"]; ok {
			walkJSONLD(g, visit)
		}
	case []any:
		for _, item := range x {
			walkJSONLD(item, visit)
		}
	}
}

func firstJSONString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(html.UnescapeString(s))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func extractJSONLDAuthor(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case map[string]any:
		return firstJSONString(x["name"], x["alternateName"])
	case []any:
		for _, item := range x {
			if s := extractJSONLDAuthor(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func extractJSONLDImages(base *url.URL, v any) []string {
	var out []string
	switch x := v.(type) {
	case string:
		if s := strings.TrimSpace(x); s != "" {
			out = append(out, resolveRef(base, s))
		}
	case map[string]any:
		if s := firstJSONString(x["contentUrl"], x["url"]); s != "" {
			out = append(out, resolveRef(base, s))
		}
	case []any:
		for _, item := range x {
			out = append(out, extractJSONLDImages(base, item)...)
		}
	}
	return dedupStrings(out)
}

func extractJSONLDKeywords(v any) []string {
	var out []string
	switch x := v.(type) {
	case string:
		for _, part := range strings.Split(x, ",") {
			if s := strings.TrimSpace(part); s != "" && len([]rune(s)) <= 24 {
				out = append(out, s)
			}
		}
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" && len([]rune(s)) <= 24 {
					out = append(out, s)
				}
			}
		}
	}
	return dedupStrings(out)
}

func parseHTMLAttrs(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range reTagAttr.FindAllStringSubmatch(tag, -1) {
		k := strings.ToLower(m[1])
		v := firstNonEmpty(m[2], m[3], m[4])
		out[k] = v
	}
	return out
}
