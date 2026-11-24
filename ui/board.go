package ui

import (
	"fmt"
	"math"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/canvas"
	"raglex/internal/model"
)

const (
	boardToolSelect    = 0
	boardToolPan       = 1
	boardToolText      = 2
	boardToolPen       = 3
	boardToolHighlight = 4
	boardToolEraser    = 5
	boardToolRect      = 6
	boardToolArrow     = 7
	boardToolFrame     = 8
)

var boardStudioColors = []string{
	"#FF5C33", // Raglex Orange
	"#FACC15", // Amber Yellow
	"#22C55E", // Emerald
	"#38BDF8", // Sky Cyan
	"#A855F7", // Violet
	"#FAFAFA", // Crisp White
	"#18181B", // Ink Charcoal
}

type gesture struct {
	kind       string
	ids        []string
	start      map[string]model.Placement
	x0, y0     float64
	wx0, wy0   float64
	mx1, my1   float64
	panX, panY float64
	cx, cy     float64
	startScale float64
	startRot   float64
	startAngle float64
	startDist  float64
	startWidth float64
	startLocal float64
	sign       float64
	pts        []float64 // live world-space x,y pairs while drawing pen/highlighter
}

func (a *App) boardView(c *gui.Context) {
	a.syncSelectionBuffers()
	gui.Column(c).Fill().Background(a.paper).Children(func() {
		a.boardToolbar(c)
		gui.Row(c).Grow(1).AlignItems(gui.Stretch).Children(func() {
			a.boardSurface(c)
			if a.boardInspectorOpen {
				a.boardInspector(c)
			}
		})
		if a.boardDrawerOpen {
			a.boardTray(c)
		}
	})
}

func (a *App) boardToolbar(c *gui.Context) {
	if a.focusMode {
		return
	}
	t := c.Theme()
	zoomPct := 100
	itemCount := 0
	if a.session != nil {
		if a.session.Zoom > 0 {
			zoomPct = int(math.Round(a.session.Zoom * 100))
		}
		itemCount = len(a.session.Items)
	}
	gui.Row(c.Key("board-floating-bar")).
		Height(48).
		FillWidth().
		Padding(0, 10, 0, 12).
		Gap(8).
		AlignItems(gui.Center).
		Background(a.pal.surface).
		BorderWidth(0, 0, 1, 0).
		BorderColor(a.chromeHairline()).
		Children(func() {
			if a.macTrafficLightSpacer(c) {
				gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
			}

			// Back & breadcrumb.
			leaveLbl := a.t("board.leave")
			if a.boardProject == "" {
				leaveLbl = a.t("board.leaveToLib")
			}
			a.toolButton(c, "board-leave", iconArrowLeft, leaveLbl, false).OnClick(func() {
				a.flushBoard(true)
				a.closeRefWindow()
				if a.boardAlwaysOnTop {
					a.toggleMainAlwaysOnTop()
				}
				a.session = nil
				a.boardID = ""
				if a.boardProject == "" {
					a.screen = screenLibrary
				} else {
					a.screen = screenProject
				}
				a.syncNavSelection()
			})
			gui.Row(c).Gap(8).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
				gui.Text(c, "/").TextColor(a.pal.inkFaint)
				gui.Text(c, a.boardName).Bold().FontSize(13).SingleLine().MaxWidth(160)
				a.pillBadge(c, fmt.Sprintf(a.t("board.placementCount"), itemCount), t.TextMuted, a.pal.surfaceMuted)
			})
			if a.boardProject == "" && len(a.projectList) > 0 {
				gui.PopoverMenuButton(c.Key("board-move-proj"), a.t("board.moveToProject"), func(m *gui.Menu) {
					for _, p := range a.projectList {
						proj := p
						if a.menuItem(m, iconFolder, proj.Name, "", false).Chosen() {
							a.moveBoardToProject(a.boardID, proj.ID)
						}
					}
				}, a.popoverMenuBtnOpts())
			}

			gui.Box(c).Grow(1).FillHeight().DragWindow()

			gui.Row(c).Gap(2).AlignItems(gui.Center).Children(func() {
				canUndo := a.session != nil && a.session.CanUndo()
				canRedo := a.session != nil && a.session.CanRedo()
				uBtn := a.iconButton(c, "board-undo", iconUndo, a.t("board.undo"), false).OnClick(func() { a.boardUndo() })
				if !canUndo {
					uBtn.Opacity(0.35)
				}
				rBtn := a.iconButton(c, "board-redo", iconRedo, a.t("board.redo"), false).OnClick(func() { a.boardRedo() })
				if !canRedo {
					rBtn.Opacity(0.35)
				}
				gui.Box(c).Width(1).Height(16).MarginX(4).Background(a.chromeHairline())
				a.iconButton(c, "board-grp", iconGroup, a.t("board.group"), false).OnClick(func() { a.groupSelection() })
				a.iconButton(c, "board-ungrp", iconUngroup, a.t("board.ungroup"), false).OnClick(func() { a.ungroupSelection() })
				a.toolButton(c, "board-front", nil, a.t("board.front"), false).OnClick(func() { a.nudgeZ(true) })
				a.toolButton(c, "board-back", nil, a.t("board.back"), false).OnClick(func() { a.nudgeZ(false) })
				a.iconButton(c, "board-del", iconTrash, a.t("board.delete"), false).OnClick(func() { a.confirmDeleteSelection() })
			})

			gui.Box(c).Grow(1).FillHeight().DragWindow()

			// Zoom controls.
			gui.Row(c).Gap(2).AlignItems(gui.Center).Children(func() {
				a.iconButton(c, "board-zout", iconZoomOut, "-", false).OnClick(func() {
					if a.session != nil {
						a.session.ZoomAt(400, 300, 1/1.15)
					}
				})
				a.toolButton(c, "board-zreset", nil, fmt.Sprintf("%d%%", zoomPct), false).
					Tooltip(a.t("board.zoomReset")).
					OnClick(func() {
						if a.session != nil {
							a.session.Zoom = 1
							a.session.PanX = 0
							a.session.PanY = 0
						}
					})
				a.iconButton(c, "board-zin", iconZoomIn, "+", false).OnClick(func() {
					if a.session != nil {
						a.session.ZoomAt(400, 300, 1.15)
					}
				})
			})

			a.toolButton(c, "board-refwin-btn", iconPin, a.t("refwin.open"), a.refWin != nil).OnClick(func() {
				if a.refWin != nil {
					a.closeRefWindow()
				} else {
					a.openRefWindow()
				}
			})
			a.iconButton(c, "board-pin-main", iconPin, a.t("refwin.pinWindow"), a.boardAlwaysOnTop).OnClick(func() {
				a.toggleMainAlwaysOnTop()
			})
			a.iconButton(c, "board-toggle-map", iconMap, a.t("board.minimap"), a.boardShowMinimap).OnClick(func() {
				a.boardShowMinimap = !a.boardShowMinimap
			})
			a.iconButton(c, "board-toggle-tray", iconPanelBottom, a.t("board.drawer"), a.boardDrawerOpen).OnClick(func() {
				a.boardDrawerOpen = !a.boardDrawerOpen
			})
			a.iconButton(c, "board-toggle-insp", iconPanelRight, a.t("board.inspector"), a.boardInspectorOpen).OnClick(func() {
				a.boardInspectorOpen = !a.boardInspectorOpen
			})

			gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
			a.topRightChrome(c)
		})
}

func (a *App) boardSurface(c *gui.Context) {
	cur := gui.CursorDefault
	switch a.tool {
	case boardToolPan:
		cur = gui.CursorGrab
	case boardToolPen, boardToolHighlight, boardToolRect, boardToolArrow, boardToolFrame, boardToolEraser:
		cur = gui.CursorCrosshair
	}
	stageWrap := gui.Box(c.Key("board-stage-wrap")).Grow(1).Clip().Background(a.paper)
	stageWrap.Children(func() {
		board := gui.Box(c.Key("board")).Fill().Focusable().Cursor(cur).HandleInput(a.boardInput).Draw(a.paintBoard)
		if a.focusBoard {
			board = board.Focus()
			a.focusBoard = false
		}
		board.Children(func() {})
		if drag, ok := gui.Drop[assetDrag](board); ok {
			if x, y, over := board.PointerPosition(); over {
				a.dropAsset(drag.ID, float64(x), float64(y))
			}
		}
		a.boardFloatingDock(c)
		if a.boardShowMinimap {
			a.boardMinimap(c, stageWrap)
		}
	})
}

