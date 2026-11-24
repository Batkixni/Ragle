package ui

import (
	"fmt"
	"math"
	"testing"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/i18n"
	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

func newLargeLibraryAppForReflow(t *testing.T, n int, mode string) *App {
	t.Helper()
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lib.Close() })

	a := NewApp()
	a.lib = lib
	a.projects = project.New(lib.DB)
	a.cfg.LibraryPath = dir
	a.lang = i18n.ZhHant
	a.screen = screenLibrary
	a.layoutMode = mode
	a.cardSize = 184
	a.sidebarOpen = true
	a.inspectorOpen = true
	a.assetIndex = -1 // nothing selected -> inspector starts closed

	assets := make([]model.Asset, n)
	byID := make(map[string]model.Asset, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("reflow-%04d", i)
		w, h := 1200, 900
		if i%3 == 1 {
			w, h = 900, 1400
		} else if i%3 == 2 {
			w, h = 1600, 900
		}
		assets[i] = model.Asset{
			ID:     id,
			Path:   fmt.Sprintf("Image_%04d.png", i),
			Mime:   "image/png",
			Width:  w,
			Height: h,
		}
		byID[id] = assets[i]
	}
	a.allAssets = assets
	a.allAssetsCount = n
	a.assetByID = byID
	a.applyFilters()
	return a
}

func TestBentoInspectorOpenAnchorsClickedCard(t *testing.T) {
	const total = 2000
	a := newLargeLibraryAppForReflow(t, total, layoutBento)
	tt := gui.NewTester(a.View, 1280, 820)
	// Let initial layout settle with inspector closed.
	tt.Frame()
	tt.Frame()

	initialCols := a.bentoCache.cols
	if initialCols < 4 {
		t.Fatalf("expected >=4 bento cols before inspector opens, got %d", initialCols)
	}

	// Scroll deep into the library around item #1200.
	targetIdx := 1200
	targetID := a.assets[targetIdx].ID
	itemY, _, ok := bentoItemRect(&a.bentoCache, targetIdx, 14)
	if !ok {
		t.Fatalf("failed to find bento rect for target %d", targetIdx)
	}
	a.bentoScroll.Y = itemY - 240
	tt.Frame()
	tt.Frame()

	beforeBounds, ok := a.cardBounds[targetID]
	if !ok || beforeBounds.H <= 0 {
		t.Fatalf("target card %s not visible before click", targetID)
	}
	beforeRelY := beforeBounds.Y - a.stageBounds.Y

	// Click the card -> opens right Inspector -> narrows stage -> drops column count.
	beforeStageW := a.stageBounds.W
	a.selectCard(targetIdx, 0)
	tt.Frame()
	for step := 0; step < 15; step++ {
		tt.Advance(40 * time.Millisecond)
	}
	tt.Frame()

	if a.bentoCache.cols >= initialCols {
		t.Fatalf("expected bento cols to shrink when inspector opens: before=%d (stageW=%.1f) after=%d (stageW=%.1f) show=%v inspW=%.1f",
			initialCols, beforeStageW, a.bentoCache.cols, a.stageBounds.W, a.inspectorShouldShow(), a.inspectorWidth)
	}
	afterBounds, ok := a.cardBounds[targetID]
	if !ok || afterBounds.H <= 0 {
		t.Fatalf("target card %s jumped out of view after inspector opened! scrollY=%.1f", targetID, a.bentoScroll.Y)
	}
	afterRelY := afterBounds.Y - a.stageBounds.Y
	if math.Abs(float64(afterRelY-beforeRelY)) > 4.0 {
		t.Fatalf("bento clicked card drifted vertically on inspector open: beforeRelY=%.1f afterRelY=%.1f", beforeRelY, afterRelY)
	}

	// Closing inspector widens stage back -> card should remain anchored at the same Y.
	a.inspectorOpen = false
	tt.Frame()
	for step := 0; step < 15; step++ {
		tt.Advance(40 * time.Millisecond)
	}
	tt.Frame()

	if a.bentoCache.cols != initialCols {
		t.Fatalf("expected bento cols to restore to %d after closing inspector, got %d", initialCols, a.bentoCache.cols)
	}
	closedBounds, ok := a.cardBounds[targetID]
	if !ok || closedBounds.H <= 0 {
		t.Fatalf("target card %s jumped out of view after inspector closed", targetID)
	}
	closedRelY := closedBounds.Y - a.stageBounds.Y
	if math.Abs(float64(closedRelY-beforeRelY)) > 4.0 {
		t.Fatalf("bento card drifted vertically on inspector close: beforeRelY=%.1f closedRelY=%.1f", beforeRelY, closedRelY)
	}
}

