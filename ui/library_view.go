package ui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
)

type masonryEntry struct {
	index int
	y     float32
	h     float32
}

const (
	libraryScrollTopPad    float32 = 8
	libraryScrollBottomPad float32 = 72
	libraryGridFitGap      float32 = 8
)

type masonryLayoutCache struct {
	gen        uint64
	count      int
	cols       int
	colW       float32
	capH       float32
	buckets    [][]masonryEntry
	colHeights []float32
	itemY      []float32
	itemH      []float32
	maxH       float32
}

type bentoBlockKind uint8

const (
	bentoBlockHero bentoBlockKind = iota
	bentoBlockWide
	bentoBlockRow
)

type bentoBlock struct {
	kind     bentoBlockKind
	blockIdx int
	startIdx int
	count    int
	widePos  int
	heroLeft bool
	y        float32
	h        float32
}

type bentoLayoutCache struct {
	gen    uint64
	count  int
	cols   int
	unitH  float32
	blocks []bentoBlock
	totalH float32
}

type cardRevealKey struct {
	gen int
	id  string
}

func (a *App) libraryView(c *gui.Context) {
	a.withAnimatedInspector(c, "library-insp-layout", func() {
		a.libraryMain(c)
	})
}

func (a *App) libraryMain(c *gui.Context) {
	if !a.dragSeenFrame {
		a.activeDragLeadID = ""
		a.activeDragIDs = nil
	}
	a.dragSeenFrame = false

	dropZone := gui.Column(c.Key("library-dropzone")).Fill().Padding(6, 12, 10, 8).Gap(10).Background(a.pal.canvas)
	if files := dropZone.DroppedFiles(); len(files) > 0 {
		a.importPaths(files)
	}
	dragOver := dropZone.FileDragOver()
	if dragOver {
		dropZone.Background(a.pal.surface)
	}

	dropZone.Children(func() {
		// Drag-and-drop import indicator banner (monochrome).
		if dragOver {
			modeLbl := a.modeLabel(storageModes[a.storageMode])
			gui.Row(c.Key("dnd-banner")).
				Padding(12, 16).
				Radius(radiusPanel).
				Gap(8).
				AlignItems(gui.Center).
				Background(a.pal.surfaceMuted).
				Transition(gui.ElementTransition{
					Animation: &springSmooth,
					Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
					Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
				}).
				Children(func() {
					gui.Icon(c, iconImport).Size(16, 16).TextColor(a.pal.ink)
					gui.Text(c, fmt.Sprintf(a.t("library.dropHint"), modeLbl)).Bold().FontSize(13).TextColor(a.pal.ink)
				})
		}

		// Arc UI Sliding Pill Filter & Storage Bar (smooth zero-bounce collapse/expand).
		if a.filterBarOpen {
			a.filterBar(c)
		}

		// Active Visual Search (Reverse Image or Color Palette) banner.
		if a.similarQueryLabel != "" || a.colorFilter != "" {
			gui.Row(c.Key("visual-search-banner")).
				Padding(8, 14).
				Radius(radiusPanel).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					if a.colorFilter != "" {
						if col, ok := parseHexColor(a.colorFilter); ok {
							gui.Box(c).Size(14, 14).Radius(radiusPill).Background(col)
						}
						gui.Text(c, fmt.Sprintf(a.t("visual.colorBanner"), a.colorFilter, len(a.assets))).
							FontSize(12).Bold().TextColor(a.pal.ink)
					} else {
						gui.Icon(c, iconImage).Size(15, 15).TextColor(a.pal.controlOn)
						gui.Text(c, fmt.Sprintf(a.t("visual.similarBanner"), a.similarQueryLabel, len(a.assets))).
							FontSize(12).Bold().TextColor(a.pal.ink)
					}
					gui.Spacer(c)
					a.toolButton(c, "visual-search-clear", iconX, a.t("common.clear"), false).OnClick(func() {
						a.clearVisualSearch()
					})
				})
		}

		// Expanded Asset Stack header banner.
		if a.openStackID != "" {
			a.stackBanner(c)
		}

		// Multi-select Batch Action Bar (appears when >1 selected or multi-select mode is ON).
		if a.multiSelectMode || len(a.selectedIDs) > 1 {
			a.batchActionBar(c)
		}

		// Project reference mode banner (pure monochrome).
		if a.referenceMode && a.project.Name != "" {
			gui.Row(c.Key("ref-banner")).
				Padding(10, 16).
				Radius(radiusPanel).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Transition(gui.ElementTransition{
					Animation: &springSmooth,
					Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
					Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
				}).
				Children(func() {
					gui.Icon(c, iconFolder).Size(15, 15).TextColor(a.pal.ink)
					gui.Text(c, fmt.Sprintf(a.t("library.referencing"), a.project.Name)).Bold().FontSize(12).TextColor(a.pal.ink)
					gui.Spacer(c)
					selCount := len(a.selectedAssetIDs())
					refLbl := a.t("library.reference")
					if selCount > 0 {
						refLbl = fmt.Sprintf("%s (%d)", refLbl, selCount)
					}
					a.arcButton(c, "ref-selected-btn", refLbl, true).
						Disabled(selCount == 0).
						OnClick(func() {
							a.referenceSelected()
						})
					a.arcButton(c, "ref-done-btn", a.t("common.done"), false).OnClick(func() {
						a.referenceMode = false
						a.multiSelectMode = false
						a.clearSelection()
						if a.projectID != "" {
							a.screen = screenProject
							a.syncNavSelection()
						}
					})
				})
		}

		// Missing-assets quick relink banner.
		if a.navFilter == "missing" && len(a.assets) > 0 {
			gui.Row(c.Key("missing-banner")).
				Padding(10, 16).
				Radius(radiusPanel).
				Gap(8).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Icon(c, iconAlert).Size(15, 15).TextColor(a.pal.danger)
					gui.Text(c, fmt.Sprintf("%s (%d)", a.t("shell.missing"), len(a.assets))).FontSize(12).Bold().TextColor(a.pal.danger)
					gui.Spacer(c)
					a.arcButton(c, "relink-file-all", a.t("inspector.relinkFile"), false).OnClick(func() {
						a.relinkMissing(false)
					})
					a.arcButton(c, "relink-dir-all", a.t("inspector.relink"), true).OnClick(func() {
						a.relinkMissing(true)
					})
				})
		}

		if len(a.assets) == 0 {
			a.libraryEmptyState(c)
		} else {
			a.libraryAssetStage(c)
		}
	})
}

func (a *App) filterBar(c *gui.Context) {
	kinds := []string{
		a.t("library.kind.all"),
		a.t("library.kind.image"),
		a.t("library.kind.video"),
		a.t("library.kind.other"),
	}
	ratings := []string{"0", "★1+", "★2+", "★3+", "★4+", "★5"}
	modes := []string{
		a.t("library.mode.index"),
		a.t("library.mode.symlink"),
		a.t("library.mode.move"),
		a.t("library.mode.copy"),
	}

	gui.Row(c.Key("arc-filter-bar")).
		Padding(4, 2).
		Gap(10).
		Radius(radiusPanel).
		AlignItems(gui.Center).
		Wrap().
		Transition(gui.ElementTransition{
			Animation: &springSmooth,
			Enter:     &gui.Motion{Y: -8, Opacity: 0, Collapse: true},
			Exit:      &gui.Motion{Y: -8, Opacity: 0, Collapse: true},
		}).
		Children(func() {
			a.arcPillTabs(c, "filter-kind", a.kindFilter, kinds, func(idx int) {
				a.kindFilter = idx
				a.revealGen++
				a.applyFilters()
			})

			gui.Row(c).Gap(4).AlignItems(gui.Center).Children(func() {
				for i, lbl := range ratings {
					idx := i
					if idx == 0 {
						continue
					}
					active := a.minRating == idx
					a.tagChip(c, fmt.Sprintf("frate-%d", idx), lbl, active).OnClick(func() {
						if a.minRating == idx {
							a.minRating = 0
						} else {
							a.minRating = idx
						}
						a.revealGen++
						a.applyFilters()
					})
				}
			})

			gui.Row(c).Gap(5).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("visual.colorFilter")).FontSize(11).TextColor(a.pal.inkMuted)
				for _, hex := range colorFilterSwatches {
					hx := hex
					col, ok := parseHexColor(hx)
					if !ok {
						continue
					}
					on := strings.EqualFold(a.colorFilter, hx)
					sz := float32(15)
					if on {
						sz = 18
					}
					btn := gui.ButtonBase(c.Key("fcol-" + hx)).
						Size(sz, sz).
						Radius(radiusPill).
						Background(col).
						Tooltip(hx).
						OnClick(func() { a.setColorFilter(hx) })
					if on {
						btn.Border(2, a.pal.ink)
					}
				}
				a.toolButton(c, "fbar-sim-img", iconImage, a.t("visual.searchByImage"), a.similarQueryLabel != "").
					Padding(4, 9).
					OnClick(func() { a.searchSimilarByFileDialog() })
			})

			if a.hasActiveFilters() {
				a.toolButton(c, "fbar-clear", iconX, a.t("common.clear"), false).OnClick(func() {
					a.revealGen++
					a.clearFilters()
				})
			}

			gui.Spacer(c)

			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("library.mode")).FontSize(11).TextColor(a.pal.inkMuted)
				a.arcPillTabs(c, "storage-mode", a.storageMode, modes, func(idx int) {
					a.setStorageMode(idx)
				})
			})
		})
}

func (a *App) batchActionBar(c *gui.Context) {
	ids := a.selectedAssetIDs()
	gui.Row(c.Key("batch-action-bar")).
		Padding(8, 14).
		Gap(8).
		Radius(radiusPanel).
		Background(a.pal.surface).
		AlignItems(gui.Center).
		Wrap().
		Transition(gui.ElementTransition{
			Animation: &springSmooth,
			Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
			Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
		}).
		Children(func() {
			gui.Icon(c, iconCheckSquare).Size(15, 15).TextColor(a.pal.ink)
			gui.Text(c, fmt.Sprintf(a.t("batch.selected"), len(ids))).FontSize(12).Bold().TextColor(a.pal.ink)
			gui.Text(c, a.t("batch.dragHint")).FontSize(11).TextColor(a.pal.inkMuted)

			gui.Spacer(c)

			a.toolButton(c, "batch-sel-all", nil, a.t("batch.selectAll"), false).OnClick(func() {
				a.selectAllVisible()
			})

			if len(ids) > 0 && len(a.projectList) > 0 {
				gui.PopoverMenuButton(c.Key("batch-add-proj"), a.t("batch.addToProject"), func(m *gui.Menu) {
					for _, p := range a.projectList {
						proj := p
						if a.menuItem(m, iconFolder, proj.Name, "", false).Chosen() {
							a.batchReferenceToProject(ids, proj.ID)
						}
					}
				}, a.popoverMenuBtnOpts())
			}

			if len(ids) > 0 && len(a.folders) > 0 {
				gui.PopoverMenuButton(c.Key("batch-apply-folder"), a.t("folder.addTo"), func(m *gui.Menu) {
					for _, f := range a.folders {
						folder := f
						if a.menuItem(m, iconFolder, a.folderFullPath(folder.ID), "", false).Chosen() {
							a.batchApplyFolder(ids, folder.ID)
						}
					}
				}, a.popoverMenuBtnOpts())
			}

			if len(ids) > 0 && len(a.tags) > 0 {
				gui.PopoverMenuButton(c.Key("batch-apply-tag"), a.t("batch.applyTag"), func(m *gui.Menu) {
					for _, tg := range a.tags {
						tag := tg
						if a.menuItem(m, iconTag, tag.Name, "", false).Chosen() {
							a.batchApplyTag(ids, tag.ID)
						}
					}
				}, a.popoverMenuBtnOpts())
			}

			if len(ids) > 0 {
				gui.PopoverMenuButton(c.Key("batch-send-board"), a.t("board.sendTo"), func(m *gui.Menu) {
					if a.menuItem(m, iconPlus, a.t("board.sendToNew"), "", false).Chosen() {
						a.sendAssetsToNewBoard(ids)
					}
					if len(a.standaloneBoards) > 0 {
						m.Separator()
						for _, sb := range a.standaloneBoards {
							b := sb
							if a.menuItem(m, iconBoard, b.Name, "", false).Chosen() {
								a.sendAssetsToBoard(ids, b.ID)
							}
						}
					}
				}, a.popoverMenuBtnOpts())
				if len(ids) >= 2 {
					a.toolButton(c, "batch-stack-btn", iconLayers, a.t("stack.create"), false).OnClick(func() {
						a.stackSelectedAssets(ids)
					})
				}
				a.toolButton(c, "batch-rename-btn", iconEdit, a.t("rename.batchAction"), false).OnClick(func() {
					a.openRenameModal(ids)
				})
				a.toolButton(c, "batch-export-btn", iconExport, a.t("export.action"), false).OnClick(func() {
					a.openExportModal(ids)
				})
				a.toolButton(c, "batch-copy-files-btn", iconCopy, a.t("clipboard.copyFiles"), false).OnClick(func() {
					a.copySelectedFilesToClipboard(ids)
				})
				a.arcButton(c, "batch-del-btn", a.t("batch.delete"), false).
					TextColor(a.pal.danger).
					OnClick(func() {
						a.batchDeletePrompt(ids)
					})
			}

			a.toolButton(c, "batch-clear", iconX, a.t("batch.clear"), false).OnClick(func() {
				a.clearSelection()
			})
		})
}