// boardFloatingDock renders the studio floating bottom tool dock (Select, Hand,
// Pen, Highlighter, Eraser, Text, Rectangle, Arrow, Section Frame, Stroke Width,
// and Color Palette).
func (a *App) boardFloatingDock(c *gui.Context) {
	if a.boardStrokeColor == "" {
		a.boardStrokeColor = "#FF5C33"
	}
	if a.boardStrokeWidth <= 0 {
		a.boardStrokeWidth = 3.0
	}
	gui.Row(c.Key("board-dock-wrap")).
		Absolute().
		Bottom(14).
		Left(0).
		Right(0).
		Justify(gui.Center).
		Children(func() {
			gui.Row(c.Key("board-studio-dock")).
				Padding(5, 8).
				Gap(3).
				Radius(radiusPanel).
				Background(a.popoverSurface()).
				Border(1, a.chromeHairline()).
				Shadow(0, 12, 32, 0, a.pal.shadowFloating).
				AlignItems(gui.Center).
				Children(func() {
					tools := []struct {
						id    int
						icon  *gui.SVG
						label string
						tip   string
					}{
						{boardToolSelect, iconCursor, a.t("board.select"), a.t("board.select") + " (V)"},
						{boardToolPan, iconHand, a.t("board.pan"), a.t("board.pan") + " (H)"},
						{boardToolPen, iconPen, a.t("board.toolPen"), a.t("board.toolPen")},
						{boardToolHighlight, iconHighlighter, a.t("board.toolHighlight"), a.t("board.toolHighlight")},
						{boardToolEraser, iconEraser, a.t("board.toolEraser"), a.t("board.toolEraser")},
						{boardToolText, iconType, a.t("board.text"), a.t("board.text") + " (T)"},
						{boardToolRect, iconRect, a.t("board.toolRect"), a.t("board.toolRect")},
						{boardToolArrow, iconArrowDiag, a.t("board.toolArrow"), a.t("board.toolArrow")},
						{boardToolFrame, iconFrame, a.t("board.toolFrame"), a.t("board.toolFrame")},
					}
					for _, t := range tools {
						tl := t
						active := a.tool == tl.id
						btn := gui.ButtonBase(c.Key(fmt.Sprintf("bdock-tool-%d", tl.id))).
							Size(32, 32).
							Radius(radiusControl - 2).
							Center().
							Label(tl.label).
							Tooltip(tl.tip).
							OnClick(func() { a.tool = tl.id })
						bg := gui.Transparent
						fg := a.pal.inkSecondary
						if active {
							bg = a.pal.controlOn
							fg = a.pal.controlGlyph
						} else if btn.Hovered() {
							bg = a.pal.surfaceMuted
							fg = a.pal.ink
						}
						btn.Background(btn.AnimateColor("bg", bg, animFast)).
							TextColor(btn.AnimateColor("fg", fg, animFast)).
							Children(func() {
								gui.Icon(c, tl.icon).Size(15, 15)
							})
					}

					gui.Box(c).Size(1, 20).MarginX(4).Background(a.chromeHairline())

					// Stroke width cycle pill (2 / 4 / 8 px).
					wLbl := fmt.Sprintf("%.0fpx", a.boardStrokeWidth)
					a.toolButton(c, "bdock-width", nil, wLbl, false).
						Padding(4, 7).
						Tooltip(a.t("board.strokeWidth")).
						OnClick(func() {
							switch {
							case a.boardStrokeWidth < 3.5:
								a.boardStrokeWidth = 5
							case a.boardStrokeWidth < 7:
								a.boardStrokeWidth = 9
							default:
								a.boardStrokeWidth = 2.5
							}
						})

					// Studio annotation color swatches.
					gui.Row(c).Gap(4).Padding(0, 4).AlignItems(gui.Center).Children(func() {
						for _, hex := range boardStudioColors {
							hc := hex
							col, ok := parseHexColor(hc)
							if !ok {
								continue
							}
							sel := a.boardStrokeColor == hc
							sz := float32(16)
							if sel {
								sz = 19
							}
							sw := gui.ButtonBase(c.Key("bdock-col-"+hc)).
								Size(sz, sz).
								Radius(radiusPill).
								Background(col).
								Tooltip(hc).
								OnClick(func() {
									a.boardStrokeColor = hc
									// If a drawn shape/frame/stroke is selected, recolor it live!
									if a.session != nil {
										snap := a.session.Snapshot()
										changed := false
										for _, id := range a.session.SelectedIDs() {
											if it, ok := a.session.Items[id]; ok && it.Kind != model.KindImage && it.Kind != model.KindGroup {
												it.Color = hc
												a.session.Items[id] = it
												changed = true
											}
										}
										if changed {
											a.session.PushUndo(snap)
											a.markBoard()
											a.flushBoard(true)
										}
									}
								})
							if sel {
								sw.Border(2, a.pal.controlOn)
							} else {
								sw.Border(1, a.chromeHairline())
							}
						}
					})
				})
		})
}

