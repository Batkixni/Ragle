package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// FeedProvider parses RSS 2.0, Atom (including YouTube channel feeds), and JSON Feed 1.1
// documents into normalized visual items.
type FeedProvider struct {
	MaxItems int // defaults to 20
}

var reHTMLImgSrc = regexp.MustCompile(`(?is)<img\b[^>]*\bsrc\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s"'=<>` + "`" + `]+))`)

func (p *FeedProvider) Name() string { return "feed" }

func (p *FeedProvider) Match(u *url.URL) bool {
	return u != nil && looksLikeFeedURL(u)
}

func looksLikeFeedURL(u *url.URL) bool {
	low := strings.ToLower(u.Path)
	switch {
	case strings.HasSuffix(low, ".xml"),
		strings.HasSuffix(low, ".rss"),
		strings.HasSuffix(low, ".atom"),
		strings.HasSuffix(low, "/feed"),
		strings.HasSuffix(low, "/feed/"),
		strings.HasSuffix(low, "/rss"),
		strings.HasSuffix(low, "/rss/"),
		strings.HasSuffix(low, "/feed.json"),
		strings.Contains(low, "/feeds/videos.xml"):
		return true
	}
	return false
}

func (p *FeedProvider) Resolve(ctx context.Context, client *http.Client, rawURL string) ([]ResolvedItem, error) {
	body, ct, finalURL, err := fetchBytes(ctx, client, rawURL, "application/rss+xml,application/atom+xml,application/feed+json,application/xml,text/xml,application/json,*/*;q=0.5", MaxHTMLBytes)
	if err != nil {
		return nil, err
	}
	return p.Parse(finalURL, ct, body)
}

// Parse decodes RSS 2.0, Atom, or JSON Feed bytes into ResolvedItem entries.
func (p *FeedProvider) Parse(feedURL, contentType string, data []byte) ([]ResolvedItem, error) {
	maxItems := p.MaxItems
	if maxItems <= 0 {
		maxItems = 20
	}
	base, _ := url.Parse(feedURL)
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty feed")
	}

	// 1. JSON Feed 1.0 / 1.1
	if trimmed[0] == '{' || strings.Contains(strings.ToLower(contentType), "json") {
		if items, ok := parseJSONFeed(base, trimmed, maxItems); ok {
			return items, nil
		}
	}

	// 2. Atom (<feed ...>)
	head := strings.ToLower(string(trimmed[:min(512, len(trimmed))]))
	if strings.Contains(head, "<feed") {
		if items, err := parseAtomFeed(base, trimmed, maxItems); err == nil && len(items) > 0 {
			return items, nil
		}
	}

	// 3. RSS 2.0 (<rss ...> or <rdf:RDF ...>)
	if strings.Contains(head, "<rss") || strings.Contains(head, "<channel") || strings.Contains(head, "<rdf:") {
		if items, err := parseRSSFeed(base, trimmed, maxItems); err == nil && len(items) > 0 {
			return items, nil
		}
	}

	return nil, fmt.Errorf("unrecognized feed format")
}

