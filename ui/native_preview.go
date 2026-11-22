package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"time"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
	"ragle/internal/preview"
)

func rgbaToBitmap(img *image.RGBA) (*gui.Bitmap, error) {
	if img == nil {
		return nil, os.ErrInvalid
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return gui.DecodeBitmap(buf.Bytes())
}

// loadNativeFormatPreview loads Font, Audio, SVG, or PDF preview state when
// opening an asset in inlinePreviewView. Returns true if handled.
func (a *App) loadNativeFormatPreview(asset model.Asset) bool {
	switch {
	case model.IsSVG(asset.Mime, asset.Path):
		a.loadSVGPreview(asset)
		return true
	case model.IsFont(asset.Mime, asset.Path):
		a.loadFontPreview(asset)
		return true
	case model.IsAudio(asset.Mime, asset.Path):
		a.loadAudioPreview(asset)
		return true
	case model.IsPDF(asset.Mime, asset.Path):
		a.loadPDFPreview(asset, 0)
		return true
	}
	return false
}

func (a *App) resetNativePreviewState() {
	a.previewSVG = nil
	a.previewFontInfo = preview.FontInfo{}
	a.previewFontBitmap = nil
	a.previewPDFBitmap = nil
	a.previewPDFPage = 0
	a.previewPDFPages = 0
	a.previewAudioWave = preview.Waveform{}
	a.previewAudioPos = 0
	a.previewAudioDur = 0
	a.previewAudioPlaying = false
	if a.previewAudioCancel != nil {
		a.previewAudioCancel()
		a.previewAudioCancel = nil
	}
	if a.previewAudio != nil {
		_ = a.previewAudio.Close()
		a.previewAudio = nil
	}
}

// --- SVG Lossless Vector Preview ---

func (a *App) loadSVGPreview(asset model.Asset) {
	if a.win == nil {
		if raw, err := os.ReadFile(asset.Path); err == nil {
			if svg, err := gui.ParseSVG(raw); err == nil {
				a.previewSVG = svg
			}
		}
		return
	}
	a.previewLoading = true
	win := a.win
	targetID := asset.ID
	path := asset.Path
	go func() {
		raw, err := os.ReadFile(path)
		var parsed *gui.SVG
		if err == nil {
			parsed, err = gui.ParseSVG(raw)
		}
		win.Update(func() {
			if a.previewAssetID != targetID || (!a.previewOpen && !a.quickLookOpen) || a.isClosed() {
				return
			}
			a.previewLoading = false
			if err != nil {
				a.previewVideoErr = err.Error()
				return
			}
			a.previewSVG = parsed
		})
	}()
}

// --- Font (.ttf / .otf) Interactive Specimen Preview ---

func (a *App) loadFontPreview(asset model.Asset) {
	if a.previewFontText == "" {
		a.previewFontText = "設計靈感 The quick brown fox 0123456789"
	}
	if a.previewFontSize < 16 {
		a.previewFontSize = 44
	}
	a.renderFontPreviewAsync(asset)
}

func (a *App) renderFontPreviewAsync(asset model.Asset) {
	fg := color.RGBA{250, 250, 250, 255}
	bg := color.RGBA{18, 18, 20, 255}
	if !a.dark {
		fg = color.RGBA{24, 24, 27, 255}
		bg = color.RGBA{248, 248, 250, 255}
	}
	sample := a.previewFontText
	sizePt := a.previewFontSize
	path := asset.Path
	targetID := asset.ID
	a.previewFontGen++
	gen := a.previewFontGen

	if a.win == nil {
		if img, info, err := preview.RenderFontSpecimen(path, sample, sizePt, 860, 520, fg, bg); err == nil {
			a.previewFontInfo = info
			if bm, berr := rgbaToBitmap(img); berr == nil {
				a.previewFontBitmap = bm
			}
		}
		return
	}

	a.previewLoading = true
	win := a.win
	go func() {
		img, info, err := preview.RenderFontSpecimen(path, sample, sizePt, 880, 540, fg, bg)
		var bm *gui.Bitmap
		if err == nil {
			bm, err = rgbaToBitmap(img)
		}
		win.Update(func() {
			if a.previewAssetID != targetID || a.previewFontGen != gen || (!a.previewOpen && !a.quickLookOpen) || a.isClosed() {
				return
			}
			a.previewLoading = false
			if err != nil {
				a.previewVideoErr = err.Error()
				return
			}
			a.previewFontInfo = info
			a.previewFontBitmap = bm
		})
	}()
}

// --- Audio Waveform + Direct Playback Preview ---

func (a *App) loadAudioPreview(asset model.Asset) {
	if a.previewAudioVol <= 0 {
		a.previewAudioVol = 0.85
	}
	path := asset.Path
	targetID := asset.ID
	vol := a.previewAudioVol

	if a.win == nil {
		if wf, err := preview.ExtractWaveform(path, 96); err == nil {
			a.previewAudioWave = wf
			a.previewAudioDur = wf.Duration
		}
		return
	}

	a.previewLoading = true
	win := a.win
	go func() {
		wf, _ := preview.ExtractWaveform(path, 96)
		player, perr := preview.OpenAudioPlayer(path)
		if player != nil {
			player.SetVolume(vol)
		}
		win.Update(func() {
			if a.previewAssetID != targetID || (!a.previewOpen && !a.quickLookOpen) || a.isClosed() {
				if player != nil {
					_ = player.Close()
				}
				return
			}
			a.previewLoading = false
			a.previewAudioWave = wf
			a.previewAudio = player
			if player != nil {
				_, dur := player.Position()
				if dur > 0 {
					a.previewAudioDur = dur
				} else {
					a.previewAudioDur = wf.Duration
				}
			} else if perr != nil {
				a.previewVideoErr = perr.Error()
			}
		})
	}()
}

func (a *App) toggleAudioPlayback() {
	if a.previewAudio == nil {
		return
	}
	if a.previewAudio.Playing() {
		_ = a.previewAudio.Pause()
		a.previewAudioPlaying = false
		if a.previewAudioCancel != nil {
			a.previewAudioCancel()
			a.previewAudioCancel = nil
		}
		return
	}
	if err := a.previewAudio.Play(); err != nil {
		a.notify(err.Error())
		return
	}
	a.previewAudioPlaying = true
	a.startAudioTicker()
}

func (a *App) startAudioTicker() {
	if a.win == nil || a.previewAudio == nil {
		return
	}
	if a.previewAudioCancel != nil {
		a.previewAudioCancel()
	}
	stop := make(chan struct{})
	a.previewAudioCancel = func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
	win := a.win
	player := a.previewAudio
	targetID := a.previewAssetID
	go func() {
		tk := time.NewTicker(80 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				cur, tot := player.Position()
				playing := player.Playing()
				win.Update(func() {
					if a.previewAssetID != targetID || !a.previewOpen || a.isClosed() {
						return
					}
					a.previewAudioPos = cur
					if tot > 0 {
						a.previewAudioDur = tot
					}
					a.previewAudioPlaying = playing
				})
				if !playing {
					return
				}
			}
		}
	}()
}

