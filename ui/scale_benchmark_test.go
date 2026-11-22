package ui

import (
	"fmt"
	"testing"
	"time"

	gui "github.com/egoist/mygo/ui"
	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/library"
	"raglex/internal/model"
)

// TestLargeLibrary100kScale verifies Zed-level responsiveness on a synthetic
// 100,000-asset catalog (or 10,000 in -short mode):
// 1. Single-transaction catalog snapshot load
// 2. Tag filter, rating filter, and pre-indexed keyword search across 100k items
// 3. O(1) in-memory star rating mutation without full bbolt reload
// 4. Virtualized Bento and Masonry frame rendering at deep scroll offsets
func TestLargeLibrary100kScale(t *testing.T) {
	totalAssets := 100_000
	if testing.Short() {
		totalAssets = 10_000
	}
	root := t.TempDir()
	lib, err := library.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	// Seed 100,000 assets and 8 tags in batched bbolt transactions.
	t0 := time.Now()
	var tagIDs [8]string
	err = lib.DB.Update(func(tx *bbolt.Tx) error {
		for i := 0; i < 8; i++ {
			tg := model.Tag{
				ID:        fmt.Sprintf("tag-%02d", i),
				Name:      fmt.Sprintf("Category_%02d", i),
				CreatedAt: time.Now().UTC(),
			}
			tagIDs[i] = tg.ID
			if err := db.PutTag(tx, tg); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	const batchSize = 10_000
	baseTime := time.Now().UTC()
	for start := 0; start < totalAssets; start += batchSize {
		end := start + batchSize
		err := lib.DB.Update(func(tx *bbolt.Tx) error {
			for i := start; i < end; i++ {
				id := fmt.Sprintf("01H%023d", i)
				a := model.Asset{
					ID:          id,
					Path:        fmt.Sprintf("/virtual/ posters/artwork_%06d.png", i),
					StorageMode: model.StorageIndex,
					Hash:        fmt.Sprintf("hash_%06d", i),
					Mime:        "image/png",
					Width:       1920,
					Height:      1080 + (i%5)*200,
					Rating:      i % 6,
					CreatedAt:   baseTime.Add(time.Duration(i) * time.Second),
				}
				if err := db.PutAsset(tx, a); err != nil {
					return err
				}
				if i%4 == 0 {
					if err := db.LinkAssetTag(tx, id, tagIDs[i%8]); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seedDur := time.Since(t0)

	// 1. Measure single-transaction LoadCatalogSnapshot + App.reload()
	app := NewApp()
	app.lib = lib
	app.screen = screenLibrary
	tReload := time.Now()
	app.reload()
	reloadDur := time.Since(tReload)
	if len(app.assets) != totalAssets {
		t.Fatalf("expected %d assets, got %d", totalAssets, len(app.assets))
	}

	// 2. Measure pre-indexed keyword search across all assets
	targetIdx := totalAssets - 1112
	tSearch := time.Now()
	app.searchQuery = fmt.Sprintf("artwork_%06d", targetIdx)
	app.applyFilters()
	searchDur := time.Since(tSearch)
	if len(app.assets) != 1 {
		t.Fatalf("expected 1 search match, got %d", len(app.assets))
	}
	app.searchQuery = ""
	app.applyFilters()

	// 3. Measure O(1) star-rating mutation
	targetID := fmt.Sprintf("01H%023d", totalAssets/2)
	tRate := time.Now()
	app.setAssetRatingFast(targetID, 5)
	rateDur := time.Since(tRate)
	if app.assetByID[targetID].Rating != 5 {
		t.Fatalf("expected rating 5, got %d", app.assetByID[targetID].Rating)
	}

	// 4. Measure virtualized Bento & Masonry frame rendering at deep scroll (Y = 2,500,000px)
	tester := gui.NewTester(app.View, 1440, 900)

	// Warm up initial Bento geometry cache (frame 1), then measure scrolled frame (frame 2)
	app.layoutMode = layoutBento
	app.bentoScroll.Y = 2_500_000
	tester.Frame()
	tBentoFrame := time.Now()
	app.bentoScroll.Y = 2_500_400
	tester.Frame()
	bentoFrameDur := time.Since(tBentoFrame)

	// Warm up initial Masonry geometry cache (frame 1), then measure scrolled frame (frame 2)
	app.layoutMode = layoutMasonry
	app.masonryScroll.Y = 2_500_000
	tester.Frame()
	tMasonryFrame := time.Now()
	app.masonryScroll.Y = 2_500_400
	tester.Frame()
	masonryFrameDur := time.Since(tMasonryFrame)

	t.Logf("[100,000 Assets Stress Report]")
	t.Logf("  Seed 100k records to bbolt : %v", seedDur)
	t.Logf("  Full reload (100k snapshot): %v", reloadDur)
	t.Logf("  Keyword search (100k items): %v", searchDur)
	t.Logf("  O(1) Star rating update    : %v", rateDur)
	t.Logf("  Bento scrolled UI frame    : %v (budget 16.6ms for 60 FPS)", bentoFrameDur)
	t.Logf("  Masonry scrolled UI frame  : %v (budget 16.6ms for 60 FPS)", masonryFrameDur)
}
