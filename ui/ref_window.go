package ui

import (
	"fmt"
	"math"

	"github.com/egoist/mygo"
	gui "github.com/egoist/mygo/ui"

	"raglex/internal/canvas"
	"raglex/internal/model"
)

// openRefWindow opens (or focuses) an Always-on-Top floating reference window
// with independent transparent background support (adjusting background opacity
// 0%..100% independently of reference images), plus horizontal flip, grayscale
// value check, image opacity, and zoom/pan.
func (a *App) openRefWindow() {
	if a.session == nil || a.win == nil {
		return
	}
	if a.refWin != nil && !a.refWin.IsDestroyed() {
		a.refWin.Show()
		a.refWin.Focus()
		return
	}
	a.refWinPinned = true
	if a.refWinImgOpacity < 0.2 || a.refWinImgOpacity > 1.0 {
		a.refWinImgOpacity = 1.0
	}
	if a.refWinBgOpacity < 0.0 || a.refWinBgOpacity > 1.0 {
		a.refWinBgOpacity = 0.85
	}
	a.refWinPanX = a.session.PanX
	a.refWinPanY = a.session.PanY
	a.refWinZoom = a.session.Zoom
	if a.refWinZoom <= 0 {
		a.refWinZoom = 1
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
		StateKey:        "refwin",
		Content:         gui.View(a.refWinView),
	})
	w.SetOpacity(1.0)
	w.OnClosed(func() {
		if a.win != nil && !a.isClosed() {
			a.win.Update(func() {
				a.refWin = nil
			})
		} else {
			a.refWin = nil
		}
	})
	a.refWin = w
}

func (a *App) closeRefWindow() {
	if a.refWin != nil && !a.refWin.IsDestroyed() {
		w := a.refWin
		a.refWin = nil
		w.Close()
	}
}

func (a *App) toggleMainAlwaysOnTop() {
	a.boardAlwaysOnTop = !a.boardAlwaysOnTop
	if a.win != nil {
		a.win.SetAlwaysOnTop(a.boardAlwaysOnTop)
	}
}

