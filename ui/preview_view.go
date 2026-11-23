package ui

import (
	"fmt"
	"math"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
)

// inlinePreviewView renders the full-stage inline Asset Viewer alongside the
// left sidebar and right Info/Similar Inspector panel with seamless edge-to-edge
// chrome, interactive zoom/pan + minimap, and full audio/timeline video playback.
func (a *App) inlinePreviewView(c *gui.Context) {
	asset, ok := a.assetByID[a.previewAssetID]
	if !ok {
		a.closePreview()
		return
	}
	services := c.Services()
	isVideo := model.MediaKind(asset.Mime) == model.MediaVideo
	isSVG := model.IsSVG(asset.Mime, asset.Path)
	isFont := model.IsFont(asset.Mime, asset.Path)
	isAudio := model.IsAudio(asset.Mime, asset.Path)
	isPDF := model.IsPDF(asset.Mime, asset.Path) || model.IsIllustrator(asset.Mime, asset.Path)
	supportsZoom := !isVideo && !isFont && !isAudio
	if a.previewZoom <= 0 {
		a.previewZoom = 1.0
	}

	// Viewer zoom keyboard shortcuts.
	if supportsZoom {
		c.OnShortcut(0, gui.KeyEqual, func() {
			a.setPreviewZoom(a.previewZoom * 1.25)
		})
		c.OnShortcut(0, gui.KeyMinus, func() {
			a.setPreviewZoom(a.previewZoom / 1.25)
		})
		c.OnShortcut(0, gui.Key0, func() {
			a.previewZoom = 1.0
			a.previewPanX = 0
			a.previewPanY = 0
		})
	}

	prevRootBg := a.pal.canvas
	if a.glassActive {
		prevRootBg = gui.Transparent
	}
	gui.Column(c.Key("inline-preview-root")).Fill().Background(prevRootBg).Children(func() {
		// Flush edge-to-edge Viewer Top Bar fused cleanly into the window top edge.
		gui.Row(c.Key("viewer-topbar")).
			Height(48).
			FillWidth().
			Padding(0, 10, 0, 12).
			Gap(6).
			Background(a.pal.surface).
			BorderWidth(0, 0, 1, 0).
			BorderColor(a.chromeHairline()).
			AlignItems(gui.Center).
			Children(func() {
				a.iconButton(c, "viewer-back", iconArrowLeft, a.t("viewer.back")+" (Esc)", false).OnClick(func() {
					a.closePreview()
				})
				gui.Row(c).Gap(8).FillHeight().Grow(1).AlignItems(gui.Center).DragWindow().Children(func() {
					gui.Text(c, a.assetName(asset)).FontSize(13).Bold().TextColor(a.pal.ink).SingleLine().MaxWidth(360)
					if asset.Width > 0 && asset.Height > 0 {
						gui.Text(c, fmt.Sprintf("%d × %d", asset.Width, asset.Height)).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
					}
				})

				if supportsZoom {
					a.iconButton(c, "viewer-zout", iconZoomOut, "-", false).OnClick(func() {
						a.setPreviewZoom(a.previewZoom / 1.25)
					})
					a.toolButton(c, "viewer-zfit", nil, fmt.Sprintf("%d%%", int(math.Round(a.previewZoom*100))), false).
						Tooltip(a.t("viewer.fit")).
						Padding(4, 8).
						OnClick(func() {
							a.previewZoom = 1.0
							a.previewPanX = 0
							a.previewPanY = 0
						})
					a.iconButton(c, "viewer-zin", iconZoomIn, "+", false).OnClick(func() {
						a.setPreviewZoom(a.previewZoom * 1.25)
					})
					a.iconButton(c, "viewer-thumbbg", iconChecker, a.t("view.thumbBg"), a.thumbBg != "").OnClick(func() {
						order := []string{"", "checker", "black", "gray", "white"}
						next := "checker"
						for i, m := range order {
							if m == a.thumbBg {
								next = order[(i+1)%len(order)]
								break
							}
						}
						a.setThumbBg(next)
					})
					a.iconButton(c, "viewer-gray", iconContrast, a.t("view.grayscale")+" (Ctrl+Alt+G)", a.grayscalePreview).OnClick(func() {
						a.grayscalePreview = !a.grayscalePreview
						a.syncConfigFromState()
					})
				}

				if len(a.assets) > 1 {
					a.iconButton(c, "viewer-prev", iconChevronLeft, a.t("preview.prev"), false).OnClick(func() {
						a.stepPreview(-1)
					})
					a.iconButton(c, "viewer-next", iconChevronRgt, a.t("preview.next"), false).OnClick(func() {
						a.stepPreview(1)
					})
				}

				if !asset.Missing && asset.Path != "" {
					a.iconButton(c, "viewer-open", iconExternal, a.t("common.openFile"), false).OnClick(func() {
						_ = openLocalFile(asset.Path)
					})
					a.iconButton(c, "viewer-reveal", iconFolder, a.t("common.revealFile"), false).OnClick(func() {
						_ = revealLocalFile(asset.Path)
					})
				}
				a.iconButton(c, "viewer-copy", iconCopy, a.t("clipboard.copyFiles"), false).OnClick(func() {
					a.copySelectedFilesToClipboard([]string{asset.ID})
				})
				if !asset.Missing && asset.Path != "" && !isVideo {
					a.iconButton(c, "viewer-sim", iconPalette, a.t("inspector.tabSimilar"), a.inspectorOpen && a.inspectorTab == 1).OnClick(func() {
						a.inspectorOpen = true
						a.inspectorTab = 1
						a.ensureInspectorSimilar(asset)
					})
				}
				a.iconButton(c, "viewer-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.closePreview()
				})

				gui.Box(c).Size(1, 18).Background(a.pal.surfaceMuted)
				a.windowControls(c)
			})

		// Center Artwork Stage + Optional Right Inspector (Info / Similar).
		stage := func() {
			stageBg := a.pal.canvas
			if a.dark && !a.glassActive {
				stageBg = gui.Hex("#0D0D0F")
			}
			box := gui.Column(c.Key("viewer-stage-" + asset.ID)).
				Fill().
				Clip().
				Background(stageBg).
				Center()
			applyBlurReveal(box, "viewer-"+asset.ID, 0)
			box.Children(func() {
				switch {
				case isSVG:
					gui.Column(c).Fill().Padding(18).Gap(12).Center().Children(func() {
						a.renderSVGStage(c, asset)
					})
				case isFont:
					gui.Column(c).Fill().Padding(18).Gap(12).Center().Children(func() {
						a.renderFontStage(c, asset)
					})
				case isAudio:
					gui.Column(c).Fill().Padding(18).Gap(12).Center().Children(func() {
						a.renderAudioStage(c, asset)
					})
				case isPDF:
					gui.Column(c).Fill().Padding(18).Gap(12).Center().Children(func() {
						a.renderPDFStage(c, asset)
					})
				case a.previewVideo != nil:
					a.renderVideoStage(c, asset)
				case isVideo && a.previewVideoErr != "":
					gui.Column(c).Center().Gap(10).Children(func() {
						gui.Icon(c, iconVideo).Size(36, 36).TextColor(a.pal.inkMuted)
						gui.Text(c, a.previewVideoErr).FontSize(12).TextColor(a.pal.inkSecondary).TextAlign(gui.Center)
						if !asset.Missing && asset.Path != "" {
							a.arcButton(c, "viewer-ext-vid", a.t("common.openFile"), true).OnClick(func() {
								_ = openLocalFile(asset.Path)
							})
						}
					})
				case a.previewBitmap != nil:
					a.renderZoomableImageStage(c, asset, a.previewBitmap)
				case a.bitmaps[asset.ID] != nil:
					a.renderZoomableImageStage(c, asset, a.bitmaps[asset.ID])
				case a.previewLoading:
					gui.Text(c, a.t("preview.loading")).FontSize(13).TextColor(a.pal.inkMuted)
				default:
					gui.Column(c).Center().Gap(10).Children(func() {
						gui.Icon(c, iconFile).Size(36, 36).TextColor(a.pal.inkMuted)
						gui.Text(c, a.assetExt(asset)).FontSize(16).Bold().TextColor(a.pal.inkSecondary)
						if !asset.Missing && asset.Path != "" {
							a.arcButton(c, "viewer-ext-file", a.t("common.openFile"), true).OnClick(func() {
								_ = openLocalFile(asset.Path)
							})
						} else {
							a.toolButton(c, "viewer-copy-path", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
								services.WriteClipboard(asset.Path)
								a.notify(a.t("common.copied"))
							})
						}
					})
				}
				a.previewStackStrip(c, asset)
			})
		}

		a.withAnimatedInspector(c, "viewer-insp-layout", stage)
	})
}

