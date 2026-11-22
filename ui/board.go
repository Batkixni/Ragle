package ui

import (
	"fmt"
	"math"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/canvas"
	"raglex/internal/model"
)

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
	t := c.Theme()
	zoomPct := 100
	itemCount := 0
	if a.session != nil {
		if a.session.Zoom > 0 {
			zoomPct = int(math.Round(a.session.Zoom * 100))
		}
		itemCount = len(a.session.Items)
	}
	gui.Box(c).Padding(10, 12, 6, 12).Children(func() {
		gui.Row(c.Key("board-floating-bar")).
			Height(48).
			Padding(0, 10, 0, 12).
			Gap(8).
			Radius(radiusPanel).
			AlignItems(gui.Center).
			Background(a.pal.surfaceRaised).
			Shadow(0, 6, 20, 0, a.pal.shadowRaised).
			Children(func() {
				// Back & breadcrumb.
				leaveLbl := a.t("board.leave")
				if a.boardProject == "" {
					leaveLbl = a.t("board.leaveToLib")
				}
				a.toolButton(c, "board-leave", iconArrowLeft, leaveLbl, false).OnClick(func() {
					a.flushBoard(true)
					a.closePureRefWindow()
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
					a.pillBadge(c, fmt.Sprintf(a.t("board.placementCount"), itemCount), t.TextMuted, a.pal.surface)
				})
				if a.boardProject == "" && len(a.projectList) > 0 {
					gui.PopoverMenuButton(c.Key("board-move-proj"), a.t("board.moveToProject"), func(m *gui.Menu) {
						for _, p := range a.projectList {
							proj := p
							if m.Item(proj.Name).Chosen() {
								a.moveBoardToProject(a.boardID, proj.ID)
							}
						}
					})
				}

				gui.Box(c).Grow(1).FillHeight().DragWindow()

				// Arc UI sliding pill tool switcher.
				a.arcPillTabsWithIcons(c, "board-tools", a.tool, []struct {
					icon  *gui.SVG
					label string
				}{
					{iconCursor, a.t("board.select")},
					{iconHand, a.t("board.pan")},
					{iconType, a.t("board.text")},
				}, func(idx int) {
					a.tool = idx
				})

				gui.Row(c).Gap(2).AlignItems(gui.Center).Children(func() {
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

				a.toolButton(c, "board-pureref-btn", iconPin, a.t("pureref.open"), a.pureRefWin != nil).OnClick(func() {
					if a.pureRefWin != nil {
						a.closePureRefWindow()
					} else {
						a.openPureRefWindow()
					}
				})
				a.iconButton(c, "board-pin-main", iconPin, a.t("pureref.pinWindow"), a.boardAlwaysOnTop).OnClick(func() {
					a.toggleMainAlwaysOnTop()
				})

				a.iconButton(c, "board-toggle-tray", iconPanelBottom, a.t("board.drawer"), a.boardDrawerOpen).OnClick(func() {
					a.boardDrawerOpen = !a.boardDrawerOpen
				})
				a.iconButton(c, "board-toggle-insp", iconPanelRight, a.t("board.inspector"), a.boardInspectorOpen).OnClick(func() {
					a.boardInspectorOpen = !a.boardInspectorOpen
				})

				gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
				a.windowControls(c)
			})
	})
}

func (a *App) boardSurface(c *gui.Context) {
	board := gui.Box(c.Key("board")).Grow(1).Clip().Focusable().Background(a.paper)
	if a.focusBoard {
		board = board.Focus()
		a.focusBoard = false
	}
	board = board.HandleInput(a.boardInput).Draw(a.paintBoard)
	board.Children(func() {})
	if drag, ok := gui.Drop[assetDrag](board); ok {
		if x, y, over := board.PointerPosition(); over {
			a.dropAsset(drag.ID, float64(x), float64(y))
		}
	}
}

func (a *App) boardTray(c *gui.Context) {
	t := c.Theme()
	standalone := a.boardProject == ""
	list := a.projectAssets
	if standalone || a.tray == 1 {
		list = a.allAssets
	}
	trays := []string{a.t("board.trayProject"), a.t("board.trayLibrary")}
	gui.Box(c).Padding(6, 12, 10, 12).Children(func() {
		gui.Column(c.Key("board-tray-panel")).
			Height(134).
			Radius(radiusPanel).
			Clip().
			Background(a.pal.surfaceRaised).
			Shadow(0, 8, 24, 0, a.pal.shadowRaised).
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
									if bm := a.bitmaps[item.ID]; bm != nil {
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
	})
}

func (a *App) boardInspector(c *gui.Context) {
	t := c.Theme()
	gui.Box(c).Padding(4, 12, 4, 6).Children(func() {
		gui.Column(c.Key("board-insp-panel")).
			Width(272).
			FillHeight().
			Radius(radiusPanel).
			Clip().
			Background(a.pal.surfaceRaised).
			Shadow(0, 8, 24, 0, a.pal.shadowRaised).
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

						if item.Kind == model.KindText {
							gui.Column(c).Gap(6).Children(func() {
								a.sectionLabel(c, a.t("board.body"))
								field := gui.TextInput(c.Key("body"), &a.editText).FillWidth().OnChange(func() {
									a.commitEditText()
								})
								if a.focusBody {
									field.Focus()
									a.focusBody = false
								}
							})
							gui.Column(c).Gap(6).Children(func() {
								a.sectionLabel(c, fmt.Sprintf("%s (%.0fpx)", a.t("board.fontSize"), a.fontSizeBuf))
								gui.Slider(c.Key("board-fsize"), &a.fontSizeBuf, 10, 72).FillWidth().OnChange(func() {
									if a.session == nil || a.boundID == "" {
										return
									}
									cur := a.session.Items[a.boundID]
									cur.FontSize = a.fontSizeBuf
									a.session.Items[a.boundID] = cur
									a.markBoard()
								})
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

func (a *App) commitEditText() {
	if a.session == nil || a.boundID == "" {
		return
	}
	item, ok := a.session.Items[a.boundID]
	if !ok || item.Text == a.editText {
		return
	}
	a.session.SetText(a.boundID, a.editText)
	a.markBoard()
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

func (a *App) groupSelection() {
	if a.session == nil {
		return
	}
	if _, err := a.session.Group(a.session.SelectedIDs()); err != nil {
		a.status = a.t("board.needTwo")
		return
	}
	a.boundID = ""
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) ungroupSelection() {
	if a.session == nil {
		return
	}
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
	if front {
		a.session.BringFront(ids[0])
	} else {
		a.session.BringBack(ids[0])
	}
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
	ctrl := ev.Mods&gui.Ctrl != 0
	shift := ev.Mods&gui.Shift != 0
	switch ev.Key {
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
		a.confirmDeleteSelection()
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
	default:
		return false
	}
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
	if a.tool == 2 {
		if id, part, ok := s.Hit(wx, wy); ok && part == "body" && s.Items[id].Kind == model.KindText {
			s.Select(id, false)
			a.boundID = ""
			a.focusBody = true
			return true
		}
		if _, _, ok := s.Hit(wx, wy); !ok {
			s.PlaceText(a.t("board.placeholder"), wx, wy)
			a.boundID = ""
			a.focusBody = true
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
	if ev.Clicks >= 2 && s.Items[id].Kind == model.KindText {
		a.boundID = ""
		a.focusBody = true
		return true
	}
	a.gesture = gesture{
		kind: "move", ids: s.SelectedIDs(), start: s.Snapshot(),
		wx0: wx, wy0: wy,
	}
	return true
}

func (a *App) pointerMove(ev gui.InputEvent) bool {
	s := a.session
	g := a.gesture
	if g.kind == "" {
		return false
	}
	wx, wy := s.ScreenToWorld(float64(ev.X), float64(ev.Y))
	switch g.kind {
	case "pan":
		s.PanX = g.panX + (float64(ev.X) - g.x0)
		s.PanY = g.panY + (float64(ev.Y) - g.y0)
	case "marquee":
		a.gesture.mx1 = wx
		a.gesture.my1 = wy
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
	if a.gesture.kind == "marquee" {
		a.applyMarquee(ev.Mods&gui.Shift != 0)
	}
	if a.gesture.kind == "move" || a.gesture.kind == "scale" || a.gesture.kind == "rotate" || a.gesture.kind == "width" {
		a.flushBoard(true)
	}
	a.gesture = gesture{}
	return true
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
	if a.gesture.kind == "marquee" {
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
			p.Fill(rect, gui.RGBA(a.ink.R, a.ink.G, a.ink.B, 0.08), 4)
			size := item.FontSize
			if size <= 0 {
				size = 18
			}
			p.Text(rect.X+6, rect.Y+6, item.Text, float32(size*scale*zoom), a.ink)
		case model.KindGroup:
			p.Stroke(rect, a.accent, 4, 1)
		default:
			if bm := a.bitmaps[item.AssetID]; bm != nil {
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
		p.Stroke(rect, a.accent, 2, 1.5)
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
			p.Fill(h, a.accent, 2)
		}
		top := gui.Rect{X: cx - 1, Y: rect.Y - 24, W: 2, H: 24}
		p.Fill(top, a.accent, 0)
		p.Fill(gui.Rect{X: cx - handle/2, Y: rect.Y - 24 - handle/2, W: handle, H: handle}, a.accent, handle/2)
		if item.Kind == model.KindText {
			p.Fill(gui.Rect{X: rect.X - handle/2, Y: cy - handle/2, W: handle, H: handle}, a.accent, 2)
			p.Fill(gui.Rect{X: rect.X + rect.W - handle/2, Y: cy - handle/2, W: handle, H: handle}, a.accent, 2)
		}
	})
}
