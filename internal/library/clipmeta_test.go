package library

import (
	"context"
	"strings"
	"testing"
)

func TestFormatClipNotesAndApplyClipMeta(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	asset, added, err := lib.ImportBytes("raw_hash_991.png", makeWebPlaceholderPNG(), nil, "", "")
	if err != nil || !added {
		t.Fatalf("ImportBytes: added=%v err=%v", added, err)
	}

	fullURL := "https://www.pixiv.net/artworks/123456789?utm_source=feed&tab=illustrations"
	title := "Neon Skyline Study — Pixiv"
	updated, err := lib.ApplyClipMeta(asset.ID, ClipMeta{
		SourceURL:    fullURL,
		SourceTitle:  title,
		ImageURL:     "https://i.pximg.net/img-original/img/2026/11/25/123456789_p0.png",
		SiteName:     "Pixiv",
		Author:       "Sora Studio",
		Description:  "Warm vermilion lighting over rain-soaked streets.",
		Alt:          "Cyberpunk alley sketch",
		SelectedText: "Use hard rim light on foreground silhouettes.",
		ClippedAt:    "2026-11-25T05:00:00Z",
		Notes:        "Color ref for poster series",
	})
	if err != nil {
		t.Fatalf("ApplyClipMeta: %v", err)
	}
	if updated.SourceURL != fullURL {
		t.Fatalf("SourceURL=%q, want %q", updated.SourceURL, fullURL)
	}
	if updated.SourceTitle != title {
		t.Fatalf("SourceTitle=%q, want %q", updated.SourceTitle, title)
	}
	if updated.DisplayName != title {
		t.Fatalf("DisplayName=%q, want %q", updated.DisplayName, title)
	}
	for _, want := range []string{
		"Color ref for poster series",
		"Author: Sora Studio",
		"Site: Pixiv",
		"Description: Warm vermilion lighting",
		"Quote: Use hard rim light",
		"Alt: Cyberpunk alley sketch",
		"Image: https://i.pximg.net/img-original/img/2026/11/25/123456789_p0.png",
		"Clipped:",
	} {
		if !strings.Contains(updated.Notes, want) {
			t.Fatalf("Notes missing %q:\n%s", want, updated.Notes)
		}
	}
}

func TestImportFallbackBookmarkPreservesFullURLAndTitle(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	fullURL := "https://x.com/artist_demo/status/199999999999?s=20&t=abc"
	title := "Artist Demo on X: Editorial Poster Process"
	asset, err := lib.ImportFallbackBookmark(context.Background(), ClipMeta{
		SourceURL:   fullURL,
		SourceTitle: title,
		Author:      "@artist_demo",
		SiteName:    "X",
		Description: "Breakdown of halftone grain and grid.",
		Notes:       "Great typography reference",
	}, nil)
	if err != nil {
		t.Fatalf("ImportFallbackBookmark: %v", err)
	}
	if !asset.IsBookmark {
		t.Fatalf("expected IsBookmark=true")
	}
	if asset.SourceURL != fullURL || asset.SourceTitle != title || asset.DisplayName != title {
		t.Fatalf("unexpected bookmark fields: url=%q title=%q display=%q", asset.SourceURL, asset.SourceTitle, asset.DisplayName)
	}
	if !strings.Contains(asset.Notes, "Great typography reference") || !strings.Contains(asset.Notes, "Author: @artist_demo") {
		t.Fatalf("unexpected bookmark notes: %q", asset.Notes)
	}
}
