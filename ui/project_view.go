package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/canvas"
	"raglex/internal/model"
)

// projectView renders a calm Arc UI editorial layout:
//  1. Minimal frameless top bar (back button, draggable strip, window controls)
//  2. Editorial Hero block (eyebrow metadata, large bold display title, spacious
//     description on its own line, interactive colored tag pills, and single actions)
//  3. Live visual Moodboard preview cards + Referenced Assets grid.
func (a *App) projectView(c *gui.Context) {
	projRootBg := a.pal.canvas
	if a.glassActive {
		projRootBg = gui.Transparent
	}
	gui.Column(c).Fill().Background(projRootBg).Children(func() {
		filteredAssets := a.filteredProjectAssets()

		// Ultra-clean edge-to-edge top chrome: zero clutter, zero overflow.
		if !a.focusMode {
			chrome := gui.Row(c.Key("proj-chrome")).
				Height(48).
				FillWidth().
				Padding(0, 10, 0, 12).
				Gap(8).
				Clip().
				AlignItems(gui.Center).
				Background(a.pal.surface).
				BorderWidth(0, 0, 1, 0).
				BorderColor(a.chromeHairline())

			barW := chrome.LayoutBounds().W
			compact := barW > 0 && barW < 880

			chrome.Children(func() {
				if !a.sidebarOpen && a.macTrafficLightSpacer(c) {
					gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
				}
				a.iconButton(c, "proj-back-lib", iconArrowLeft, a.t("shell.library"), false).OnClick(func() {
					a.handleNav("tag-all")
				})
				gui.Row(c).Gap(8).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
					gui.Text(c, a.project.Name).Bold().FontSize(13.5).SingleLine().MaxWidth(220)
					if a.showSidebarCounts && !compact {
						a.pillBadge(c, fmt.Sprintf(a.t("project.summary"), len(a.boards), len(a.projectAssets)), a.pal.inkSecondary, a.pal.surfaceMuted)
					}
				})

				gui.Box(c).Grow(1).FillHeight().DragWindow()

				a.arcButton(c, "proj-from-lib", "+ "+a.t("project.fromLibrary"), true).
					Padding(5, 11).
					OnClick(func() {
						a.referenceMode = true
						a.multiSelectMode = true
						a.clearSelection()
						a.screen = screenLibrary
					})
				a.iconButton(c, "proj-edit-btn", iconEdit, a.t("project.edit"), false).OnClick(func() {
					a.openEditProjectModal()
				})
				a.iconButton(c, "proj-share-btn", iconShare, a.t("share.send"), false).OnClick(func() {
					a.shareOpen = true
				})
				gui.PopoverMenuButton(c.Key("proj-more-btn"), "⋯", func(m *gui.Menu) {
					if a.menuRadio(m, a.t("library.kind.all"), a.projKindFilter == 0).Chosen() {
						a.projKindFilter = 0
					}
					if a.menuRadio(m, a.t("library.kind.image"), a.projKindFilter == 1).Chosen() {
						a.projKindFilter = 1
					}
					if a.menuRadio(m, a.t("library.kind.video"), a.projKindFilter == 2).Chosen() {
						a.projKindFilter = 2
					}
					m.Separator()
					if a.menuRadio(m, a.t("view.showTitle"), a.showCardCaption).Chosen() {
						a.showCardCaption = !a.showCardCaption
						a.layoutGen++
						a.syncConfigFromState()
					}
					if a.menuRadio(m, a.t("view.grayscale"), a.grayscalePreview).Chosen() {
						a.grayscalePreview = !a.grayscalePreview
						a.syncConfigFromState()
					}
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
					m.Separator()
					if a.menuItem(m, iconTrash, a.t("project.delete"), "Del", true).Chosen() {
						a.deleteProjectPrompt(a.projectID)
					}
				}, a.popoverMenuBtnOpts())

				gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
				a.topRightChrome(c)
			})
		}

		// Scrollable minimalist project stage: calm editorial header -> sleek boards strip -> borderless waterfall assets.
		scroll := gui.Scroll(c.Key("proj-scroll")).Grow(1).FillWidth().Background(a.pal.canvas)
		availW := scroll.LayoutBounds().W
		if availW <= 0 {
			availW = 1040
		}
		scroll.Children(func() {
			col := gui.Column(c.Key("proj-view-"+a.projectID)).FillWidth().Padding(22, 28, 36, 28).Gap(20)
			applyPanelSwitch(col, a.projectID, 0, 6)
			col.Children(func() {
				// Calm, ultra-clean Editorial Header (only title, optional description & subtle tag chips).
				var appliedTags []model.Tag
				for _, tg := range a.tags {
					if a.projectTags[tg.ID] {
						appliedTags = append(appliedTags, tg)
					}
				}
				gui.Column(c.Key("proj-hero")).
					FillWidth().
					Padding(0, 0, 16, 0).
					Gap(8).
					BorderWidth(0, 0, 1, 0).
					BorderColor(a.chromeHairline()).
					Children(func() {
						gui.Row(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
							gui.Text(c, a.project.Name).
								FontSize(20).
								Bold().
								LetterSpacing(-0.4).
								SingleLine().
								MinWidth(0)
							gui.Spacer(c)
							if a.searchQuery != "" || a.projSearchQuery != "" || a.projKindFilter != 0 {
								a.toolButton(c, "proj-filter-clear", iconX, a.t("common.clear"), false).
									Padding(4, 8).
									OnClick(func() {
										a.searchQuery = ""
										a.projSearchQuery = ""
										a.projKindFilter = 0
										a.applyFilters()
									})
							}
						})

						if a.project.Description != "" {
							gui.Text(c, a.project.Description).
								MaxWidth(720).
								FontSize(12.5).
								TextColor(a.pal.inkSecondary).
								LineHeight(1.55)
						}

						if len(appliedTags) > 0 {
							gui.Row(c).FillWidth().Padding(2, 0, 0, 0).Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
								for _, tag := range appliedTags {
									tg := tag
									a.tintedTagChip(c, "ptag-"+tg.ID, tg, nil)
								}
							})
						}
					})

				// Section 1: Sleek Moodboards Strip.
				gui.Column(c).
					FillWidth().
					Padding(0, 0, 18, 0).
					Gap(12).
					BorderWidth(0, 0, 1, 0).
					BorderColor(a.chromeHairline()).
					Children(func() {
						gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
							a.sectionLabel(c, a.t("project.boards"))
							if a.showSidebarCounts {
								gui.Text(c, fmt.Sprintf("%d", len(a.boards))).FontSize(11).TextColor(a.pal.inkMuted)
							}
							gui.Spacer(c)
							a.toolButton(c, "create-board-btn", nil, "+ "+a.t("project.newBoard"), false).
								Padding(5, 10).
								OnClick(func() {
									a.createBoard()
								})
						})

						if len(a.boards) == 0 {
							gui.Row(c).
								FillWidth().
								Padding(18, 12).
								Gap(10).
								AlignItems(gui.Center).
								Children(func() {
									gui.Icon(c, iconBoard).Size(16, 16).TextColor(a.pal.inkFaint)
									gui.Text(c, a.t("project.emptyBoardsHint")).FontSize(12).TextColor(a.pal.inkMuted)
								})
						} else {
							gui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
								for i, b := range a.boards {
									a.boardCard(c, i, b)
								}
							})
						}
					})

				// Section 2: Seamless Waterfall Masonry Assets.
				gui.Column(c).FillWidth().Gap(12).Children(func() {
					gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
						a.sectionLabel(c, a.t("project.assets"))
						if a.showSidebarCounts {
							gui.Text(c, fmt.Sprintf("%d", len(filteredAssets))).FontSize(11).TextColor(a.pal.inkMuted)
						}
					})

					if len(a.projectAssets) == 0 {
						gui.Column(c).
							FillWidth().
							Padding(40, 20).
							Gap(8).
							Center().
							Children(func() {
								gui.Icon(c, iconImage).Size(22, 22).TextColor(a.pal.inkFaint)
								gui.Text(c, a.t("project.emptyAssets")).FontSize(13).Bold()
								gui.Text(c, a.t("project.emptyAssetsHint")).FontSize(12).TextColor(a.pal.inkMuted)
							})
					} else if len(filteredAssets) == 0 {
						gui.Box(c).FillWidth().Padding(32).Center().Children(func() {
							gui.Text(c, a.t("library.noMatch")).FontSize(12).TextColor(a.pal.inkSecondary)
						})
					} else {
						a.projectAssetsWaterfall(c, filteredAssets, availW-56)
					}
				})
			})
		})
	})
}