func (a *App) setPreviewZoom(z float64) {
	if z < 0.15 {
		z = 0.15
	} else if z > 8.0 {
		z = 8.0
	}
	a.previewZoom = z
	if z <= 1.01 {
		a.previewPanX = 0
		a.previewPanY = 0
	} else {
		a.clampPreviewPan()
	}
}

func (a *App) clampPreviewPan() {
	if a.previewStageW <= 0 || a.previewStageH <= 0 || a.previewZoom <= 1.0 {
		a.previewPanX = 0
		a.previewPanY = 0
		return
	}
	maxX := float64(a.previewStageW) * (a.previewZoom - 1.0) * 0.55
	maxY := float64(a.previewStageH) * (a.previewZoom - 1.0) * 0.55
	if maxX < 120 {
		maxX = 120
	}
	if maxY < 120 {
		maxY = 120
	}
	if a.previewPanX > maxX {
		a.previewPanX = maxX
	} else if a.previewPanX < -maxX {
		a.previewPanX = -maxX
	}
	if a.previewPanY > maxY {
		a.previewPanY = maxY
	} else if a.previewPanY < -maxY {
		a.previewPanY = -maxY
	}
}

// previewBaseRect computes the fitted (100%) image rectangle inside stage r.
func previewBaseRect(r gui.Rect, imgW, imgH float32) gui.Rect {
	pad := float32(20)
	availW := r.W - pad*2
	availH := r.H - pad*2
	if availW < 40 {
		availW = r.W
	}
	if availH < 40 {
		availH = r.H
	}
	if imgW <= 0 || imgH <= 0 {
		return gui.Rect{X: r.X + pad, Y: r.Y + pad, W: availW, H: availH}
	}
	scale := availW / imgW
	if availH/imgH < scale {
		scale = availH / imgH
	}
	w := imgW * scale
	h := imgH * scale
	return gui.Rect{
		X: r.X + (r.W-w)/2,
		Y: r.Y + (r.H-h)/2,
		W: w,
		H: h,
	}
}