func parseJSONFeed(base *url.URL, data []byte, maxItems int) ([]ResolvedItem, bool) {
	var jf struct {
		Version string `json:"version"`
		Title   string `json:"title"`
		Items   []struct {
			ID          string `json:"id"`
			URL         string `json:"url"`
			ExternalURL string `json:"external_url"`
			Title       string `json:"title"`
			Summary     string `json:"summary"`
			ContentHTML string `json:"content_html"`
			ContentText string `json:"content_text"`
			Image       string `json:"image"`
			BannerImage string `json:"banner_image"`
			Tags        []string `json:"tags"`
			Authors     []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Attachments []struct {
				URL      string `json:"url"`
				MimeType string `json:"mime_type"`
			} `json:"attachments"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &jf); err != nil || len(jf.Items) == 0 {
		return nil, false
	}
	if !strings.Contains(strings.ToLower(jf.Version), "jsonfeed.org") && jf.Title == "" {
		return nil, false
	}
	var out []ResolvedItem
	for _, it := range jf.Items {
		if len(out) >= maxItems {
			break
		}
		link := firstNonEmpty(it.URL, it.ExternalURL)
		if link == "" {
			continue
		}
		link = resolveRef(base, link)
		var imgs []string
		for _, cand := range []string{it.Image, it.BannerImage} {
			if cand != "" {
				imgs = append(imgs, resolveRef(base, cand))
			}
		}
		for _, att := range it.Attachments {
			if strings.HasPrefix(strings.ToLower(att.MimeType), "image/") && att.URL != "" {
				imgs = append(imgs, resolveRef(base, att.URL))
			}
		}
		if len(imgs) == 0 && it.ContentHTML != "" {
			imgs = append(imgs, extractFirstHTMLImages(base, it.ContentHTML, 3)...)
		}
		author := ""
		if len(it.Authors) > 0 {
			author = strings.TrimSpace(it.Authors[0].Name)
		}
		tags := append([]string{"RSS"}, it.Tags...)
		if jf.Title != "" {
			tags = append(tags, strings.TrimSpace(jf.Title))
		}
		out = append(out, ResolvedItem{
			Source:      "feed",
			SourceLabel: firstNonEmpty(strings.TrimSpace(jf.Title), "Feed"),
			PageURL:     link,
			Title:       firstNonEmpty(strings.TrimSpace(it.Title), link),
			Author:      author,
			Description: trimSummary(firstNonEmpty(it.Summary, it.ContentText)),
			ImageURLs:   dedupStrings(imgs),
			Tags:        dedupStrings(tags),
			IsBookmark:  true,
		})
	}
	return out, len(out) > 0
}

type rssDoc struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Encoded     string `xml:"encoded"`
	Author      string `xml:"author"`
	Creator     string `xml:"creator"`
	Categories  []string `xml:"category"`
	Enclosures  []struct {
		URL  string `xml:"url,attr"`
		Type string `xml:"type,attr"`
	} `xml:"enclosure"`
	MediaThumbs []struct {
		URL string `xml:"url,attr"`
	} `xml:"thumbnail"`
	MediaContents []struct {
		URL    string `xml:"url,attr"`
		Medium string `xml:"medium,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"content"`
}

func parseRSSFeed(base *url.URL, data []byte, maxItems int) ([]ResolvedItem, error) {
	var doc rssDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	feedTitle := strings.TrimSpace(doc.Channel.Title)
	var out []ResolvedItem
	for _, it := range doc.Channel.Items {
		if len(out) >= maxItems {
			break
		}
		link := resolveRef(base, strings.TrimSpace(it.Link))
		if link == "" {
			continue
		}
		var imgs []string
		for _, mt := range it.MediaThumbs {
			if mt.URL != "" {
				imgs = append(imgs, resolveRef(base, mt.URL))
			}
		}
		for _, mc := range it.MediaContents {
			if mc.URL != "" && (mc.Medium == "" || mc.Medium == "image" || strings.HasPrefix(strings.ToLower(mc.Type), "image/")) {
				imgs = append(imgs, resolveRef(base, mc.URL))
			}
		}
		for _, enc := range it.Enclosures {
			if enc.URL != "" && strings.HasPrefix(strings.ToLower(enc.Type), "image/") {
				imgs = append(imgs, resolveRef(base, enc.URL))
			}
		}
		if len(imgs) == 0 {
			imgs = append(imgs, extractFirstHTMLImages(base, it.Encoded+"\n"+it.Description, 3)...)
		}
		tags := []string{"RSS"}
		if feedTitle != "" {
			tags = append(tags, feedTitle)
		}
		for _, c := range it.Categories {
			if s := strings.TrimSpace(c); s != "" {
				tags = append(tags, s)
			}
		}
		out = append(out, ResolvedItem{
			Source:      "feed",
			SourceLabel: firstNonEmpty(feedTitle, "RSS"),
			PageURL:     link,
			Title:       firstNonEmpty(strings.TrimSpace(html.UnescapeString(it.Title)), link),
			Author:      firstNonEmpty(strings.TrimSpace(it.Creator), strings.TrimSpace(it.Author)),
			Description: trimSummary(stripHTMLTags(it.Description)),
			ImageURLs:   dedupStrings(imgs),
			Tags:        dedupStrings(tags),
			IsBookmark:  true,
		})
	}
	return out, nil
}

type atomFeedDoc struct {
	Title   string          `xml:"title"`
	Entries []atomEntryItem `xml:"entry"`
}

type atomEntryItem struct {
	Title   string `xml:"title"`
	Summary string `xml:"summary"`
	Content string `xml:"content"`
	VideoID string `xml:"videoId"`
	Author  struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Links []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	} `xml:"link"`
	MediaGroup struct {
		Description string `xml:"description"`
		Thumbs      []struct {
			URL string `xml:"url,attr"`
		} `xml:"thumbnail"`
	} `xml:"group"`
	MediaThumbs []struct {
		URL string `xml:"url,attr"`
	} `xml:"thumbnail"`
}

func parseAtomFeed(base *url.URL, data []byte, maxItems int) ([]ResolvedItem, error) {
	var doc atomFeedDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	feedTitle := strings.TrimSpace(doc.Title)
	var out []ResolvedItem
	for _, ent := range doc.Entries {
		if len(out) >= maxItems {
			break
		}
		link := ""
		var imgs []string
		for _, l := range ent.Links {
			if l.Href == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(l.Type), "image/") {
				imgs = append(imgs, resolveRef(base, l.Href))
				continue
			}
			if link == "" || l.Rel == "" || strings.EqualFold(l.Rel, "alternate") {
				link = resolveRef(base, l.Href)
			}
		}
		if link == "" && ent.VideoID != "" {
			link = "https://www.youtube.com/watch?v=" + strings.TrimSpace(ent.VideoID)
		}
		if link == "" {
			continue
		}
		var fallbacks []string
		src := "feed"
		srcLabel := firstNonEmpty(feedTitle, "Atom")
		tags := []string{}
		if vid := strings.TrimSpace(ent.VideoID); vid != "" {
			src = "youtube"
			srcLabel = "YouTube"
			imgs = append(imgs, "https://i.ytimg.com/vi/"+vid+"/maxresdefault.jpg")
			fallbacks = append(fallbacks, "https://i.ytimg.com/vi/"+vid+"/hqdefault.jpg")
			tags = append(tags, "YouTube")
		} else {
			tags = append(tags, "RSS")
		}
		for _, mt := range ent.MediaGroup.Thumbs {
			if mt.URL != "" {
				if len(imgs) == 0 {
					imgs = append(imgs, resolveRef(base, mt.URL))
				} else {
					fallbacks = append(fallbacks, resolveRef(base, mt.URL))
				}
			}
		}
		for _, mt := range ent.MediaThumbs {
			if mt.URL != "" {
				imgs = append(imgs, resolveRef(base, mt.URL))
			}
		}
		if len(imgs) == 0 {
			imgs = append(imgs, extractFirstHTMLImages(base, ent.Content+"\n"+ent.Summary, 3)...)
		}
		if feedTitle != "" {
			tags = append(tags, feedTitle)
		}
		desc := firstNonEmpty(ent.MediaGroup.Description, stripHTMLTags(ent.Summary), stripHTMLTags(ent.Content))
		out = append(out, ResolvedItem{
			Source:      src,
			SourceLabel: srcLabel,
			PageURL:     link,
			Title:       firstNonEmpty(strings.TrimSpace(html.UnescapeString(ent.Title)), link),
			Author:      strings.TrimSpace(ent.Author.Name),
			Description: trimSummary(desc),
			ImageURLs:   dedupStrings(imgs),
			FallbackImg: dedupStrings(fallbacks),
			Tags:        dedupStrings(tags),
			IsBookmark:  true,
		})
	}
	return out, nil
}

func extractFirstHTMLImages(base *url.URL, fragment string, limit int) []string {
	var out []string
	for _, m := range reHTMLImgSrc.FindAllStringSubmatch(fragment, -1) {
		raw := firstNonEmpty(m[1], m[2], m[3])
		raw = strings.TrimSpace(html.UnescapeString(raw))
		if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
			continue
		}
		out = append(out, resolveRef(base, raw))
		if len(out) >= limit {
			break
		}
	}
	return dedupStrings(out)
}

var reAnyHTMLTag = regexp.MustCompile(`(?is)<[^>]+>`)

func stripHTMLTags(s string) string {
	s = reAnyHTMLTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func trimSummary(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 320 {
		return strings.TrimSpace(string(runes[:320])) + "…"
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