// --- PDF Multi-Page Preview ---

func (a *App) loadPDFPreview(asset model.Asset, page int) {
	if page < 0 {
		page = 0
	}
	a.previewPDFPage = page
	targetID := asset.ID
	path := asset.Path
	a.previewPDFGen++
	gen := a.previewPDFGen

	if a.win == nil {
		if img, total, err := preview.RenderPDFPage(path, page, 900); err == nil {
			a.previewPDFPages = total
			if bm, berr := rgbaToBitmap(img); berr == nil {
				a.previewPDFBitmap = bm
			}
		}
		return
	}

	a.previewLoading = true
	win := a.win
	go func() {
		img, total, err := preview.RenderPDFPage(path, page, 1360)
		var bm *gui.Bitmap
		if err == nil {
			bm, err = rgbaToBitmap(img)
		}
		win.Update(func() {
			if a.previewAssetID != targetID || a.previewPDFGen != gen || (!a.previewOpen && !a.quickLookOpen) || a.isClosed() {
				return
			}
			a.previewLoading = false
			if total > 0 {
				a.previewPDFPages = total
			}
			if err != nil {
				a.previewVideoErr = err.Error()
				return
			}
			a.previewVideoErr = ""
			a.previewPDFBitmap = bm
		})
	}()
}

// --- Inline Preview Stage Renderers for Designer Formats ---

