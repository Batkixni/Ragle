package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestExtractYouTubeVideoID(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?si=abc":               "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/AbCdEf12345":        "AbCdEf12345",
		"https://m.youtube.com/live/ZyXwVu98765":            "ZyXwVu98765",
		"https://example.com/watch?v=dQw4w9WgXcQ":           "dQw4w9WgXcQ",
	}
	for raw, want := range cases {
		u, _ := url.Parse(raw)
		if got := ExtractYouTubeVideoID(u); got != want {
			t.Fatalf("ExtractYouTubeVideoID(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestYouTubeProviderResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"Lighting Masterclass","author_name":"Studio Lab","thumbnail_url":"https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"}`))
	}))
	defer srv.Close()

	p := &YouTubeProvider{OEmbedBase: srv.URL, ThumbBase: srv.URL + "/vi"}
	items, err := p.Resolve(context.Background(), srv.Client(), "https://youtu.be/dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d, want 1", len(items))
	}
	it := items[0]
	if it.Title != "Lighting Masterclass" || it.Author != "Studio Lab" {
		t.Fatalf("unexpected metadata: %+v", it)
	}
	if it.Source != "youtube" || !it.IsBookmark {
		t.Fatalf("unexpected source/bookmark: %+v", it)
	}
	if len(it.ImageURLs) != 1 || it.ImageURLs[0] != srv.URL+"/vi/dQw4w9WgXcQ/maxresdefault.jpg" {
		t.Fatalf("unexpected primary cover: %v", it.ImageURLs)
	}
	if len(it.FallbackImg) < 2 {
		t.Fatalf("expected HD fallbacks, got %v", it.FallbackImg)
	}
}

func TestUniversalOEmbedAndJSONLDMultiImage(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gallery":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><html><head>
				<link rel="alternate" type="application/json+oembed" href="/oembed.json">
				<script type="application/ld+json">{
					"@context": "https://schema.org",
					"@type": "CreativeWork",
					"name": "Poster Series 04",
					"author": {"@type": "Person", "name": "Hana"},
					"keywords": ["Typography", "Poster"],
					"image": ["/img/1.png", "/img/2.png", "/img/3.png"]
				}</script>
			</head><body></body></html>`))
		case "/oembed.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"type":"rich","provider_name":"CreativeShowcase","author_name":"Hana","thumbnail_url":"` + srv.URL + `/img/cover.png"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := DefaultRegistry().WithClient(srv.Client())
	items, err := reg.Resolve(context.Background(), srv.URL+"/gallery")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d, want 1", len(items))
	}
	it := items[0]
	if it.Title != "Poster Series 04" || it.SourceLabel != "CreativeShowcase" {
		t.Fatalf("unexpected resolved item: %+v", it)
	}
	if len(it.ImageURLs) != 4 {
		t.Fatalf("expected 4 images (oEmbed cover + 3 JSON-LD images), got %d: %v", len(it.ImageURLs), it.ImageURLs)
	}
}

func TestFeedProviderRSSAtomAndJSON(t *testing.T) {
	fp := &FeedProvider{}

	// RSS 2.0
	rss := []byte(`<?xml version="1.0"?><rss version="2.0" xmlns:media="http://search.yahoo.com/mrss/"><channel>
		<title>Visual Dispatch</title>
		<item><title>Color Study</title><link>https://example.com/1</link><media:thumbnail url="https://example.com/1.jpg"/></item>
	</channel></rss>`)
	items, err := fp.Parse("https://example.com/rss.xml", "application/rss+xml", rss)
	if err != nil || len(items) != 1 || items[0].Title != "Color Study" || len(items[0].ImageURLs) != 1 {
		t.Fatalf("RSS parse failed: items=%+v err=%v", items, err)
	}

	// YouTube Atom Feed
	atom := []byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom" xmlns:yt="http://www.youtube.com/xml/schemas/2015">
		<title>Design Channel</title>
		<entry><yt:videoId>AbCdEf12345</yt:videoId><title>Motion Study</title><link rel="alternate" href="https://www.youtube.com/watch?v=AbCdEf12345"/></entry>
	</feed>`)
	items, err = fp.Parse("https://www.youtube.com/feeds/videos.xml?channel_id=UC123", "application/atom+xml", atom)
	if err != nil || len(items) != 1 || items[0].Source != "youtube" || items[0].Title != "Motion Study" {
		t.Fatalf("Atom parse failed: items=%+v err=%v", items, err)
	}

	// JSON Feed 1.1
	jf := []byte(`{"version":"https://jsonfeed.org/version/1.1","title":"Studio Feed","items":[{"id":"1","url":"https://example.com/p1","title":"Grid System","image":"https://example.com/p1.png"}]}`)
	items, err = fp.Parse("https://example.com/feed.json", "application/feed+json", jf)
	if err != nil || len(items) != 1 || items[0].Title != "Grid System" || items[0].ImageURLs[0] != "https://example.com/p1.png" {
		t.Fatalf("JSON Feed parse failed: items=%+v err=%v", items, err)
	}
}

func TestParseURLListAndDetect(t *testing.T) {
	urls := ParseURLList("https://youtu.be/dQw4w9WgXcQ\n# comment\nexample.com/feed.xml\nhttps://youtu.be/dQw4w9WgXcQ\n")
	if len(urls) != 2 {
		t.Fatalf("got %d urls: %v", len(urls), urls)
	}
	if DetectSourceKind(urls[0]) != "youtube" {
		t.Fatalf("expected youtube, got %s", DetectSourceKind(urls[0]))
	}
	if DetectSourceKind(urls[1]) != "feed" {
		t.Fatalf("expected feed, got %s", DetectSourceKind(urls[1]))
	}
}