func (a *App) fitRefWinView(viewW, viewH float64) {
	if a.session == nil || len(a.session.Items) == 0 || viewW <= 0 || viewH <= 0 {
		a.refWinZoom = 1
		a.refWinPanX = 0
		a.refWinPanY = 0
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
	a.refWinZoom = z
	a.refWinPanX = viewW/2 - cx*z
	a.refWinPanY = viewH/2 - cy*z
}

func (a *App) refWinView(c *gui.Context) {
	a.captureTheme(c)
	// Clear the transparent swapchain with alpha 0 so refWinBgOpacity
	// genuinely controls background transparency independently of reference images.
	refTheme := *c.Theme()
	refTheme.Background = gui.Transparent
	c.SetTheme(&refTheme)
	c.OnShortcut(0, gui.KeyEscape, func() {
		a.closeRefWindow()
	})
	c.OnShortcut(0, gui.KeyF, func() {
		a.fitRefWinView(float64(a.refWinViewW), float64(a.refWinViewH))
	})
	c.OnShortcut(0, gui.KeyH, func() {
		a.refWinFlipH = !a.refWinFlipH
	})
	c.OnShortcut(0, gui.KeyG, func() {
		a.refWinGray = !a.refWinGray
	})

	if a.refWinImgOpacity < 0.2 || a.refWinImgOpacity > 1.0 {
		a.refWinImgOpacity = 1.0
	}
	bgAlpha := float32(a.refWinBgOpacity)
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

	root := gui.Box(c.Key("refwin-root")).Fill().Background(bg).Clip()
	rootHovered := root.Hovered()
	if rootHovered || bgAlpha > 0.08 {
		root.Border(1, gui.RGBA(255, 255, 255, 0.10))
	}

	root.Children(func() {
		// Full-bleed reference canvas with independent image opacity, flip H, grayscale, pan & zoom.
		stage := gui.Box(c.Key("refwin-stage")).
			Fill().
			Opacity(float32(a.refWinImgOpacity)).
			Cursor(gui.CursorGrab).
			HandleInput(func(ev gui.InputEvent) bool {
				switch ev.Kind {
				case gui.InputPointerDown:
					if ev.Button == 0 || ev.Button == 1 || ev.Button == 2 {
						a.refWinDragging = true
						a.refWinDragX = float64(ev.X)
						a.refWinDragY = float64(ev.Y)
						a.refWinStartPanX = a.refWinPanX
						a.refWinStartPanY = a.refWinPanY
						return true
					}
				case gui.InputPointerMove:
					if !a.refWinDragging {
						return false
					}
					dx := float64(ev.X) - a.refWinDragX
					if a.refWinFlipH {
						dx = -dx
					}
					a.refWinPanX = a.refWinStartPanX + dx
					a.refWinPanY = a.refWinStartPanY + (float64(ev.Y) - a.refWinDragY)
					return true
				case gui.InputPointerUp:
					if a.refWinDragging {
						a.refWinDragging = false
						return true
					}
				case gui.InputScroll:
					factor := 1.12
					if ev.DY > 0 {
						factor = 1 / 1.12
					}
					oldZ := a.refWinZoom
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
					if a.refWinFlipH && a.refWinViewW > 0 {
						ex = float64(a.refWinViewW) - ex
					}
					wx := (ex - a.refWinPanX) / oldZ
					wy := (float64(ev.Y) - a.refWinPanY) / oldZ
					a.refWinZoom = newZ
					a.refWinPanX = ex - wx*newZ
					a.refWinPanY = float64(ev.Y) - wy*newZ
					return true
				}
				return false
			})
		if a.refWinGray {
			stage.Grayscale()
		}
		stage.Draw(func(p *gui.Painter, r gui.Rect) {
			a.refWinViewW = r.W
			a.refWinViewH = r.H
			if a.session == nil {
				return
			}
			cam := &canvas.Session{
				Items: a.session.Items,
				PanX:  a.refWinPanX,
				PanY:  a.refWinPanY,
				Zoom:  a.refWinZoom,
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
							txtCol := a.pal.ink
							if c2, ok := parseHexColor(item.Color); ok {
								txtCol = c2
							}
							sz := item.FontSize
							if sz <= 0 {
								sz = 18
							}
							fs := float32(max(10.0, sz*sc*cam.Zoom))
							paintWrappedBoardText(p, dst, 8, item.Text, fs, txtCol)
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
				if a.refWinFlipH {
					p.Transform(gui.Scaling(-1, 1), r.X+r.W/2, r.Y+r.H/2, drawAll)
				} else {
					drawAll()
				}
			})
		})
		stage.Children(func() {})

		// Compact hover-revealed floating HUD pill bar (fades away cleanly when not hovered).
		hudWrap := gui.Box(c.Key("refwin-hud")).
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
				Height(36).
				Padding(0, 8, 0, 12).
				Gap(8).
				Radius(radiusPill).
				Background(gui.RGBA(20, 20, 24, 0.88)).
				Border(1, gui.RGBA(255, 255, 255, 0.10)).
				Shadow(0, 8, 22, 0, a.pal.shadowFloating).
				AlignItems(gui.Center).
				Children(func() {
					// Clean draggable handle + background opacity slider.
					gui.Row(c).Gap(6).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
						gui.Box(c).Size(18, 4).Radius(radiusPill).Background(gui.RGBA(255, 255, 255, 0.24))
					})

					gui.Slider(c.Key("refwin-bg-opacity"), &a.refWinBgOpacity, 0.0, 1.0).
						Width(74).
						Tooltip(fmt.Sprintf("%s: %d%%", a.t("refwin.bgOpacity"), int(math.Round(a.refWinBgOpacity*100)))).
						OnChange(func() {
							a.syncConfigFromState()
						})

					gui.Box(c).Size(1, 14).Background(gui.RGBA(255, 255, 255, 0.12))

					gui.Slider(c.Key("refwin-img-opacity"), &a.refWinImgOpacity, 0.2, 1.0).
						Width(68).
						Tooltip(fmt.Sprintf("%s: %d%%", a.t("refwin.imgOpacity"), int(math.Round(a.refWinImgOpacity*100))))

					gui.Box(c).Grow(1).FillHeight().DragWindow()

					a.iconButton(c, "refwin-fit", iconMaximize, a.t("refwin.fit"), false).
						Size(26, 26).
						OnClick(func() {
							a.fitRefWinView(float64(a.refWinViewW), float64(a.refWinViewH))
						})
					a.iconButton(c, "refwin-pin", iconPin, a.t("refwin.pinWindow"), a.refWinPinned).
						Size(26, 26).
						OnClick(func() {
							a.refWinPinned = !a.refWinPinned
							if a.refWin != nil {
								a.refWin.SetAlwaysOnTop(a.refWinPinned)
							}
						})
					a.iconButton(c, "refwin-close", iconX, a.t("common.close"), false).
						Size(26, 26).
						OnClick(func() {
							a.closeRefWindow()
						})
				})
		})

		// Quiet bottom-left board title watermark (only gently appears on hover).
		if a.boardName != "" {
			footOp := float32(0.0)
			if rootHovered {
				footOp = 0.72
			}
			foot := gui.Row(c.Key("refwin-foot-title")).
				Absolute().
				Left(12).
				Bottom(10).
				Padding(3, 9).
				Gap(6).
				Radius(radiusPill).
				Background(gui.RGBA(18, 18, 22, 0.72)).
				Border(1, gui.RGBA(255, 255, 255, 0.08)).
				PassThrough()
			foot.Opacity(foot.AnimateTo("foot-op", footOp, animFast)).Children(func() {
				gui.Text(c, a.boardName).FontSize(11).TextColor(gui.Hex("#E4E4E7")).SingleLine().MaxWidth(180)
				gui.Text(c, fmt.Sprintf("%d%%", int(math.Round(a.refWinZoom*100)))).FontSize(10).TextColor(gui.Hex("#A1A1AA")).SingleLine()
			})
		}
	})
}