func (a *App) renderSVGStage(c *gui.Context, asset model.Asset) {
	if a.previewSVG == nil {
		gui.Text(c, a.t("common.loading")).FontSize(13).TextColor(a.pal.inkSecondary)
		return
	}
	svgW, svgH := a.previewSVG.Size()
	if svgW <= 0 || svgH <= 0 {
		svgW, svgH = 420, 420
	}
	baseScale := float32(math.Min(640/float64(svgW), 500/float64(svgH)))
	drawW := svgW * baseScale * float32(a.previewZoom)
	drawH := svgH * baseScale * float32(a.previewZoom)
	if drawW < 48 {
		drawW = 48
	}
	if drawH < 48 {
		drawH = 48
	}

	gui.Column(c).Fill().Center().Gap(12).Children(func() {
		frame := gui.Box(c.Key("svg-canvas-" + asset.ID)).
			Grow(1).
			FillWidth().
			Center().
			Radius(radiusControl).
			Clip()
		if a.previewSVGChecker {
			c1 := gui.Hex("#1E1E22")
			c2 := gui.Hex("#2A2A30")
			if !a.dark {
				c1 = gui.Hex("#F4F4F5")
				c2 = gui.Hex("#E4E4E7")
			}
			frame.Draw(func(p *gui.Painter, r gui.Rect) {
				const cell float32 = 16
				cols := int(r.W/cell) + 1
				rows := int(r.H/cell) + 1
				for y := 0; y < rows; y++ {
					for x := 0; x < cols; x++ {
						col := c1
						if (x+y)%2 == 1 {
							col = c2
						}
						p.Fill(gui.Rect{X: r.X + float32(x)*cell, Y: r.Y + float32(y)*cell, W: cell, H: cell}, col, 0)
					}
				}
			})
		}
		frame.Children(func() {
			gui.Image(c, a.previewSVG).Size(drawW, drawH).Fit(gui.Contain)
		})

		gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
			a.pillBadge(c, a.t("preview.svgVector"), a.pal.inkSecondary, a.pal.surface)
			a.toolButton(c, "svg-checker-toggle", iconChecker, a.t("preview.svgChecker"), a.previewSVGChecker).OnClick(func() {
				a.previewSVGChecker = !a.previewSVGChecker
			})
		})
	})
}

func (a *App) renderFontStage(c *gui.Context, asset model.Asset) {
	gui.Column(c).Fill().Gap(12).Children(func() {
		// Interactive font preview toolbar: custom sample text + point size switcher.
		gui.Row(c).FillWidth().Padding(8, 12).Gap(10).Radius(radiusPanel).Background(a.pal.surface).AlignItems(gui.Center).Children(func() {
			gui.Icon(c, iconType).Size(15, 15).TextColor(a.pal.inkSecondary)
			gui.TextInput(c.Key("font-sample-input"), &a.previewFontText).
				Placeholder(a.t("preview.fontSample")).
				Grow(1).
				OnChange(func() {
					a.renderFontPreviewAsync(asset)
				})
			sizes := []float64{24, 36, 48, 72}
			selIdx := 2
			for i, sz := range sizes {
				if math.Abs(a.previewFontSize-sz) < 4 {
					selIdx = i
					break
				}
			}
			a.arcPillTabs(c, "font-size-tabs", selIdx, []string{"24pt", "36pt", "48pt", "72pt"}, func(idx int) {
				a.previewFontSize = sizes[idx]
				a.renderFontPreviewAsync(asset)
			})
		})

		// Font specimen canvas.
		gui.Box(c).Grow(1).FillWidth().Radius(radiusControl).Clip().Center().Children(func() {
			if a.previewFontBitmap != nil {
				gui.Image(c, a.previewFontBitmap).Fill().Fit(gui.Contain).Radius(radiusControl)
			} else if a.previewLoading {
				gui.Text(c, a.t("common.loading")).FontSize(13).TextColor(a.pal.inkSecondary)
			}
		})

		// Metadata footer strip.
		if a.previewFontInfo.Family != "" {
			gui.Row(c).FillWidth().Padding(4, 8).Gap(10).AlignItems(gui.Center).Children(func() {
				a.pillBadge(c, a.previewFontInfo.Format, a.pal.ink, a.pal.surfaceMuted)
				gui.Text(c, a.previewFontInfo.Family+" "+a.previewFontInfo.Subfamily).FontSize(12).Bold().TextColor(a.pal.ink)
				gui.Spacer(c)
				gui.Text(c, fmt.Sprintf(a.t("preview.fontGlyphs"), a.previewFontInfo.NumGlyphs)).FontSize(11).TextColor(a.pal.inkMuted)
			})
		}
	})
}

