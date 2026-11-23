package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// YouTubeProvider resolves YouTube videos, Shorts, live streams, and channel Atom feeds
// with high-resolution covers (maxresdefault -> sddefault -> hqdefault) and oEmbed metadata.
type YouTubeProvider struct {
	Feed       *FeedProvider
	OEmbedBase string // optional override for tests (defaults to https://www.youtube.com/oembed)
	ThumbBase  string // optional override for tests (defaults to https://i.ytimg.com/vi)
}

var reYouTubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func (p *YouTubeProvider) Name() string { return "youtube" }

func (p *YouTubeProvider) Match(u *url.URL) bool {
	return u != nil && isYouTubeHost(u.Host)
}

func isYouTubeHost(host string) bool {
	h := strings.ToLower(strings.TrimPrefix(host, "www."))
	if idx := strings.IndexByte(h, ':'); idx > 0 {
		h = h[:idx]
	}
	switch h {
	case "youtube.com", "m.youtube.com", "music.youtube.com", "youtu.be", "youtube-nocookie.com":
		return true
	}
	return false
}

// ExtractYouTubeVideoID parses a standard 11-char YouTube video ID from watch,
// shorts, live, embed, or youtu.be URLs.
func ExtractYouTubeVideoID(u *url.URL) string {
	if u == nil {
		return ""
	}
	h := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	if idx := strings.IndexByte(h, ':'); idx > 0 {
		h = h[:idx]
	}
	if h == "youtu.be" {
		seg := strings.Trim(u.Path, "/")
		if slash := strings.IndexByte(seg, '/'); slash >= 0 {
			seg = seg[:slash]
		}
		if reYouTubeID.MatchString(seg) {
			return seg
		}
		return ""
	}
	if v := u.Query().Get("v"); reYouTubeID.MatchString(v) {
		return v
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		switch strings.ToLower(parts[0]) {
		case "shorts", "embed", "live", "v":
			if reYouTubeID.MatchString(parts[1]) {
				return parts[1]
			}
		}
	}
	return ""
}

func (p *YouTubeProvider) Resolve(ctx context.Context, client *http.Client, rawURL string) ([]ResolvedItem, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	// YouTube Channel / Playlist Atom feed (/feeds/videos.xml?channel_id=...)
	if strings.Contains(strings.ToLower(u.Path), "/feeds/videos.xml") && p.Feed != nil {
		return p.Feed.Resolve(ctx, client, rawURL)
	}
	// Channel URL with UC... channel_id -> auto-resolve channel Atom feed!
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && strings.EqualFold(parts[0], "channel") && strings.HasPrefix(parts[1], "UC") && p.Feed != nil {
		feedURL := "https://www.youtube.com/feeds/videos.xml?channel_id=" + url.QueryEscape(parts[1])
		if items, err := p.Feed.Resolve(ctx, client, feedURL); err == nil && len(items) > 0 {
			return items, nil
		}
	}

	videoID := ExtractYouTubeVideoID(u)
	if videoID == "" {
		return nil, fmt.Errorf("not a direct YouTube video URL")
	}

	canonical := "https://www.youtube.com/watch?v=" + videoID
	oembedBase := p.OEmbedBase
	if oembedBase == "" {
		oembedBase = "https://www.youtube.com/oembed"
	}
	thumbBase := strings.TrimRight(p.ThumbBase, "/")
	if thumbBase == "" {
		thumbBase = "https://i.ytimg.com/vi"
	}

	title := "YouTube " + videoID
	author := ""
	oembedThumb := ""
	oembedURL := oembedBase + "?url=" + url.QueryEscape(canonical) + "&format=json"
	if raw, _, _, err := fetchBytes(ctx, client, oembedURL, "application/json", 512<<10); err == nil {
		var oe struct {
			Title        string `json:"title"`
			AuthorName   string `json:"author_name"`
			ThumbnailURL string `json:"thumbnail_url"`
		}
		if json.Unmarshal(raw, &oe) == nil {
			if strings.TrimSpace(oe.Title) != "" {
				title = strings.TrimSpace(oe.Title)
			}
			author = strings.TrimSpace(oe.AuthorName)
			oembedThumb = strings.TrimSpace(oe.ThumbnailURL)
		}
	}

	// Prefer crisp 1280x720 maxresdefault, falling back smoothly to sddefault / hqdefault / oEmbed thumb.
	primary := thumbBase + "/" + videoID + "/maxresdefault.jpg"
	fallbacks := []string{
		thumbBase + "/" + videoID + "/sddefault.jpg",
		thumbBase + "/" + videoID + "/hqdefault.jpg",
	}
	if oembedThumb != "" {
		fallbacks = append(fallbacks, oembedThumb)
	}
	desc := ""
	tags := []string{"YouTube"}
	if author != "" {
		desc = "Channel: " + author
		tags = append(tags, author)
	}
	if strings.Contains(strings.ToLower(u.Path), "/shorts/") {
		tags = append(tags, "Shorts")
	}

	return []ResolvedItem{{
		Source:      "youtube",
		SourceLabel: "YouTube",
		PageURL:     canonical,
		Title:       title,
		Author:      author,
		Description: desc,
		ImageURLs:   []string{primary},
		FallbackImg: dedupStrings(fallbacks),
		Tags:        dedupStrings(tags),
		IsBookmark:  true,
	}}, nil
}