func (a *App) filteredProjectAssets() []model.Asset {
	q := strings.ToLower(strings.TrimSpace(a.searchQuery))
	if q == "" {
		q = strings.ToLower(strings.TrimSpace(a.projSearchQuery))
	}
	if q == "" && a.projKindFilter == 0 {
		return a.projectAssets
	}
	out := make([]model.Asset, 0, len(a.projectAssets))
	for _, it := range a.projectAssets {
		switch a.projKindFilter {
		case 1:
			if model.MediaKind(it.Mime) != model.MediaImage {
				continue
			}
		case 2:
			if model.MediaKind(it.Mime) != model.MediaVideo {
				continue
			}
		}
		if q != "" {
			name := strings.ToLower(a.assetName(it))
			notes := strings.ToLower(it.Notes)
			if !strings.Contains(name, q) && !strings.Contains(notes, q) {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

// projectAssetsWaterfall lays out referenced project assets in a true
// aspect-ratio-preserving multi-column waterfall masonry grid respecting global
// caption/extension/rating/thumbBg/grayscale settings.
func (a *App) projectAssetsWaterfall(c *gui.Context, list []model.Asset, innerW float32) {
	if innerW < 320 {
		innerW = 320
	}
	const gap float32 = 12
	targetW := float32(a.cardSize)
	if targetW < 140 {
		targetW = 184
	}
	cols := int((innerW + gap) / (targetW + gap))
	if cols < 2 {
		cols = 2
	}
	if cols > 6 {
		cols = 6
	}
	colW := (innerW - gap*float32(cols-1)) / float32(cols)
	colHeights := make([]float32, cols)
	type colEntry struct {
		idx   int
		asset model.Asset
		imgH  float32
	}
	buckets := make([][]colEntry, cols)
	capH := float32(0)
	if a.showCardCaption {
		capH = 32
	}
	for i, item := range list {
		best := 0
		for j := 1; j < cols; j++ {
			if colHeights[j] < colHeights[best] {
				best = j
			}
		}
		aspect := float32(0.78)
		if item.Width > 0 && item.Height > 0 {
			aspect = float32(item.Height) / float32(item.Width)
			if aspect < 0.45 {
				aspect = 0.45
			} else if aspect > 2.1 {
				aspect = 2.1
			}
		}
		imgH := colW * aspect
		buckets[best] = append(buckets[best], colEntry{idx: i, asset: item, imgH: imgH})
		colHeights[best] += imgH + capH + gap
	}

	gui.Row(c.Key("proj-waterfall")).FillWidth().Gap(gap).AlignItems(gui.Start).Children(func() {
		for ci := 0; ci < cols; ci++ {
			gui.Column(c.Key(fmt.Sprintf("proj-wf-col-%d", ci))).Grow(1).Gap(gap).Children(func() {
				for _, entry := range buckets[ci] {
					a.projectAssetWaterfallCard(c, entry.idx, entry.asset, entry.imgH)
				}
			})
		}
	})
}

func (a *App) coloredTagChip(c *gui.Context, key string, tg model.Tag, active bool) gui.Element {
	btn := gui.ButtonBase(c.Key(key)).Padding(5, 11).Gap(6).Radius(radiusPill).AlignItems(gui.Center)
	targetBg := a.pal.surfaceMuted
	targetFg := a.pal.inkSecondary
	if active {
		targetBg = a.pal.controlOn
		targetFg = a.pal.controlGlyph
	}
	scale := float32(1)
	if btn.Pressed() {
		scale = 0.96
	} else if btn.Hovered() && !active {
		targetFg = a.pal.ink
		targetBg = a.pal.surfaceActive
	}
	bg := btn.AnimateColor("bg", targetBg, animFast)
	fg := btn.AnimateColor("fg", targetFg, animFast)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, springResponsive).Children(func() {
		if col, ok := a.effectiveTagColor(tg.ID); ok {
			gui.Box(c).Size(7, 7).Radius(radiusPill).Background(col)
		} else if active {
			gui.Icon(c, iconCheck).Size(11, 11)
		}
		gui.Text(c, tg.Name).FontSize(12).SingleLine()
	})
	return btn
}

// tintedTagChip renders a sleek translucent tinted tag badge with a delicate
// matching border and an optional inline remove ("×") button.
func (a *App) tintedTagChip(c *gui.Context, key string, tg model.Tag, onRemove func()) {
	bg := a.pal.surfaceMuted
	border := gui.RGBA(255, 255, 255, 0.14)
	if !a.dark {
		border = gui.RGBA(24, 24, 27, 0.16)
	}
	if col, ok := a.effectiveTagColor(tg.ID); ok {
		alphaBg := float32(0.17)
		if !a.dark {
			alphaBg = 0.12
		}
		bg = gui.RGBA(col.R, col.G, col.B, alphaBg)
		border = gui.RGBA(col.R, col.G, col.B, 0.45)
	}
	gui.Row(c.Key(key)).
		Padding(4, 9).
		Gap(6).
		Radius(8).
		Background(bg).
		Border(1, border).
		AlignItems(gui.Center).
		Children(func() {
			gui.Text(c, tg.Name).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine()
			if onRemove != nil {
				rmBtn := gui.ButtonBase(c.Key(key+"-rm")).
					Size(16, 16).
					Radius(4).
					Center().
					Tooltip(a.t("common.delete")).
					OnClick(onRemove)
				fg := a.pal.inkSecondary
				if rmBtn.Hovered() || rmBtn.Pressed() {
					fg = a.pal.ink
				}
				rmBtn.TextColor(fg).Children(func() {
					gui.Icon(c, iconX).Size(10, 10)
				})
			}
		})
}

// boardCard renders a live miniature preview of the board's actual canvas
// placements (images, rotations, text blocks, and dot grid) instead of a generic box.
func (a *App) boardCard(c *gui.Context, index int, board model.Board) {
	items := a.boardPlacements[board.ID]
	count := len(items)
	if count == 0 {
		count = a.boardCounts[board.ID]
	}

	card := gui.Column(c.Key("board-card-" + board.ID)).
		Width(236).
		Radius(radiusControl).
		Border(1, a.chromeHairline()).
		Clip()

	card.PopoverMenu(func(m *gui.Menu) {
		if a.menuItem(m, iconBoard, a.t("project.openBoard"), "", false).Chosen() {
			a.openBoard(board)
		}
		m.Separator()
		if a.menuItem(m, iconTrash, a.t("project.deleteBoard"), "Del", true).Chosen() {
			a.deleteBoardPrompt(board)
		}
	}, a.popoverMenuOpts())

	hovered := card.Hovered()
	targetBg := a.pal.surface
	liftY := float32(0)
	shadow := a.pal.shadowResting
	if hovered {
		targetBg = a.pal.surfaceMuted
		liftY = -2
		shadow = a.pal.shadowRaised
	}
	card.Background(card.AnimateColor("bg", targetBg, animFast)).
		Shadow(0, 4, 14, 0, shadow).
		AnimatePose(gui.Pose{Y: liftY}, springGentle)
	applyCardBlurFade(card, board.ID, time.Duration(min(index, 6))*arcStaggerItem)

	card.Children(func() {
		// Live miniature moodboard preview canvas (click anywhere to open board).
		preview := gui.ButtonBase(c.Key("open-b-" + board.ID)).
			FillWidth().
			Height(134).
			Clip().
			Background(a.pal.paper).
			Label(board.Name).
			OnClick(func() {
				a.openBoard(board)
			})

		preview.Draw(func(p *gui.Painter, r gui.Rect) {
			a.paintBoardMiniPreview(p, r, items)
		})

		preview.Children(func() {
			if len(items) == 0 {
				gui.Column(c).Fill().Center().Gap(5).Children(func() {
					gui.Icon(c, iconBoard).Size(18, 18).TextColor(a.pal.inkFaint)
					gui.Text(c, a.t("project.boardBlank")).FontSize(11).TextColor(a.pal.inkMuted)
				})
			}
		})

		// Sleek single-line footer with hover-revealed delete icon.
		gui.Row(c).
			FillWidth().
			Padding(8, 10).
			Gap(6).
			AlignItems(gui.Center).
			BorderWidth(1, 0, 0, 0).
			BorderColor(a.chromeHairline()).
			Children(func() {
				gui.Text(c, board.Name).FontSize(12.5).Bold().Grow(1).SingleLine().MinWidth(0)
				if a.showSidebarCounts {
					gui.Text(c, fmt.Sprintf("%d", count)).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
				}
				if hovered {
					a.miniHeaderButton(c, "del-b-"+board.ID, iconTrash, a.t("project.deleteBoard"), false).OnClick(func() {
						a.deleteBoardPrompt(board)
					})
				}
			})
	})
}

// paintBoardMiniPreview renders a scaled, auto-centered live thumbnail of all
// placements on a moodboard directly inside r.
func (a *App) paintBoardMiniPreview(p *gui.Painter, r gui.Rect, items []model.Placement) {
	p.Clip(r, 0, func() {
		// Subtle studio dot grid.
		dotCol := a.pal.paperGrid
		if dotCol.A > 0 {
			for y := float32(12); y < r.H; y += 16 {
				for x := float32(12); x < r.W; x += 16 {
					p.Fill(gui.Rect{X: r.X + x, Y: r.Y + y, W: 1.5, H: 1.5}, dotCol, 0.75)
				}
			}
		}
		if len(items) == 0 {
			return
		}

		byID := make(map[string]model.Placement, len(items))
		ordered := make([]model.Placement, 0, len(items))
		for _, it := range items {
			byID[it.ID] = it
			if it.Kind != model.KindGroup {
				ordered = append(ordered, it)
			}
		}
		if len(ordered) == 0 {
			return
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].ZIndex != ordered[j].ZIndex {
				return ordered[i].ZIndex < ordered[j].ZIndex
			}
			return ordered[i].ID < ordered[j].ID
		})

		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for _, it := range ordered {
			b := canvas.Bounds(byID, it.ID)
			if b.MinX < minX {
				minX = b.MinX
			}
			if b.MinY < minY {
				minY = b.MinY
			}
			if b.MaxX > maxX {
				maxX = b.MaxX
			}
			if b.MaxY > maxY {
				maxY = b.MaxY
			}
		}
		bw := maxX - minX
		bh := maxY - minY
		if bw < 1 {
			bw = 100
		}
		if bh < 1 {
			bh = 100
		}

		const pad float64 = 16
		availW := math.Max(10, float64(r.W)-pad*2)
		availH := math.Max(10, float64(r.H)-pad*2)
		fit := math.Min(availW/bw, availH/bh)
		if fit > 0.55 {
			fit = 0.55
		}
		midX := (minX + maxX) / 2
		midY := (minY + maxY) / 2
		rcx := float64(r.X) + float64(r.W)/2
		rcy := float64(r.Y) + float64(r.H)/2

		for _, it := range ordered {
			w := canvas.World(byID, it.ID)
			sc := w.Scale
			if sc == 0 {
				sc = 1
			}
			cx := float32(rcx + (w.X-midX)*fit)
			cy := float32(rcy + (w.Y-midY)*fit)
			dw := float32(it.Width * sc * fit)
			dh := float32(it.Height * sc * fit)
			if dw < 6 {
				dw = 6
			}
			if dh < 6 {
				dh = 6
			}
			p.Transform(gui.Rotation(float32(w.Rot)), cx, cy, func() {
				rect := gui.Rect{X: cx - dw/2, Y: cy - dh/2, W: dw, H: dh}
				switch it.Kind {
				case model.KindText:
					p.Fill(rect, gui.RGBA(a.ink.R, a.ink.G, a.ink.B, 0.14), 3)
					barW := rect.W * 0.65
					if barW > 4 {
						p.Fill(gui.Rect{X: rect.X + 4, Y: rect.Y + rect.H/2 - 1.5, W: barW, H: 3}, gui.RGBA(a.ink.R, a.ink.G, a.ink.B, 0.45), 1.5)
					}
				default:
					if bm := a.requestCardBitmap(a.assetByID[it.AssetID]); bm != nil {
						p.Image(bm, rect, gui.Contain)
					} else {
						p.Fill(rect, a.pal.surfaceMuted, 3)
					}
				}
			})
		}
	})
}

func (a *App) deleteBoardPrompt(board model.Board) {
	a.ask(a.t("project.deleteBoard"), a.t("project.deleteBoardNote"), a.t("common.delete"), func() {
		if a.boardID == board.ID {
			a.session = nil
			a.boardID = ""
		}
		if err := a.projects.DeleteBoard(board.ID); err != nil {
			a.status = err.Error()
			return
		}
		a.reload()
	})
}

func (a *App) projectAssetWaterfallCard(c *gui.Context, index int, item model.Asset, imgH float32) {
	services := c.Services()
	name := a.assetName(item)
	ext := a.assetExt(item)

	card := gui.Column(c.Key("pref-" + item.ID)).
		FillWidth().
		Radius(radiusControl).
		Clip()

	card.PopoverMenu(func(m *gui.Menu) {
		if u := normalizeWebURL(item.SourceURL); u != "" {
			if a.menuItem(m, iconLink, a.t("webimport.openPage"), "", false).Chosen() {
				services.OpenURL(u)
			}
		}
		if a.menuItem(m, iconImage, a.t("library.preview"), "Space", false).Chosen() {
			a.openPreview(item)
		}
		if !item.Missing && item.Path != "" {
			if a.menuItem(m, iconExternal, a.t("common.openFile"), "", false).Chosen() {
				_ = openLocalFile(item.Path)
			}
			if a.menuItem(m, iconFolder, a.t("common.revealFile"), "", false).Chosen() {
				_ = revealLocalFile(item.Path)
			}
		}
		if a.menuItem(m, iconCopy, a.t("common.copyPath"), "", false).Chosen() {
			services.WriteClipboard(item.Path)
			a.notify(a.t("common.copied"))
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
		if a.menuItem(m, iconContrast, grayMark+a.t("view.grayscale"), "Ctrl+Alt+G", false).Chosen() {
			a.grayscalePreview = !a.grayscalePreview
			a.syncConfigFromState()
		}
		m.Separator()
		if a.menuItem(m, iconTrash, a.t("project.unreference"), "Del", true).Chosen() {
			a.unreferencePrompt(item)
		}
	}, a.popoverMenuOpts())

	hovered := card.Hovered()
	targetBg := a.pal.surface
	liftY := float32(0)
	imgScale := float32(1)
	shadow := a.pal.shadowResting
	if hovered {
		targetBg = a.pal.surfaceMuted
		liftY = -3
		imgScale = 1.035
		shadow = a.pal.shadowRaised
	}
	card.Background(card.AnimateColor("bg", targetBg, animFast)).
		Shadow(0, 4, 14, 0, shadow).
		AnimatePose(gui.Pose{Y: liftY}, springGentle)
	applyCardBlurFade(card, item.ID, time.Duration(min(index, 6))*arcStaggerItem)

	card.Children(func() {
		thumb := gui.ButtonBase(c.Key("pref-open-"+item.ID)).
			FillWidth().
			Height(imgH).
			Clip().
			Center().
			OnClick(func() {
				if u := normalizeWebURL(item.SourceURL); item.IsBookmark && u != "" {
					services.OpenURL(u)
				} else {
					a.openPreview(item)
				}
			})
		a.applyThumbBg(thumb, a.pal.surfaceMuted, 10)
		thumb.Children(func() {
			if bm := a.requestCardBitmap(item); bm != nil {
				imgEl := gui.Image(c, bm).Fit(gui.Cover).Fill().AnimatePose(gui.Pose{Scale: imgScale}, springGentle)
				if a.grayscalePreview {
					imgEl.Grayscale()
				}
			} else {
				gui.Text(c, ext).FontSize(11).Bold().TextColor(a.pal.inkMuted)
			}
			if item.IsBookmark && item.SourceURL != "" {
				host := shortWebHost(item.SourceURL)
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
			if hovered {
				gui.ButtonBase(c.Key("unref-"+item.ID)).
					Absolute().
					Top(8).
					Right(8).
					Size(24, 24).
					Radius(radiusPill).
					Background(gui.RGBA(18, 18, 22, 0.78)).
					TextColor(gui.Hex("#FAFAFA")).
					Center().
					Tooltip(a.t("project.unreference")).
					OnClick(func() {
						a.unreferencePrompt(item)
					}).
					Children(func() {
						gui.Icon(c, iconX).Size(12, 12)
					})
			}
		})

		if a.showCardCaption {
			gui.Row(c).FillWidth().Gap(6).AlignItems(gui.Center).Padding(7, 10).Children(func() {
				gui.Text(c, name).Grow(1).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine().MinWidth(0)
				if a.showCardRating && item.Rating > 0 {
					gui.Text(c, fmt.Sprintf("★%d", item.Rating)).FontSize(10).TextColor(a.pal.inkSecondary).SingleLine()
				} else if a.showCardExt {
					gui.Text(c, ext).FontSize(10).TextColor(a.pal.inkMuted).SingleLine()
				}
			})
		}
	})
}

func (a *App) unreferencePrompt(item model.Asset) {
	a.ask(a.t("project.unreference"), a.t("project.unreferenceNote"), a.t("common.remove"), func() {
		a.dropAssetFromSession(item.ID)
		if err := a.projects.Unreference(a.projectID, item.ID); err != nil {
			a.status = err.Error()
			return
		}
		a.reload()
	})
}

func (a *App) createBoard() {
	name := trim(a.newBoardName)
	if name == "" {
		name = a.t("project.newBoard")
	}
	b, err := a.projects.CreateBoard(a.projectID, name)
	if err != nil {
		a.status = err.Error()
		return
	}
	a.newBoardName = ""
	a.openBoard(b)
}

func (a *App) dropAssetFromSession(assetID string) {
	if a.session == nil || a.boardProject != a.projectID {
		return
	}
	var drop []string
	for _, item := range a.session.Items {
		if item.Kind == model.KindImage && item.AssetID == assetID {
			drop = append(drop, item.ID)
		}
	}
	if len(drop) == 0 {
		return
	}
	a.session.Delete(drop)
	a.flushBoard(true)
}