func (a *App) renderAudioStage(c *gui.Context, asset model.Asset) {
	dur := a.previewAudioDur
	if dur <= 0 {
		dur = a.previewAudioWave.Duration
	}
	cur := a.previewAudioPos
	var progress float32
	if dur > 0 {
		progress = float32(cur / dur)
		if progress < 0 {
			progress = 0
		} else if progress > 1 {
			progress = 1
		}
	}

	gui.Column(c).Fill().Center().Gap(20).Padding(24).Children(func() {
		// Center audio emblem & title.
		gui.Row(c).Gap(14).AlignItems(gui.Center).Children(func() {
			gui.Box(c).Size(52, 52).Radius(radiusPanel).Background(a.pal.surface).Center().Children(func() {
				gui.Icon(c, iconMusic).Size(24, 24).TextColor(a.pal.ink)
			})
			gui.Column(c).Gap(4).Children(func() {
				gui.Text(c, a.assetName(asset)).FontSize(16).Bold().TextColor(a.pal.ink)
				meta := a.assetExt(asset)
				if a.previewAudioWave.SampleRate > 0 {
					meta += fmt.Sprintf(" · %d Hz", a.previewAudioWave.SampleRate)
				}
				if dur > 0 {
					meta += " · " + formatDuration(dur)
				}
				gui.Text(c, meta).FontSize(12).TextColor(a.pal.inkSecondary)
			})
		})

		// Interactive clickable 96-bar Waveform scrubber.
		peaks := a.previewAudioWave.Peaks
		waveBox := gui.Box(c.Key("audio-waveform-" + asset.ID)).
			FillWidth().
			MaxWidth(760).
			Height(144).
			Radius(radiusPanel).
			Background(a.pal.surface).
			Cursor(gui.CursorPointer).
			HandleInput(func(ev gui.InputEvent) bool {
				if ev.Kind == gui.InputPointerDown && ev.Button == 0 && a.previewAudio != nil && dur > 0 && a.previewWaveW > 0 {
					frac := float64(ev.X / a.previewWaveW)
					if frac < 0 {
						frac = 0
					} else if frac > 1 {
						frac = 1
					}
					target := frac * dur
					_ = a.previewAudio.Seek(target)
					a.previewAudioPos = target
					return true
				}
				return false
			}).
			Draw(func(p *gui.Painter, r gui.Rect) {
				if len(peaks) == 0 {
					return
				}
				padX := float32(18)
				usableW := r.W - padX*2
				if usableW <= 20 {
					return
				}
				bars := len(peaks)
				step := usableW / float32(bars)
				barW := step * 0.58
				if barW < 2 {
					barW = 2
				}
				midY := r.Y + r.H/2
				maxHalf := (r.H - 32) / 2
				playedCol := a.pal.controlOn
				restCol := a.pal.surfaceActive

				for i, pk := range peaks {
					amp := pk
					if amp < 0.08 {
						amp = 0.08
					}
					half := maxHalf * amp
					if half < 3 {
						half = 3
					}
					x := r.X + padX + float32(i)*step
					col := restCol
					if float32(i)/float32(bars) <= progress {
						col = playedCol
					}
					p.Fill(gui.Rect{X: x, Y: midY - half, W: barW, H: half * 2}, col, barW/2)
				}
				// Crisp playhead hairline.
				headX := r.X + padX + usableW*progress
				p.Fill(gui.Rect{X: headX - 1, Y: r.Y + 12, W: 2, H: r.H - 24}, playedCol, 1)
			})
		a.previewWaveW = waveBox.Bounds().W
		waveBox.Children(func() {})

		// Transport bar: Play/Pause + Time + Volume Slider.
		gui.Row(c).
			FillWidth().
			MaxWidth(760).
			Padding(10, 16).
			Gap(14).
			Radius(radiusPanel).
			Background(a.pal.surface).
			AlignItems(gui.Center).
			Children(func() {
				playIcon := iconPlay
				playLbl := a.t("preview.audioPlay")
				if a.previewAudioPlaying {
					playIcon = iconPause
					playLbl = a.t("preview.audioPause")
				}
				a.arcButton(c, "audio-play-toggle", playLbl, true).
					Disabled(a.previewAudio == nil).
					OnClick(func() { a.toggleAudioPlayback() })
				_ = playIcon

				gui.Text(c, fmt.Sprintf("%s / %s", formatDuration(cur), formatDuration(dur))).
					FontSize(12).
					Bold().
					TextColor(a.pal.ink)

				gui.Spacer(c)

				gui.Icon(c, iconVolume).Size(15, 15).TextColor(a.pal.inkSecondary)
				gui.Slider(c.Key("audio-vol-slider"), &a.previewAudioVol, 0, 1).
					Width(110).
					OnChange(func() {
						if a.previewAudio != nil {
							a.previewAudio.SetVolume(a.previewAudioVol)
						}
					})
			})
	})
}