// boardMinimap renders an interactive floating bottom-right overview map of all
// board items, section frames, and the live camera viewport rectangle.
func (a *App) boardMinimap(c *gui.Context, boardEl gui.Element) {
	if a.session == nil {
		return
	}
	const mw, mh float32 = 164, 108
	viewR := boardEl.LayoutBounds()

	computeMapTransform := func() (minX, minY, scale float64) {
		minX, minY = math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for id := range a.session.Items {
			b := canvas.Bounds(a.session.Items, id)
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
		if viewR.W > 0 && viewR.H > 0 {
			vb := a.session.ViewWorld(float64(viewR.W), float64(viewR.H))
			if vb.MinX < minX {
				minX = vb.MinX
			}
			if vb.MinY < minY {
				minY = vb.MinY
			}
			if vb.MaxX > maxX {
				maxX = vb.MaxX
			}
			if vb.MaxY > maxY {
				maxY = vb.MaxY
			}
		}
		if math.IsInf(minX, 1) {
			return -400, -300, 0.15
		}
		pad := 120.0
		minX -= pad
		minY -= pad
		maxX += pad
		maxY += pad
		bw := max(200.0, maxX-minX)
		bh := max(150.0, maxY-minY)
		s := math.Min(float64(mw-12)/bw, float64(mh-12)/bh)
		return minX, minY, s
	}

	panFromMini := func(mx, my float32) {
		if viewR.W <= 0 || viewR.H <= 0 {
			return
		}
		minX, minY, sc := computeMapTransform()
		if sc <= 0 {
			return
		}
		wx := minX + (float64(mx)-6)/sc
		wy := minY + (float64(my)-6)/sc
		z := a.session.Zoom
		if z <= 0 {
			z = 1
		}
		a.session.PanX = float64(viewR.W)/2 - wx*z
		a.session.PanY = float64(viewR.H)/2 - wy*z
	}

	miniBg := gui.RGBA(20, 20, 24, 0.92)
	if !a.dark {
		miniBg = gui.RGBA(255, 255, 255, 0.92)
	}
	mini := gui.Box(c.Key("board-minimap")).
		Absolute().
		Right(14).
		Bottom(14).
		Size(mw, mh).
		Radius(radiusControl).
		Clip().
		Background(miniBg).
		Border(1, a.chromeHairline()).
		Shadow(0, 10, 24, 0, a.pal.shadowFloating).
		Cursor(gui.CursorPointer).
		HandleInput(func(ev gui.InputEvent) bool {
			switch ev.Kind {
			case gui.InputPointerDown:
				if ev.Button != 0 {
					return false
				}
				a.boardMiniDrag = true
				panFromMini(ev.X, ev.Y)
				return true
			case gui.InputPointerMove:
				if !a.boardMiniDrag {
					return false
				}
				panFromMini(ev.X, ev.Y)
				return true
			case gui.InputPointerUp:
				if a.boardMiniDrag {
					a.boardMiniDrag = false
					return true
				}
			}
			return false
		})

	mini.Draw(func(p *gui.Painter, r gui.Rect) {
		minX, minY, sc := computeMapTransform()
		if sc <= 0 {
			return
		}
		toMini := func(wx, wy float64) (float32, float32) {
			return r.X + 6 + float32((wx-minX)*sc), r.Y + 6 + float32((wy-minY)*sc)
		}
		for id, it := range a.session.Items {
			if it.Kind == model.KindGroup {
				continue
			}
			b := canvas.Bounds(a.session.Items, id)
			x0, y0 := toMini(b.MinX, b.MinY)
			x1, y1 := toMini(b.MaxX, b.MaxY)
			ir := gui.Rect{X: x0, Y: y0, W: max(2, x1-x0), H: max(2, y1-y0)}
			switch it.Kind {
			case model.KindFrame:
				col := gui.Hex("#FF5C33")
				if c2, ok := parseHexColor(it.Color); ok {
					col = c2
				}
				p.Stroke(ir, col, 2, 1)
			case model.KindStroke, model.KindHighlight, model.KindRect, model.KindArrow:
				col := gui.Hex("#FF5C33")
				if c2, ok := parseHexColor(it.Color); ok {
					col = c2
				}
				p.Fill(ir, gui.RGBA(col.R, col.G, col.B, 0.75), 1)
			default:
				p.Fill(ir, gui.RGBA(250, 250, 250, 0.42), 1)
			}
		}
		if viewR.W > 0 && viewR.H > 0 {
			vb := a.session.ViewWorld(float64(viewR.W), float64(viewR.H))
			vx0, vy0 := toMini(vb.MinX, vb.MinY)
			vx1, vy1 := toMini(vb.MaxX, vb.MaxY)
			vr := gui.Rect{X: vx0, Y: vy0, W: max(8, vx1-vx0), H: max(6, vy1-vy0)}
			p.Fill(vr, gui.RGBA(255, 255, 255, 0.08), 2)
			p.Stroke(vr, gui.Hex("#FFFFFF"), 2, 1.2)
		}
	})
}

func (a *App) boardTray(c *gui.Context) {
	t := c.Theme()
	standalone := a.boardProject == ""
	list := a.projectAssets
	if standalone || a.tray == 1 {
		list = a.allAssets
	}
	trays := []string{a.t("board.trayProject"), a.t("board.trayLibrary")}
	gui.Column(c.Key("board-tray-panel")).
		Height(130).
		FillWidth().
		Clip().
		Background(a.pal.surface).
		BorderWidth(1, 0, 0, 0).
		BorderColor(a.chromeHairline()).
		Transition(gui.ElementTransition{
			Animation: &springSmooth,
			Enter:     &gui.Motion{Y: 20, Opacity: 0, Collapse: true},
			Exit:      &gui.Motion{Y: 20, Opacity: 0, Collapse: true},
		}).
		Children(func() {
				gui.Row(c).Padding(8, 14, 4, 14).Gap(10).AlignItems(gui.Center).Children(func() {
					if standalone {
						gui.Text(c, a.t("board.trayLibrary")).FontSize(12).Bold().TextColor(a.pal.ink)
					} else {
						a.arcPillTabs(c, "board-tray-tabs", a.tray, trays, func(idx int) {
							a.tray = idx
						})
					}
					gui.Text(c, a.t("board.clickToPlace")).FontSize(11).TextColor(a.pal.inkMuted)
					gui.Spacer(c)
					gui.Text(c, fmt.Sprintf(a.t("common.itemsCount"), len(list))).FontSize(11).TextColor(t.TextMuted)
				})
				gui.ScrollHorizontal(c.Key("board-tray-scroll")).Grow(1).FillWidth().Children(func() {
					gui.Row(c).Padding(4, 14, 10, 14).Gap(10).AlignItems(gui.Center).Children(func() {
						if len(list) == 0 {
							gui.Text(c, a.t("board.empty")).FontSize(12).TextColor(t.TextMuted)
							return
						}
						for _, asset := range list {
							item := asset
							name := a.assetName(item)
							card := gui.ButtonBase(c.Key("tray-" + item.ID)).
								Column().
								Width(98).
								Height(78).
								Padding(6).
								Gap(4).
								Radius(radiusControl).
								Drag(assetDrag{ID: item.ID}).
								Label(name).
								Tooltip(name).
								OnClick(func() {
									a.dropAsset(item.ID, 420, 260)
								})
							bg := a.pal.surface
							scale := float32(1)
							if card.Dragging() {
								scale = 0.54
								card.Opacity(0.28)
							} else if card.Pressed() {
								scale = 0.95
								bg = a.pal.surfaceMuted
							} else if card.Hovered() {
								bg = a.pal.surfaceMuted
							}
							card.Background(card.AnimateColor("bg", bg, animFast)).
								AnimatePose(gui.Pose{Scale: scale}, springResponsive)
							card.Children(func() {
								gui.Box(c).Grow(1).FillWidth().Center().Clip().Children(func() {
									if bm := a.requestCardBitmap(item); bm != nil {
										gui.Image(c, bm).Fit(gui.Contain).Grow(1).Radius(6)
									} else {
										gui.Text(c, a.assetExt(item)).FontSize(10).Bold().TextColor(a.pal.inkMuted)
									}
								})
								gui.Text(c, name).FontSize(10).TextColor(t.TextMuted).SingleLine().FillWidth()
							})
						}
					})
				})
			})
}

func (a *App) boardInspector(c *gui.Context) {
	t := c.Theme()
	gui.Column(c.Key("board-insp-panel")).
		Width(272).
		FillHeight().
		Clip().
		Background(a.pal.surface).
		BorderWidth(0, 0, 0, 1).
		BorderColor(a.chromeHairline()).
		Transition(gui.ElementTransition{
			Animation: &springSmooth,
			Enter:     &gui.Motion{X: 20, Opacity: 0},
		}).
		Children(func() {
				gui.Scroll(c).Fill().Children(func() {
					gui.Column(c).Padding(16).Gap(14).Children(func() {
						if a.session == nil {
							return
						}
						ids := a.session.SelectedIDs()
						if len(ids) == 0 {
							gui.Column(c).Padding(24, 8).Center().Gap(10).Children(func() {
								gui.Box(c).Size(44, 44).Radius(radiusControl).Background(a.pal.surface).Center().Children(func() {
									gui.Icon(c, iconBoard).Size(20, 20).TextColor(a.pal.inkMuted)
								})
								gui.Text(c, a.t("board.empty")).FontSize(12).TextColor(t.TextMuted).TextAlign(gui.Center).LineHeight(1.55)
							})
							return
						}
						if len(ids) != 1 {
							gui.Text(c, a.t("inspector.multi")).FontSize(13).Bold()
							gui.Row(c).Gap(6).Children(func() {
								gui.Button(c.Key("binsp-grp"), a.t("board.group")).Grow(1).Radius(radiusControl).OnClick(func() { a.groupSelection() })
								gui.Button(c.Key("binsp-del"), a.t("board.delete")).Grow(1).Radius(radiusControl).TextColor(t.Danger).OnClick(func() { a.confirmDeleteSelection() })
							})
							return
						}
						item := a.session.Items[ids[0]]

						gui.Column(c).Gap(6).Children(func() {
							a.sectionLabel(c, a.t("board.notes"))
							gui.TextInput(c.Key("notes"), &a.notesBuf).FillWidth().OnChange(func() {
								if a.session == nil || a.boundID == "" {
									return
								}
								a.session.SetNotes(a.boundID, a.notesBuf)
								a.markBoard()
							})
						})

						if item.Kind == model.KindText || item.Kind == model.KindFrame {
							gui.Column(c).Gap(6).Children(func() {
								lbl := a.t("board.body")
								if item.Kind == model.KindFrame {
									lbl = a.t("board.toolFrame")
								}
								a.sectionLabel(c, lbl)
								var field gui.Element
								if item.Kind == model.KindText {
									field = gui.TextArea(c.Key("body"), &a.editText).
										FillWidth().
										Height(96).
										OnChange(func() {
											a.commitEditText(c)
										})
								} else {
									field = gui.TextInput(c.Key("body"), &a.editText).FillWidth().OnChange(func() {
										a.commitEditText(c)
									})
								}
								if a.focusBody {
									field.Focus()
									a.focusBody = false
								}
							})
						}
						if item.Kind == model.KindText {
							gui.Column(c).Gap(6).Children(func() {
								a.sectionLabel(c, fmt.Sprintf("%s (%.0fpx)", a.t("board.fontSize"), a.fontSizeBuf))
								gui.Slider(c.Key("board-fsize"), &a.fontSizeBuf, 10, 72).FillWidth().OnChange(func() {
									if a.session == nil || a.boundID == "" {
										return
									}
									cur := a.session.Items[a.boundID]
									cur.FontSize = a.fontSizeBuf
									a.fitBoardTextHeight(c, &cur)
									a.session.Items[a.boundID] = cur
									a.markBoard()
								})
							})
						}
						if item.Kind == model.KindText || item.Kind == model.KindRect || item.Kind == model.KindArrow || item.Kind == model.KindFrame || item.Kind == model.KindStroke || item.Kind == model.KindHighlight {
							gui.Column(c).Gap(8).Padding(12).Radius(radiusControl).Background(a.pal.surfaceRaised).Border(1, a.chromeHairline()).Children(func() {
								a.sectionLabel(c, a.t("board.strokeColor"))
								gui.Row(c).Gap(5).Wrap().AlignItems(gui.Center).Children(func() {
									for _, hex := range boardStudioColors {
										hc := hex
										col, ok := parseHexColor(hc)
										if !ok {
											continue
										}
										sel := item.Color == hc
										sw := gui.ButtonBase(c.Key("binsp-col-"+hc)).
											Size(18, 18).
											Radius(radiusPill).
											Background(col).
											Tooltip(hc).
											OnClick(func() {
												a.session.RecordUndo()
												cur := a.session.Items[item.ID]
												cur.Color = hc
												a.session.Items[item.ID] = cur
												a.boardStrokeColor = hc
												a.markBoard()
												a.flushBoard(true)
											})
										if sel {
											sw.Border(2, a.pal.controlOn)
										} else {
											sw.Border(1, a.chromeHairline())
										}
									}
								})
								if item.Kind != model.KindText {
									a.sectionLabel(c, a.t("board.strokeWidth"))
									gui.Row(c).Gap(4).Children(func() {
										for _, wOpt := range []struct {
											lbl string
											val float64
										}{{"2px", 2}, {"4px", 4}, {"8px", 8}} {
											wo := wOpt
											active := math.Abs(item.StrokeWidth-wo.val) < 1.1
											a.toolButton(c, "binsp-sw-"+wo.lbl, nil, wo.lbl, active).Grow(1).Center().Padding(4, 6).OnClick(func() {
												a.session.RecordUndo()
												cur := a.session.Items[item.ID]
												cur.StrokeWidth = wo.val
												a.session.Items[item.ID] = cur
												a.boardStrokeWidth = wo.val
												a.markBoard()
												a.flushBoard(true)
											})
										}
									})
								}
								if item.Kind == model.KindRect || item.Kind == model.KindArrow || item.Kind == model.KindFrame {
									gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
										gui.Text(c, "虛線 / Dashed").FontSize(12).TextColor(a.pal.ink).Grow(1)
										dashed := item.Dashed
										a.arcSwitch(c, "binsp-dashed-"+item.ID, &dashed, func() {
											a.session.RecordUndo()
											cur := a.session.Items[item.ID]
											cur.Dashed = dashed
											a.session.Items[item.ID] = cur
											a.markBoard()
											a.flushBoard(true)
										})
									})
								}
							})
						}

						gui.Column(c).Gap(6).Padding(12).Radius(radiusControl).Background(a.pal.surface).Children(func() {
							a.sectionLabel(c, a.t("board.transform"))
							a.metaRow(c, "X / Y", fmt.Sprintf("%.0f, %.0f", item.X, item.Y))
							scale := item.Scale
							if scale == 0 {
								scale = 1
							}
							a.metaRow(c, "Scale", fmt.Sprintf("%.0f%%", scale*100))
							a.metaRow(c, "Rotation", fmt.Sprintf("%.1f°", item.Rotation))
							a.metaRow(c, "Layer", fmt.Sprintf("Z %d", item.ZIndex))
						})

						gui.Button(c.Key("binsp-remove"), a.t("board.delete")).
							FillWidth().
							Radius(radiusControl).
							TextColor(t.Danger).
							OnClick(func() { a.confirmDeleteSelection() })
					})
				})
			})
}