func (a *App) libraryEmptyState(c *gui.Context) {
	empty := gui.Column(c.Key("lib-empty")).Grow(1).Center().Gap(14).Padding(32)
	applyBlurReveal(empty, a.revealGen, 0)
	empty.Children(func() {
		gui.Box(c).Size(56, 56).Radius(radiusPanel).Background(a.pal.surface).Center().Children(func() {
			gui.Icon(c, iconImage).Size(24, 24).TextColor(a.pal.inkMuted)
		})
		if a.hasActiveFilters() {
			gui.Text(c, a.t("library.noMatch")).FontSize(15).Bold()
			a.arcButton(c, "empty-clear", a.t("common.clear"), false).OnClick(func() { a.clearFilters() })
		} else {
			gui.Text(c, a.t("library.empty")).FontSize(16).Bold()
			gui.Text(c, a.t("library.emptyHint")).
				Width(380).
				TextAlign(gui.Center).
				FontSize(12).
				TextColor(a.pal.inkSecondary).
				LineHeight(1.55)
			gui.Row(c).Gap(10).Padding(6, 0).Children(func() {
				a.arcButton(c, "empty-import-files", a.t("library.files"), true).OnClick(func() {
					a.startImport(false)
				})
				a.arcButton(c, "empty-import-folder", a.t("library.folder"), false).OnClick(func() {
					a.startImport(true)
				})
			})
		}
	})
}

// libraryAssetStage wraps Bento, Waterfall Masonry, and Grid views with
// native rubber-band marquee box selection (hold left mouse button on empty
// space, gaps, or margins and drag a rectangle to multi-select cards live).
func (a *App) libraryAssetStage(c *gui.Context) {
	if a.layoutMode == layoutGrid && a.reflowAnchorValid {
		if a.reflowAnchorGen != a.layoutGen {
			a.reflowAnchorValid = false
		} else if a.gridReflowGrace > 0 {
			a.gridReflowGrace--
		} else if len(a.cardBounds) > 0 {
			if _, visible := a.cardBounds[a.reflowAnchorID]; !visible {
				a.reflowAnchorValid = false
			}
		}
	}
	a.cardBounds = make(map[string]gui.Rect, 64)
	stage := gui.Box(c.Key("library-stage")).
		Grow(1).
		FillWidth().
		Clip().
		HandleInput(a.libraryMarqueeInput).
		DrawOver(a.paintLibraryMarquee)
	a.stageBounds = stage.Bounds()
	stage.Children(func() {
		switch a.layoutMode {
		case layoutMasonry:
			a.assetMasonry(c)
		case layoutGrid:
			a.assetGrid(c)
		default:
			a.assetBento(c)
		}
	})
}

func (a *App) libraryMarqueeInput(ev gui.InputEvent) bool {
	switch ev.Kind {
	case gui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		// If pressing directly on an asset card (without Alt override), let the
		// card's ButtonBase handle click, double-click, and drag-to-project.
		if ev.Mods&gui.Alt == 0 {
			wx := a.stageBounds.X + ev.X
			wy := a.stageBounds.Y + ev.Y
			for _, cb := range a.cardBounds {
				if cb.W > 0 && cb.H > 0 && wx >= cb.X && wx <= cb.X+cb.W && wy >= cb.Y && wy <= cb.Y+cb.H {
					return false
				}
			}
		}
		a.marqueeActive = true
		a.marqueeDragging = false
		a.marqueeX0, a.marqueeY0 = ev.X, ev.Y
		a.marqueeX1, a.marqueeY1 = ev.X, ev.Y
		a.marqueeAdditive = ev.Mods&(gui.Shift|gui.Cmd|gui.Ctrl) != 0
		a.marqueeBase = make(map[string]bool, len(a.selectedIDs))
		if a.marqueeAdditive {
			for id := range a.selectedIDs {
				a.marqueeBase[id] = true
			}
		}
		return true
	case gui.InputPointerMove:
		if !a.marqueeActive {
			return false
		}
		a.marqueeX1, a.marqueeY1 = ev.X, ev.Y
		dx := a.marqueeX1 - a.marqueeX0
		dy := a.marqueeY1 - a.marqueeY0
		if !a.marqueeDragging && (dx*dx+dy*dy) >= 16 {
			a.marqueeDragging = true
		}
		if a.marqueeDragging {
			a.applyLibraryMarquee()
		}
		return true
	case gui.InputPointerUp:
		if !a.marqueeActive {
			return false
		}
		wasDragging := a.marqueeDragging
		a.marqueeActive = false
		a.marqueeDragging = false
		if !wasDragging && !a.marqueeAdditive {
			// Plain click on empty canvas clears selection cleanly.
			a.selectedIDs = map[string]bool{}
			a.assetIndex = -1
			a.syncInspectorSelection(false)
		}
		return true
	}
	return false
}

func (a *App) applyLibraryMarquee() {
	x0, x1 := a.marqueeX0, a.marqueeX1
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	y0, y1 := a.marqueeY0, a.marqueeY1
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	winBox := gui.Rect{
		X: a.stageBounds.X + x0,
		Y: a.stageBounds.Y + y0,
		W: x1 - x0,
		H: y1 - y0,
	}
	next := make(map[string]bool, len(a.marqueeBase))
	if a.marqueeAdditive {
		for id := range a.marqueeBase {
			next[id] = true
		}
	}
	firstIdx := -1
	for i, asset := range a.assets {
		cb, ok := a.cardBounds[asset.ID]
		if !ok || cb.W <= 0 || cb.H <= 0 {
			continue
		}
		if rectsIntersect(winBox, cb) {
			next[asset.ID] = true
			if firstIdx < 0 {
				firstIdx = i
			}
		}
	}
	a.selectedIDs = next
	if firstIdx >= 0 {
		a.assetIndex = firstIdx
		a.anchorIndex = firstIdx
	} else if len(next) == 0 {
		a.assetIndex = -1
	}
	a.syncInspectorSelection(false)
}

func (a *App) paintLibraryMarquee(p *gui.Painter, r gui.Rect) {
	if !a.marqueeActive || !a.marqueeDragging {
		return
	}
	x0, x1 := a.marqueeX0, a.marqueeX1
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	y0, y1 := a.marqueeY0, a.marqueeY1
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	box := gui.Rect{
		X: r.X + x0,
		Y: r.Y + y0,
		W: x1 - x0,
		H: y1 - y0,
	}
	p.Clip(r, 0, func() {
		fill := gui.RGBA(a.pal.controlOn.R, a.pal.controlOn.G, a.pal.controlOn.B, 0.12)
		stroke := gui.RGBA(a.pal.controlOn.R, a.pal.controlOn.G, a.pal.controlOn.B, 0.72)
		p.Fill(box, fill, 4)
		p.Stroke(box, stroke, 4, 1.25)
	})
}

func rectsIntersect(a, b gui.Rect) bool {
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

func (a *App) assetGrid(c *gui.Context) {
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	// Keep GridState.Selected nil so GridView does NOT paint its default boxy
	// selection chrome; each frameless Arc card manages its own click,
	// double-click, hover lift, press scale, and monochrome selected state.
	a.grid.Selected = nil
	a.grid.Key = func(i int) any {
		if i < 0 || i >= len(a.assets) {
			return i
		}
		return a.assets[i].ID
	}
	a.grid.Label = func(i int) string {
		if i < 0 || i >= len(a.assets) {
			return ""
		}
		return a.assetName(a.assets[i])
	}

	cellW := float32(a.cardSize)
	cellH := cellW + 38
	if !a.showCardCaption {
		cellH = cellW
	}

	gv := gui.GridView(c.Key("asset-grid"), &a.grid, len(a.assets), cellW, cellH, func(i int) {
		asset := a.assets[i]
		a.assetCard(c, i, asset, a.isCardSelected(i, asset.ID))
	}).
		Gap(16).
		Grow(1)

	gvBounds := gv.LayoutBounds()
	availW := gvBounds.W
	if availW <= 0 {
		availW = a.stageBounds.W
	}
	viewH := gvBounds.H
	if viewH <= 0 {
		viewH = a.stageBounds.H
	}
	if availW > 0 {
		curCols := int((availW + libraryGridFitGap) / (cellW + libraryGridFitGap))
		if curCols < 1 {
			curCols = 1
		}
		if a.gridLastGen == a.layoutGen && a.gridLastCols > 0 && a.gridLastCols != curCols && a.reflowAnchorValid && a.reflowAnchorGen == a.layoutGen {
			if idx := a.findAssetIndex(a.reflowAnchorID); idx >= 0 {
				a.grid.ScrollTo(idx, gridAlignForOffset(a.reflowAnchorOffsetY, viewH))
				a.gridReflowGrace = 2
			}
		}
		a.gridLastCols = curCols
		a.gridLastGen = a.layoutGen
	}

	gv.OnShortcut(0, gui.KeySpace, func() {
		a.handleSpacePreview()
	})
	gv.OnShortcut(0, gui.KeyDelete, func() {
		ids := a.selectedAssetIDs()
		if len(ids) > 0 {
			a.batchDeletePrompt(ids)
		}
	})
}

func (a *App) findAssetIndex(id string) int {
	if id == "" {
		return -1
	}
	if a.assetIdx != nil {
		if idx, ok := a.assetIdx[id]; ok && idx >= 0 && idx < len(a.assets) && a.assets[idx].ID == id {
			return idx
		}
	}
	for i := range a.assets {
		if a.assets[i].ID == id {
			return i
		}
	}
	return -1
}

func gridAlignForOffset(offsetY, viewH float32) gui.Align {
	if viewH <= 0 {
		return gui.Center
	}
	ratio := offsetY / viewH
	switch {
	case ratio < 0.28:
		return gui.Start
	case ratio > 0.68:
		return gui.End
	default:
		return gui.Center
	}
}

func bentoItemRect(cache *bentoLayoutCache, idx int, gap float32) (float32, float32, bool) {
	if cache == nil || idx < 0 || idx >= cache.count || len(cache.blocks) == 0 {
		return 0, 0, false
	}
	bIdx := sort.Search(len(cache.blocks), func(k int) bool {
		return cache.blocks[k].startIdx+cache.blocks[k].count > idx
	})
	if bIdx >= len(cache.blocks) {
		return 0, 0, false
	}
	blk := cache.blocks[bIdx]
	if idx < blk.startIdx || idx >= blk.startIdx+blk.count {
		return 0, 0, false
	}
	if blk.kind == bentoBlockHero {
		rel := idx - blk.startIdx
		if rel == 0 {
			return libraryScrollTopPad + blk.y, blk.h, true
		}
		sideCols := cache.cols - 2
		if rel <= sideCols {
			return libraryScrollTopPad + blk.y, cache.unitH, true
		}
		return libraryScrollTopPad + blk.y + cache.unitH + gap, cache.unitH, true
	}
	return libraryScrollTopPad + blk.y, blk.h, true
}

func bentoAnchorIndexAtY(cache *bentoLayoutCache, contentY float32) int {
	if cache == nil || len(cache.blocks) == 0 {
		return -1
	}
	localY := contentY - libraryScrollTopPad
	if localY < 0 {
		localY = 0
	}
	bIdx := sort.Search(len(cache.blocks), func(k int) bool {
		return cache.blocks[k].y+cache.blocks[k].h >= localY
	})
	if bIdx >= len(cache.blocks) {
		bIdx = len(cache.blocks) - 1
	}
	return cache.blocks[bIdx].startIdx
}

func masonryItemRect(cache *masonryLayoutCache, idx int) (float32, float32, bool) {
	if cache == nil || idx < 0 || idx >= len(cache.itemY) || idx >= len(cache.itemH) {
		return 0, 0, false
	}
	return libraryScrollTopPad + cache.itemY[idx], cache.itemH[idx], true
}

func masonryAnchorIndexAtY(cache *masonryLayoutCache, contentY float32) int {
	if cache == nil || len(cache.buckets) == 0 {
		return -1
	}
	localY := contentY - libraryScrollTopPad
	if localY < 0 {
		localY = 0
	}
	bestIdx := -1
	bestDist := float32(1e30)
	for _, entries := range cache.buckets {
		if len(entries) == 0 {
			continue
		}
		pos := sort.Search(len(entries), func(k int) bool {
			return entries[k].y+entries[k].h >= localY
		})
		if pos >= len(entries) {
			pos = len(entries) - 1
		}
		ent := entries[pos]
		mid := ent.y + ent.h*0.5
		d := mid - localY
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestIdx = ent.index
		}
	}
	return bestIdx
}

