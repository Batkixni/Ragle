package ui

import (
	"fmt"
	"math"

	"github.com/egoist/mygo"
	gui "github.com/egoist/mygo/ui"

	"raglex/internal/canvas"
	"raglex/internal/model"
)

// openPureRefWindow opens (or focuses) a PureRef-style Always-on-Top floating
// reference window for the active board so artists can keep references hovering
// over Photoshop, Blender, Premiere, Clip Studio Paint, etc.
func (a *App) openPureRefWindow() {
	if a.session == nil || a.win == nil {
		return
	}
	if a.pureRefWin != nil && !a.pureRefWin.IsDestroyed() {
		a.pureRefWin.Show()
		a.pureRefWin.Focus()
		return
	}
	a.pureRefPinned = true
	if a.pureRefOpacity < 0.35 || a.pureRefOpacity > 1.0 {
		a.pureRefOpacity = 0.96
	}
	a.pureRefPanX = a.session.PanX
	a.pureRefPanY = a.session.PanY
	a.pureRefZoom = a.session.Zoom
	if a.pureRefZoom <= 0 {
		a.pureRefZoom = 1
	}

	title := "Raglex Ref"
	if a.boardName != "" {
		title = "Raglex Ref · " + a.boardName
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:             title,
		Width:             540,
		Height:            420,
		MinWidth:          220,
		MinHeight:         160,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		AlwaysOnTop:       true,
		StateKey:          "pureref",
		Content:           gui.View(a.pureRefView),
	})
	w.SetOpacity(a.pureRefOpacity)
	w.OnClosed(func() {
		if a.win != nil && !a.isClosed() {
			a.win.Update(func() {
				a.pureRefWin = nil
			})
		} else {
			a.pureRefWin = nil
		}
	})
	a.pureRefWin = w
}

func (a *App) closePureRefWindow() {
	if a.pureRefWin != nil && !a.pureRefWin.IsDestroyed() {
		w := a.pureRefWin
		a.pureRefWin = nil
		w.Close()
	}
}

func (a *App) toggleMainAlwaysOnTop() {
	a.boardAlwaysOnTop = !a.boardAlwaysOnTop
	if a.win != nil {
		a.win.SetAlwaysOnTop(a.boardAlwaysOnTop)
	}
}