func (a *App) syncSelectionBuffers() {
	if a.session == nil {
		a.boundID = ""
		return
	}
	ids := a.session.SelectedIDs()
	if len(ids) != 1 {
		a.boundID = ""
		return
	}
	if ids[0] == a.boundID {
		return
	}
	a.boundID = ids[0]
	item := a.session.Items[ids[0]]
	a.notesBuf = item.Notes
	a.editText = item.Text
	if item.FontSize > 0 {
		a.fontSizeBuf = item.FontSize
	} else {
		a.fontSizeBuf = 18
	}
}

func (a *App) commitEditText(c *gui.Context) {
	if a.session == nil || a.boundID == "" {
		return
	}
	item, ok := a.session.Items[a.boundID]
	if !ok || item.Text == a.editText {
		return
	}
	a.session.SetText(a.boundID, a.editText)
	if cur, exists := a.session.Items[a.boundID]; exists && cur.Kind == model.KindText {
		a.fitBoardTextHeight(c, &cur)
		a.session.Items[a.boundID] = cur
	}
	a.markBoard()
}

// fitBoardTextHeight grows a text box's height automatically when wrapped or
// multiline text exceeds its current vertical bounds.
func (a *App) fitBoardTextHeight(c *gui.Context, cur *model.Placement) {
	if c == nil || cur == nil || cur.Kind != model.KindText || cur.Text == "" {
		return
	}
	fs := cur.FontSize
	if fs <= 0 {
		fs = 18
	}
	maxW := float32(cur.Width - 12)
	if maxW < 24 {
		maxW = 24
	}
	_, h := c.MeasureText(maxW, gui.Span{Text: cur.Text, Size: float32(fs)})
	needH := float64(h) + 14
	if needH > cur.Height {
		cur.Height = needH
	}
}

// paintWrappedBoardText draws multiline, auto-wrapping text cleanly clipped
// inside a board text item's bounding rect.
func paintWrappedBoardText(p *gui.Painter, rect gui.Rect, pad float32, text string, fontSize float32, col gui.Color) {
	if text == "" || rect.W <= 2 || rect.H <= 2 {
		return
	}
	if fontSize < 1 {
		fontSize = 1
	}
	if pad < 2 {
		pad = 2
	}
	if pad > 16 {
		pad = 16
	}
	maxW := rect.W - pad*2
	if maxW < 4 {
		maxW = 4
	}
	p.Clip(rect, 0, func() {
		p.RichText(rect.X+pad, rect.Y+pad, maxW, gui.Span{
			Text:  text,
			Size:  fontSize,
			Color: col,
		})
	})
}

func (a *App) markBoard() {
	if a.session == nil {
		return
	}
	a.session.Touch(time.Now())
	a.armFlush()
}

func (a *App) armFlush() {
	if a.flushArmed || a.win == nil || a.session == nil || !a.session.Dirty() {
		return
	}
	a.flushArmed = true
	win := a.win
	go func() {
		time.Sleep(canvas.FlushDelay)
		win.Update(func() {
			a.flushArmed = false
			a.mu.Lock()
			closed := a.closed
			a.mu.Unlock()
			if closed {
				return
			}
			a.flushBoard(false)
			if a.session != nil && a.session.Dirty() {
				a.armFlush()
			}
		})
	}()
}

func (a *App) flushBoard(force bool) {
	if a.session == nil || a.projects == nil || a.boardID == "" {
		return
	}
	items, ok := a.session.Flush(time.Now(), force)
	if !ok {
		return
	}
	if err := a.projects.SavePlacements(a.boardID, items); err != nil {
		a.status = err.Error()
		a.session.Touch(time.Now())
		a.armFlush()
	}
}