// captureScrollAnchor records the clicked asset's current vertical offset inside
// the library viewport so subsequent width reflows (e.g. opening or resizing the
// right Inspector panel) keep the clicked image anchored in place.
func (a *App) captureScrollAnchor(assetID string) {
	idx := a.findAssetIndex(assetID)
	if idx < 0 {
		return
	}
	viewH := a.stageBounds.H
	if viewH < 240 {
		viewH = 820
	}
	var (
		offsetY float32
		cardH   float32
		scrollY float32
		found   bool
	)
	switch a.layoutMode {
	case layoutMasonry:
		scrollY = a.masonryScroll.Y
		if a.masonryCache.gen == a.layoutGen && a.masonryCache.count == len(a.assets) {
			if y, h, ok := masonryItemRect(&a.masonryCache, idx); ok {
				offsetY = y - scrollY
				cardH = h
				found = true
			}
		}
	case layoutGrid:
		if cb, ok := a.cardBounds[assetID]; ok && cb.H > 0 && a.stageBounds.H > 0 {
			offsetY = cb.Y - a.stageBounds.Y
			cardH = cb.H
			found = true
		}
	default:
		scrollY = a.bentoScroll.Y
		if a.bentoCache.gen == a.layoutGen && a.bentoCache.count == len(a.assets) {
			if y, h, ok := bentoItemRect(&a.bentoCache, idx, 14); ok {
				offsetY = y - scrollY
				cardH = h
				found = true
			}
		}
	}
	if !found {
		if cb, ok := a.cardBounds[assetID]; ok && cb.H > 0 && a.stageBounds.H > 0 {
			offsetY = cb.Y - a.stageBounds.Y
			cardH = cb.H
			found = true
		}
	}
	if !found || offsetY < -cardH || offsetY > viewH {
		offsetY = viewH * 0.32
	}
	a.reflowAnchorID = assetID
	a.reflowAnchorOffsetY = offsetY
	a.reflowAnchorViewH = viewH
	a.reflowAnchorValid = true
	a.reflowAnchorGen = a.layoutGen
	a.reflowLastScrollY = scrollY
}

func (a *App) syncReflowAnchorManualScroll(curScrollY, maxScrollY float32) {
	if !a.reflowAnchorValid {
		return
	}
	if a.reflowAnchorGen != a.layoutGen {
		a.reflowAnchorValid = false
		return
	}
	diff := curScrollY - a.reflowLastScrollY
	if diff < 0 {
		diff = -diff
	}
	if diff > 2.0 {
		clampedAtEnd := maxScrollY >= 0 && curScrollY <= maxScrollY+1 && curScrollY >= maxScrollY-1 && a.reflowLastScrollY > maxScrollY
		if !clampedAtEnd {
			a.reflowAnchorValid = false
			return
		}
		a.reflowLastScrollY = curScrollY
	}
}

func (a *App) autoCaptureBentoAnchor(scrollY, viewH, gap float32) {
	if scrollY <= 1 && a.assetIndex < 0 {
		return
	}
	if a.assetIndex >= 0 && a.assetIndex < len(a.assets) {
		if y, h, ok := bentoItemRect(&a.bentoCache, a.assetIndex, gap); ok && y+h >= scrollY && y <= scrollY+viewH {
			a.reflowAnchorID = a.assets[a.assetIndex].ID
			a.reflowAnchorOffsetY = y - scrollY
			a.reflowAnchorViewH = viewH
			a.reflowAnchorValid = true
			a.reflowAnchorGen = a.layoutGen
			a.reflowLastScrollY = scrollY
			return
		}
	}
	if scrollY <= 1 {
		return
	}
	if idx := bentoAnchorIndexAtY(&a.bentoCache, scrollY+viewH*0.35); idx >= 0 && idx < len(a.assets) {
		if y, _, ok := bentoItemRect(&a.bentoCache, idx, gap); ok {
			a.reflowAnchorID = a.assets[idx].ID
			a.reflowAnchorOffsetY = y - scrollY
			a.reflowAnchorViewH = viewH
			a.reflowAnchorValid = true
			a.reflowAnchorGen = a.layoutGen
			a.reflowLastScrollY = scrollY
		}
	}
}

func (a *App) applyBentoReflowAnchor(viewH, gap float32) {
	idx := a.findAssetIndex(a.reflowAnchorID)
	if idx < 0 {
		a.reflowAnchorValid = false
		return
	}
	newY, _, ok := bentoItemRect(&a.bentoCache, idx, gap)
	if !ok {
		return
	}
	maxScroll := a.bentoCache.totalH + libraryScrollTopPad + libraryScrollBottomPad - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	targetY := newY - a.reflowAnchorOffsetY
	if targetY < 0 {
		targetY = 0
	} else if targetY > maxScroll {
		targetY = maxScroll
	}
	a.bentoScroll.Y = targetY
	a.reflowLastScrollY = targetY
}

func (a *App) autoCaptureMasonryAnchor(scrollY, viewH float32) {
	if scrollY <= 1 && a.assetIndex < 0 {
		return
	}
	if a.assetIndex >= 0 && a.assetIndex < len(a.assets) {
		if y, h, ok := masonryItemRect(&a.masonryCache, a.assetIndex); ok && y+h >= scrollY && y <= scrollY+viewH {
			a.reflowAnchorID = a.assets[a.assetIndex].ID
			a.reflowAnchorOffsetY = y - scrollY
			a.reflowAnchorViewH = viewH
			a.reflowAnchorValid = true
			a.reflowAnchorGen = a.layoutGen
			a.reflowLastScrollY = scrollY
			return
		}
	}
	if scrollY <= 1 {
		return
	}
	if idx := masonryAnchorIndexAtY(&a.masonryCache, scrollY+viewH*0.35); idx >= 0 && idx < len(a.assets) {
		if y, _, ok := masonryItemRect(&a.masonryCache, idx); ok {
			a.reflowAnchorID = a.assets[idx].ID
			a.reflowAnchorOffsetY = y - scrollY
			a.reflowAnchorViewH = viewH
			a.reflowAnchorValid = true
			a.reflowAnchorGen = a.layoutGen
			a.reflowLastScrollY = scrollY
		}
	}
}

func (a *App) applyMasonryReflowAnchor(viewH float32) {
	idx := a.findAssetIndex(a.reflowAnchorID)
	if idx < 0 {
		a.reflowAnchorValid = false
		return
	}
	newY, _, ok := masonryItemRect(&a.masonryCache, idx)
	if !ok {
		return
	}
	maxScroll := a.masonryCache.maxH + libraryScrollTopPad + libraryScrollBottomPad - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	targetY := newY - a.reflowAnchorOffsetY
	if targetY < 0 {
		targetY = 0
	} else if targetY > maxScroll {
		targetY = maxScroll
	}
	a.masonryScroll.Y = targetY
	a.reflowLastScrollY = targetY
}

func (a *App) bindAssetShortcuts(el gui.Element) {
	el.OnShortcut(0, gui.KeySpace, func() {
		a.handleSpacePreview()
	})
	el.OnShortcut(0, gui.KeyDelete, func() {
		ids := a.selectedAssetIDs()
		if len(ids) > 0 {
			a.batchDeletePrompt(ids)
		}
	})
}

// assetMasonry renders a multi-column waterfall masonry layout
// preserving each asset's natural aspect ratio in balanced shortest-column order,
// with cached prefix-sum geometry and O(log N) binary search viewport culling.
func (a *App) assetMasonry(c *gui.Context) {
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	scroll := gui.Scroll(c.Key("asset-masonry")).Grow(1).FillWidth()
	a.bindAssetShortcuts(scroll)

	layout := scroll.LayoutBounds()
	availW := layout.W
	if availW < 240 {
		availW = 780
	}
	viewH := layout.H
	if viewH < 240 {
		viewH = 820
	}

	a.syncReflowAnchorManualScroll(a.masonryScroll.Y, a.masonryScroll.MaxY)

	const gap float32 = 14
	cellW := float32(a.cardSize)
	cols := int((availW + gap) / (cellW + gap))
	if cols < 2 {
		if availW < 360 {
			cols = 1
		} else {
			cols = 2
		}
	}
	if cols > 6 {
		cols = 6
	}
	colW := (availW - float32(cols-1)*gap) / float32(cols)
	if colW < 120 {
		colW = 120
	}
	var capH float32 = 36
	if !a.showCardCaption {
		capH = 0
	}

	if a.masonryCache.gen != a.layoutGen ||
		a.masonryCache.count != len(a.assets) ||
		a.masonryCache.cols != cols ||
		a.masonryCache.colW != colW ||
		a.masonryCache.capH != capH {
		isReflow := a.masonryCache.gen == a.layoutGen &&
			a.masonryCache.count == len(a.assets) &&
			a.masonryCache.cols > 0
		if isReflow && !a.reflowAnchorValid {
			a.autoCaptureMasonryAnchor(a.masonryScroll.Y, viewH)
		}
		buckets := make([][]masonryEntry, cols)
		colHeights := make([]float32, cols)
		itemY := make([]float32, len(a.assets))
		itemH := make([]float32, len(a.assets))
		estPerCol := len(a.assets)/cols + 1
		for col := 0; col < cols; col++ {
			buckets[col] = make([]masonryEntry, 0, estPerCol)
		}
		for i, asset := range a.assets {
			ratio := float32(0.82)
			if asset.Width > 0 && asset.Height > 0 {
				ratio = float32(asset.Height) / float32(asset.Width)
			} else {
				presets := [...]float32{0.72, 1.15, 0.92, 1.28, 0.78}
				ratio = presets[i%len(presets)]
			}
			if ratio < 0.55 {
				ratio = 0.55
			} else if ratio > 1.85 {
				ratio = 1.85
			}
			h := colW*ratio + capH
			best := 0
			for k := 1; k < cols; k++ {
				if colHeights[k] < colHeights[best] {
					best = k
				}
			}
			y := colHeights[best]
			itemY[i] = y
			itemH[i] = h
			buckets[best] = append(buckets[best], masonryEntry{index: i, y: y, h: h})
			colHeights[best] = y + h + gap
		}
		var maxH float32
		for _, ch := range colHeights {
			h := ch
			if h > gap {
				h -= gap
			}
			if h > maxH {
				maxH = h
			}
		}
		a.masonryCache = masonryLayoutCache{
			gen:        a.layoutGen,
			count:      len(a.assets),
			cols:       cols,
			colW:       colW,
			capH:       capH,
			buckets:    buckets,
			colHeights: colHeights,
			itemY:      itemY,
			itemH:      itemH,
			maxH:       maxH,
		}
		if isReflow && a.reflowAnchorValid {
			a.applyMasonryReflowAnchor(viewH)
		}
	}

	scroll.TrackScroll(&a.masonryScroll)

	scrollY := a.masonryScroll.Y
	const overscan float32 = 720
	minY := scrollY - overscan
	maxY := scrollY + viewH + overscan

	buckets := a.masonryCache.buckets
	colHeights := a.masonryCache.colHeights

	scroll.Children(func() {
		gui.Row(c).FillWidth().Padding(8, 12, 72, 8).Gap(gap).AlignItems(gui.Start).Children(func() {
			for col := 0; col < cols; col++ {
				entries := buckets[col]
				totalH := colHeights[col]
				if totalH > gap {
					totalH -= gap
				}
				gui.Column(c.Key(fmt.Sprintf("mcol-%d", col))).Grow(1).Gap(gap).Children(func() {
					if len(entries) == 0 {
						return
					}
					// O(log N) binary search for first entry whose bottom >= minY.
					firstVis := sort.Search(len(entries), func(idx int) bool {
						return entries[idx].y+entries[idx].h >= minY
					})
					if firstVis >= len(entries) || entries[firstVis].y > maxY {
						gui.Box(c).FillWidth().Height(totalH)
						return
					}
					// O(log N) binary search for first entry whose top > maxY.
					afterLast := sort.Search(len(entries)-firstVis, func(offset int) bool {
						return entries[firstVis+offset].y > maxY
					})
					lastVis := firstVis + afterLast - 1
					if lastVis < firstVis {
						lastVis = firstVis
					}

					if topPad := entries[firstVis].y - gap; topPad > 0 {
						gui.Box(c).FillWidth().Height(topPad)
					}
					for idx := firstVis; idx <= lastVis; idx++ {
						ent := entries[idx]
						asset := a.assets[ent.index]
						gui.Box(c.Key("mwrap-" + asset.ID)).FillWidth().Height(ent.h).Children(func() {
							a.assetCard(c, ent.index, asset, a.isCardSelected(ent.index, asset.ID))
						})
					}
					lastEnt := entries[lastVis]
					if botPad := totalH - (lastEnt.y + lastEnt.h) - gap; botPad > 0 {
						gui.Box(c).FillWidth().Height(botPad)
					}
				})
			}
		})
	})
}