func TestMasonryInspectorOpenAnchorsClickedCard(t *testing.T) {
	const total = 2000
	a := newLargeLibraryAppForReflow(t, total, layoutMasonry)
	tt := gui.NewTester(a.View, 1280, 820)
	tt.Frame()
	tt.Frame()

	initialCols := a.masonryCache.cols
	if initialCols < 4 {
		t.Fatalf("expected >=4 masonry cols before inspector opens, got %d", initialCols)
	}

	targetIdx := 1250
	targetID := a.assets[targetIdx].ID
	itemY, _, ok := masonryItemRect(&a.masonryCache, targetIdx)
	if !ok {
		t.Fatalf("failed to find masonry rect for target %d", targetIdx)
	}
	a.masonryScroll.Y = itemY - 260
	tt.Frame()
	tt.Frame()

	beforeBounds, ok := a.cardBounds[targetID]
	if !ok || beforeBounds.H <= 0 {
		t.Fatalf("target card %s not visible before click", targetID)
	}
	beforeRelY := beforeBounds.Y - a.stageBounds.Y

	// Select card -> triggers Inspector slide animation (width changes continuously across frames).
	a.selectCard(targetIdx, 0)
	tt.Frame()
	for step := 0; step < 16; step++ {
		tt.Advance(35 * time.Millisecond)
		midRect, ok := tt.Find(fmt.Sprintf("Image_%04d.png", targetIdx))
		if !ok || midRect.H <= 0 {
			t.Fatalf("step %d: target card %s disappeared mid-animation (scrollY=%.1f, cols=%d)", step, targetID, a.masonryScroll.Y, a.masonryCache.cols)
		}
		if midRect.Y+midRect.H < a.stageBounds.Y || midRect.Y > a.stageBounds.Y+a.stageBounds.H {
			t.Fatalf("step %d: masonry card left viewport mid-animation: rect=%+v stage=%+v", step, midRect, a.stageBounds)
		}
	}
	tt.Frame()
	afterBounds, ok := a.cardBounds[targetID]
	if !ok || afterBounds.H <= 0 {
		t.Fatalf("target card %s missing after masonry animation settled (scrollY=%.1f)", targetID, a.masonryScroll.Y)
	}
	afterRelY := afterBounds.Y - a.stageBounds.Y
	if math.Abs(float64(afterRelY-beforeRelY)) > 4.0 {
		t.Fatalf("masonry card drifted after animation: beforeRelY=%.1f afterRelY=%.1f", beforeRelY, afterRelY)
	}

	if a.masonryCache.cols >= initialCols {
		t.Fatalf("expected masonry cols to shrink when inspector opens: before=%d after=%d", initialCols, a.masonryCache.cols)
	}

	// Manual scroll should release the anchor cleanly without snapping back.
	manualY := a.masonryScroll.Y + 900
	a.masonryScroll.Y = manualY
	tt.Frame()
	if a.reflowAnchorValid {
		t.Fatalf("expected manual scroll to release reflowAnchorValid")
	}
	if math.Abs(float64(a.masonryScroll.Y-manualY)) > 4.0 {
		t.Fatalf("expected manual scroll position %.1f to stick, got %.1f", manualY, a.masonryScroll.Y)
	}
}

func TestGridInspectorOpenKeepsClickedCardVisible(t *testing.T) {
	const total = 1500
	a := newLargeLibraryAppForReflow(t, total, layoutGrid)
	tt := gui.NewTester(a.View, 1280, 820)
	tt.Frame()
	tt.Frame()

	initialCols := a.gridLastCols
	if initialCols < 4 {
		t.Fatalf("expected >=4 grid cols before inspector opens, got %d", initialCols)
	}

	targetIdx := 900
	targetID := a.assets[targetIdx].ID
	a.grid.ScrollTo(targetIdx, gui.Center)
	tt.Frame()
	tt.Frame()

	if _, ok := a.cardBounds[targetID]; !ok {
		t.Fatalf("target card %s not visible before click in grid", targetID)
	}

	a.selectCard(targetIdx, 0)
	tt.Frame()
	for step := 0; step < 15; step++ {
		tt.Advance(40 * time.Millisecond)
	}
	tt.Frame()

	if a.gridLastCols >= initialCols {
		t.Fatalf("expected grid cols to shrink when inspector opens: before=%d after=%d", initialCols, a.gridLastCols)
	}
	afterBounds, ok := a.cardBounds[targetID]
	if !ok || afterBounds.H <= 0 {
		t.Fatalf("target card %s jumped out of view in Grid mode after inspector opened", targetID)
	}
	if afterBounds.Y+afterBounds.H < a.stageBounds.Y || afterBounds.Y > a.stageBounds.Y+a.stageBounds.H {
		t.Fatalf("target card %s outside stage bounds after grid reflow: card=%+v stage=%+v", targetID, afterBounds, a.stageBounds)
	}
}
