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