func (a *App) boardUndo() {
	if a.session == nil || !a.session.Undo() {
		return
	}
	a.boundID = ""
	a.gesture = gesture{}
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) boardRedo() {
	if a.session == nil || !a.session.Redo() {
		return
	}
	a.boundID = ""
	a.gesture = gesture{}
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) groupSelection() {
	if a.session == nil {
		return
	}
	snap := a.session.Snapshot()
	if _, err := a.session.Group(a.session.SelectedIDs()); err != nil {
		a.status = a.t("board.needTwo")
		return
	}
	a.session.PushUndo(snap)
	a.boundID = ""
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) ungroupSelection() {
	if a.session == nil {
		return
	}
	a.session.RecordUndo()
	a.session.Ungroup(a.session.SelectedIDs())
	a.boundID = ""
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) nudgeZ(front bool) {
	if a.session == nil {
		return
	}
	ids := a.session.SelectedIDs()
	if len(ids) != 1 {
		return
	}
	a.session.RecordUndo()
	if front {
		a.session.BringFront(ids[0])
	} else {
		a.session.BringBack(ids[0])
	}
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) deleteBoardSelectionInstant() {
	if a.session == nil || len(a.session.SelectedIDs()) == 0 {
		return
	}
	a.session.RecordUndo()
	a.session.Delete(a.session.SelectedIDs())
	a.boundID = ""
	a.gesture = gesture{}
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) confirmDeleteSelection() {
	if a.session == nil || len(a.session.SelectedIDs()) == 0 {
		return
	}
	a.ask(a.t("board.delete"), a.t("board.deleteNote"), a.t("common.remove"), func() {
		if a.session == nil {
			return
		}
		a.session.RecordUndo()
		a.session.Delete(a.session.SelectedIDs())
		a.boundID = ""
		a.markBoard()
		a.flushBoard(true)
	})
}

func (a *App) dropAsset(id string, x, y float64) {
	if a.session == nil {
		return
	}
	asset, ok := a.assetByID[id]
	if !ok && a.lib != nil {
		loaded, err := a.lib.Get(id)
		if err != nil {
			a.status = err.Error()
			return
		}
		asset = loaded
		a.assetByID[id] = asset
	}
	if asset.ID == "" {
		return
	}
	wx, wy := a.session.ScreenToWorld(x, y)
	a.session.RecordUndo()
	a.session.PlaceImage(asset.ID, wx, wy, float64(asset.Width), float64(asset.Height))
	if a.projectID != "" && a.projects != nil {
		_ = a.projects.Reference(a.projectID, asset.ID)
	}
	a.boundID = ""
	a.markBoard()
	a.flushBoard(true)
	a.reload()
}

func (a *App) boardInput(ev gui.InputEvent) bool {
	s := a.session
	if s == nil {
		return false
	}
	switch ev.Kind {
	case gui.InputKeyUp:
		if ev.Key == gui.KeySpace {
			a.spaceDown = false
			return true
		}
		return false
	case gui.InputKeyDown:
		return a.boardKey(ev)
	case gui.InputScroll:
		factor := 1.0
		if ev.Precise {
			factor = math.Exp(float64(-ev.DY) * 0.01)
		} else if ev.DY < 0 {
			factor = 1.1
		} else if ev.DY > 0 {
			factor = 1 / 1.1
		}
		s.ZoomAt(float64(ev.X), float64(ev.Y), factor)
		return true
	case gui.InputPointerDown:
		return a.pointerDown(ev)
	case gui.InputPointerMove:
		return a.pointerMove(ev)
	case gui.InputPointerUp:
		return a.pointerUp(ev)
	default:
		return false
	}
}

func (a *App) boardKey(ev gui.InputEvent) bool {
	s := a.session
	if ev.Key == gui.KeySpace {
		a.spaceDown = true
		return true
	}
	if ev.Repeat {
		return false
	}
	ctrl := ev.Mods&(gui.Ctrl|gui.Cmd) != 0
	shift := ev.Mods&gui.Shift != 0
	switch ev.Key {
	case gui.KeyZ:
		if ctrl {
			if shift {
				a.boardRedo()
			} else {
				a.boardUndo()
			}
			return true
		}
	case gui.KeyY:
		if ctrl {
			a.boardRedo()
			return true
		}
	case gui.KeyV:
		if !ctrl {
			a.tool = boardToolSelect
			return true
		}
	case gui.KeyH:
		if !ctrl {
			a.tool = boardToolPan
			return true
		}
	case gui.KeyT:
		if !ctrl {
			a.tool = boardToolText
			return true
		}
	case gui.KeyP:
		if !ctrl {
			a.tool = boardToolPen
			return true
		}
	case gui.KeyM:
		if !ctrl {
			a.tool = boardToolHighlight
			return true
		}
	case gui.KeyE:
		if !ctrl {
			a.tool = boardToolEraser
			return true
		}
	case gui.KeyR:
		if !ctrl {
			a.tool = boardToolRect
			return true
		}
	case gui.KeyA:
		if !ctrl {
			a.tool = boardToolArrow
			return true
		}
	case gui.KeyF:
		if !ctrl {
			a.tool = boardToolFrame
			return true
		}
	case gui.KeyG:
		if !ctrl {
			return false
		}
		if shift {
			a.ungroupSelection()
		} else {
			a.groupSelection()
		}
		return true
	case gui.KeyDelete, gui.KeyBackspace:
		a.deleteBoardSelectionInstant()
		return true
	case gui.KeyBracketRight:
		a.nudgeZ(true)
		return true
	case gui.KeyBracketLeft:
		a.nudgeZ(false)
		return true
	case gui.KeyEscape:
		s.Select("", false)
		a.boundID = ""
		a.gesture = gesture{}
		return true
	}
	return false
}

func (a *App) pointerDown(ev gui.InputEvent) bool {
	s := a.session
	wx, wy := s.ScreenToWorld(float64(ev.X), float64(ev.Y))
	if ev.Button == 2 || a.spaceDown || (a.tool == 1 && ev.Button == 0) {
		a.gesture = gesture{kind: "pan", x0: float64(ev.X), y0: float64(ev.Y), panX: s.PanX, panY: s.PanY}
		return true
	}
	if ev.Button != 0 {
		return false
	}
	switch a.tool {
	case boardToolPen, boardToolHighlight:
		kind := "draw-pen"
		if a.tool == boardToolHighlight {
			kind = "draw-hl"
		}
		a.gesture = gesture{kind: kind, wx0: wx, wy0: wy, mx1: wx, my1: wy, pts: []float64{wx, wy}}
		return true
	case boardToolEraser:
		a.gesture = gesture{kind: "erase", start: s.Snapshot(), wx0: wx, wy0: wy}
		a.eraseAnnotationAt(wx, wy)
		return true
	case boardToolRect:
		a.gesture = gesture{kind: "draw-rect", wx0: wx, wy0: wy, mx1: wx, my1: wy}
		return true
	case boardToolArrow:
		a.gesture = gesture{kind: "draw-arrow", wx0: wx, wy0: wy, mx1: wx, my1: wy}
		return true
	case boardToolFrame:
		a.gesture = gesture{kind: "draw-frame", wx0: wx, wy0: wy, mx1: wx, my1: wy}
		return true
	case boardToolText:
		if id, part, ok := s.Hit(wx, wy); ok && part == "body" && s.Items[id].Kind == model.KindText {
			s.Select(id, false)
			a.boundID = ""
			a.focusBody = true
			return true
		}
		if _, _, ok := s.Hit(wx, wy); !ok {
			s.RecordUndo()
			s.PlaceText(a.t("board.placeholder"), wx, wy)
			a.boundID = ""
			a.focusBody = true
			a.tool = boardToolSelect
			a.markBoard()
			a.flushBoard(true)
			return true
		}
	}
	id, part, ok := s.Hit(wx, wy)
	if !ok {
		s.Select("", false)
		a.boundID = ""
		a.gesture = gesture{kind: "marquee", wx0: wx, wy0: wy, mx1: wx, my1: wy}
		return true
	}
	if part == "arrow-p0" || part == "arrow-p1" || part == "resize-tl" || part == "resize-tr" || part == "resize-br" || part == "resize-bl" {
		a.gesture = gesture{
			kind: part, ids: []string{id}, start: s.Snapshot(),
			wx0: wx, wy0: wy,
		}
		return true
	}
	if part == "scale" || part == "rotate" || part == "width" {
		item := s.Items[id]
		w := canvas.World(s.Items, id)
		g := gesture{
			kind: part, ids: []string{id}, start: s.Snapshot(),
			wx0: wx, wy0: wy, cx: w.X, cy: w.Y,
			startScale: item.ScaleOrOne(), startRot: item.Rotation, startWidth: item.Width,
		}
		g.startDist = math.Hypot(wx-w.X, wy-w.Y)
		g.startAngle = math.Atan2(wy-w.Y, wx-w.X) * 180 / math.Pi
		lx, _ := canvas.WorldToLocal(s.Items, id, wx, wy)
		g.startLocal = lx
		g.sign = 1
		if lx < 0 {
			g.sign = -1
		}
		a.gesture = g
		return true
	}
	shift := ev.Mods&gui.Shift != 0
	if shift {
		s.Select(id, true)
	} else if !s.Selected[id] {
		s.Select(id, false)
	}
	if ev.Clicks >= 2 && (s.Items[id].Kind == model.KindText || s.Items[id].Kind == model.KindFrame) {
		a.boundID = ""
		a.focusBody = true
		return true
	}
	// Section Frame behavior: moving a Section Frame moves all items inside it together.
	moveIDs := s.SelectedIDs()
	seenMove := map[string]bool{}
	for _, mid := range moveIDs {
		seenMove[mid] = true
	}
	for _, mid := range s.SelectedIDs() {
		if s.Items[mid].Kind == model.KindFrame {
			for _, childID := range s.FrameContainedIDs(mid) {
				if !seenMove[childID] {
					seenMove[childID] = true
					moveIDs = append(moveIDs, childID)
				}
			}
		}
	}
	a.gesture = gesture{
		kind: "move", ids: moveIDs, start: s.Snapshot(),
		wx0: wx, wy0: wy,
	}
	return true
}