// assetBento renders an editorial Arc UI Bento grid with alternating 2x2 hero
// blocks and wide accent rows, using cached block geometry, O(log N) binary
// search, and a SINGLE topPad + SINGLE botPad spacer for 250k+ asset scale.
func (a *App) assetBento(c *gui.Context) {
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	scroll := gui.Scroll(c.Key("asset-bento")).Grow(1).FillWidth()
	a.bindAssetShortcuts(scroll)

	layout := scroll.LayoutBounds()
	availW := layout.W
	if availW < 240 {
		availW = 780
	}
	viewH := layout.H
	if viewH < 240 {
		viewH = 820
	}

	a.syncReflowAnchorManualScroll(a.bentoScroll.Y, a.bentoScroll.MaxY)

	const gap float32 = 14
	cellW := float32(a.cardSize)
	cols := int((availW + gap) / (cellW + gap))
	if cols < 2 {
		cols = 2
	}
	if cols > 5 {
		cols = 5
	}
	var capH float32 = 34
	if !a.showCardCaption {
		capH = 0
	}
	unitH := cellW*0.86 + capH
	if unitH < 150 {
		unitH = 150
	}
	if unitH > 290 {
		unitH = 290
	}

	if a.bentoCache.gen != a.layoutGen ||
		a.bentoCache.count != len(a.assets) ||
		a.bentoCache.cols != cols ||
		a.bentoCache.unitH != unitH {
		isReflow := a.bentoCache.gen == a.layoutGen &&
			a.bentoCache.count == len(a.assets) &&
			a.bentoCache.cols > 0
		if isReflow && !a.reflowAnchorValid {
			a.autoCaptureBentoAnchor(a.bentoScroll.Y, viewH, gap)
		}
		n := len(a.assets)
		blocks := make([]bentoBlock, 0, n/cols+2)
		i := 0
		blockIdx := 0
		var curY float32 = 0
		sideCols := cols - 2
		heroBlockSize := 1 + sideCols*2
		wideRowSize := cols - 1

		for i < n {
			rem := n - i
			if cols >= 3 && rem >= heroBlockSize && blockIdx%2 == 0 {
				heroH := unitH*2 + gap
				blocks = append(blocks, bentoBlock{
					kind:     bentoBlockHero,
					blockIdx: blockIdx,
					startIdx: i,
					count:    heroBlockSize,
					heroLeft: (blockIdx/2)%2 == 0,
					y:        curY,
					h:        heroH,
				})
				curY += heroH + gap
				i += heroBlockSize
				blockIdx++
				continue
			}
			if cols >= 3 && rem >= wideRowSize && rem != cols {
				widePos := 0
				if blockIdx%2 == 1 {
					widePos = wideRowSize - 1
				}
				blocks = append(blocks, bentoBlock{
					kind:     bentoBlockWide,
					blockIdx: blockIdx,
					startIdx: i,
					count:    wideRowSize,
					widePos:  widePos,
					y:        curY,
					h:        unitH,
				})
				curY += unitH + gap
				i += wideRowSize
				blockIdx++
				continue
			}
			take := min(rem, cols)
			blocks = append(blocks, bentoBlock{
				kind:     bentoBlockRow,
				blockIdx: blockIdx,
				startIdx: i,
				count:    take,
				y:        curY,
				h:        unitH,
			})
			curY += unitH + gap
			i += take
			blockIdx++
		}
		totalH := curY
		if totalH > gap {
			totalH -= gap
		}
		a.bentoCache = bentoLayoutCache{
			gen:    a.layoutGen,
			count:  n,
			cols:   cols,
			unitH:  unitH,
			blocks: blocks,
			totalH: totalH,
		}
		if isReflow && a.reflowAnchorValid {
			a.applyBentoReflowAnchor(viewH, gap)
		}
	}

	scroll.TrackScroll(&a.bentoScroll)

	scrollY := a.bentoScroll.Y
	const overscan float32 = 720
	minY := scrollY - overscan
	maxY := scrollY + viewH + overscan

	blocks := a.bentoCache.blocks
	totalH := a.bentoCache.totalH
	sideCols := cols - 2

	scroll.Children(func() {
		gui.Column(c).FillWidth().Padding(8, 12, 72, 8).Gap(gap).Children(func() {
			if len(blocks) == 0 {
				return
			}
			// O(log N) binary search for visible block range.
			firstVis := sort.Search(len(blocks), func(idx int) bool {
				return blocks[idx].y+blocks[idx].h >= minY
			})
			if firstVis >= len(blocks) || blocks[firstVis].y > maxY {
				gui.Box(c).FillWidth().Height(totalH)
				return
			}
			afterLast := sort.Search(len(blocks)-firstVis, func(offset int) bool {
				return blocks[firstVis+offset].y > maxY
			})
			lastVis := firstVis + afterLast - 1
			if lastVis < firstVis {
				lastVis = firstVis
			}

			// Single top spacer box instead of thousands of individual row spacers!
			if topPad := blocks[firstVis].y - gap; topPad > 0 {
				gui.Box(c.Key("bento-top-pad")).FillWidth().Height(topPad)
			}

			for bIdx := firstVis; bIdx <= lastVis; bIdx++ {
				blk := blocks[bIdx]
				switch blk.kind {
				case bentoBlockHero:
					heroIdx := blk.startIdx
					heroH := blk.h
					renderHero := func() {
						gui.Box(c).Grow(2).Height(heroH).Children(func() {
							a.assetCard(c, heroIdx, a.assets[heroIdx], a.isCardSelected(heroIdx, a.assets[heroIdx].ID))
						})
					}
					renderSide := func() {
						gui.Column(c).Grow(float32(sideCols)).Height(heroH).Gap(gap).Children(func() {
							gui.Row(c).Grow(1).FillWidth().Gap(gap).Children(func() {
								for k := 0; k < sideCols; k++ {
									idx := heroIdx + 1 + k
									gui.Box(c).Grow(1).FillHeight().Children(func() {
										a.assetCard(c, idx, a.assets[idx], a.isCardSelected(idx, a.assets[idx].ID))
									})
								}
							})
							gui.Row(c).Grow(1).FillWidth().Gap(gap).Children(func() {
								for k := 0; k < sideCols; k++ {
									idx := heroIdx + 1 + sideCols + k
									gui.Box(c).Grow(1).FillHeight().Children(func() {
										a.assetCard(c, idx, a.assets[idx], a.isCardSelected(idx, a.assets[idx].ID))
									})
								}
							})
						})
					}
					gui.Row(c.Key(fmt.Sprintf("bento-blk-%d", blk.blockIdx))).FillWidth().Gap(gap).Children(func() {
						if blk.heroLeft {
							renderHero()
							renderSide()
						} else {
							renderSide()
							renderHero()
						}
					})

				case bentoBlockWide:
					start := blk.startIdx
					gui.Row(c.Key(fmt.Sprintf("bento-wide-%d", blk.blockIdx))).FillWidth().Height(blk.h).Gap(gap).Children(func() {
						for k := 0; k < blk.count; k++ {
							idx := start + k
							span := float32(1)
							if k == blk.widePos {
								span = 2
							}
							gui.Box(c).Grow(span).FillHeight().Children(func() {
								a.assetCard(c, idx, a.assets[idx], a.isCardSelected(idx, a.assets[idx].ID))
							})
						}
					})

				case bentoBlockRow:
					start := blk.startIdx
					take := blk.count
					gui.Row(c.Key(fmt.Sprintf("bento-row-%d", blk.blockIdx))).FillWidth().Height(blk.h).Gap(gap).Children(func() {
						usedSpan := 0
						for k := 0; k < take; k++ {
							idx := start + k
							span := 1
							if take == 1 && cols >= 3 {
								span = 2
							}
							usedSpan += span
							gui.Box(c).Grow(float32(span)).FillHeight().Children(func() {
								a.assetCard(c, idx, a.assets[idx], a.isCardSelected(idx, a.assets[idx].ID))
							})
						}
						if usedSpan < cols {
							gui.Box(c).Grow(float32(cols - usedSpan))
						}
					})
				}
			}

			// Single bottom spacer box!
			lastBlk := blocks[lastVis]
			if botPad := totalH - (lastBlk.y + lastBlk.h) - gap; botPad > 0 {
				gui.Box(c.Key("bento-bot-pad")).FillWidth().Height(botPad)
			}
		})
	})
}

