package watcher

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"raglex/internal/library"
	"raglex/internal/model"
)

func makePNGBytes(t *testing.T, r, g, b uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	img.SetNRGBA(0, 0, color.NRGBA{R: r, G: g, B: b, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWatchFolderScanAndLiveAutoImport(t *testing.T) {
	lib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	watchDir := t.TempDir()
	// Pre-existing file + ignored .crdownload partial file.
	if err := os.WriteFile(filepath.Join(watchDir, "initial.png"), makePNGBytes(t, 255, 92, 51), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(watchDir, "downloading.png.crdownload"), makePNGBytes(t, 10, 10, 10), 0o644); err != nil {
		t.Fatal(err)
	}

	var changes atomic.Int32
	w, err := Start(lib, func() {
		changes.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.SetWatchRules([]WatchRule{{
		Path:        watchDir,
		TagName:     "InboxWatch",
		StorageMode: model.StorageIndex,
		Recursive:   true,
		Enabled:     true,
	}})

	added, err := w.ScanWatchRules(context.Background())
	if err != nil {
		t.Fatalf("ScanWatchRules: %v", err)
	}
	if added != 1 {
		t.Fatalf("ScanWatchRules added=%d, want 1", added)
	}

	// Drop a brand-new file into the watched folder and verify live auto-import!
	if err := os.WriteFile(filepath.Join(watchDir, "live_drop.png"), makePNGBytes(t, 56, 189, 248), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		assets, _ := lib.List(library.Filter{})
		if len(assets) >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assets, err := lib.List(library.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 auto-imported assets, got %d", len(assets))
	}
	tags, err := lib.Tags()
	if err != nil {
		t.Fatal(err)
	}
	foundTag := false
	for _, tg := range tags {
		if tg.Name == "InboxWatch" {
			foundTag = true
		}
	}
	if !foundTag {
		t.Fatalf("expected auto-created Watch Folder tag 'InboxWatch', got %+v", tags)
	}
}

func TestClipperInboxSidecarIngest(t *testing.T) {
	lib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	inboxDir := t.TempDir()
	imgPath := filepath.Join(inboxDir, "offline_shot.png")
	if err := os.WriteFile(imgPath, makePNGBytes(t, 16, 185, 129), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecarPath := imgPath + ClipSidecarSuffix
	sidecarJSON := `{"version":1,"kind":"image","filename":"offline_shot.png","source_url":"https://example.com/art/1","source_title":"Offline Art","notes":"Saved while closed","tag_names":["OfflineTag"]}`
	if err := os.WriteFile(sidecarPath, []byte(sidecarJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Start(lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.SetWatchRules([]WatchRule{{
		Path:           inboxDir,
		StorageMode:    model.StorageIndex,
		Enabled:        true,
		IsClipperInbox: true,
	}})

	added, err := w.ScanWatchRules(context.Background())
	if err != nil {
		t.Fatalf("ScanWatchRules: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 ingested offline clip, got %d", added)
	}
	if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar cleaned up after ingest, stat err=%v", err)
	}
	assets, err := lib.List(library.Filter{})
	if err != nil || len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d err=%v", len(assets), err)
	}
	if assets[0].SourceURL != "https://example.com/art/1" || assets[0].Notes != "Saved while closed" {
		t.Fatalf("expected offline metadata preserved, got %+v", assets[0])
	}
	tags, _ := lib.Tags()
	found := false
	for _, tg := range tags {
		if tg.Name == "OfflineTag" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected OfflineTag created from sidecar, got %+v", tags)
	}
}

func TestClipperInboxRichMetadataAndFuzzyPairing(t *testing.T) {
	lib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	inboxDir := t.TempDir()
	// Simulate browser rewriting .jpg -> .png and adding " (1)" collision suffix!
	actualImg := filepath.Join(inboxDir, "poster_ref_1764042100 (1).png")
	if err := os.WriteFile(actualImg, makePNGBytes(t, 255, 92, 51), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(inboxDir, "poster_ref_1764042100.jpg"+ClipSidecarSuffix)
	sidecarJSON := `{
		"version": 2,
		"kind": "image",
		"filename": "poster_ref_1764042100.jpg",
		"url": "https://cdn.example.com/posters/01.jpg",
		"source_url": "https://gallery.example.com/works/998?view=full&ref=home",
		"source_title": "Swiss Brutalist Poster 01 — Gallery",
		"site_name": "Gallery",
		"author": "Studio K",
		"description": "High-contrast grid experiment",
		"alt": "Black and red poster",
		"clipped_at": "2026-11-25T05:00:00Z",
		"notes": "Saved offline while closed",
		"tag_names": ["PosterRef"]
	}`
	if err := os.WriteFile(sidecarPath, []byte(sidecarJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Start(lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.SetWatchRules([]WatchRule{{
		Path:           inboxDir,
		StorageMode:    model.StorageIndex,
		Enabled:        true,
		IsClipperInbox: true,
	}})

	added, err := w.ScanWatchRules(context.Background())
	if err != nil {
		t.Fatalf("ScanWatchRules: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 ingested offline clip, got %d", added)
	}
	assets, err := lib.List(library.Filter{})
	if err != nil || len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d err=%v", len(assets), err)
	}
	a := assets[0]
	if a.SourceURL != "https://gallery.example.com/works/998?view=full&ref=home" {
		t.Fatalf("SourceURL=%q", a.SourceURL)
	}
	if a.SourceTitle != "Swiss Brutalist Poster 01 — Gallery" || a.DisplayName != "Swiss Brutalist Poster 01 — Gallery" {
		t.Fatalf("title/display=%q / %q", a.SourceTitle, a.DisplayName)
	}
	if a.Notes == "" || !containsAll(a.Notes, "Saved offline while closed", "Author: Studio K", "Site: Gallery", "Image: https://cdn.example.com/posters/01.jpg") {
		t.Fatalf("expected rich notes preserved, got %q", a.Notes)
	}
}

func TestClipperInboxOfflineSourceFallback(t *testing.T) {
	lib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	inboxDir := t.TempDir()
	sidecarPath := filepath.Join(inboxDir, "source_1764042200"+ClipSidecarSuffix)
	// Unreachable URL simulates a login-walled or currently offline site when Raglex reopens.
	sidecarJSON := `{
		"version": 2,
		"kind": "source",
		"url": "http://127.0.0.1:1/private/work?id=42&tab=detail",
		"source_url": "http://127.0.0.1:1/private/work?id=42&tab=detail",
		"source_title": "Private Moodboard Inspiration",
		"site_name": "PrivateSite",
		"author": "Bax",
		"description": "Layout notes from client deck",
		"notes": "Keep for v2 hero",
		"tag_names": ["OfflineBookmark"]
	}`
	if err := os.WriteFile(sidecarPath, []byte(sidecarJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Start(lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.SetWatchRules([]WatchRule{{
		Path:           inboxDir,
		StorageMode:    model.StorageIndex,
		Enabled:        true,
		IsClipperInbox: true,
	}})

	added, err := w.ScanWatchRules(context.Background())
	if err != nil {
		t.Fatalf("ScanWatchRules: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 fallback bookmark ingested, got %d", added)
	}
	if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar cleaned up after fallback ingest")
	}
	assets, err := lib.List(library.Filter{})
	if err != nil || len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d err=%v", len(assets), err)
	}
	a := assets[0]
	if !a.IsBookmark || a.SourceURL != "http://127.0.0.1:1/private/work?id=42&tab=detail" || a.SourceTitle != "Private Moodboard Inspiration" {
		t.Fatalf("unexpected fallback bookmark: %+v", a)
	}
	if !containsAll(a.Notes, "Keep for v2 hero", "Author: Bax", "Site: PrivateSite") {
		t.Fatalf("unexpected fallback notes: %q", a.Notes)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !bytes.Contains([]byte(s), []byte(sub)) {
			return false
		}
	}
	return true
}