func snapAngle15(x0, y0, x1, y1 float64) (float64, float64) {
	dx, dy := x1-x0, y1-y0
	d := math.Hypot(dx, dy)
	if d < 1 {
		return x1, y1
	}
	step := math.Pi / 12.0 // 15 degrees
	ang := math.Round(math.Atan2(dy, dx)/step) * step
	return x0 + d*math.Cos(ang), y0 + d*math.Sin(ang)
}

func (a *App) pointerMove(ev gui.InputEvent) bool {
	s := a.session
	g := a.gesture
	if g.kind == "" {
		return false
	}
	wx, wy := s.ScreenToWorld(float64(ev.X), float64(ev.Y))
	shift := ev.Mods&gui.Shift != 0
	switch g.kind {
	case "pan":
		s.PanX = g.panX + (float64(ev.X) - g.x0)
		s.PanY = g.panY + (float64(ev.Y) - g.y0)
	case "marquee":
		a.gesture.mx1 = wx
		a.gesture.my1 = wy
	case "draw-pen", "draw-hl":
		pts := a.gesture.pts
		if len(pts) >= 2 {
			lx, ly := pts[len(pts)-2], pts[len(pts)-1]
			if math.Hypot(wx-lx, wy-ly) >= 2.5 {
				a.gesture.pts = append(pts, wx, wy)
			}
		} else {
			a.gesture.pts = append(pts, wx, wy)
		}
		a.gesture.mx1 = wx
		a.gesture.my1 = wy
	case "draw-arrow":
		if shift {
			wx, wy = snapAngle15(g.wx0, g.wy0, wx, wy)
		}
		a.gesture.mx1 = wx
		a.gesture.my1 = wy
	case "draw-rect", "draw-frame":
		a.gesture.mx1 = wx
		a.gesture.my1 = wy
	case "arrow-p0", "arrow-p1":
		if len(g.ids) == 1 {
			idx := 0
			if g.kind == "arrow-p1" {
				idx = 1
			}
			if shift {
				if it, ok := s.Items[g.ids[0]]; ok && len(it.Points) >= 4 {
					ax, ay := it.X+it.Points[2], it.Y+it.Points[3]
					if idx == 1 {
						ax, ay = it.X+it.Points[0], it.Y+it.Points[1]
					}
					wx, wy = snapAngle15(ax, ay, wx, wy)
				}
			}
			s.SetArrowEndpoint(g.ids[0], idx, wx, wy)
			a.markBoard()
		}
	case "resize-tl", "resize-tr", "resize-br", "resize-bl":
		if len(g.ids) == 1 {
			s.ResizeCorner(g.start, g.ids[0], g.kind, wx, wy)
			a.markBoard()
		}
	case "erase":
		a.eraseAnnotationAt(wx, wy)
	case "move":
		s.Move(g.start, g.ids, wx-g.wx0, wy-g.wy0)
		a.markBoard()
	case "scale":
		cur := math.Hypot(wx-g.cx, wy-g.cy)
		if g.startDist > 0.5 && len(g.ids) == 1 {
			s.SetScale(g.ids[0], g.startScale*cur/g.startDist)
			a.markBoard()
		}
	case "rotate":
		if len(g.ids) == 1 {
			ang := math.Atan2(wy-g.cy, wx-g.cx) * 180 / math.Pi
			s.SetRotation(g.ids[0], g.startRot+(ang-g.startAngle))
			a.markBoard()
		}
	case "width":
		if len(g.ids) == 1 {
			lx, _ := canvas.WorldToLocal(s.Items, g.ids[0], wx, wy)
			s.SetWidth(g.ids[0], g.startWidth+g.sign*(lx-g.startLocal)*2)
			a.markBoard()
		}
	}
	return true
}

func (a *App) pointerUp(ev gui.InputEvent) bool {
	s := a.session
	g := a.gesture
	switch g.kind {
	case "marquee":
		a.applyMarquee(ev.Mods&gui.Shift != 0)
	case "draw-pen", "draw-hl":
		if s != nil && len(g.pts) >= 4 {
			minX, minY := g.pts[0], g.pts[1]
			maxX, maxY := minX, minY
			for i := 2; i+1 < len(g.pts); i += 2 {
				if g.pts[i] < minX {
					minX = g.pts[i]
				}
				if g.pts[i] > maxX {
					maxX = g.pts[i]
				}
				if g.pts[i+1] < minY {
					minY = g.pts[i+1]
				}
				if g.pts[i+1] > maxY {
					maxY = g.pts[i+1]
				}
			}
			cx, cy := (minX+maxX)/2, (minY+maxY)/2
			local := make([]float64, len(g.pts))
			for i := 0; i+1 < len(g.pts); i += 2 {
				local[i] = g.pts[i] - cx
				local[i+1] = g.pts[i+1] - cy
			}
			kind := model.KindStroke
			sw := a.boardStrokeWidth
			op := 1.0
			if g.kind == "draw-hl" {
				kind = model.KindHighlight
				sw = max(14.0, a.boardStrokeWidth*4.5)
				op = 0.34
			}
			s.RecordUndo()
			s.PlaceAnnotation(model.Placement{
				Kind:        kind,
				X:           cx,
				Y:           cy,
				Width:       max(12.0, maxX-minX),
				Height:      max(12.0, maxY-minY),
				Scale:       1,
				Color:       a.boardStrokeColor,
				StrokeWidth: sw,
				Opacity:     op,
				Points:      local,
			})
			a.markBoard()
			a.flushBoard(true)
		}
	case "draw-rect", "draw-frame":
		if s != nil {
			w := math.Abs(g.mx1 - g.wx0)
			h := math.Abs(g.my1 - g.wy0)
			if w >= 12 && h >= 12 {
				cx := (g.wx0 + g.mx1) / 2
				cy := (g.wy0 + g.my1) / 2
				kind := model.KindRect
				label := ""
				if g.kind == "draw-frame" {
					kind = model.KindFrame
					label = "Section"
				}
				s.RecordUndo()
				s.PlaceAnnotation(model.Placement{
					Kind:        kind,
					Text:        label,
					X:           cx,
					Y:           cy,
					Width:       w,
					Height:      h,
					Scale:       1,
					Color:       a.boardStrokeColor,
					StrokeWidth: a.boardStrokeWidth,
				})
				a.tool = boardToolSelect
				a.markBoard()
				a.flushBoard(true)
			}
		}
	case "draw-arrow":
		if s != nil && math.Hypot(g.mx1-g.wx0, g.my1-g.wy0) >= 12 {
			cx := (g.wx0 + g.mx1) / 2
			cy := (g.wy0 + g.my1) / 2
			s.RecordUndo()
			s.PlaceAnnotation(model.Placement{
				Kind:        model.KindArrow,
				X:           cx,
				Y:           cy,
				Width:       max(16.0, math.Abs(g.mx1-g.wx0)),
				Height:      max(16.0, math.Abs(g.my1-g.wy0)),
				Scale:       1,
				Color:       a.boardStrokeColor,
				StrokeWidth: a.boardStrokeWidth,
				Points:      []float64{g.wx0 - cx, g.wy0 - cy, g.mx1 - cx, g.my1 - cy},
			})
			a.tool = boardToolSelect
			a.markBoard()
			a.flushBoard(true)
		}
	case "erase":
		if s != nil && len(s.Items) != len(g.start) {
			s.PushUndo(g.start)
		}
		a.flushBoard(true)
	case "move", "scale", "rotate", "width", "arrow-p0", "arrow-p1", "resize-tl", "resize-tr", "resize-br", "resize-bl":
		if s != nil && g.start != nil && boardItemsMoved(g.start, s.Items) {
			s.PushUndo(g.start)
		}
		a.flushBoard(true)
	}
	a.gesture = gesture{}
	return true
}

