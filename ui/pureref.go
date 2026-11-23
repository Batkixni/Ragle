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
// reference window with TRUE independent transparent background support
// (adjusting background opacity 0%..100% independently of reference images),
// plus horizontal flip, grayscale value check, image opacity, and zoom/pan.
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
	if a.pureRefImgOpacity < 0.2 || a.pureRefImgOpacity > 1.0 {
		a.pureRefImgOpacity = 1.0
	}
	if a.pureRefBgOpacity < 0.0 || a.pureRefBgOpacity > 1.0 {
		a.pureRefBgOpacity = 0.85
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
		Title:           title,
		Width:           580,
		Height:          440,
		MinWidth:        260,
		MinHeight:       180,
		Frameless:       true,
		Transparent:     true,
		BackgroundColor: "rgba(0,0,0,0)",
		AlwaysOnTop:     true,
		StateKey:        "pureref",
		Content:         gui.View(a.pureRefView),
	})
	w.SetOpacity(1.0)
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
	// Critical: clear the transparent swapchain with alpha 0 so pureRefBgOpacity
	// genuinely controls background transparency independently of reference images!
	refTheme := *c.Theme()
	refTheme.Background = gui.Transparent
	c.SetTheme(&refTheme)
	c.OnShortcut(0, gui.KeyEscape, func() {
		a.closePureRefWindow()
	})
	c.OnShortcut(0, gui.KeyF, func() {
		a.fitPureRefView(float64(a.pureRefViewW), float64(a.pureRefViewH))
	})
	c.OnShortcut(0, gui.KeyH, func() {
		a.pureRefFlipH = !a.pureRefFlipH
	})
	c.OnShortcut(0, gui.KeyG, func() {
		a.pureRefGray = !a.pureRefGray
	})

	if a.pureRefImgOpacity < 0.2 || a.pureRefImgOpacity > 1.0 {
		a.pureRefImgOpacity = 1.0
	}
	bgAlpha := float32(a.pureRefBgOpacity)
	if bgAlpha < 0 {
		bgAlpha = 0
	} else if bgAlpha > 1 {
		bgAlpha = 1
	}

	var bg gui.Color
	if a.dark {
		bg = gui.RGBA(18, 18, 20, bgAlpha)
	} else {
		bg = gui.RGBA(244, 244, 245, bgAlpha)
	}

	root := gui.Box(c.Key("pureref-root")).Fill().Background(bg).Clip()
	rootHovered := root.Hovered()
	if rootHovered || bgAlpha > 0.08 {
		root.Border(1, gui.RGBA(255, 255, 255, 0.10))
	}

	root.Children(func() {
		// Full-bleed reference canvas with independent image opacity, flip H, grayscale, pan & zoom.
		stage := gui.Box(c.Key("pureref-stage")).
			Fill().
			Opacity(float32(a.pureRefImgOpacity)).
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
					dx := float64(ev.X) - a.pureRefDragX
					if a.pureRefFlipH {
						dx = -dx
					}
					a.pureRefPanX = a.pureRefStartPanX + dx
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
					ex := float64(ev.X)
					if a.pureRefFlipH && a.pureRefViewW > 0 {
						ex = float64(a.pureRefViewW) - ex
					}
					wx := (ex - a.pureRefPanX) / oldZ
					wy := (float64(ev.Y) - a.pureRefPanY) / oldZ
					a.pureRefZoom = newZ
					a.pureRefPanX = ex - wx*newZ
					a.pureRefPanY = float64(ev.Y) - wy*newZ
					return true
				}
				return false
			})
		if a.pureRefGray {
			stage.Grayscale()
		}
		stage.Draw(func(p *gui.Painter, r gui.Rect) {
			a.pureRefViewW = r.W
			a.pureRefViewH = r.H
			if a.session == nil {
				return
			}
			cam := &canvas.Session{
				Items: a.session.Items,
				PanX:  a.pureRefPanX,
				PanY:  a.pureRefPanY,
				Zoom:  a.pureRefZoom,
			}
			if cam.Zoom <= 0 {
				cam.Zoom = 1
			}
			drawAll := func() {
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
						switch item.Kind {
						case model.KindText:
							p.Fill(dst, a.pal.surface, 8)
							sz := item.FontSize
							if sz <= 0 {
								sz = 18
							}
							fs := float32(max(10.0, sz*sc*cam.Zoom))
							p.Text(dst.X+8, dst.Y+8, item.Text, fs, a.pal.ink)
						case model.KindFrame, model.KindRect, model.KindArrow, model.KindStroke, model.KindHighlight:
							paintPlacementAnnotation(p, item, dst, cx, cy, sc, cam.Zoom)
						default:
							if bm := a.requestCardBitmap(a.assetByID[item.AssetID]); bm != nil {
								p.Image(bm, dst, gui.Contain)
							} else {
								p.Fill(dst, a.pal.surfaceMuted, 4)
							}
						}
					})
				}
			}
			p.Clip(r, 0, func() {
				if a.pureRefFlipH {
					p.Transform(gui.Scaling(-1, 1), r.X+r.W/2, r.Y+r.H/2, drawAll)
				} else {
					drawAll()
				}
			})
		})
		stage.Children(func() {})

		// Compact hover-revealed PureRef HUD pill bar (fades away cleanly when not hovered
		// so transparent-background floating references have zero visual chrome clutter).
		hudWrap := gui.Box(c.Key("pureref-hud")).
			Absolute().
			Top(8).
			Left(8).
			Right(8)
		hudOp := float32(0.08)
		if bgAlpha > 0.25 {
			hudOp = 0.36
		}
		if rootHovered || hudWrap.Hovered() {
			hudOp = 0.98
		}
		hudWrap.Opacity(hudWrap.AnimateTo("hud-op", hudOp, animFast)).Children(func() {
			gui.Row(c).
				Height(38).
				Padding(0, 8, 0, 10).
				Gap(6).
				Radius(radiusPill).
				Background(gui.RGBA(24, 24, 28, 0.92)).
				Border(1, gui.RGBA(255, 255, 255, 0.10)).
				Shadow(0, 8, 22, 0, a.pal.shadowFloating).
				AlignItems(gui.Center).
				Children(func() {
					// Draggable title region.
					gui.Row(c).Gap(6).FillHeight().Grow(1).AlignItems(gui.Center).DragWindow().Children(func() {
						gui.Icon(c, iconPin).Size(13, 13).TextColor(gui.Hex("#FAFAFA"))
						lbl := a.boardName
						if lbl == "" {
							lbl = a.t("pureref.open")
						}
						gui.Text(c, lbl).FontSize(12).Bold().TextColor(gui.Hex("#FAFAFA")).SingleLine().MaxWidth(96)
						gui.Text(c, fmt.Sprintf("%d%%", int(math.Round(a.pureRefZoom*100)))).FontSize(11).TextColor(gui.Hex("#A1A1AA")).SingleLine()
					})

					// Independent Background Opacity toggle + slider (0% fully transparent glass -> 100% solid).
					a.iconButton(c, "pureref-bg-toggle", iconChecker, a.t("pureref.transparentBg"), a.pureRefBgOpacity < 0.05).
						Size(26, 26).
						OnClick(func() {
							if a.pureRefBgOpacity < 0.05 {
								a.pureRefBgOpacity = 0.85
							} else {
								a.pureRefBgOpacity = 0.0
							}
							a.syncConfigFromState()
						})
					gui.Slider(c.Key("pureref-bg-opacity"), &a.pureRefBgOpacity, 0.0, 1.0).
						Width(58).
						Tooltip(fmt.Sprintf("%s: %d%%", a.t("pureref.bgOpacity"), int(math.Round(a.pureRefBgOpacity*100)))).
						OnChange(func() {
							a.syncConfigFromState()
						})

					gui.Box(c).Size(1, 16).Background(gui.RGBA(255, 255, 255, 0.12))

					// Independent Image Opacity slider (20% .. 100%).
					gui.Slider(c.Key("pureref-img-opacity"), &a.pureRefImgOpacity, 0.2, 1.0).
						Width(52).
						Tooltip(fmt.Sprintf("%s: %d%%", a.t("pureref.imgOpacity"), int(math.Round(a.pureRefImgOpacity*100))))

					gui.Box(c).Size(1, 16).Background(gui.RGBA(255, 255, 255, 0.12))

					// Artist Ref Tools: Flip Horizontal (H), Grayscale (G), Fit (F), Pin, Close.
					a.iconButton(c, "pureref-fliph", iconFlipH, a.t("pureref.flipH"), a.pureRefFlipH).
						Size(26, 26).
						OnClick(func() { a.pureRefFlipH = !a.pureRefFlipH })
					a.iconButton(c, "pureref-gray", iconContrast, a.t("pureref.grayscale"), a.pureRefGray).
						Size(26, 26).
						OnClick(func() { a.pureRefGray = !a.pureRefGray })
					a.iconButton(c, "pureref-fit", iconMaximize, a.t("pureref.fit"), false).
						Size(26, 26).
						OnClick(func() {
							a.fitPureRefView(float64(a.pureRefViewW), float64(a.pureRefViewH))
						})
					a.iconButton(c, "pureref-pin", iconPin, a.t("pureref.pinWindow"), a.pureRefPinned).
						Size(26, 26).
						OnClick(func() {
							a.pureRefPinned = !a.pureRefPinned
							if a.pureRefWin != nil {
								a.pureRefWin.SetAlwaysOnTop(a.pureRefPinned)
							}
						})
					a.iconButton(c, "pureref-close", iconX, a.t("common.close"), false).
						Size(26, 26).
						OnClick(func() {
							a.closePureRefWindow()
						})
				})
		})
	})
}