func (a *App) renderPDFStage(c *gui.Context, asset model.Asset) {
	total := a.previewPDFPages
	if total < 1 {
		total = 1
	}
	page := a.previewPDFPage
	if page >= total {
		page = total - 1
	}

	gui.Column(c).Fill().Gap(10).Children(func() {
		gui.Box(c).Grow(1).FillWidth().Center().Clip().Children(func() {
			if a.previewPDFBitmap != nil {
				img := gui.Image(c, a.previewPDFBitmap).Fit(gui.Contain).Radius(6)
				if math.Abs(a.previewZoom-1.0) > 0.01 {
					baseW := float32(760)
					baseH := float32(580)
					img.Size(baseW*float32(a.previewZoom), baseH*float32(a.previewZoom))
				} else {
					img.Fill()
				}
			} else if a.previewLoading {
				gui.Text(c, a.t("preview.pdfLoading")).FontSize(13).TextColor(a.pal.inkSecondary)
			} else if a.previewVideoErr != "" {
				gui.Text(c, a.previewVideoErr).FontSize(12).TextColor(a.pal.inkSecondary)
			}
		})

		// Multi-page navigation bar.
		gui.Row(c).
			Padding(8, 14).
			Gap(10).
			Radius(radiusPanel).
			Background(a.pal.surface).
			AlignItems(gui.Center).
			Justify(gui.Center).
			Children(func() {
				a.toolButton(c, "pdf-prev-page", iconChevronLeft, a.t("preview.pdfPrev"), false).
					Disabled(page <= 0 || a.previewLoading).
					OnClick(func() {
						a.loadPDFPreview(asset, page-1)
					})
				a.pillBadge(c, fmt.Sprintf(a.t("preview.pdfPage"), page+1, total), a.pal.ink, a.pal.surfaceMuted)
				a.toolButton(c, "pdf-next-page", iconChevronRgt, a.t("preview.pdfNext"), false).
					Disabled(page+1 >= total || a.previewLoading).
					OnClick(func() {
						a.loadPDFPreview(asset, page+1)
					})
			})
	})
}

// loadSVGThumbAsync rasterizes an SVG asset via MyGo's native vector engine for
// Library card thumbnails (called when ensureBitmap detects an SVG).
func (a *App) loadSVGThumbAsync(id, path string) {
	if path == "" || a.bitmaps[id] != nil || a.loading[id] || a.win == nil {
		return
	}
	a.loading[id] = true
	win := a.win
	go func() {
		raw, err := os.ReadFile(path)
		var bm *gui.Bitmap
		if err == nil {
			if svg, serr := gui.ParseSVG(raw); serr == nil {
				img := svg.Image(400, 400, color.RGBA{240, 240, 242, 255})
				bm, err = rgbaToBitmap(img)
			} else {
				err = serr
			}
		}
		win.Update(func() {
			delete(a.loading, id)
			if a.lib == nil || err != nil || bm == nil {
				return
			}
			a.bitmaps[id] = bm
		})
	}()
}