// renderZoomableImageStage renders an interactive mouse-wheel zoomable,
// click-and-drag pannable image viewer with an Eagle-style bottom-right
// Navigator Minimap whenever zoomed > 100%.
func (a *App) renderZoomableImageStage(c *gui.Context, asset model.Asset, bm *gui.Bitmap) {
	imgW, imgH := gui.ImageSize(bm)
	if imgW <= 0 || imgH <= 0 {
		imgW = float32(max(asset.Width, 1))
		imgH = float32(max(asset.Height, 1))
	}

	cur := gui.CursorDefault
	if a.previewDragging {
		cur = gui.CursorGrabbing
	} else if a.previewZoom > 1.01 {
		cur = gui.CursorGrab
	}

	wrap := gui.Box(c.Key("prev-img-wrap-" + asset.ID)).
		Grow(1).
		FillWidth().
		Clip()

	wrap.Children(func() {
		stageCanvas := gui.Box(c.Key("prev-img-canvas-" + asset.ID)).
			Fill().
			Cursor(cur).
			HandleInput(func(ev gui.InputEvent) bool {
				switch ev.Kind {
				case gui.InputScroll:
					factor := 1.14
					if ev.DY > 0 {
						factor = 1 / 1.14
					}
					oldZ := a.previewZoom
					if oldZ <= 0 {
						oldZ = 1
					}
					newZ := oldZ * factor
					if newZ < 0.15 {
						newZ = 0.15
					} else if newZ > 8.0 {
						newZ = 8.0
					}
					if a.previewStageW > 0 && a.previewStageH > 0 {
						cx := float64(a.previewStageW) / 2
						cy := float64(a.previewStageH) / 2
						relX := float64(ev.X) - cx - a.previewPanX
						relY := float64(ev.Y) - cy - a.previewPanY
						ratio := newZ / oldZ
						a.previewPanX = float64(ev.X) - cx - relX*ratio
						a.previewPanY = float64(ev.Y) - cy - relY*ratio
					}
					a.previewZoom = newZ
					if newZ <= 1.01 {
						a.previewPanX = 0
						a.previewPanY = 0
					} else {
						a.clampPreviewPan()
					}
					return true
				case gui.InputPointerDown:
					if ev.Button != 0 && ev.Button != 1 {
						return false
					}
					if ev.Clicks >= 2 {
						if a.previewZoom > 1.05 {
							a.previewZoom = 1.0
							a.previewPanX = 0
							a.previewPanY = 0
						} else {
							oldZ := a.previewZoom
							if oldZ <= 0 {
								oldZ = 1
							}
							newZ := 2.25
							cx := float64(a.previewStageW) / 2
							cy := float64(a.previewStageH) / 2
							a.previewPanX = (cx - float64(ev.X)) * (newZ/oldZ - 1)
							a.previewPanY = (cy - float64(ev.Y)) * (newZ/oldZ - 1)
							a.previewZoom = newZ
							a.clampPreviewPan()
						}
						return true
					}
					if a.previewZoom > 1.01 || ev.Button == 1 {
						a.previewDragging = true
						a.previewDragX = float64(ev.X)
						a.previewDragY = float64(ev.Y)
						a.previewDragStartX = a.previewPanX
						a.previewDragStartY = a.previewPanY
						return true
					}
				case gui.InputPointerMove:
					if !a.previewDragging {
						return false
					}
					a.previewPanX = a.previewDragStartX + (float64(ev.X) - a.previewDragX)
					a.previewPanY = a.previewDragStartY + (float64(ev.Y) - a.previewDragY)
					a.clampPreviewPan()
					return true
				case gui.InputPointerUp:
					if a.previewDragging {
						a.previewDragging = false
						return true
					}
				}
				return false
			})
		if a.grayscalePreview {
			stageCanvas.Grayscale()
		}
		stageCanvas.Draw(func(p *gui.Painter, r gui.Rect) {
			a.previewStageW = r.W
			a.previewStageH = r.H
			base := previewBaseRect(r, imgW, imgH)
			z := float32(a.previewZoom)
			if z <= 0 {
				z = 1
			}
			dw := base.W * z
			dh := base.H * z
			cx := r.X + r.W/2 + float32(a.previewPanX)
			cy := r.Y + r.H/2 + float32(a.previewPanY)
			dst := gui.Rect{X: cx - dw/2, Y: cy - dh/2, W: dw, H: dh}

			p.Clip(r, 0, func() {
				// Paint Thumbnail Background strictly behind the image rectangle so
				// transparent cutout PNG/PSD/WebP edges pop cleanly!
				if a.thumbBg == "checker" {
					p.Clip(dst, 6, func() {
						a.paintCheckerboard(p, dst, 12)
					})
				} else if col, ok := a.thumbBgSolidColor(); ok {
					p.Fill(dst, col, 6)
				}
				p.Image(bm, dst, gui.Contain)
			})
		})

		// Interactive Eagle-style Bottom-Right Navigator Minimap when zoomed in.
		if a.previewZoom > 1.02 && a.previewStageW > 0 && a.previewStageH > 0 {
			const maxMini float32 = 132
			miniW, miniH := maxMini, maxMini
			if imgW > imgH {
				miniH = max(48, maxMini*(imgH/imgW))
			} else if imgH > imgW {
				miniW = max(48, maxMini*(imgW/imgH))
			}
			panFromMini := func(mx, my float32) {
				stageR := gui.Rect{W: a.previewStageW, H: a.previewStageH}
				base := previewBaseRect(stageR, imgW, imgH)
				z := a.previewZoom
				dw := float64(base.W) * z
				dh := float64(base.H) * z
				nx := float64(mx)/float64(miniW) - 0.5
				ny := float64(my)/float64(miniH) - 0.5
				a.previewPanX = -nx * dw
				a.previewPanY = -ny * dh
				a.clampPreviewPan()
			}
			mini := gui.Box(c.Key("prev-minimap-"+asset.ID)).
				Absolute().
				Right(16).
				Bottom(16).
				Size(miniW, miniH).
				Radius(8).
				Clip().
				Background(gui.RGBA(14, 14, 18, 0.88)).
				Border(1, gui.RGBA(255, 255, 255, 0.16)).
				Shadow(0, 8, 22, 0, a.pal.shadowFloating).
				Cursor(gui.CursorPointer).
				HandleInput(func(ev gui.InputEvent) bool {
					switch ev.Kind {
					case gui.InputPointerDown:
						if ev.Button != 0 {
							return false
						}
						a.previewMiniDrag = true
						panFromMini(ev.X, ev.Y)
						return true
					case gui.InputPointerMove:
						if !a.previewMiniDrag {
							return false
						}
						panFromMini(ev.X, ev.Y)
						return true
					case gui.InputPointerUp:
						if a.previewMiniDrag {
							a.previewMiniDrag = false
							return true
						}
					}
					return false
				})
			mini.Draw(func(p *gui.Painter, r gui.Rect) {
				p.Image(bm, r, gui.Contain)
				stageR := gui.Rect{W: a.previewStageW, H: a.previewStageH}
				base := previewBaseRect(stageR, imgW, imgH)
				z := float32(a.previewZoom)
				dw := base.W * z
				dh := base.H * z
				if dw <= 0 || dh <= 0 {
					return
				}
				// Image top-left in stage coordinates.
				imgLeft := a.previewStageW/2 + float32(a.previewPanX) - dw/2
				imgTop := a.previewStageH/2 + float32(a.previewPanY) - dh/2
				// Visible stage [0..W, 0..H] mapped to [0..1] in image space.
				u0 := clamp01f(-imgLeft / dw)
				v0 := clamp01f(-imgTop / dh)
				u1 := clamp01f((a.previewStageW - imgLeft) / dw)
				v1 := clamp01f((a.previewStageH - imgTop) / dh)
				vx := r.X + u0*r.W
				vy := r.Y + v0*r.H
				vw := max(10, (u1-u0)*r.W)
				vh := max(10, (v1-v0)*r.H)
				// Subtle dim outside viewport rect.
				dim := gui.RGBA(0, 0, 0, 0.42)
				if vy > r.Y {
					p.Fill(gui.Rect{X: r.X, Y: r.Y, W: r.W, H: vy - r.Y}, dim, 0)
				}
				if vy+vh < r.Y+r.H {
					p.Fill(gui.Rect{X: r.X, Y: vy + vh, W: r.W, H: r.Y + r.H - (vy + vh)}, dim, 0)
				}
				if vx > r.X {
					p.Fill(gui.Rect{X: r.X, Y: vy, W: vx - r.X, H: vh}, dim, 0)
				}
				if vx+vw < r.X+r.W {
					p.Fill(gui.Rect{X: vx + vw, Y: vy, W: r.X + r.W - (vx + vw), H: vh}, dim, 0)
				}
				// Crisp white viewport border.
				p.Stroke(gui.Rect{X: vx, Y: vy, W: vw, H: vh}, gui.Hex("#FFFFFF"), 1.6, 3)
			})
		}
	})
}