func boardItemsMoved(before, after map[string]model.Placement) bool {
	if len(before) != len(after) {
		return true
	}
	for id, a := range before {
		b, ok := after[id]
		if !ok {
			return true
		}
		if a.X != b.X || a.Y != b.Y || a.Scale != b.Scale || a.Rotation != b.Rotation || a.Width != b.Width || a.Height != b.Height || len(a.Points) != len(b.Points) {
			return true
		}
		for i := range a.Points {
			if a.Points[i] != b.Points[i] {
				return true
			}
		}
	}
	return false
}

func (a *App) eraseAnnotationAt(wx, wy float64) {
	s := a.session
	if s == nil {
		return
	}
	id, _, ok := s.Hit(wx, wy)
	if !ok {
		return
	}
	it, exists := s.Items[id]
	if !exists {
		return
	}
	switch it.Kind {
	case model.KindStroke, model.KindHighlight, model.KindRect, model.KindArrow, model.KindFrame:
		s.Delete([]string{id})
		a.markBoard()
	}
}

func (a *App) applyMarquee(shift bool) {
	s := a.session
	g := a.gesture
	box := canvas.Box{
		MinX: math.Min(g.wx0, g.mx1),
		MinY: math.Min(g.wy0, g.my1),
		MaxX: math.Max(g.wx0, g.mx1),
		MaxY: math.Max(g.wy0, g.my1),
	}
	if box.MaxX-box.MinX < 2 && box.MaxY-box.MinY < 2 {
		return
	}
	hit := map[string]bool{}
	for id := range s.Items {
		if canvas.Bounds(s.Items, id).Intersects(box) {
			hit[id] = true
		}
	}
	if !shift {
		s.Selected = map[string]bool{}
	}
	for id := range hit {
		parent := s.Items[id].ParentID
		covered := false
		for parent != "" {
			if hit[parent] {
				covered = true
				break
			}
			next, ok := s.Items[parent]
			if !ok {
				break
			}
			parent = next.ParentID
		}
		if !covered {
			s.Selected[id] = true
		}
	}
	a.boundID = ""
}

func (a *App) paintBoard(p *gui.Painter, r gui.Rect) {
	s := a.session
	if s == nil {
		return
	}
	// Strictly clip all canvas rendering to the board viewport so pan/zoomed
	// placements never bleed over the toolbar, inspector, or bottom tray.
	p.Clip(r, 0, func() {
		a.paintBoardClipped(p, r)
	})
}

func (a *App) paintBoardClipped(p *gui.Painter, r gui.Rect) {
	s := a.session
	// Subtle infinite studio dot grid.
	zoom := s.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	step := 28.0 * zoom
	for step < 18 {
		step *= 2
	}
	for step > 64 {
		step /= 2
	}
	offX := math.Mod(s.PanX*zoom, step)
	if offX < 0 {
		offX += step
	}
	offY := math.Mod(s.PanY*zoom, step)
	if offY < 0 {
		offY += step
	}
	dotColor := a.pal.paperGrid
	if dotColor.A > 0 {
		for y := offY; y < float64(r.H); y += step {
			for x := offX; x < float64(r.W); x += step {
				p.Fill(gui.Rect{X: r.X + float32(x) - 1, Y: r.Y + float32(y) - 1, W: 2, H: 2}, dotColor, 1)
			}
		}
	}

	for _, id := range s.Visible(float64(r.W), float64(r.H)) {
		a.paintItem(p, r, id)
	}
	// Live preview while drawing Pen / Highlighter / Rect / Arrow / Frame / Marquee.
	switch a.gesture.kind {
	case "marquee":
		x0, y0 := s.WorldToScreen(a.gesture.wx0, a.gesture.wy0)
		x1, y1 := s.WorldToScreen(a.gesture.mx1, a.gesture.my1)
		rect := gui.Rect{
			X: r.X + float32(math.Min(x0, x1)),
			Y: r.Y + float32(math.Min(y0, y1)),
			W: float32(math.Abs(x1 - x0)),
			H: float32(math.Abs(y1 - y0)),
		}
		p.Fill(rect, gui.RGBA(a.accent.R, a.accent.G, a.accent.B, 0.18), 0)
		p.Stroke(rect, a.accent, 0, 1)
	case "draw-pen", "draw-hl":
		if len(a.gesture.pts) >= 4 {
			col := gui.Hex("#FF5C33")
			if c2, ok := parseHexColor(a.boardStrokeColor); ok {
				col = c2
			}
			sw := float32(max(1.5, a.boardStrokeWidth*zoom))
			if a.gesture.kind == "draw-hl" {
				col = gui.RGBA(col.R, col.G, col.B, 0.34)
				sw = float32(max(10.0, a.boardStrokeWidth*4.5*zoom))
			}
			var path gui.Path
			path.Cap(gui.RoundCap).Join(gui.RoundJoin)
			sx0, sy0 := s.WorldToScreen(a.gesture.pts[0], a.gesture.pts[1])
			path.MoveTo(r.X+float32(sx0), r.Y+float32(sy0))
			for i := 2; i+1 < len(a.gesture.pts); i += 2 {
				sx, sy := s.WorldToScreen(a.gesture.pts[i], a.gesture.pts[i+1])
				path.LineTo(r.X+float32(sx), r.Y+float32(sy))
			}
			p.StrokePath(&path, sw, col)
		}
	case "draw-rect", "draw-frame":
		col := gui.Hex("#FF5C33")
		if c2, ok := parseHexColor(a.boardStrokeColor); ok {
			col = c2
		}
		x0, y0 := s.WorldToScreen(a.gesture.wx0, a.gesture.wy0)
		x1, y1 := s.WorldToScreen(a.gesture.mx1, a.gesture.my1)
		rect := gui.Rect{
			X: r.X + float32(math.Min(x0, x1)),
			Y: r.Y + float32(math.Min(y0, y1)),
			W: float32(math.Abs(x1 - x0)),
			H: float32(math.Abs(y1 - y0)),
		}
		if a.gesture.kind == "draw-frame" {
			p.Fill(rect, gui.RGBA(col.R, col.G, col.B, 0.04), 8)
			p.Stroke(rect, gui.RGBA(col.R, col.G, col.B, 0.78), 8, 1.5)
		} else {
			p.Fill(rect, gui.RGBA(col.R, col.G, col.B, 0.08), 8)
			p.Stroke(rect, col, 8, float32(max(1.5, a.boardStrokeWidth*zoom)))
		}
	case "draw-arrow":
		col := gui.Hex("#FF5C33")
		if c2, ok := parseHexColor(a.boardStrokeColor); ok {
			col = c2
		}
		x0, y0 := s.WorldToScreen(a.gesture.wx0, a.gesture.wy0)
		x1, y1 := s.WorldToScreen(a.gesture.mx1, a.gesture.my1)
		paintArrowVec(p, r.X+float32(x0), r.Y+float32(y0), r.X+float32(x1), r.Y+float32(y1), float32(max(2.0, a.boardStrokeWidth*zoom)), col, false)
	}
}

