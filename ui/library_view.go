package ui

import (
	"fmt"
	"strings"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
)

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
					a.arcButton(c, "ref-selected-btn", a.t("library.reference"), true).OnClick(func() {
						a.referenceSelected()
					})
					a.arcButton(c, "ref-done-btn", a.t("common.done"), false).OnClick(func() {
						a.referenceMode = false
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
						if m.Item(proj.Name).Chosen() {
							a.batchReferenceToProject(ids, proj.ID)
						}
					}
				})
			}

			if len(ids) > 0 && len(a.tags) > 0 {
				gui.PopoverMenuButton(c.Key("batch-apply-tag"), a.t("batch.applyTag"), func(m *gui.Menu) {
					for _, tg := range a.tags {
						tag := tg
						if m.Item(tag.Name).Chosen() {
							a.batchApplyTag(ids, tag.ID)
						}
					}
				})
			}

			if len(ids) > 0 {
				gui.PopoverMenuButton(c.Key("batch-send-board"), a.t("board.sendTo"), func(m *gui.Menu) {
					if m.Item(a.t("board.sendToNew")).Chosen() {
						a.sendAssetsToNewBoard(ids)
					}
					if len(a.standaloneBoards) > 0 {
						m.Separator()
						for _, sb := range a.standaloneBoards {
							b := sb
							if m.Item(b.Name).Chosen() {
								a.sendAssetsToBoard(ids, b.ID)
							}
						}
					}
				})
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

// assetMasonry renders an Eagle-style multi-column waterfall masonry layout
// preserving each asset's natural aspect ratio in balanced shortest-column order,
// with full viewport virtualization so 250,000+ item libraries scroll at 60 FPS.
func (a *App) assetMasonry(c *gui.Context) {
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	scroll := gui.Scroll(c.Key("asset-masonry")).Grow(1).FillWidth().TrackScroll(&a.masonryScroll)
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
	scrollY := a.masonryScroll.Y
	const overscan float32 = 720
	minY := scrollY - overscan
	maxY := scrollY + viewH + overscan

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

	type masonryEntry struct {
		index int
		y     float32
		h     float32
	}
	buckets := make([][]masonryEntry, cols)
	colHeights := make([]float32, cols)

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
		itemH := colW*ratio + capH
		best := 0
		for k := 1; k < cols; k++ {
			if colHeights[k] < colHeights[best] {
				best = k
			}
		}
		y := colHeights[best]
		buckets[best] = append(buckets[best], masonryEntry{index: i, y: y, h: itemH})
		colHeights[best] = y + itemH + gap
	}

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
					firstVis := -1
					lastVis := -1
					for idx, ent := range entries {
						if ent.y+ent.h >= minY && ent.y <= maxY {
							if firstVis < 0 {
								firstVis = idx
							}
							lastVis = idx
						} else if firstVis >= 0 && ent.y > maxY {
							break
						}
					}
					if firstVis < 0 {
						gui.Box(c).FillWidth().Height(totalH)
						return
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
// blocks and wide accent rows, with viewport virtualization for huge libraries.
func (a *App) assetBento(c *gui.Context) {
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	scroll := gui.Scroll(c.Key("asset-bento")).Grow(1).FillWidth().TrackScroll(&a.bentoScroll)
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
	scrollY := a.bentoScroll.Y
	const overscan float32 = 720
	minY := scrollY - overscan
	maxY := scrollY + viewH + overscan

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

	scroll.Children(func() {
		gui.Column(c).FillWidth().Padding(8, 12, 72, 8).Gap(gap).Children(func() {
			n := len(a.assets)
			i := 0
			blockIdx := 0
			var curY float32 = 8
			sideCols := cols - 2
			sideCount := sideCols * 2
			heroBlockSize := 1 + sideCount

			for i < n {
				rem := n - i
				// Hero Bento block: 2x2 hero tile + 2 rows of companion tiles.
				if cols >= 3 && rem >= heroBlockSize && blockIdx%2 == 0 {
					heroIdx := i
					heroH := unitH*2 + gap
					vis := curY+heroH >= minY && curY <= maxY
					curY += heroH + gap
					if !vis {
						gui.Box(c.Key(fmt.Sprintf("bento-blk-sp-%d", blockIdx))).FillWidth().Height(heroH)
						i += heroBlockSize
						blockIdx++
						continue
					}
					heroLeft := (blockIdx/2)%2 == 0

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

					gui.Row(c.Key(fmt.Sprintf("bento-blk-%d", blockIdx))).FillWidth().Gap(gap).Children(func() {
						if heroLeft {
							renderHero()
							renderSide()
						} else {
							renderSide()
							renderHero()
						}
					})
					i += heroBlockSize
					blockIdx++
					continue
				}

				// Wide accent Bento row: one 2-column wide card + standard companions.
				wideRowSize := cols - 1
				if cols >= 3 && rem >= wideRowSize && rem != cols {
					vis := curY+unitH >= minY && curY <= maxY
					curY += unitH + gap
					if !vis {
						gui.Box(c.Key(fmt.Sprintf("bento-wide-sp-%d", blockIdx))).FillWidth().Height(unitH)
						i += wideRowSize
						blockIdx++
						continue
					}
					widePos := 0
					if blockIdx%2 == 1 {
						widePos = wideRowSize - 1
					}
					start := i
					gui.Row(c.Key(fmt.Sprintf("bento-wide-%d", blockIdx))).FillWidth().Height(unitH).Gap(gap).Children(func() {
						for k := 0; k < wideRowSize; k++ {
							idx := start + k
							span := float32(1)
							if k == widePos {
								span = 2
							}
							gui.Box(c).Grow(span).FillHeight().Children(func() {
								a.assetCard(c, idx, a.assets[idx], a.isCardSelected(idx, a.assets[idx].ID))
							})
						}
					})
					i += wideRowSize
					blockIdx++
					continue
				}

				// Standard balanced row (fills cols, or wide card if single trailing item).
				take := min(rem, cols)
				vis := curY+unitH >= minY && curY <= maxY
				curY += unitH + gap
				if !vis {
					gui.Box(c.Key(fmt.Sprintf("bento-row-sp-%d", blockIdx))).FillWidth().Height(unitH)
					i += take
					blockIdx++
					continue
				}
				start := i
				gui.Row(c.Key(fmt.Sprintf("bento-row-%d", blockIdx))).FillWidth().Height(unitH).Gap(gap).Children(func() {
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
				i += take
				blockIdx++
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
			if m.Item(fmt.Sprintf(a.t("stack.expand"), stackCount)).Chosen() {
				a.openStackView(stID)
			}
			if m.Item(a.t("stack.ungroup")).Chosen() {
				a.ungroupStack(stID)
			}
			m.Separator()
		} else if a.openStackID != "" && asset.StackID == a.openStackID {
			stID := asset.StackID
			if st, ok := a.stackByID[stID]; ok && st.CoverID != asset.ID {
				if m.Item(a.t("stack.setCover")).Chosen() {
					a.setStackCover(stID, asset.ID)
				}
			}
			if m.Item(a.t("stack.removeAsset")).Chosen() {
				a.removeFromStack([]string{asset.ID})
			}
			if m.Item(a.t("stack.ungroup")).Chosen() {
				a.ungroupStack(stID)
			}
			m.Separator()
		}
		if multi {
			if m.Item(fmt.Sprintf("%s (%d) (Ctrl+G)", a.t("stack.create"), len(batchIDs))).Chosen() {
				a.stackSelectedAssets(batchIDs)
			}
		}
		if !multi {
			if m.Item(a.t("library.preview")).Chosen() {
				a.openPreview(asset)
			}
		}
		if len(a.projectList) > 0 {
			addProjTitle := a.t("library.addToProject")
			if multi {
				addProjTitle = fmt.Sprintf("%s (%d)", addProjTitle, len(batchIDs))
			}
			m.Submenu(addProjTitle, func(sm *gui.Menu) {
				for _, p := range a.projectList {
					proj := p
					if sm.Item(proj.Name).Chosen() {
						if multi {
							a.batchReferenceToProject(batchIDs, proj.ID)
						} else {
							a.referenceAssetToProject(asset.ID, proj.ID)
						}
					}
				}
			})
		}
		if multi && len(a.tags) > 0 {
			m.Submenu(fmt.Sprintf("%s (%d)", a.t("batch.applyTag"), len(batchIDs)), func(sm *gui.Menu) {
				for _, tg := range a.tags {
					tag := tg
					if sm.Item(tag.Name).Chosen() {
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
		m.Submenu(boardMenuTitle, func(sm *gui.Menu) {
			if sm.Item(a.t("board.sendToNew")).Chosen() {
				a.sendAssetsToNewBoard(targetIDs)
			}
			if len(a.standaloneBoards) > 0 {
				sm.Separator()
				for _, sb := range a.standaloneBoards {
					b := sb
					if sm.Item(b.Name).Chosen() {
						a.sendAssetsToBoard(targetIDs, b.ID)
					}
				}
			}
		})
		if !multi {
			m.Submenu(a.t("library.rating"), func(sm *gui.Menu) {
				for r := 0; r <= 5; r++ {
					rating := r
					lbl := "0"
					if rating > 0 {
						lbl = strings.Repeat("★", rating)
					}
					if sm.Item(lbl).Radio(asset.Rating == rating).Chosen() {
						if err := a.lib.SetRating(asset.ID, rating); err == nil {
							a.reload()
						}
					}
				}
			})
			m.Separator()
			if !asset.Missing && asset.Path != "" {
				if m.Item(a.t("common.openFile")).Chosen() {
					_ = openLocalFile(asset.Path)
				}
				if m.Item(a.t("common.revealFile")).Chosen() {
					_ = revealLocalFile(asset.Path)
				}
			}
			if m.Item(a.t("common.copyPath")).Chosen() {
				services.WriteClipboard(asset.Path)
				a.notify(a.t("common.copied"))
			}
			if asset.Missing {
				if m.Item(a.t("inspector.relinkFile")).Chosen() {
					a.relinkMissing(false)
				}
				if m.Item(a.t("inspector.relink")).Chosen() {
					a.relinkMissing(true)
				}
			}
		}
		m.Separator()
		if !multi && model.MediaKind(asset.Mime) == model.MediaImage && !asset.Missing {
			if m.Item(a.t("visual.findSimilar")).Chosen() {
				a.searchSimilarByAsset(asset)
			}
		}
		layaLabel := a.t("laya.classify")
		layaIDs := []string{asset.ID}
		if multi {
			layaLabel = fmt.Sprintf("%s (%d)", a.t("laya.classify"), len(batchIDs))
			layaIDs = batchIDs
		}
		if m.Item(layaLabel).Chosen() {
			a.classifyAssetsWithLaya(layaIDs)
		}
		if a.plugins != nil {
			hook := "context_menu"
			if multi {
				hook = "batch_action"
			}
			if plugs := a.plugins.ForHook(hook); len(plugs) > 0 {
				m.Submenu(a.t("plugins.menu"), func(sm *gui.Menu) {
					for _, inst := range plugs {
						pl := inst
						if sm.Item(pl.Manifest.Name).Chosen() {
							a.runPlugin(pl.Manifest.ID, hook, layaIDs)
						}
					}
				})
			}
		}
		m.Separator()
		if multi {
			if m.Item(fmt.Sprintf("%s (%d)", a.t("rename.batchAction"), len(batchIDs))).Chosen() {
				a.openRenameModal(batchIDs)
			}
			if m.Item(fmt.Sprintf("%s (%d)", a.t("export.action"), len(batchIDs))).Chosen() {
				a.openExportModal(batchIDs)
			}
			if m.Item(fmt.Sprintf("%s (%d)", a.t("clipboard.copyFiles"), len(batchIDs))).Chosen() {
				a.copySelectedFilesToClipboard(batchIDs)
			}
			m.Separator()
			if m.Item(fmt.Sprintf("%s (%d)", a.t("batch.delete"), len(batchIDs))).Chosen() {
				a.batchDeletePrompt(batchIDs)
			}
		} else {
			if m.Item(a.t("rename.action") + " (F2)").Chosen() {
				a.openRenameModal([]string{asset.ID})
			}
			if m.Item(a.t("export.action")).Chosen() {
				a.openExportModal([]string{asset.ID})
			}
			if !asset.Missing && asset.Path != "" {
				if m.Item(a.t("clipboard.copyFiles") + " (Ctrl+C)").Chosen() {
					a.copySelectedFilesToClipboard([]string{asset.ID})
				}
			}
			m.Separator()
			if m.Item(a.t("library.delete")).Chosen() {
				a.deleteAssetPrompt(asset)
			}
		}
	})

	card.OnClick(func() {
		a.selectCard(index, c.Modifiers())
	})
	if card.DoubleClicked() {
		a.selectCard(index, 0)
		a.openPreview(asset)
	}

	hovered := card.Hovered()
	pressed := card.Pressed()
	dragging := card.Dragging()

	targetBg := a.pal.surface
	liftY := float32(0)
	cardScale := float32(1)
	imgScale := float32(1)
	shadow := a.pal.shadowResting
	switch {
	case dragging:
		// Shrink dragged preview to a crisp ~56px miniature so the cursor stays
		// clearly visible for accurate aiming at sidebar tags, boards & projects.
		bounds := card.LayoutBounds()
		maxDim := max(bounds.W, bounds.H)
		dragScale := float32(0.30)
		if maxDim > 0 {
			dragScale = 56.0 / maxDim
			if dragScale < 0.14 {
				dragScale = 0.14
			} else if dragScale > 0.42 {
				dragScale = 0.42
			}
		}
		cardScale = dragScale
		shadow = a.pal.shadowFloating
		card.Opacity(0.26)
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
	if dragging || pressed {
		card.AnimatePose(gui.Pose{Y: liftY, Scale: cardScale}, springResponsive)
	} else {
		card.AnimatePose(gui.Pose{Y: liftY, Scale: cardScale}, springGentle)
	}
	if len(a.assets) <= 250 {
		stagger := time.Duration(min(index, 6)) * arcStaggerItem
		applyCardBlurFade(card, cardRevealKey{gen: a.revealGen, id: asset.ID}, stagger)
	}

	card.Children(func() {
		// Full-bleed edge-to-edge media slot with subtle hover zoom & optional stack badge.
		gui.Box(c).Grow(1).FillWidth().Clip().Background(a.pal.surfaceMuted).Center().Children(func() {
			if bm := a.requestCardBitmap(asset); bm != nil {
				gui.Image(c, bm).
					Fit(gui.Cover).
					Fill().
					AnimatePose(gui.Pose{Scale: imgScale}, springGentle)
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

		// Clean frameless caption strip (pure monochrome; toggleable in Preferences).
		if a.showCardCaption {
			gui.Row(c).FillWidth().Padding(8, 11).Gap(6).AlignItems(gui.Center).Children(func() {
				if selected || a.multiSelectMode || hovered {
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
				} else if asset.Rating > 0 {
					gui.Text(c, fmt.Sprintf("★%d", asset.Rating)).FontSize(11).TextColor(a.pal.inkSecondary).SingleLine()
				} else {
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

	gui.Box(c).Fill().Padding(6, 12, 10, 4).Background(a.pal.canvas).Children(func() {
		gui.Column(c.Key("arc-inspector")).
			Fill().
			Radius(radiusPanel).
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
					// Top Info / Similar segmented tab switcher (matching reference design).
					gui.Row(c).FillWidth().Padding(12, 14, 6, 14).Justify(gui.Center).Children(func() {
						a.arcPillTabs(c, "insp-mode-tabs", a.inspectorTab, []string{
							a.t("inspector.tabInfo"),
							a.t("inspector.tabSimilar"),
						}, func(idx int) {
							a.inspectorTab = idx
							if idx == 1 {
								a.ensureInspectorSimilar(asset)
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
						gui.Scroll(c).Grow(1).FillWidth().Children(func() {
							body := gui.Column(c.Key("insp-body-" + asset.ID)).Padding(10, 16, 16, 16).Gap(14)
							applyBlurReveal(body, asset.ID, 0)
							body.Children(func() {
								// Full uncropped hero preview when browsing grid (omitted in inline viewer).
								if !a.previewOpen {
									hero := gui.ButtonBase(c.Key("insp-hero-" + asset.ID)).
										FillWidth().
										Height(176).
										Radius(radiusControl).
										Clip().
										Background(a.pal.surfaceRaised).
										Center().
										Label(a.t("library.preview")).
										Tooltip(a.t("library.preview")).
										OnClick(func() {
											a.openPreview(asset)
										})
									heroScale := float32(1)
									if hero.Pressed() {
										heroScale = 0.985
									} else if hero.Hovered() {
										heroScale = 1.02
									}
									hero.AnimatePose(gui.Pose{Scale: heroScale}, springGentle)
									hero.Children(func() {
										if bm := a.requestCardBitmap(asset); bm != nil {
											gui.Image(c, bm).Fit(gui.Contain).Grow(1).Padding(6).Radius(radiusControl)
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
								}

						// Quick action bar right below preview.
						gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
							a.toolButton(c, "insp-preview-btn", iconEye, a.t("library.preview"), false).
								Grow(1).
								OnClick(func() { a.openPreview(asset) })
							if !asset.Missing && asset.Path != "" {
								a.iconButton(c, "insp-open-btn", iconExternal, a.t("common.openFile"), false).OnClick(func() {
									_ = openLocalFile(asset.Path)
								})
								a.iconButton(c, "insp-reveal-btn", iconFolder, a.t("common.revealFile"), false).OnClick(func() {
									_ = revealLocalFile(asset.Path)
								})
							}
							a.iconButton(c, "insp-copy-path", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
								services.WriteClipboard(asset.Path)
								a.notify(a.t("common.copied"))
							})
						})

						// Extracted 5-color OKLab dominant palette strip (Eagle-style).
						if len(asset.Palette) > 0 {
							gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
								gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
									for i, pc := range asset.Palette {
										hex := pc.Hex
										col, ok := parseHexColor(hex)
										if !ok {
											continue
										}
										gui.ButtonBase(c.Key(fmt.Sprintf("insp-pal-%s-%d", asset.ID, i))).
											Size(16, 16).
											Radius(radiusPill).
											Background(col).
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
									OnClick(func() { a.setColorFilter(asset.Palette[0].Hex) })
								if model.MediaKind(asset.Mime) == model.MediaImage && !asset.Missing {
									a.iconButton(c, "insp-sim-btn", iconImage, a.t("visual.findSimilar"), false).
										OnClick(func() { a.searchSimilarByAsset(asset) })
								}
							})
						}

						// Title + quiet monochrome metadata chain (design.md §13 inspired).
						gui.Column(c).Gap(6).Children(func() {
							gui.Row(c).FillWidth().Gap(6).AlignItems(gui.Center).Children(func() {
								gui.Text(c, a.assetName(asset)).FontSize(14).Bold().LineHeight(1.3).Grow(1)
								a.iconButton(c, "insp-rename-btn", iconEdit, a.t("rename.action")+" (F2)", false).OnClick(func() {
									a.openRenameModal([]string{asset.ID})
								})
								a.iconButton(c, "insp-export-btn", iconExport, a.t("export.action"), false).OnClick(func() {
									a.openExportModal([]string{asset.ID})
								})
							})
							parts := []string{a.assetExt(asset), a.modeLabel(asset.StorageMode)}
							if asset.Width > 0 && asset.Height > 0 {
								parts = append(parts, fmt.Sprintf("%d × %d", asset.Width, asset.Height))
							}
							gui.Text(c, strings.Join(parts, "  ·  ")).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
						})

						// Editable notes & source URL.
						if a.inspectorMetaID != asset.ID {
							a.inspectorMetaID = asset.ID
							a.inspectorNotes = asset.Notes
							a.inspectorSrcURL = asset.SourceURL
						}
						gui.Column(c).Gap(6).Children(func() {
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
								FillWidth().
								OnSubmit(func() {
									if _, err := a.lib.UpdateAssetMeta(asset.ID, a.inspectorNotes, a.inspectorSrcURL, asset.SourceTitle); err == nil {
										a.reload()
										a.notify(a.t("inspector.metaSaved"))
									}
								})
							gui.Row(c).FillWidth().Gap(6).AlignItems(gui.Center).Children(func() {
								gui.TextInput(c.Key("insp-srcurl-"+asset.ID), &a.inspectorSrcURL).
									Placeholder(a.t("inspector.source")).
									Grow(1).
									OnSubmit(func() {
										if _, err := a.lib.UpdateAssetMeta(asset.ID, a.inspectorNotes, a.inspectorSrcURL, asset.SourceTitle); err == nil {
											a.reload()
											a.notify(a.t("inspector.metaSaved"))
										}
									})
								if strings.HasPrefix(asset.SourceURL, "http://") || strings.HasPrefix(asset.SourceURL, "https://") {
									a.iconButton(c, "insp-open-url", iconLink, a.t("inspector.source"), false).OnClick(func() {
										services.OpenURL(asset.SourceURL)
									})
								}
							})
						})

						// Missing file alert & relink CTAs.
						if asset.Missing {
							gui.Column(c).Padding(12).Gap(8).Radius(radiusControl).Background(a.pal.surfaceRaised).Children(func() {
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

						// Interactive Rating.
						gui.Column(c).Gap(6).Children(func() {
							a.sectionLabel(c, a.t("library.rating"))
							gui.Rating(c.Key("rating-"+asset.ID), &a.inspectorRating, 5).OnChange(func() {
								if err := a.lib.SetRating(asset.ID, a.inspectorRating); err == nil {
									a.reload()
								}
							})
						})

						// Applied translucent tinted tag badges (Eagle-style) + "+" tag picker + quick-add.
						gui.Column(c).Gap(8).Children(func() {
							a.sectionLabel(c, a.t("library.tags"))
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
							gui.Row(c).Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
								for _, tag := range applied {
									tg := tag
									a.tintedTagChip(c, "insp-tag-"+asset.ID+"-"+tg.ID, tg, func() {
										if err := a.lib.SetAssetTag(asset.ID, tg.ID, false); err != nil {
											a.status = err.Error()
											return
										}
										a.reload()
									})
								}
								if len(available) > 0 {
									addBtn := gui.ButtonBase(c.Key("insp-add-tag-pop-" + asset.ID)).
										Size(26, 26).
										Radius(8).
										Center().
										Label(a.t("inspector.addTag")).
										Tooltip(a.t("inspector.addTag")).
										PopoverMenuButton(func(m *gui.Menu) {
											for _, cand := range available {
												tg := cand
												if m.Item(tg.Name).Chosen() {
													if err := a.lib.SetAssetTag(asset.ID, tg.ID, true); err != nil {
														a.status = err.Error()
														return
													}
													a.reload()
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
											gui.Icon(c, iconPlus).Size(14, 14)
										})
								}
							})
							gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
								gui.TextInput(c.Key("insp-quick-tag"), &a.quickTagName).
									Placeholder(a.t("inspector.quickTag")).
									Grow(1).
									OnSubmit(func() { a.quickAddAssetTag(asset.ID) })
								a.iconButton(c, "insp-quick-tag-add", iconPlus, a.t("tag.create"), false).OnClick(func() {
									a.quickAddAssetTag(asset.ID)
								})
								a.iconButton(c, "insp-laya-btn", iconSparkle, a.t("laya.classify"), false).OnClick(func() {
									a.classifyAssetsWithLaya([]string{asset.ID})
								})
							})
							if sugs := a.layaSuggestions[asset.ID]; len(sugs) > 0 {
								gui.Column(c).FillWidth().Gap(5).Padding(4, 0, 0, 0).Children(func() {
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

						// Used in Projects.
						gui.Column(c).Gap(8).Children(func() {
							gui.Row(c).AlignItems(gui.Center).Children(func() {
								a.sectionLabel(c, a.t("inspector.usedIn"))
								gui.Spacer(c)
								if a.inspectorPlaces > 0 {
									gui.Text(c, fmt.Sprintf(a.t("inspector.placements"), a.inspectorPlaces)).FontSize(10).TextColor(a.pal.inkMuted)
								}
							})
							if len(a.inspectorProjects) == 0 {
								gui.Text(c, a.t("inspector.usedNone")).FontSize(11).TextColor(a.pal.inkMuted)
							} else {
								gui.Row(c).Gap(6).Wrap().Children(func() {
									for _, p := range a.inspectorProjects {
										proj := p
										a.tagChip(c, "insp-used-"+proj.ID, proj.Name, true).OnClick(func() {
											a.openProject(proj.ID)
										})
									}
								})
							}
							if len(a.projectList) > 0 {
								gui.PopoverMenuButton(c.Key("insp-add-proj"), "+ "+a.t("library.addToProject"), func(m *gui.Menu) {
									for _, p := range a.projectList {
										proj := p
										if m.Item(proj.Name).Chosen() {
											a.referenceAssetToProject(asset.ID, proj.ID)
										}
									}
								}).FillWidth()
							}
						})

						// Clean key-value metadata (no noisy nested card boxes).
						gui.Column(c).Gap(5).Children(func() {
							a.sectionLabel(c, a.t("inspector.details"))
							if asset.Width > 0 && asset.Height > 0 {
								a.metaRow(c, a.t("inspector.dimensions"), fmt.Sprintf("%d × %d px", asset.Width, asset.Height))
								a.metaRow(c, a.t("inspector.aspect"), aspectRatioLabel(asset.Width, asset.Height))
							}
							if a.inspectorFileSize > 0 {
								a.metaRow(c, a.t("inspector.size"), formatBytes(a.inspectorFileSize))
							}
							if asset.Mime != "" {
								a.metaRow(c, a.t("library.mime"), asset.Mime)
							}
							if len(asset.Hash) >= 12 {
								a.metaRow(c, a.t("inspector.hash"), asset.Hash[:12]+"…")
							}
							if !asset.CreatedAt.IsZero() {
								a.metaRow(c, a.t("inspector.added"), asset.CreatedAt.Local().Format("2006-01-02 15:04"))
							}
							gui.Text(c, asset.Path).FontSize(11).TextColor(a.pal.inkMuted).LineHeight(1.4)
						})

						a.arcButton(c, "insp-delete", a.t("library.delete"), false).
							FillWidth().
							TextColor(a.pal.danger).
							OnClick(func() { a.deleteAssetPrompt(asset) })
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
	})
}