func (a *App) fitPureRefView(viewW, viewH float64) {
	if a.session == nil || len(a.session.Items) == 0 || viewW <= 0 || viewH <= 0 {
		a.pureRefZoom = 1
		a.pureRefPanX = 0
		a.pureRefPanY = 0
		return
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, it := range a.session.Items {
		if it.Kind == model.KindGroup {
			continue
		}
		sc := it.ScaleOrOne()
		hw := (it.Width * sc) / 2
		hh := (it.Height * sc) / 2
		if it.X-hw < minX {
			minX = it.X - hw
		}
		if it.Y-hh < minY {
			minY = it.Y - hh
		}
		if it.X+hw > maxX {
			maxX = it.X + hw
		}
		if it.Y+hh > maxY {
			maxY = it.Y + hh
		}
	}
	if math.IsInf(minX, 1) {
		return
	}
	bw := max(40.0, maxX-minX)
	bh := max(40.0, maxY-minY)
	pad := 32.0
	z := math.Min((viewW-pad*2)/bw, (viewH-pad*2)/bh)
	if z < 0.15 {
		z = 0.15
	} else if z > 4.0 {
		z = 4.0
	}
	cx := (minX + maxX) / 2
	cy := (minY + maxY) / 2
	a.pureRefZoom = z
	a.pureRefPanX = viewW/2 - cx*z
	a.pureRefPanY = viewH/2 - cy*z
}

func (a *App) pureRefView(c *gui.Context) {
	a.captureTheme(c)
	c.OnShortcut(0, gui.KeyEscape, func() {
		a.closePureRefWindow()
	})
	c.OnShortcut(0, gui.KeyF, func() {
		a.fitPureRefView(float64(a.pureRefViewW), float64(a.pureRefViewH))
	})

	bg := gui.Hex("#121214")
	if !a.dark {
		bg = gui.Hex("#F4F4F5")
	}

	root := gui.Box(c.Key("pureref-root")).Fill().Background(bg).Clip()
	root.Children(func() {
		// Full-bleed PureRef canvas with left-drag pan & wheel zoom.
		stage := gui.Box(c.Key("pureref-stage")).
			Fill().
			Cursor(gui.CursorGrab).
			HandleInput(func(ev gui.InputEvent) bool {
				switch ev.Kind {
				case gui.InputPointerDown:
					if ev.Button == 0 || ev.Button == 1 || ev.Button == 2 {
						a.pureRefDragging = true
						a.pureRefDragX = float64(ev.X)
						a.pureRefDragY = float64(ev.Y)
						a.pureRefStartPanX = a.pureRefPanX
						a.pureRefStartPanY = a.pureRefPanY
						return true
					}
				case gui.InputPointerMove:
					if !a.pureRefDragging {
						return false
					}
					a.pureRefPanX = a.pureRefStartPanX + (float64(ev.X) - a.pureRefDragX)
					a.pureRefPanY = a.pureRefStartPanY + (float64(ev.Y) - a.pureRefDragY)
					return true
				case gui.InputPointerUp:
					if a.pureRefDragging {
						a.pureRefDragging = false
						return true
					}
				case gui.InputScroll:
					factor := 1.12
					if ev.DY > 0 {
						factor = 1 / 1.12
					}
					oldZ := a.pureRefZoom
					if oldZ <= 0 {
						oldZ = 1
					}
					newZ := oldZ * factor
					if newZ < 0.1 {
						newZ = 0.1
					} else if newZ > 8.0 {
						newZ = 8.0
					}
					wx := (float64(ev.X) - a.pureRefPanX) / oldZ
					wy := (float64(ev.Y) - a.pureRefPanY) / oldZ
					a.pureRefZoom = newZ
					a.pureRefPanX = float64(ev.X) - wx*newZ
					a.pureRefPanY = float64(ev.Y) - wy*newZ
					return true
				}
				return false
			}).
			Draw(func(p *gui.Painter, r gui.Rect) {
				a.pureRefViewW = r.W
				a.pureRefViewH = r.H
				if a.session == nil {
					return
				}
				// Paint using the PureRef window's own independent camera & Z-order!
				cam := &canvas.Session{
					Items: a.session.Items,
					PanX:  a.pureRefPanX,
					PanY:  a.pureRefPanY,
					Zoom:  a.pureRefZoom,
				}
				if cam.Zoom <= 0 {
					cam.Zoom = 1
				}
				p.Clip(r, 0, func() {
					for _, id := range cam.Visible(float64(r.W), float64(r.H)) {
						item, ok := cam.Items[id]
						if !ok || item.Kind == model.KindGroup {
							continue
						}
						wt := canvas.World(cam.Items, id)
						sc := wt.Scale
						if sc == 0 {
							sc = 1
						}
						sx, sy := cam.WorldToScreen(wt.X, wt.Y)
						cx := r.X + float32(sx)
						cy := r.Y + float32(sy)
						sw := float32(item.Width * sc * cam.Zoom)
						sh := float32(item.Height * sc * cam.Zoom)
						if sw < 1 || sh < 1 {
							continue
						}
						p.Transform(gui.Rotation(float32(wt.Rot)), cx, cy, func() {
							dst := gui.Rect{X: cx - sw/2, Y: cy - sh/2, W: sw, H: sh}
							if item.Kind == model.KindText {
								p.Fill(dst, a.pal.surface, 8)
								sz := item.FontSize
								if sz <= 0 {
									sz = 18
								}
								fs := float32(max(10.0, sz*sc*cam.Zoom))
								p.Text(dst.X+8, dst.Y+8, item.Text, fs, a.pal.ink)
								return
							}
							if bm := a.bitmaps[item.AssetID]; bm != nil {
								p.Image(bm, dst, gui.Contain)
							} else {
								p.Fill(dst, a.pal.surfaceMuted, 4)
							}
						})
					}
				})
			})
		stage.Children(func() {})

		// Compact floating PureRef HUD pill bar at the top (auto-dimmed until hovered).
		hudWrap := gui.Box(c.Key("pureref-hud")).
			Absolute().
			Top(8).
			Left(8).
			Right(8)
		hudOp := float32(0.42)
		if hudWrap.Hovered() {
			hudOp = 0.98
		}
		hudWrap.Opacity(hudWrap.AnimateTo("hud-op", hudOp, animFast)).Children(func() {
			gui.Row(c).
				Height(36).
				Padding(0, 8, 0, 10).
				Gap(6).
				Radius(radiusPill).
				Background(a.pal.surfaceRaised).
				Shadow(0, 6, 18, 0, a.pal.shadowFloating).
				AlignItems(gui.Center).
				Children(func() {
					gui.Row(c).Gap(6).FillHeight().Grow(1).AlignItems(gui.Center).DragWindow().Children(func() {
						gui.Icon(c, iconPin).Size(13, 13).TextColor(a.pal.ink)
						lbl := a.boardName
						if lbl == "" {
							lbl = a.t("pureref.open")
						}
						gui.Text(c, lbl).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine()
						gui.Text(c, fmt.Sprintf("%d%%", int(math.Round(a.pureRefZoom*100)))).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
					})

					// Window opacity slider (35% .. 100%).
					gui.Slider(c.Key("pureref-opacity"), &a.pureRefOpacity, 0.35, 1.0).
						Width(62).
						Tooltip(a.t("pureref.opacity")).
						OnChange(func() {
							if a.pureRefWin != nil {
								a.pureRefWin.SetOpacity(a.pureRefOpacity)
							}
						})

					a.iconButton(c, "pureref-fit", iconMaximize, a.t("pureref.fit"), false).OnClick(func() {
						a.fitPureRefView(float64(a.pureRefViewW), float64(a.pureRefViewH))
					})

					a.iconButton(c, "pureref-pin", iconPin, a.t("pureref.pinWindow"), a.pureRefPinned).OnClick(func() {
						a.pureRefPinned = !a.pureRefPinned
						if a.pureRefWin != nil {
							a.pureRefWin.SetAlwaysOnTop(a.pureRefPinned)
						}
					})

					a.iconButton(c, "pureref-close", iconX, a.t("common.close"), false).OnClick(func() {
						a.closePureRefWindow()
					})
				})
		})
	})
}