// paintArrowVec draws a true 360-degree vector arrow using gui.Path (never
// Painter.Line, which is axis-aligned only!) with an open
// rounded chevron arrowhead and optional dashed shaft.
func paintArrowVec(p *gui.Painter, x0, y0, x1, y1, width float32, col gui.Color, dashed bool) {
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	dist := math.Hypot(dx, dy)
	if dist < 2 {
		return
	}
	var shaft gui.Path
	shaft.Cap(gui.RoundCap).Join(gui.RoundJoin).MoveTo(x0, y0).LineTo(x1, y1)
	if dashed {
		shaft.Dash(0, max(6, width*3), max(5, width*2.2))
	}
	p.StrokePath(&shaft, width, col)

	if dist < 6 {
		return
	}
	ang := math.Atan2(dy, dx)
	head := math.Min(20.0, math.Max(10.0, float64(width)*3.6))
	if head > dist*0.45 {
		head = dist * 0.45
	}
	spread := 0.48
	ax := x1 - float32(head*math.Cos(ang-spread))
	ay := y1 - float32(head*math.Sin(ang-spread))
	bx := x1 - float32(head*math.Cos(ang+spread))
	by := y1 - float32(head*math.Sin(ang+spread))
	var chevron gui.Path
	chevron.Cap(gui.RoundCap).Join(gui.RoundJoin).
		MoveTo(ax, ay).
		LineTo(x1, y1).
		LineTo(bx, by)
	p.StrokePath(&chevron, width, col)
}

func paintPlacementAnnotation(p *gui.Painter, item model.Placement, rect gui.Rect, cx, cy float32, scale, zoom float64) {
	col := gui.Hex("#FF5C33")
	if c2, ok := parseHexColor(item.Color); ok {
		col = c2
	}
	sw := item.StrokeWidth
	if sw <= 0 {
		sw = 3
	}
	pxW := float32(max(1.5, sw*scale*zoom))
	switch item.Kind {
	case model.KindFrame:
		p.Fill(rect, gui.RGBA(col.R, col.G, col.B, 0.035), 8)
		if item.Dashed {
			p.StrokeDashed(rect, gui.RGBA(col.R, col.G, col.B, 0.72), 8, 1.5)
		} else {
			p.Stroke(rect, gui.RGBA(col.R, col.G, col.B, 0.72), 8, 1.5)
		}
		title := item.Text
		if title == "" {
			title = "Section"
		}
		fs := float32(max(11.0, 12.5*zoom))
		p.Text(rect.X+6, rect.Y-fs-6, title, fs, gui.RGBA(col.R, col.G, col.B, 0.92))
	case model.KindRect:
		p.Fill(rect, gui.RGBA(col.R, col.G, col.B, 0.08), 8)
		if item.Dashed {
			p.StrokeDashed(rect, col, 8, pxW)
		} else {
			p.Stroke(rect, col, 8, pxW)
		}
	case model.KindArrow:
		if len(item.Points) >= 4 {
			x0 := cx + float32(item.Points[0]*scale*zoom)
			y0 := cy + float32(item.Points[1]*scale*zoom)
			x1 := cx + float32(item.Points[2]*scale*zoom)
			y1 := cy + float32(item.Points[3]*scale*zoom)
			paintArrowVec(p, x0, y0, x1, y1, pxW, col, item.Dashed)
		}
	case model.KindStroke, model.KindHighlight:
		if len(item.Points) >= 4 {
			c := col
			if item.Kind == model.KindHighlight {
				op := item.Opacity
				if op <= 0 {
					op = 0.34
				}
				c = gui.RGBA(col.R, col.G, col.B, float32(op))
			}
			var path gui.Path
			path.Cap(gui.RoundCap).Join(gui.RoundJoin)
			path.MoveTo(cx+float32(item.Points[0]*scale*zoom), cy+float32(item.Points[1]*scale*zoom))
			for i := 2; i+1 < len(item.Points); i += 2 {
				path.LineTo(cx+float32(item.Points[i]*scale*zoom), cy+float32(item.Points[i+1]*scale*zoom))
			}
			p.StrokePath(&path, pxW, c)
		}
	}
}

func (a *App) paintItem(p *gui.Painter, origin gui.Rect, id string) {
	s := a.session
	item, ok := s.Items[id]
	if !ok {
		return
	}
	w := canvas.World(s.Items, id)
	scale := w.Scale
	if scale == 0 {
		scale = 1
	}
	zoom := s.Zoom
	if zoom == 0 {
		zoom = 1
	}
	sx, sy := s.WorldToScreen(w.X, w.Y)
	cx := origin.X + float32(sx)
	cy := origin.Y + float32(sy)
	dw := float32(item.Width * scale * zoom)
	dh := float32(item.Height * scale * zoom)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	p.Transform(gui.Rotation(float32(w.Rot)), cx, cy, func() {
		rect := gui.Rect{X: cx - dw/2, Y: cy - dh/2, W: dw, H: dh}
		switch item.Kind {
		case model.KindText:
			txtCol := a.ink
			if c2, ok := parseHexColor(item.Color); ok {
				txtCol = c2
			}
			size := item.FontSize
			if size <= 0 {
				size = 18
			}
			paintWrappedBoardText(p, rect, float32(6*scale*zoom), item.Text, float32(size*scale*zoom), txtCol)
		case model.KindFrame, model.KindRect, model.KindArrow, model.KindStroke, model.KindHighlight:
			paintPlacementAnnotation(p, item, rect, cx, cy, scale, zoom)
		case model.KindGroup:
			p.Stroke(rect, a.accent, 4, 1)
		default:
			if bm := a.requestCardBitmap(a.assetByID[item.AssetID]); bm != nil {
				p.Image(bm, rect, gui.Contain)
			} else {
				p.Fill(rect, gui.RGBA(a.ink.R, a.ink.G, a.ink.B, 0.08), 4)
				label := a.t("library.missing")
				if asset, ok := a.assetByID[item.AssetID]; ok && !asset.Missing {
					label = a.assetName(asset)
				}
				p.Text(rect.X+6, rect.Y+6, label, 13, a.ink)
			}
		}
		if !s.Selected[id] {
			return
		}
		// Vector Arrow selection: crisp circular draggable endpoint handles only.
		if item.Kind == model.KindArrow && len(item.Points) >= 4 {
			if len(s.Selected) == 1 {
				for _, pt := range [][2]float64{{item.Points[0], item.Points[1]}, {item.Points[2], item.Points[3]}} {
					hx := cx + float32(pt[0]*scale*zoom)
					hy := cy + float32(pt[1]*scale*zoom)
					hr := gui.Rect{X: hx - 5.5, Y: hy - 5.5, W: 11, H: 11}
					p.Fill(hr, a.pal.surface, 5.5)
					p.Stroke(hr, a.accent, 5.5, 2)
				}
			}
			return
		}
		// Delicate selection outline (crisp 1.5px, never loud solid white cover).
		selColor := gui.Hex("#38BDF8")
		if item.Kind == model.KindImage || item.Kind == model.KindGroup {
			selColor = a.accent
		}
		p.Stroke(rect, selColor, 4, 1.5)
		if len(s.Selected) != 1 {
			return
		}
		handle := float32(8)
		corners := []gui.Rect{
			{X: rect.X - handle/2, Y: rect.Y - handle/2, W: handle, H: handle},
			{X: rect.X + rect.W - handle/2, Y: rect.Y - handle/2, W: handle, H: handle},
			{X: rect.X + rect.W - handle/2, Y: rect.Y + rect.H - handle/2, W: handle, H: handle},
			{X: rect.X - handle/2, Y: rect.Y + rect.H - handle/2, W: handle, H: handle},
		}
		for _, h := range corners {
			p.Fill(h, a.pal.surface, 2)
			p.Stroke(h, selColor, 2, 1.5)
		}
		// Frames stay cleanly axis-aligned (no rotation handle colliding with "Section" label!).
		if item.Kind != model.KindFrame {
			top := gui.Rect{X: cx - 0.75, Y: rect.Y - 22, W: 1.5, H: 22}
			p.Fill(top, selColor, 0)
			rotKnob := gui.Rect{X: cx - handle/2, Y: rect.Y - 22 - handle/2, W: handle, H: handle}
			p.Fill(rotKnob, a.pal.surface, handle/2)
			p.Stroke(rotKnob, selColor, handle/2, 1.5)
		}
		if item.Kind == model.KindText {
			lh := gui.Rect{X: rect.X - handle/2, Y: cy - handle/2, W: handle, H: handle}
			rh := gui.Rect{X: rect.X + rect.W - handle/2, Y: cy - handle/2, W: handle, H: handle}
			p.Fill(lh, a.pal.surface, 2)
			p.Stroke(lh, selColor, 2, 1.5)
			p.Fill(rh, a.pal.surface, 2)
			p.Stroke(rh, selColor, 2, 1.5)
		}
	})
}
