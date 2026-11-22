package ui

import (
	"fmt"
	"math"
	"sort"
	"time"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/canvas"
	"ragle/internal/model"
)

// projectView renders a calm Arc UI editorial layout:
// 1. Minimal frameless top bar (back button, draggable strip, window controls)
// 2. Editorial Hero block (eyebrow metadata, large bold display title, spacious
//    description on its own line, interactive colored tag pills, and single actions)
// 3. Live visual Moodboard preview cards + Referenced Assets grid.
func (a *App) projectView(c *gui.Context) {
	gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
		// Minimal frameless top chrome (transparent on canvas, zero visual bloat).
		gui.Row(c.Key("proj-chrome")).
			Height(44).
			Padding(6, 12, 0, 12).
			Gap(8).
			AlignItems(gui.Center).
			Children(func() {
				a.toolButton(c, "proj-back-lib", iconArrowLeft, a.t("shell.library"), false).OnClick(func() {
					a.handleNav("tag-all")
				})
				gui.Box(c).Grow(1).FillHeight().DragWindow()
				a.windowControls(c)
			})

		// Scrollable project content with Arc UI Editorial Hero.
		gui.Scroll(c.Key("proj-scroll")).Grow(1).FillWidth().Background(a.pal.canvas).Children(func() {
			col := gui.Column(c.Key("proj-view-" + a.projectID)).Padding(12, 26, 32, 18).Gap(28)
			applyPanelSwitch(col, a.projectID, 0, 6)
			col.Children(func() {
				if a.status != "" {
					gui.Row(c.Key("proj-status-toast")).
						Padding(6, 12).
						Radius(radiusControl).
						Background(a.pal.surface).
						AlignItems(gui.Center).
						Gap(8).
						Children(func() {
							gui.Text(c, a.status).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
							a.iconButton(c, "proj-status-dismiss", iconX, a.t("common.close"), false).OnClick(func() {
								a.status = ""
							})
						})
				}

				// Arc UI Editorial Project Hero (frameless surface, clear display scale).
				gui.Column(c.Key("proj-hero")).
					FillWidth().
					Padding(24, 28).
					Gap(14).
					Radius(radiusSurface).
					Background(a.pal.surface).
					Children(func() {
						// Eyebrow metadata + Primary & Secondary actions.
						gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
							a.pillBadge(c, a.t("project.heroBadge"), a.pal.inkSecondary, a.pal.surfaceMuted)
							gui.Text(c, fmt.Sprintf(a.t("project.summary"), len(a.boards), len(a.projectAssets))).
								FontSize(12).
								TextColor(a.pal.inkMuted)
							gui.Spacer(c)
							a.arcButton(c, "proj-from-lib", a.t("project.fromLibrary"), true).OnClick(func() {
								a.referenceMode = true
								a.screen = screenLibrary
							})
							a.toolButton(c, "proj-edit-btn", iconEdit, a.t("project.edit"), false).OnClick(func() {
								a.openEditProjectModal()
							})
							a.toolButton(c, "proj-share-btn", iconShare, a.t("share.send"), false).OnClick(func() {
								a.shareOpen = true
							})
							a.iconButton(c, "proj-del-btn", iconTrash, a.t("project.delete"), false).OnClick(func() {
								id := a.projectID
								a.ask(a.t("project.delete"), a.t("project.deleteNote"), a.t("common.delete"), func() {
									if a.backupBeforeDelete && a.lib != nil {
										_ = a.lib.DB.Backup()
									}
									if a.boardProject == id {
										a.session = nil
										a.boardID = ""
										a.boardProject = ""
									}
									if err := a.projects.Delete(id); err != nil {
										a.status = err.Error()
										return
									}
									a.projectID = ""
									a.project = model.Project{}
									a.screen = screenLibrary
									a.reload()
								})
							})
						})

						// Hero display title (design.md §4.3 scale).
						gui.Text(c, a.project.Name).
							FontSize(28).
							Bold().
							LetterSpacing(-0.5).
							LineHeight(1.18)

						// Independent description line with editorial breathing room.
						if a.project.Description != "" {
							gui.Text(c, a.project.Description).
								MaxWidth(720).
								FontSize(14).
								TextColor(a.pal.inkSecondary).
								LineHeight(1.6)
						}

						// Independent tag strip with optional tag colors.
						if len(a.tags) > 0 {
							gui.Row(c).FillWidth().Padding(4, 0, 0, 0).Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
								for _, tag := range a.tags {
									tg := tag
									isOn := a.projectTags[tg.ID]
									a.coloredTagChip(c, "ptag-"+tg.ID, tg, isOn).OnClick(func() {
										if err := a.projects.SetTag(a.projectID, tg.ID, !isOn); err != nil {
											a.status = err.Error()
											return
										}
										a.reload()
									})
								}
							})
						}
					})

				// Section 1: Moodboards with live miniature canvas previews.
				gui.Column(c).Gap(14).Children(func() {
					gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Text(c, a.t("project.boards")).FontSize(17).Bold().LetterSpacing(-0.2)
						a.pillBadge(c, fmt.Sprintf("%d", len(a.boards)), a.pal.inkSecondary, a.pal.surface)
						gui.Spacer(c)
						gui.TextInput(c.Key("new-board"), &a.newBoardName).
							Placeholder(a.t("project.newBoard")).
							Width(196).
							OnSubmit(func() { a.createBoard() })
						a.arcButton(c, "create-board-btn", "+ "+a.t("project.newBoard"), true).OnClick(func() {
							a.createBoard()
						})
					})

					if len(a.boards) == 0 {
						gui.Column(c).
							FillWidth().
							Padding(32).
							Gap(8).
							Radius(radiusPanel).
							Background(a.pal.surface).
							Center().
							Children(func() {
								gui.Icon(c, iconBoard).Size(24, 24).TextColor(a.pal.inkFaint)
								gui.Text(c, a.t("project.emptyBoards")).FontSize(14).Bold()
								gui.Text(c, a.t("project.emptyBoardsHint")).FontSize(12).TextColor(a.pal.inkSecondary)
							})
					} else {
						gui.Row(c).Gap(16).Wrap().Children(func() {
							for i, b := range a.boards {
								a.boardCard(c, i, b)
							}
						})
					}
				})

				// Section 2: Referenced Assets.
				gui.Column(c).Gap(14).Children(func() {
					gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Text(c, a.t("project.assets")).FontSize(17).Bold().LetterSpacing(-0.2)
						a.pillBadge(c, fmt.Sprintf("%d", len(a.projectAssets)), a.pal.inkSecondary, a.pal.surface)
					})

					if len(a.projectAssets) == 0 {
						gui.Column(c).
							FillWidth().
							Padding(32).
							Gap(8).
							Radius(radiusPanel).
							Background(a.pal.surface).
							Center().
							Children(func() {
								gui.Icon(c, iconImage).Size(24, 24).TextColor(a.pal.inkFaint)
								gui.Text(c, a.t("project.emptyAssets")).FontSize(14).Bold()
								gui.Text(c, a.t("project.emptyAssetsHint")).FontSize(12).TextColor(a.pal.inkSecondary)
							})
					} else {
						gui.Row(c).Gap(16).Wrap().Children(func() {
							for i, asset := range a.projectAssets {
								a.projectAssetCard(c, i, asset)
							}
						})
					}
				})
			})
		})
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