// assetCard is a pure monochrome frameless Arc UI card:
// - Staggered blur-in reveal (4px -> 0px, 35ms stagger, easeArcEnter)
// - Hover lift (-3px, springGentle), image subtle scale (1.04x), shadow lift
// - Active press scale (0.985, springResponsive)
// - Full-bleed edge-to-edge rounded artwork, zero borders, zero blue
func (a *App) assetCard(c *gui.Context, index int, asset model.Asset, selected bool) {
	services := c.Services()
	name := a.assetName(asset)
	ext := a.assetExt(asset)

	card := gui.ButtonBase(c.Key("card-" + asset.ID)).
		Column().
		Fill().
		Radius(radiusPanel).
		Clip().
		Drag(assetBatchDrag{IDs: a.dragIDsFor(asset.ID)}).
		Label(name)
	a.cardBounds[asset.ID] = card.Bounds()

	card.PopoverMenu(func(m *gui.Menu) {
		batchIDs := a.dragIDsFor(asset.ID)
		multi := len(batchIDs) > 1
		stackCount := 0
		if a.openStackID == "" && asset.StackID != "" {
			stackCount = a.stackVisibleCounts[asset.StackID]
		}
		if stackCount >= 2 {
			stID := asset.StackID
			if a.menuItem(m, iconLayoutBento, fmt.Sprintf(a.t("stack.expand"), stackCount), "", false).Chosen() {
				a.openStackView(stID)
			}
			if a.menuItem(m, iconX, a.t("stack.ungroup"), "", false).Chosen() {
				a.ungroupStack(stID)
			}
			m.Separator()
		} else if a.openStackID != "" && asset.StackID == a.openStackID {
			stID := asset.StackID
			if st, ok := a.stackByID[stID]; ok && st.CoverID != asset.ID {
				if a.menuItem(m, iconImage, a.t("stack.setCover"), "", false).Chosen() {
					a.setStackCover(stID, asset.ID)
				}
			}
			if a.menuItem(m, iconX, a.t("stack.removeAsset"), "", false).Chosen() {
				a.removeFromStack([]string{asset.ID})
			}
			if a.menuItem(m, iconX, a.t("stack.ungroup"), "", false).Chosen() {
				a.ungroupStack(stID)
			}
			m.Separator()
		}
		if multi {
			if a.menuItem(m, iconLayoutBento, fmt.Sprintf("%s (%d)", a.t("stack.create"), len(batchIDs)), a.shortcutLabel("stack_assets"), false).Chosen() {
				a.stackSelectedAssets(batchIDs)
			}
		}
		if !multi {
			if u := normalizeWebURL(asset.SourceURL); u != "" {
				if a.menuItem(m, iconLink, a.t("webimport.openPage"), "", false).Chosen() {
					services.OpenURL(u)
				}
			}
			if a.menuItem(m, iconImage, a.t("library.preview"), "Space", false).Chosen() {
				a.openPreview(asset)
			}
		}
		if len(a.projectList) > 0 {
			addProjTitle := a.t("library.addToProject")
			if multi {
				addProjTitle = fmt.Sprintf("%s (%d)", addProjTitle, len(batchIDs))
			}
			a.menuSub(m, iconFolder, addProjTitle, func(sm *gui.Menu) {
				for _, p := range a.projectList {
					proj := p
					if a.menuItem(sm, iconFolder, proj.Name, "", false).Chosen() {
						if multi {
							a.batchReferenceToProject(batchIDs, proj.ID)
						} else {
							a.referenceAssetToProject(asset.ID, proj.ID)
						}
					}
				}
			})
		}
		if len(a.folders) > 0 {
			addFolderTitle := a.t("folder.addTo")
			if multi {
				addFolderTitle = fmt.Sprintf("%s (%d)", addFolderTitle, len(batchIDs))
			}
			a.menuSub(m, iconFolder, addFolderTitle, func(sm *gui.Menu) {
				for _, f := range a.folders {
					folder := f
					lbl := a.folderFullPath(folder.ID)
					if multi {
						if a.menuItem(sm, iconFolder, lbl, "", false).Chosen() {
							a.batchApplyFolder(batchIDs, folder.ID)
						}
					} else {
						has := a.hasFolder(asset.ID, folder.ID)
						if a.menuRadio(sm, lbl, has).Chosen() {
							a.setAssetFolderFast(asset.ID, folder.ID, !has)
						}
					}
				}
			})
		}
		if multi && len(a.tags) > 0 {
			a.menuSub(m, iconTag, fmt.Sprintf("%s (%d)", a.t("batch.applyTag"), len(batchIDs)), func(sm *gui.Menu) {
				for _, tg := range a.tags {
					tag := tg
					if a.menuItem(sm, iconTag, tag.Name, "", false).Chosen() {
						a.batchApplyTag(batchIDs, tag.ID)
					}
				}
			})
		}
		targetIDs := []string{asset.ID}
		boardMenuTitle := a.t("board.sendTo")
		if multi {
			targetIDs = batchIDs
			boardMenuTitle = fmt.Sprintf("%s (%d)", a.t("board.sendTo"), len(batchIDs))
		}
		a.menuSub(m, iconBoard, boardMenuTitle, func(sm *gui.Menu) {
			if a.menuItem(sm, iconPlus, a.t("board.sendToNew"), "", false).Chosen() {
				a.sendAssetsToNewBoard(targetIDs)
			}
			if len(a.standaloneBoards) > 0 {
				sm.Separator()
				for _, sb := range a.standaloneBoards {
					b := sb
					if a.menuItem(sm, iconBoard, b.Name, "", false).Chosen() {
						a.sendAssetsToBoard(targetIDs, b.ID)
					}
				}
			}
		})
		if !multi {
			a.menuSub(m, iconSparkle, a.t("library.rating"), func(sm *gui.Menu) {
				for r := 0; r <= 5; r++ {
					rating := r
					lbl := "0"
					if rating > 0 {
						lbl = strings.Repeat("★", rating)
					}
					if a.menuRadio(sm, lbl, asset.Rating == rating).Chosen() {
						a.setAssetRatingFast(asset.ID, rating)
					}
				}
			})
			m.Separator()
			if !asset.Missing && asset.Path != "" {
				if a.menuItem(m, iconExternal, a.t("common.openFile"), "", false).Chosen() {
					_ = openLocalFile(asset.Path)
				}
				if a.menuItem(m, iconFolder, a.t("common.revealFile"), "", false).Chosen() {
					_ = revealLocalFile(asset.Path)
				}
			}
			if a.menuItem(m, iconCopy, a.t("common.copyPath"), "", false).Chosen() {
				services.WriteClipboard(asset.Path)
				a.notify(a.t("common.copied"))
			}
			if asset.Missing {
				if a.menuItem(m, iconLink, a.t("inspector.relinkFile"), "", false).Chosen() {
					a.relinkMissing(false)
				}
				if a.menuItem(m, iconFolder, a.t("inspector.relink"), "", false).Chosen() {
					a.relinkMissing(true)
				}
			}
		}
		m.Separator()
		if !multi && model.MediaKind(asset.Mime) == model.MediaImage && !asset.Missing {
			if a.menuItem(m, iconPalette, a.t("visual.findSimilar"), "", false).Chosen() {
				a.searchSimilarByAsset(asset)
			}
		}
		layaLabel := a.t("laya.classify")
		layaIDs := []string{asset.ID}
		if multi {
			layaLabel = fmt.Sprintf("%s (%d)", a.t("laya.classify"), len(batchIDs))
			layaIDs = batchIDs
		}
		if a.menuItem(m, iconSparkle, layaLabel, "", false).Chosen() {
			a.classifyAssetsWithLaya(layaIDs)
		}
		if a.plugins != nil {
			hook := "context_menu"
			if multi {
				hook = "batch_action"
			}
			if plugs := a.plugins.ForHook(hook); len(plugs) > 0 {
				a.menuSub(m, iconSliders, a.t("plugins.menu"), func(sm *gui.Menu) {
					for _, inst := range plugs {
						pl := inst
						if a.menuItem(sm, iconSparkle, pl.Manifest.Name, "", false).Chosen() {
							a.runPlugin(pl.Manifest.ID, hook, layaIDs)
						}
					}
				})
			}
		}
		m.Separator()
		a.menuSub(m, iconChecker, a.t("view.thumbBg"), func(sm *gui.Menu) {
			if a.menuRadio(sm, a.t("view.thumbBgNone"), a.thumbBg == "").Chosen() {
				a.setThumbBg("")
			}
			sm.Separator()
			if a.menuRadio(sm, a.t("view.thumbBgBlack"), a.thumbBg == "black").Chosen() {
				a.setThumbBg("black")
			}
			if a.menuRadio(sm, a.t("view.thumbBgGray"), a.thumbBg == "gray").Chosen() {
				a.setThumbBg("gray")
			}
			if a.menuRadio(sm, a.t("view.thumbBgWhite"), a.thumbBg == "white").Chosen() {
				a.setThumbBg("white")
			}
			if a.menuRadio(sm, a.t("view.thumbBgChecker"), a.thumbBg == "checker").Chosen() {
				a.setThumbBg("checker")
			}
		})
		grayMark := ""
		if a.grayscalePreview {
			grayMark = "✓ "
		}
		if a.menuItem(m, iconContrast, grayMark+a.t("view.grayscale"), a.shortcutLabel("grayscale"), false).Chosen() {
			a.grayscalePreview = !a.grayscalePreview
			a.syncConfigFromState()
		}
		m.Separator()
		if multi {
			if a.menuItem(m, iconEdit, fmt.Sprintf("%s (%d)", a.t("rename.batchAction"), len(batchIDs)), a.shortcutLabel("batch_rename"), false).Chosen() {
				a.openRenameModal(batchIDs)
			}
			if a.menuItem(m, iconExport, fmt.Sprintf("%s (%d)", a.t("export.action"), len(batchIDs)), "", false).Chosen() {
				a.openExportModal(batchIDs)
			}
			if a.menuItem(m, iconCopy, fmt.Sprintf("%s (%d)", a.t("clipboard.copyFiles"), len(batchIDs)), a.shortcutLabel("copy_files"), false).Chosen() {
				a.copySelectedFilesToClipboard(batchIDs)
			}
			m.Separator()
			if a.menuItem(m, iconTrash, fmt.Sprintf("%s (%d)", a.t("batch.delete"), len(batchIDs)), a.shortcutLabel("delete_asset"), true).Chosen() {
				a.batchDeletePrompt(batchIDs)
			}
		} else {
			if a.menuItem(m, iconEdit, a.t("rename.action"), a.shortcutLabel("batch_rename"), false).Chosen() {
				a.openRenameModal([]string{asset.ID})
			}
			if a.menuItem(m, iconExport, a.t("export.action"), "", false).Chosen() {
				a.openExportModal([]string{asset.ID})
			}
			if !asset.Missing && asset.Path != "" {
				if a.menuItem(m, iconCopy, a.t("clipboard.copyFiles"), a.shortcutLabel("copy_files"), false).Chosen() {
					a.copySelectedFilesToClipboard([]string{asset.ID})
				}
			}
			m.Separator()
			if a.menuItem(m, iconTrash, a.t("library.delete"), a.shortcutLabel("delete_asset"), true).Chosen() {
				a.deleteAssetPrompt(asset)
			}
		}
	}, a.popoverMenuOpts())

	card.OnClick(func() {
		a.selectCard(index, c.Modifiers())
	})
	if card.DoubleClicked() {
		a.selectCard(index, 0)
		if u := normalizeWebURL(asset.SourceURL); asset.IsBookmark && u != "" {
			services.OpenURL(u)
		} else {
			a.openPreview(asset)
		}
	}

	hovered := card.Hovered()
	pressed := card.Pressed()
	dragging := card.Dragging()
	dragIDs := a.dragIDsFor(asset.ID)
	if dragging {
		a.dragSeenFrame = true
		a.activeDragLeadID = asset.ID
		a.activeDragIDs = dragIDs
	}

	// Detect whether this card is one of the companion cards in an active multi-select drag bundle.
	companionDragging := !dragging && a.activeDragLeadID != "" && len(a.activeDragIDs) > 1 && a.selectedIDs[asset.ID]

	targetBg := a.pal.surface
	liftX := float32(0)
	liftY := float32(0)
	cardScale := float32(1)
	imgScale := float32(1)
	shadow := a.pal.shadowResting
	switch {
	case dragging:
		// Shrink dragged preview to a crisp ~64px miniature (slightly larger for
		// multi-image stacked bundles so the count pill & stack edge read cleanly).
		bounds := card.LayoutBounds()
		maxDim := max(bounds.W, bounds.H)
		targetSize := float32(58.0)
		if len(dragIDs) > 1 {
			targetSize = 72.0
		}
		dragScale := float32(0.32)
		if maxDim > 0 {
			dragScale = targetSize / maxDim
			if dragScale < 0.16 {
				dragScale = 0.16
			} else if dragScale > 0.48 {
				dragScale = 0.48
			}
		}
		cardScale = dragScale
		shadow = a.pal.shadowFloating
		card.Opacity(0.24)
	case companionDragging:
		// Gather all other selected cards inward toward the lead dragged card so
		// the entire multi-selection visually lifts and bundles together!
		leadB := a.cardBounds[a.activeDragLeadID]
		myB := card.LayoutBounds()
		if leadB.W > 0 && myB.W > 0 {
			dx := (leadB.X + leadB.W/2) - (myB.X + myB.W/2)
			dy := (leadB.Y + leadB.H/2) - (myB.Y + myB.H/2)
			const maxPull = float32(28)
			if dx > maxPull {
				dx = maxPull
			} else if dx < -maxPull {
				dx = -maxPull
			}
			if dy > maxPull {
				dy = maxPull
			} else if dy < -maxPull {
				dy = -maxPull
			}
			liftX = dx
			liftY = dy
		}
		cardScale = 0.86
		card.Opacity(0.22)
	case pressed:
		targetBg = a.pal.surfaceActive
		cardScale = 0.985
	case selected:
		targetBg = a.pal.surfaceMuted
		shadow = a.pal.shadowRaised
		if hovered {
			imgScale = 1.03
		}
	case hovered:
		targetBg = a.pal.surfaceMuted
		liftY = -3
		imgScale = 1.04
		shadow = a.pal.shadowRaised
	}

	bg := card.AnimateColor("card-bg", targetBg, animFast)
	card.Background(bg).Shadow(0, 4, 14, 0, shadow)
	if dragging || companionDragging || pressed {
		card.AnimatePose(gui.Pose{X: liftX, Y: liftY, Scale: cardScale}, springSnappy)
	} else {
		card.AnimatePose(gui.Pose{X: 0, Y: liftY, Scale: cardScale}, springGentle)
	}
	if len(a.assets) <= 250 {
		stagger := time.Duration(min(index, 6)) * arcStaggerItem
		applyCardBlurFade(card, cardRevealKey{gen: a.revealGen, id: asset.ID}, stagger)
	}

	card.Children(func() {
		// Full-bleed edge-to-edge media slot with thumbnail background, optional grayscale & stack badge.
		mediaBox := gui.Box(c).Grow(1).FillWidth().Clip().Center()
		a.applyThumbBg(mediaBox, a.pal.surfaceMuted, 10)
		mediaBox.Children(func() {
			if bm := a.requestCardBitmap(asset); bm != nil {
				imgEl := gui.Image(c, bm).
					Fit(gui.Cover).
					Fill().
					AnimatePose(gui.Pose{Scale: imgScale}, springGentle)
				if a.grayscalePreview {
					imgEl.Grayscale()
				}
			} else if isTextSnippetExt(ext) {
				// Sleek miniature document card preview for .MD / .TXT notes instead of a blank grey box.
				snippet := a.cachedTextSnippet(asset.Path)
				gui.Column(c).
					Fill().
					Padding(12, 12, 10, 12).
					Gap(6).
					Background(a.pal.surface).
					Border(1, a.chromeHairline()).
					Children(func() {
						gui.Row(c).FillWidth().Gap(5).AlignItems(gui.Center).Children(func() {
							gui.Icon(c, iconFileText).Size(12, 12).TextColor(gui.Hex("#38BDF8"))
							gui.Text(c, ext).FontSize(10).Bold().TextColor(gui.Hex("#38BDF8")).SingleLine()
						})
						gui.Text(c, a.assetName(asset)).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine()
						if snippet != "" {
							gui.Text(c, snippet).
								FontSize(10.5).
								TextColor(a.pal.inkMuted).
								LineHeight(1.45).
								Grow(1)
						}
					})
			} else {
				gui.Column(c).Center().Gap(6).Children(func() {
					ic := iconFile
					switch {
					case model.IsFont(asset.Mime, asset.Path):
						ic = iconType
					case model.IsAudio(asset.Mime, asset.Path):
						ic = iconMusic
					case model.IsPDF(asset.Mime, asset.Path):
						ic = iconFileText
					case model.MediaKind(asset.Mime) == model.MediaVideo:
						ic = iconVideo
					case model.MediaKind(asset.Mime) == model.MediaImage:
						ic = iconImage
					}
					gui.Icon(c, ic).Size(24, 24).TextColor(a.pal.inkFaint)
					gui.Text(c, ext).FontSize(11).Bold().TextColor(a.pal.inkMuted)
				})
			}

			// Multi-select stacked drag bundle indicator directly on the dragged miniature.
			if dragging && len(dragIDs) > 1 {
				invScale := float32(1)
				if cardScale > 0.15 {
					invScale = 1.0 / cardScale
					if invScale > 2.6 {
						invScale = 2.6
					}
				}
				// Stacked border ring + bold orange count pill so the floating bundle reads unmistakably as N items.
				gui.Box(c).
					Absolute().
					Top(0).
					Left(0).
					Right(0).
					Bottom(0).
					Radius(radiusPanel).
					Border(2.5*invScale, gui.Hex("#FF5C33"))
				gui.Row(c).
					Absolute().
					Top(8).
					Right(8).
					Padding(4*invScale, 9*invScale).
					Radius(radiusPill).
					Background(gui.Hex("#FF5C33")).
					Center().
					Children(func() {
						gui.Text(c, fmt.Sprintf("%d", len(dragIDs))).
							FontSize(12 * invScale).
							Bold().
							TextColor(gui.Hex("#FFFFFF")).
							SingleLine()
					})
			}

			// Floating selection check badge when caption strip is hidden (only when selected or in multi-select mode).
			if !a.showCardCaption && (selected || a.multiSelectMode) {
				chkBg := gui.RGBA(24, 24, 27, 0.72)
				if selected {
					chkBg = a.pal.controlOn
				}
				gui.ButtonBase(c.Key("card-chk-"+asset.ID)).
					Absolute().
					Top(8).
					Left(8).
					Size(18, 18).
					Radius(radiusPill).
					Background(chkBg).
					Center().
					OnClick(func() {
						a.toggleCardSelection(index)
					}).
					Children(func() {
						if selected {
							gui.Icon(c, iconCheck).Size(10, 10).TextColor(a.pal.controlGlyph)
						}
					})
			}

			// Web Bookmark floating domain badge (bottom-left).
			if asset.IsBookmark && asset.SourceURL != "" {
				host := shortWebHost(asset.SourceURL)
				if host == "" {
					host = a.t("webimport.badge")
				}
				gui.Row(c).
					Absolute().
					Bottom(8).
					Left(8).
					Padding(3, 8).
					Gap(4).
					Radius(radiusPill).
					Background(gui.RGBA(18, 18, 22, 0.84)).
					AlignItems(gui.Center).
					Children(func() {
						gui.Icon(c, iconGlobe).Size(11, 11).TextColor(gui.Hex("#FAFAFA"))
						gui.Text(c, host).FontSize(10).Bold().TextColor(gui.Hex("#FAFAFA")).SingleLine().MaxWidth(130)
					})
			}

			// Collapsed Asset Stack floating version-count pill (click to expand versions!).
			if a.openStackID == "" && asset.StackID != "" {
				if cnt := a.stackVisibleCounts[asset.StackID]; cnt >= 2 {
					stID := asset.StackID
					gui.ButtonBase(c.Key("stack-badge-"+asset.ID)).
						Absolute().
						Top(8).
						Right(8).
						Padding(3, 8).
						Gap(4).
						Radius(radiusPill).
						Background(gui.RGBA(18, 18, 22, 0.82)).
						AlignItems(gui.Center).
						Tooltip(fmt.Sprintf(a.t("stack.expand"), cnt)).
						OnClick(func() {
							a.openStackView(stID)
						}).
						Children(func() {
							gui.Icon(c, iconLayers).Size(11, 11).TextColor(gui.Hex("#FAFAFA"))
							gui.Text(c, fmt.Sprintf("%d", cnt)).FontSize(11).Bold().TextColor(gui.Hex("#FAFAFA")).SingleLine()
						})
				}
			}
		})

		// Clean frameless caption strip (pure monochrome; toggleable globally in Display Options).
		if a.showCardCaption {
			gui.Row(c).FillWidth().Padding(8, 11).Gap(6).AlignItems(gui.Center).Children(func() {
				if selected || a.multiSelectMode {
					chkBg := a.pal.surfaceActive
					if selected {
						chkBg = a.pal.controlOn
					}
					gui.ButtonBase(c.Key("card-chk-"+asset.ID)).
						Size(16, 16).
						Radius(radiusPill).
						Background(chkBg).
						Center().
						OnClick(func() {
							a.toggleCardSelection(index)
						}).
						Children(func() {
							if selected {
								gui.Icon(c, iconCheck).Size(10, 10).TextColor(a.pal.controlGlyph)
							}
						})
				}
				gui.Text(c, name).Grow(1).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine()
				if asset.Missing {
					gui.Text(c, a.t("library.missing")).FontSize(10).Bold().TextColor(a.pal.danger).SingleLine()
				} else if model.MediaKind(asset.Mime) == model.MediaVideo {
					vidLbl := "▶ " + ext
					if asset.Duration > 0 {
						vidLbl = "▶ " + formatDuration(asset.Duration)
					}
					gui.Text(c, vidLbl).FontSize(10).Bold().TextColor(a.pal.inkSecondary).SingleLine()
				} else if a.showCardRating && asset.Rating > 0 {
					gui.Text(c, fmt.Sprintf("★%d", asset.Rating)).FontSize(11).TextColor(a.pal.inkSecondary).SingleLine()
				} else if a.showCardExt {
					gui.Text(c, ext).FontSize(10).TextColor(a.pal.inkMuted).SingleLine()
				}
			})
		}
	})
}