func clamp01f(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// renderVideoStage renders the native GPU hardware-decoded VideoPlayer with
// synchronized audio, draggable timeline scrubber, volume/mute, speed, and loop.
func (a *App) renderVideoStage(c *gui.Context, asset model.Asset) {
	vp := a.previewVideo
	if vp == nil {
		return
	}
	playing := vp.Playing()
	if playing {
		c.AnimationFrame()
	}
	dur := vp.Duration()
	pos := vp.Position()
	durSec := dur.Seconds()
	posSec := pos.Seconds()
	if durSec < 0 {
		durSec = 0
	}
	if posSec < 0 {
		posSec = 0
	}
	if durSec > 0 && posSec > durSec {
		posSec = durSec
	}
	if !a.previewScrubbing {
		a.previewScrubVal = posSec
	}

	gui.Column(c).Fill().Padding(16, 20, 14, 20).Gap(12).AlignItems(gui.Center).Children(func() {
		gui.Box(c).Grow(1).FillWidth().Center().Children(func() {
			vidEl := gui.Video(c.Key("prev-video-"+asset.ID), vp).
				Fit(gui.Contain).
				Fill().
				Radius(8)
			if a.grayscalePreview {
				vidEl.Grayscale()
			}
		})

		// Studio Video Transport Bar with scrubbable timeline + volume + speed + loop.
		gui.Column(c.Key("prev-vid-bar")).
			FillWidth().
			MaxWidth(760).
			Padding(10, 16, 10, 16).
			Gap(6).
			Radius(radiusPanel).
			Background(a.pal.surface).
			Border(1, a.chromeHairline()).
			Shadow(0, 8, 22, 0, a.pal.shadowRaised).
			Children(func() {
				// Interactive scrubbable timeline bar.
				if durSec > 0.05 {
					gui.Row(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Text(c, formatDuration(a.previewScrubVal)).
							FontSize(11).
							TextColor(a.pal.inkSecondary).
							Width(42).
							SingleLine()
						gui.Slider(c.Key("prev-vid-seek-"+asset.ID), &a.previewScrubVal, 0, durSec).
							Grow(1).
							OnChange(func() {
								vp.Seek(time.Duration(a.previewScrubVal * float64(time.Second)))
							})
						gui.Text(c, formatDuration(durSec)).
							FontSize(11).
							TextColor(a.pal.inkMuted).
							Width(42).
							TextAlign(gui.End).
							SingleLine()
					})
				}

				// Playback, Audio Volume, Speed, and Loop controls.
				gui.Row(c).FillWidth().Gap(6).AlignItems(gui.Center).Children(func() {
					playIcon := iconPlay
					playLbl := a.t("preview.play")
					if playing {
						playIcon = iconPause
						playLbl = a.t("preview.pause")
					}
					a.toolButton(c, "prev-vid-toggle", playIcon, playLbl, playing).OnClick(func() {
						if vp.Playing() {
							vp.Pause()
						} else {
							if vp.Ended() {
								vp.Seek(0)
							}
							vp.Play()
						}
					})

					a.toolButton(c, "prev-vid-loop", iconRepeat, a.t("preview.loop"), a.previewLoop).OnClick(func() {
						a.previewLoop = !a.previewLoop
						vp.SetLoop(a.previewLoop)
					})

					if a.previewVideoRate <= 0 {
						a.previewVideoRate = 1.0
					}
					rateLbl := fmt.Sprintf("%.1f×", a.previewVideoRate)
					a.toolButton(c, "prev-vid-rate", nil, rateLbl, a.previewVideoRate != 1.0).
						Tooltip(a.t("video.speed")).
						Padding(4, 8).
						OnClick(func() {
							rates := []float64{0.5, 1.0, 1.5, 2.0}
							next := 1.0
							for i, r := range rates {
								if math.Abs(r-a.previewVideoRate) < 0.05 {
									next = rates[(i+1)%len(rates)]
									break
								}
							}
							a.previewVideoRate = next
							vp.SetRate(next)
						})

					gui.Spacer(c)

					// Audio Mute toggle + Volume slider.
					volIcon := iconVolume
					volTip := a.t("video.mute")
					if a.previewVideoMuted || a.previewVideoVol <= 0.01 {
						volIcon = iconVolumeMute
						volTip = a.t("video.unmute")
					}
					a.iconButton(c, "prev-vid-mute", volIcon, volTip, a.previewVideoMuted).OnClick(func() {
						a.previewVideoMuted = !a.previewVideoMuted
						vp.SetMuted(a.previewVideoMuted)
					})
					gui.Slider(c.Key("prev-vid-vol"), &a.previewVideoVol, 0, 1).
						Width(86).
						OnChange(func() {
							if a.previewVideoVol > 0.01 && a.previewVideoMuted {
								a.previewVideoMuted = false
								vp.SetMuted(false)
							}
							vp.SetVolume(a.previewVideoVol)
						})
				})
			})
	})
}