// boardCard renders a live miniature preview of the board's actual canvas
// placements (images, rotations, text blocks, and dot grid) instead of a generic box.
func (a *App) boardCard(c *gui.Context, index int, board model.Board) {
	items := a.boardPlacements[board.ID]
	count := len(items)
	if count == 0 {
		count = a.boardCounts[board.ID]
	}

	card := gui.Column(c.Key("board-card-" + board.ID)).
		Width(272).
		Radius(radiusPanel).
		Clip()

	card.PopoverMenu(func(m *gui.Menu) {
		if m.Item(a.t("project.openBoard")).Chosen() {
			a.openBoard(board)
		}
		m.Separator()
		if m.Item(a.t("project.deleteBoard")).Chosen() {
			a.deleteBoardPrompt(board)
		}
	})

	targetBg := a.pal.surface
	liftY := float32(0)
	shadow := a.pal.shadowResting
	if card.Hovered() {
		targetBg = a.pal.surfaceMuted
		liftY = -3
		shadow = a.pal.shadowRaised
	}
	card.Background(card.AnimateColor("bg", targetBg, animFast)).
		Shadow(0, 6, 18, 0, shadow).
		AnimatePose(gui.Pose{Y: liftY}, springGentle)
	applyCardBlurFade(card, board.ID, time.Duration(min(index, 6))*arcStaggerItem)

	card.Children(func() {
		// Live miniature moodboard preview canvas (click anywhere to open board).
		preview := gui.ButtonBase(c.Key("open-b-" + board.ID)).
			FillWidth().
			Height(158).
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
				gui.Column(c).Fill().Center().Gap(6).Children(func() {
					gui.Icon(c, iconBoard).Size(22, 22).TextColor(a.pal.inkFaint)
					gui.Text(c, a.t("project.boardBlank")).FontSize(11).TextColor(a.pal.inkMuted)
				})
			}
		})

		// Clean card footer with board title, placement count, and delete button.
		gui.Row(c).FillWidth().Padding(10, 12).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.Column(c).Grow(1).Gap(2).Children(func() {
				gui.Text(c, board.Name).FontSize(13).Bold().SingleLine()
				gui.Text(c, fmt.Sprintf(a.t("project.boardItems"), count)).FontSize(11).TextColor(a.pal.inkMuted)
			})
			a.iconButton(c, "del-b-"+board.ID, iconTrash, a.t("project.deleteBoard"), false).OnClick(func() {
				a.deleteBoardPrompt(board)
			})
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
					if bm := a.bitmaps[it.AssetID]; bm != nil {
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

func (a *App) projectAssetCard(c *gui.Context, index int, item model.Asset) {
	services := c.Services()
	name := a.assetName(item)

	card := gui.Column(c.Key("pref-" + item.ID)).
		Width(176).
		Radius(radiusPanel).
		Clip()

	card.PopoverMenu(func(m *gui.Menu) {
		if m.Item(a.t("library.preview")).Chosen() {
			a.openPreview(item)
		}
		if !item.Missing && item.Path != "" {
			if m.Item(a.t("common.openFile")).Chosen() {
				_ = openLocalFile(item.Path)
			}
			if m.Item(a.t("common.revealFile")).Chosen() {
				_ = revealLocalFile(item.Path)
			}
		}
		if m.Item(a.t("common.copyPath")).Chosen() {
			services.WriteClipboard(item.Path)
			a.status = a.t("common.copied")
		}
		m.Separator()
		if m.Item(a.t("project.unreference")).Chosen() {
			a.unreferencePrompt(item)
		}
	})

	hovered := card.Hovered()
	targetBg := a.pal.surface
	liftY := float32(0)
	imgScale := float32(1)
	shadow := a.pal.shadowResting
	if hovered {
		targetBg = a.pal.surfaceMuted
		liftY = -3
		imgScale = 1.04
		shadow = a.pal.shadowRaised
	}
	card.Background(card.AnimateColor("bg", targetBg, animFast)).
		Shadow(0, 4, 14, 0, shadow).
		AnimatePose(gui.Pose{Y: liftY}, springGentle)
	applyCardBlurFade(card, item.ID, time.Duration(min(index, 6))*arcStaggerItem)

	card.Children(func() {
		thumb := gui.ButtonBase(c.Key("pref-open-" + item.ID)).
			Size(176, 132).
			Clip().
			Background(a.pal.surfaceMuted).
			Center().
			OnClick(func() { a.openPreview(item) })
		thumb.Children(func() {
			if bm := a.bitmaps[item.ID]; bm != nil {
				gui.Image(c, bm).Fit(gui.Cover).Fill().AnimatePose(gui.Pose{Scale: imgScale}, springGentle)
			} else {
				gui.Text(c, a.assetExt(item)).FontSize(11).Bold().TextColor(a.pal.inkMuted)
			}
		})

		gui.Row(c).Gap(4).AlignItems(gui.Center).Padding(7, 10).Children(func() {
			gui.Text(c, name).Grow(1).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine()
			a.iconButton(c, "unref-"+item.ID, iconX, a.t("project.unreference"), false).OnClick(func() {
				a.unreferencePrompt(item)
			})
		})
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
