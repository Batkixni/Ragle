package ui

import (
	"fmt"
	"math"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

// inlinePreviewView renders the full-stage inline Asset Viewer (NOT a popup!)
// inside the main workspace alongside the left sidebar and right Info/Similar
// Inspector panel.
func (a *App) inlinePreviewView(c *gui.Context) {
	asset, ok := a.assetByID[a.previewAssetID]
	if !ok {
		a.closePreview()
		return
	}
	services := c.Services()
	isVideo := model.MediaKind(asset.Mime) == model.MediaVideo
	if a.previewZoom <= 0 {
		a.previewZoom = 1.0
	}

	gui.Column(c.Key("inline-preview-root")).Fill().Background(a.pal.canvas).Children(func() {
		// Clean Viewer Top Bar (Back + Filename on left; Zoom + Prev/Next + Actions + Window Controls on right).
		gui.Box(c).Padding(10, 12, 6, 6).Children(func() {
			gui.Row(c.Key("viewer-topbar")).
				Height(46).
				Padding(0, 8, 0, 10).
				Gap(6).
				Radius(radiusPanel).
				Background(a.pal.surface).
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

					if !isVideo {
						a.iconButton(c, "viewer-zout", iconZoomOut, "-", false).OnClick(func() {
							a.previewZoom = math.Max(0.25, a.previewZoom/1.25)
						})
						a.toolButton(c, "viewer-zfit", nil, fmt.Sprintf("%d%%", int(math.Round(a.previewZoom*100))), false).
							Tooltip(a.t("viewer.fit")).
							Padding(4, 8).
							OnClick(func() { a.previewZoom = 1.0 })
						a.iconButton(c, "viewer-zin", iconZoomIn, "+", false).OnClick(func() {
							a.previewZoom = math.Min(4.0, a.previewZoom*1.25)
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
		})

		// Center Artwork Stage + Optional Right Inspector (Info / Similar).
		stage := func() {
			stageBg := a.pal.canvas
			if a.dark {
				stageBg = gui.Hex("#0D0D0F")
			}
			gui.Box(c).Fill().Padding(4, 8, 10, 8).Children(func() {
				box := gui.Column(c.Key("viewer-stage-"+asset.ID)).
					Fill().
					Radius(radiusPanel).
					Clip().
					Background(stageBg).
					Center().
					Padding(18).
					Gap(12)
				applyBlurReveal(box, "viewer-"+asset.ID, 0)
				box.Children(func() {
					switch {
					case a.previewVideo != nil:
						gui.Box(c).Grow(1).FillWidth().Center().Children(func() {
							gui.Video(c.Key("prev-video-"+asset.ID), a.previewVideo).
								Fit(gui.Contain).
								Fill().
								Radius(8)
						})
						gui.Row(c).
							Padding(6, 12).
							Gap(8).
							Radius(radiusPill).
							Background(a.pal.surface).
							AlignItems(gui.Center).
							Children(func() {
								playing := a.previewVideo.Playing()
								playIcon := iconPlay
								playLbl := a.t("preview.play")
								if playing {
									playIcon = iconPause
									playLbl = a.t("preview.pause")
								}
								a.toolButton(c, "prev-vid-toggle", playIcon, playLbl, playing).OnClick(func() {
									if a.previewVideo.Playing() {
										a.previewVideo.Pause()
									} else {
										a.previewVideo.Play()
									}
								})
								a.toolButton(c, "prev-vid-loop", iconRepeat, a.t("preview.loop"), a.previewLoop).OnClick(func() {
									a.previewLoop = !a.previewLoop
									if a.previewVideo != nil {
										a.previewVideo.SetLoop(a.previewLoop)
									}
								})
								dur := a.previewVideo.Duration()
								pos := a.previewVideo.Position()
								if dur > 0 {
									gui.Text(c, fmt.Sprintf("%s / %s", formatDuration(pos.Seconds()), formatDuration(dur.Seconds()))).
										FontSize(11).
										TextColor(a.pal.inkSecondary).
										SingleLine()
								}
							})
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
						imgEl := gui.Image(c, a.previewBitmap).Fit(gui.Contain).Fill().Radius(8)
						if math.Abs(a.previewZoom-1.0) > 0.01 {
							imgEl.AnimatePose(gui.Pose{Scale: float32(a.previewZoom)}, springSnappy)
						}
					case a.bitmaps[asset.ID] != nil:
						imgEl := gui.Image(c, a.bitmaps[asset.ID]).Fit(gui.Contain).Fill().Radius(8)
						if math.Abs(a.previewZoom-1.0) > 0.01 {
							imgEl.AnimatePose(gui.Pose{Scale: float32(a.previewZoom)}, springSnappy)
						}
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
				})
			})
		}

		a.withAnimatedInspector(c, "viewer-insp-layout", stage)
	})
}