func (a *App) inspector(c *gui.Context) {
	services := c.Services()
	asset, ok := a.selectedAsset()
	if a.previewOpen {
		if pv, exists := a.assetByID[a.previewAssetID]; exists {
			asset = pv
			ok = true
		}
	}
	a.syncInspectorSelection(false)

	gui.Column(c.Key("arc-inspector")).
		Fill().
		Clip().
		Background(a.pal.surface).
		Transition(gui.ElementTransition{
			Animation: &springSmooth,
			Enter:     &gui.Motion{X: 18, Opacity: 0},
		}).
		Children(func() {
				if !ok {
					gui.Column(c).Grow(1).FillWidth().Center().Padding(24).Gap(12).Children(func() {
						gui.Box(c).Size(46, 46).Radius(radiusControl).Background(a.pal.surfaceMuted).Center().Children(func() {
							gui.Icon(c, iconPanelRight).Size(20, 20).TextColor(a.pal.inkMuted)
						})
						gui.Text(c, a.t("inspector.none")).
							FontSize(12).
							TextColor(a.pal.inkSecondary).
							TextAlign(gui.Center).
							LineHeight(1.55)
					})
				} else {
					// Top Info / Similar segmented tab switcher + dismiss X button (matching reference design).
					gui.Row(c).FillWidth().Padding(10, 12, 6, 14).AlignItems(gui.Center).Children(func() {
						gui.Box(c).Size(28, 28)
						gui.Spacer(c)
						a.arcPillTabs(c, "insp-mode-tabs", a.inspectorTab, []string{
							a.t("inspector.tabInfo"),
							a.t("inspector.tabSimilar"),
						}, func(idx int) {
							a.inspectorTab = idx
							if idx == 1 {
								a.ensureInspectorSimilar(asset)
							}
						})
						gui.Spacer(c)
						a.iconButton(c, "insp-close-x", iconX, a.t("common.close"), false).
							Size(28, 28).
							OnClick(func() {
								if a.previewOpen {
									a.inspectorOpen = false
								} else {
									a.clearSelection()
									a.assetIndex = -1
									a.syncInspectorSelection(false)
								}
							})
					})

					if a.inspectorTab == 1 {
						a.ensureInspectorSimilar(asset)
						gui.Scroll(c.Key("insp-sim-scroll")).Grow(1).FillWidth().Children(func() {
							gui.Column(c).FillWidth().Padding(10, 14, 16, 14).Gap(12).Children(func() {
								gui.Text(c, fmt.Sprintf(a.t("inspector.similarLike"), a.assetName(asset))).
									FontSize(12).
									TextColor(a.pal.inkSecondary).
									SingleLine()
								if len(a.inspectorSimilar) == 0 {
									gui.Box(c).FillWidth().Padding(24, 12).Center().Children(func() {
										gui.Text(c, a.t("inspector.similarNone")).FontSize(12).TextColor(a.pal.inkMuted).TextAlign(gui.Center)
									})
								} else {
									for rowStart := 0; rowStart < len(a.inspectorSimilar); rowStart += 2 {
										gui.Row(c.Key(fmt.Sprintf("sim-row-%d", rowStart))).FillWidth().Gap(8).Children(func() {
											for col := 0; col < 2; col++ {
												idx := rowStart + col
												if idx >= len(a.inspectorSimilar) {
													gui.Box(c).Grow(1)
													continue
												}
												m := a.inspectorSimilar[idx]
												hit := m.Asset
												if a.bitmaps[hit.ID] == nil {
													p := hit.ThumbnailPath
													if p == "" {
														p = hit.Path
													}
													if p != "" {
														a.ensureBitmap(hit.ID, p)
													}
												}
												btn := gui.ButtonBase(c.Key("sim-hit-"+hit.ID)).
													Grow(1).
													Height(124).
													Radius(radiusControl).
													Clip().
													Background(a.pal.surfaceRaised).
													Tooltip(fmt.Sprintf("%s (%.0f%%)", a.assetName(hit), m.Similarity*100)).
													OnClick(func() {
														a.openPreview(hit)
													})
												btn.Children(func() {
													if bm := a.bitmaps[hit.ID]; bm != nil {
														gui.Image(c, bm).Fit(gui.Cover).Fill()
													} else {
														gui.Box(c).Fill().Center().Children(func() {
															gui.Text(c, a.assetExt(hit)).FontSize(11).TextColor(a.pal.inkMuted)
														})
													}
												})
											}
										})
									}
								}
							})
						})
					} else {
						hairline := gui.RGBA(255, 255, 255, 0.06)
						swatchBorder := gui.RGBA(255, 255, 255, 0.16)
						dangerHover := gui.RGBA(224, 122, 122, 0.14)
						if !a.dark {
							hairline = gui.RGBA(0, 0, 0, 0.07)
							swatchBorder = gui.RGBA(0, 0, 0, 0.12)
							dangerHover = gui.RGBA(185, 28, 28, 0.10)
						}
						inspectorCard := func(key string, gap float32, fn func()) {
							gui.Column(c.Key(key)).
								FillWidth().
								Padding(12).
								Gap(gap).
								Radius(radiusControl).
								Background(a.pal.surfaceRaised).
								Border(1, hairline).
								Children(fn)
						}
						detailRow := func(label, value string) {
							gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
								gui.Text(c, label).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
								gui.Spacer(c)
								gui.Text(c, value).FontSize(11).TextColor(a.pal.ink).SingleLine().MinWidth(0)
							})
						}

						gui.Scroll(c).Grow(1).FillWidth().Children(func() {
							body := gui.Column(c.Key("insp-body-" + asset.ID)).Padding(8, 14, 16, 14).Gap(10)
							applyBlurReveal(body, asset.ID, 0)
							body.Children(func() {
								// Exact aspect-ratio-fitted centered preview thumbnail when browsing grid (omitted in inline viewer).
								// Sized pixel-tight to the image's natural aspect ratio so rounded corners hug the artwork edges cleanly.
								if !a.previewOpen {
									heroTip := a.t("library.preview")
									if asset.IsBookmark && normalizeWebURL(asset.SourceURL) != "" {
										heroTip = a.t("webimport.openPage")
									}
									maxW := a.inspectorWidth - 28
									if maxW < 180 {
										maxW = 240
									}
									const maxH float32 = 250
									bm := a.requestCardBitmap(asset)
									var natW, natH float32
									if bm != nil {
										if bw, bh := bm.Size(); bw > 0 && bh > 0 {
											natW, natH = float32(bw), float32(bh)
										}
									}
									if (natW <= 0 || natH <= 0) && asset.Width > 0 && asset.Height > 0 {
										natW, natH = float32(asset.Width), float32(asset.Height)
									}
									dispW, dispH := maxW, float32(168)
									if natW > 0 && natH > 0 {
										scale := min(maxW/natW, maxH/natH)
										dispW = natW * scale
										dispH = natH * scale
										if dispW < 72 {
											dispW = 72
										}
										if dispH < 72 {
											dispH = 72
										}
									}
									gui.Row(c).FillWidth().Justify(gui.Center).Padding(2, 0, 2, 0).Children(func() {
										hero := gui.ButtonBase(c.Key("insp-hero-" + asset.ID)).
											Size(dispW, dispH).
											Radius(10).
											Clip().
											Border(1, hairline).
											Shadow(0, 6, 18, 0, a.pal.shadowResting).
											Center().
											Label(a.t("library.preview")).
											Tooltip(heroTip).
											OnClick(func() {
												if u := normalizeWebURL(asset.SourceURL); asset.IsBookmark && u != "" {
													services.OpenURL(u)
												} else {
													a.openPreview(asset)
												}
											})
										a.applyThumbBg(hero, a.pal.surfaceRaised, 10)
										heroScale := float32(1)
										if hero.Pressed() {
											heroScale = 0.985
										} else if hero.Hovered() {
											heroScale = 1.02
										}
										hero.AnimatePose(gui.Pose{Scale: heroScale}, springGentle)
										hero.Children(func() {
											if bm != nil {
												imgEl := gui.Image(c, bm).Fit(gui.Cover).Fill()
												if a.grayscalePreview {
													imgEl.Grayscale()
												}
											} else {
												gui.Column(c).Center().Gap(6).Children(func() {
													ic := iconFile
													switch model.MediaKind(asset.Mime) {
													case model.MediaVideo:
														ic = iconVideo
													case model.MediaImage:
														ic = iconImage
													}
													gui.Icon(c, ic).Size(28, 28).TextColor(a.pal.inkFaint)
													gui.Text(c, a.assetExt(asset)).Bold().TextColor(a.pal.inkMuted)
												})
											}
										})
									})
								}

								// Title + compact Rename/Export actions + quiet metadata chain.
								gui.Column(c).FillWidth().Gap(3).Padding(2, 2, 0, 2).Children(func() {
									gui.Row(c).FillWidth().Gap(4).AlignItems(gui.Center).Children(func() {
										gui.Text(c, a.assetName(asset)).
											FontSize(14).
											Bold().
											SingleLine().
											MinWidth(0).
											Grow(1).
											Tooltip(a.assetName(asset))
										a.iconButton(c, "insp-rename-btn", iconEdit, a.t("rename.action")+" (F2)", false).
											Size(28, 28).
											OnClick(func() {
												a.openRenameModal([]string{asset.ID})
											})
										a.iconButton(c, "insp-export-btn", iconExport, a.t("export.action"), false).
											Size(28, 28).
											OnClick(func() {
												a.openExportModal([]string{asset.ID})
											})
									})
									parts := []string{a.assetExt(asset), a.modeLabel(asset.StorageMode)}
									if asset.Width > 0 && asset.Height > 0 {
										parts = append(parts, fmt.Sprintf("%d × %d", asset.Width, asset.Height))
									}
									gui.Text(c, strings.Join(parts, "  ·  ")).
										FontSize(11).
										TextColor(a.pal.inkMuted).
										SingleLine().
										MinWidth(0)
								})

								// Consolidated hover-to-select file actions button (Open / Reveal / Copy Path / Copy File).
								trigger := gui.ButtonBase(c.Key("insp-file-act-btn-" + asset.ID)).
									FillWidth().
									Height(34).
									Padding(0, 12).
									Gap(8).
									Radius(radiusControl).
									AlignItems(gui.Center).
									Border(1, hairline).
									Label(a.t("inspector.fileActions")).
									OnClick(func() {
										a.inspFileMenuPinned = !a.inspFileMenuPinned
										a.inspFileMenuOpen = a.inspFileMenuPinned
									})
								btnHovered := trigger.Hovered()
								if a.inspFilePrevMode != a.previewOpen {
									a.inspFilePrevMode = a.previewOpen
									a.inspFileMenuOpen = false
									a.inspFileMenuPinned = false
									a.inspFileSuppressHover = true
									a.inspFileSuppressFrames = 2
								}
								if a.inspFileSuppressFrames > 0 {
									a.inspFileSuppressFrames--
								} else if !btnHovered {
									a.inspFileSuppressHover = false
								} else if !a.inspFileSuppressHover {
									a.inspFileMenuOpen = true
								}
								actExpanded := a.inspFileMenuOpen
								trigBg := a.pal.surfaceRaised
								if trigger.Pressed() {
									trigBg = a.pal.surfaceActive
								} else if actExpanded || btnHovered {
									trigBg = a.pal.surfaceMuted
								}
								chevTarget := float32(0)
								if actExpanded {
									chevTarget = 180
								}
								chevRot := trigger.AnimateTo("chev-rot", chevTarget, springSnappy)
								trigger.
									Background(trigger.AnimateColor("bg", trigBg, animFast)).
									TextColor(a.pal.ink).
									Children(func() {
										gui.Icon(c, iconExternal).Size(14, 14).TextColor(a.pal.inkSecondary)
										gui.Text(c, a.t("inspector.fileActions")).FontSize(12).Bold().SingleLine()
										gui.Spacer(c)
										gui.Icon(c, iconChevronDown).Size(13, 13).TextColor(a.pal.inkMuted).Rotate(chevRot)
									})

								popW := a.inspectorWidth - 28
								if popW < 200 {
									popW = 240
								}
								panelRendered := false
								panelHovered := false
								panelDismissed := false
								gui.PopoverBase(c.Key("insp-file-act-pop-"+asset.ID), trigger, &a.inspFileMenuOpen, func(p gui.Element) {
									panelRendered = true
									panelHovered = p.Hovered()
									panelDismissed = p.Dismissed()
									p.AttachTo(trigger, gui.AnchorBottomLeft, gui.AnchorTopLeft).
										Margin(0).
										Width(popW).
										Padding(4, 0, 0, 0).
										Transition(gui.ElementTransition{
											Animation: &springSnappy,
											Enter:     &gui.Motion{Y: -4, Opacity: 0},
											Exit:      &gui.Motion{Y: -4, Opacity: 0},
										}).
										Children(func() {
											gui.Column(c).
												FillWidth().
												Padding(4).
												Gap(2).
												Radius(radiusControl).
												Background(a.popoverSurface()).
												Border(1, swatchBorder).
												Shadow(0, 10, 24, 0, a.pal.shadowFloating).
												Children(func() {
													menuRow := func(key string, ic *gui.SVG, label string, onClick func()) {
														row := gui.ButtonBase(c.Key(key)).
															FillWidth().
															Height(30).
															Padding(0, 10).
															Gap(8).
															Radius(radiusControl - 3).
															AlignItems(gui.Center).
															Label(label).
															OnClick(func() {
																a.inspFileMenuOpen = false
																a.inspFileMenuPinned = false
																onClick()
															})
														rBg := gui.Transparent
														rFg := a.pal.inkSecondary
														if row.Pressed() {
															rBg = a.pal.surfaceActive
															rFg = a.pal.ink
														} else if row.Hovered() {
															rBg = a.pal.surfaceMuted
															rFg = a.pal.ink
														}
														row.Background(row.AnimateColor("bg", rBg, animFast)).
															TextColor(row.AnimateColor("fg", rFg, animFast)).
															Children(func() {
																gui.Icon(c, ic).Size(14, 14)
																gui.Text(c, label).FontSize(12).SingleLine()
															})
													}
													if u := normalizeWebURL(asset.SourceURL); u != "" {
														menuRow("insp-open-web", iconGlobe, a.t("webimport.openPage"), func() {
															services.OpenURL(u)
														})
														menuRow("insp-copy-web", iconLink, a.t("webimport.copyURL"), func() {
															services.WriteClipboard(u)
															a.notify(a.t("common.copied"))
														})
													}
													if !asset.Missing && asset.Path != "" {
														menuRow("insp-open-btn", iconExternal, a.t("common.openFile"), func() {
															_ = openLocalFile(asset.Path)
														})
														menuRow("insp-reveal-btn", iconFolder, a.t("common.revealFile"), func() {
															_ = revealLocalFile(asset.Path)
														})
													}
													menuRow("insp-copy-path", iconCopy, a.t("common.copyPath"), func() {
														services.WriteClipboard(asset.Path)
														a.notify(a.t("common.copied"))
													})
													if !asset.Missing && asset.Path != "" {
														menuRow("insp-copy-file", iconFile, a.t("clipboard.copyFiles"), func() {
															a.copySelectedFilesToClipboard([]string{asset.ID})
														})
													}
												})
										})
								})
								if panelRendered {
									if panelDismissed {
										a.inspFileMenuPinned = false
									} else if !btnHovered && !panelHovered && !a.inspFileMenuPinned {
										a.inspFileMenuOpen = false
										c.Invalidate()
									}
								}

								// Extracted 5-color OKLab dominant palette bar.
								if len(asset.Palette) > 0 {
									gui.Row(c.Key("insp-pal-bar-" + asset.ID)).
										FillWidth().
										Padding(7, 10).
										Gap(6).
										Radius(radiusControl).
										Background(a.pal.surfaceRaised).
										Border(1, hairline).
										AlignItems(gui.Center).
										Children(func() {
											gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
												for i, pc := range asset.Palette {
													hex := pc.Hex
													col, ok := parseHexColor(hex)
													if !ok {
														continue
													}
													gui.ButtonBase(c.Key(fmt.Sprintf("insp-pal-%s-%d", asset.ID, i))).
														Size(18, 18).
														Radius(radiusPill).
														Background(col).
														Border(1, swatchBorder).
														Tooltip(fmt.Sprintf(a.t("visual.copyHex"), hex)).
														OnClick(func() {
															services.WriteClipboard(hex)
															a.notify(fmt.Sprintf("%s %s", a.t("common.copied"), hex))
														}).
														ContextMenu(func(m *gui.Menu) {
															if m.Item(a.t("visual.searchColor")).Chosen() {
																a.setColorFilter(hex)
															}
														})
												}
											})
											gui.Spacer(c)
											a.iconButton(c, "insp-pal-search", iconPalette, a.t("visual.searchColor"), strings.EqualFold(a.colorFilter, asset.Palette[0].Hex)).
												Size(26, 26).
												OnClick(func() { a.setColorFilter(asset.Palette[0].Hex) })
											if model.MediaKind(asset.Mime) == model.MediaImage && !asset.Missing {
												a.iconButton(c, "insp-sim-btn", iconImage, a.t("visual.findSimilar"), false).
													Size(26, 26).
													OnClick(func() { a.searchSimilarByAsset(asset) })
											}
										})
								}

								// Missing file alert & relink CTAs.
								if asset.Missing {
									gui.Column(c).FillWidth().Padding(12).Gap(8).Radius(radiusControl).Background(a.pal.surfaceRaised).Border(1, dangerHover).Children(func() {
										gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
											gui.Icon(c, iconAlert).Size(14, 14).TextColor(a.pal.danger)
											gui.Text(c, a.t("library.missing")).FontSize(12).Bold().TextColor(a.pal.danger)
										})
										gui.Row(c).Gap(6).Children(func() {
											a.arcButton(c, "insp-relink-file", a.t("inspector.relinkFile"), false).Grow(1).OnClick(func() {
												a.relinkMissing(false)
											})
											a.arcButton(c, "insp-relink-dir", a.t("inspector.relink"), true).Grow(1).OnClick(func() {
												a.relinkMissing(true)
											})
										})
									})
								}

								// Card 1: Rating & Tags grouped cleanly.
								inspectorCard("insp-card-org-"+asset.ID, 10, func() {
									gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
										a.sectionLabel(c, a.t("library.rating"))
										gui.Spacer(c)
										gui.Rating(c.Key("rating-"+asset.ID), &a.inspectorRating, 5).OnChange(func() {
											a.setAssetRatingFast(asset.ID, a.inspectorRating)
										})
									})
									if len(a.folders) > 0 || len(a.assetFolders[asset.ID]) > 0 {
										gui.Box(c).FillWidth().Height(1).Background(hairline)
										// Folders section (parallel hierarchical classification).
										gui.Column(c).FillWidth().Gap(8).Children(func() {
											ownedFolders := map[string]bool{}
											appliedFolders := a.assetFolders[asset.ID]
											for _, f := range appliedFolders {
												ownedFolders[f.ID] = true
											}
											var availFolders []model.Folder
											for _, f := range a.folders {
												if !ownedFolders[f.ID] {
													availFolders = append(availFolders, f)
												}
											}
											gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
												a.sectionLabel(c, a.t("inspector.folders"))
												gui.Spacer(c)
												if len(availFolders) > 0 {
													addFBtn := gui.ButtonBase(c.Key("insp-add-folder-pop-" + asset.ID)).
														Size(24, 24).
														Radius(7).
														Center().
														Label(a.t("inspector.addFolder")).
														Tooltip(a.t("inspector.addFolder")).
														PopoverMenuButton(func(m *gui.Menu) {
															for _, cand := range availFolders {
																f := cand
																if m.Item(a.folderFullPath(f.ID)).Chosen() {
																	a.setAssetFolderFast(asset.ID, f.ID, true)
																}
															}
														})
													abg := gui.Transparent
													afg := a.pal.inkMuted
													if addFBtn.Hovered() || addFBtn.Pressed() {
														abg = a.pal.surfaceMuted
														afg = a.pal.ink
													}
													addFBtn.Background(addFBtn.AnimateColor("fbg", abg, animFast)).
														TextColor(addFBtn.AnimateColor("ffg", afg, animFast)).
														Children(func() {
															gui.Icon(c, iconPlus).Size(13, 13)
														})
												}
											})
											if len(appliedFolders) > 0 {
												gui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
													for _, folder := range appliedFolders {
														f := folder
														a.tintedFolderChip(c, "insp-folder-"+asset.ID+"-"+f.ID, f, func() {
															a.setAssetFolderFast(asset.ID, f.ID, false)
														})
													}
												})
											} else {
												gui.Text(c, a.t("inspector.noFolders")).FontSize(11).TextColor(a.pal.inkMuted)
											}
										})
									}
									gui.Box(c).FillWidth().Height(1).Background(hairline)
									gui.Column(c).FillWidth().Gap(8).Children(func() {
										owned := map[string]bool{}
										applied := a.assetTags[asset.ID]
										for _, tg := range applied {
											owned[tg.ID] = true
										}
										var available []model.Tag
										for _, tg := range a.tags {
											if !owned[tg.ID] {
												available = append(available, tg)
											}
										}
										gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
											a.sectionLabel(c, a.t("library.tags"))
											gui.Spacer(c)
											if len(available) > 0 {
												addBtn := gui.ButtonBase(c.Key("insp-add-tag-pop-" + asset.ID)).
													Size(24, 24).
													Radius(7).
													Center().
													Label(a.t("inspector.addTag")).
													Tooltip(a.t("inspector.addTag")).
													PopoverMenuButton(func(m *gui.Menu) {
														for _, cand := range available {
															tg := cand
															if m.Item(tg.Name).Chosen() {
																a.setAssetTagFast(asset.ID, tg.ID, true)
															}
														}
													})
												abg := gui.Transparent
												afg := a.pal.inkMuted
												if addBtn.Hovered() || addBtn.Pressed() {
													abg = a.pal.surfaceMuted
													afg = a.pal.ink
												}
												addBtn.Background(addBtn.AnimateColor("abg", abg, animFast)).
													TextColor(addBtn.AnimateColor("afg", afg, animFast)).
													Children(func() {
														gui.Icon(c, iconPlus).Size(13, 13)
													})
											}
										})
										if len(applied) > 0 {
											gui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
												for _, tag := range applied {
													tg := tag
													a.tintedTagChip(c, "insp-tag-"+asset.ID+"-"+tg.ID, tg, func() {
														a.setAssetTagFast(asset.ID, tg.ID, false)
													})
												}
											})
										}
										gui.Row(c).FillWidth().Gap(4).AlignItems(gui.Center).Children(func() {
											gui.TextInput(c.Key("insp-quick-tag"), &a.quickTagName).
												Placeholder(a.t("inspector.quickTag")).
												Background(a.pal.surface).
												Grow(1).
												OnSubmit(func() { a.quickAddAssetTag(asset.ID) })
											a.iconButton(c, "insp-quick-tag-add", iconPlus, a.t("tag.create"), false).
												Size(28, 28).
												OnClick(func() {
													a.quickAddAssetTag(asset.ID)
												})
											a.iconButton(c, "insp-laya-btn", iconSparkle, a.t("laya.classify"), false).
												Size(28, 28).
												OnClick(func() {
													a.classifyAssetsWithLaya([]string{asset.ID})
												})
										})
										if sugs := a.layaSuggestions[asset.ID]; len(sugs) > 0 {
											gui.Column(c).FillWidth().Gap(5).Padding(2, 0, 0, 0).Children(func() {
												gui.Text(c, a.t("laya.suggestions")).FontSize(10).TextColor(a.pal.inkMuted)
												gui.Row(c).Gap(5).Wrap().Children(func() {
													for _, sg := range sugs {
														sug := sg
														lbl := fmt.Sprintf("+ %s %.0f%%", sug.Tag.Name, sug.Probability*100)
														a.toolButton(c, "laya-sug-"+asset.ID+"-"+sug.Tag.ID, nil, lbl, false).
															Padding(3, 8).
															Border(1, a.pal.controlOn).
															OnClick(func() { a.applyLayaSuggestion(asset.ID, sug.Tag.ID) })
													}
												})
											})
										}
									})
								})

								// Card 2: Editable notes & source URL.
								if a.inspectorMetaID != asset.ID {
									a.inspectorMetaID = asset.ID
									a.inspectorNotes = asset.Notes
									a.inspectorSrcURL = asset.SourceURL
								}
								inspectorCard("insp-card-meta-"+asset.ID, 8, func() {
									gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
										a.sectionLabel(c, a.t("inspector.notes"))
										gui.Spacer(c)
										if a.inspectorNotes != asset.Notes || a.inspectorSrcURL != asset.SourceURL {
											a.toolButton(c, "insp-save-meta", iconCheck, a.t("inspector.saveMeta"), true).
												Padding(3, 8).
												OnClick(func() {
													if _, err := a.lib.UpdateAssetMeta(asset.ID, a.inspectorNotes, a.inspectorSrcURL, asset.SourceTitle); err == nil {
														a.reload()
														a.notify(a.t("inspector.metaSaved"))
													}
												})
										}
									})
									gui.TextInput(c.Key("insp-notes-"+asset.ID), &a.inspectorNotes).
										Placeholder(a.t("inspector.notesPlaceholder")).
										Background(a.pal.surface).
										FillWidth().
										OnSubmit(func() {
											if _, err := a.lib.UpdateAssetMeta(asset.ID, a.inspectorNotes, a.inspectorSrcURL, asset.SourceTitle); err == nil {
												a.reload()
												a.notify(a.t("inspector.metaSaved"))
											}
										})
									gui.Row(c).FillWidth().Gap(4).AlignItems(gui.Center).Children(func() {
										gui.TextInput(c.Key("insp-srcurl-"+asset.ID), &a.inspectorSrcURL).
											Placeholder(a.t("inspector.source")).
											Background(a.pal.surface).
											Grow(1).
											OnSubmit(func() {
												if _, err := a.lib.UpdateAssetMeta(asset.ID, a.inspectorNotes, a.inspectorSrcURL, asset.SourceTitle); err == nil {
													a.reload()
													a.notify(a.t("inspector.metaSaved"))
												}
											})
										if openURL := normalizeWebURL(a.inspectorSrcURL); openURL != "" {
											a.iconButton(c, "insp-open-url", iconExternal, a.t("webimport.openPage"), false).
												Size(28, 28).
												OnClick(func() {
													services.OpenURL(openURL)
												})
										}
									})
								})

								// Card 3: Used in Projects.
								inspectorCard("insp-card-proj-"+asset.ID, 8, func() {
									gui.Row(c).FillWidth().Gap(6).AlignItems(gui.Center).Children(func() {
										a.sectionLabel(c, a.t("inspector.usedIn"))
										if a.inspectorPlaces > 0 {
											gui.Text(c, fmt.Sprintf("· %d", a.inspectorPlaces)).
												FontSize(10).
												TextColor(a.pal.inkMuted).
												Tooltip(fmt.Sprintf(a.t("inspector.placements"), a.inspectorPlaces))
										}
										gui.Spacer(c)
										if len(a.projectList) > 0 {
											gui.PopoverMenuButton(c.Key("insp-add-proj"), "+ "+a.t("library.addToProject"), func(m *gui.Menu) {
												for _, p := range a.projectList {
													proj := p
													if a.menuItem(m, iconFolder, proj.Name, "", false).Chosen() {
														a.referenceAssetToProject(asset.ID, proj.ID)
													}
												}
											}, a.popoverMenuBtnOpts()).Padding(3, 8).FontSize(11)
										}
									})
									if len(a.inspectorProjects) == 0 {
										gui.Text(c, a.t("inspector.usedNone")).FontSize(11).TextColor(a.pal.inkMuted)
									} else {
										gui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
											for _, p := range a.inspectorProjects {
												proj := p
												a.tagChip(c, "insp-used-"+proj.ID, proj.Name, true).OnClick(func() {
													a.openProject(proj.ID)
												})
											}
										})
									}
								})

								// Card 4: Compact key-value file details + single-line clickable path.
								inspectorCard("insp-card-details-"+asset.ID, 6, func() {
									a.sectionLabel(c, a.t("inspector.details"))
									if asset.Width > 0 && asset.Height > 0 {
										detailRow(a.t("inspector.dimensions"), fmt.Sprintf("%d × %d px", asset.Width, asset.Height))
										detailRow(a.t("inspector.aspect"), aspectRatioLabel(asset.Width, asset.Height))
									}
									if a.inspectorFileSize > 0 {
										detailRow(a.t("inspector.size"), formatBytes(a.inspectorFileSize))
									}
									if asset.Mime != "" {
										detailRow(a.t("library.mime"), asset.Mime)
									}
									if len(asset.Hash) >= 12 {
										detailRow(a.t("inspector.hash"), asset.Hash[:12]+"…")
									}
									if !asset.CreatedAt.IsZero() {
										detailRow(a.t("inspector.added"), asset.CreatedAt.Local().Format("2006-01-02 15:04"))
									}
									if asset.Path != "" {
										gui.Box(c).FillWidth().Height(1).MarginY(2).Background(hairline)
										pathBtn := gui.ButtonBase(c.Key("insp-path-copy-" + asset.ID)).
											FillWidth().
											Padding(4, 6).
											Gap(6).
											Radius(radiusControl - 4).
											AlignItems(gui.Center).
											Tooltip(asset.Path + " — " + a.t("common.copyPath")).
											OnClick(func() {
												services.WriteClipboard(asset.Path)
												a.notify(a.t("common.copied"))
											})
										pBg := gui.Transparent
										if pathBtn.Hovered() {
											pBg = a.pal.surfaceMuted
										}
										pathBtn.Background(pathBtn.AnimateColor("bg", pBg, animFast)).Children(func() {
											gui.Icon(c, iconFolder).Size(12, 12).TextColor(a.pal.inkMuted)
											gui.Text(c, asset.Path).FontSize(10).TextColor(a.pal.inkMuted).SingleLine().MinWidth(0).Grow(1)
										})
									}
								})

								// Quiet destructive delete button.
								delBtn := gui.ButtonBase(c.Key("insp-delete")).
									FillWidth().
									Height(34).
									Gap(6).
									Radius(radiusControl).
									Center().
									Border(1, hairline).
									Label(a.t("library.delete")).
									OnClick(func() { a.deleteAssetPrompt(asset) })
								delBg := a.pal.surfaceRaised
								if delBtn.Pressed() || delBtn.Hovered() {
									delBg = dangerHover
								}
								delBtn.Background(delBtn.AnimateColor("bg", delBg, animFast)).
									TextColor(a.pal.danger).
									Children(func() {
										gui.Icon(c, iconTrash).Size(13, 13)
										gui.Text(c, a.t("library.delete")).FontSize(12).Bold().SingleLine()
									})
							})
						})
					}
				}

				// Bottom footer bar inside the Inspector panel with collapse button.
				gui.Row(c).
					Height(42).
					Padding(0, 10).
					AlignItems(gui.Center).
					Children(func() {
						gui.Spacer(c)
					if a.inspectorOpen {
						a.iconButton(c, "insp-collapse-btn", iconPanelRight, a.t("inspector.toggle"), false).OnClick(func() {
							a.inspectorOpen = false
						})
					}
				})
		})
}

func isTextSnippetExt(ext string) bool {
	switch strings.ToUpper(strings.TrimSpace(ext)) {
	case "MD", "MARKDOWN", "TXT":
		return true
	}
	return false
}

func (a *App) cachedTextSnippet(path string) string {
	if path == "" {
		return ""
	}
	if a.mdSnippets == nil {
		a.mdSnippets = map[string]string{}
	}
	if s, ok := a.mdSnippets[path]; ok {
		return s
	}
	var snippet string
	if f, err := os.Open(path); err == nil {
		raw, _ := io.ReadAll(io.LimitReader(f, 512))
		_ = f.Close()
		var lines []string
		for _, ln := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
			ln = strings.TrimSpace(strings.TrimLeft(ln, "#>-*` "))
			if ln != "" {
				lines = append(lines, ln)
				if len(lines) >= 4 {
					break
				}
			}
		}
		snippet = strings.Join(lines, "\n")
		if len(snippet) > 180 {
			snippet = snippet[:180] + "…"
		}
	}
	a.mdSnippets[path] = snippet
	return snippet
}
