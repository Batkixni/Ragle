package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
	"raglex/internal/plugin"
	"raglex/internal/preview"
)

func imageToBitmap(img image.Image) (*gui.Bitmap, error) {
	if img == nil {
		return nil, os.ErrInvalid
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return gui.DecodeBitmap(buf.Bytes())
}

func rgbaToBitmap(img *image.RGBA) (*gui.Bitmap, error) {
	return imageToBitmap(img)
}

// loadNativeFormatPreview loads Font, Audio, SVG, PDF, or Plugin Document
// (e.g. Markdown) preview state when opening an asset. Returns true if handled.
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
	case model.IsPDF(asset.Mime, asset.Path) || model.IsIllustrator(asset.Mime, asset.Path):
		a.loadPDFPreview(asset, 0)
		return true
	case a.isPluginDocPreview(asset):
		a.loadPluginDocPreview(asset)
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
	a.previewDocTitle = ""
	a.previewDocText = ""
	a.previewDocDraft = ""
	a.previewDocErr = ""
	a.previewDocMarkdown = false
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

// --- Plugin Document & Markdown Preview (hook: preview_renderer) ---

func (a *App) isPluginDocPreview(asset model.Asset) bool {
	if a.plugins == nil || asset.Path == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(asset.Path))
	if ext == "" {
		return false
	}
	if _, ok := a.plugins.PreviewProviderForExt(ext); ok {
		return true
	}
	if a.isPluginEnabled("raglex.markdown-notes") && (ext == ".md" || ext == ".markdown" || ext == ".txt") {
		return true
	}
	return false
}

func (a *App) loadPluginDocPreview(asset model.Asset) {
	if a.plugins == nil || asset.Path == "" {
		return
	}
	ext := strings.ToLower(filepath.Ext(asset.Path))
	pluginID := ""
	if prov, ok := a.plugins.PreviewProviderForExt(ext); ok {
		pluginID = prov.Manifest.ID
	} else if a.isPluginEnabled("raglex.markdown-notes") && (ext == ".md" || ext == ".markdown" || ext == ".txt") {
		pluginID = "raglex.markdown-notes"
	} else {
		return
	}
	isMD := ext == ".md" || ext == ".markdown"
	title := a.assetName(asset)
	path := asset.Path
	targetID := asset.ID
	libRoot := a.cfg.LibraryPath
	plugins := a.plugins
	win := a.win

	readDirect := func() (string, string, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", "", err
		}
		if len(raw) > 96*1024 {
			raw = append(raw[:96*1024], []byte("\n\n… (truncated)")...)
		}
		// Strip UTF-8 BOM if present.
		raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
		return title, string(raw), nil
	}

	applyResult := func(docTitle, docText string, err error) {
		a.previewLoading = false
		if err != nil {
			a.previewDocErr = err.Error()
			return
		}
		a.previewDocTitle = docTitle
		a.previewDocText = docText
		a.previewDocDraft = docText
		if a.previewDocMode == "" {
			a.previewDocMode = "read"
		}
		a.previewDocMarkdown = isMD
		a.previewDocErr = ""
	}

	run := func() {
		var (
			docTitle string
			docText  string
			err      error
		)
		if pluginID == "raglex.markdown-notes" {
			docTitle, docText, err = readDirect()
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			resp, execErr := plugins.Execute(ctx, pluginID, plugin.Request{
				Hook:        "preview_renderer",
				LibraryPath: libRoot,
				Assets: []plugin.AssetPayload{{
					ID:     asset.ID,
					Name:   title,
					Path:   path,
					Mime:   asset.Mime,
					Rating: asset.Rating,
					Notes:  asset.Notes,
				}},
			})
			if execErr != nil {
				err = execErr
			} else if !resp.OK {
				err = fmt.Errorf("%s", resp.Error)
			} else {
				docTitle = resp.PreviewTitle
				if docTitle == "" {
					docTitle = title
				}
				docText = resp.PreviewText
			}
		}
		if win == nil {
			applyResult(docTitle, docText, err)
			return
		}
		win.Update(func() {
			if a.previewAssetID != targetID || (!a.previewOpen && !a.quickLookOpen) || a.isClosed() {
				return
			}
			applyResult(docTitle, docText, err)
		})
	}

	if win == nil {
		run()
		return
	}
	a.previewLoading = true
	go run()
}

type mdBlock struct {
	kind    string     // "h1", "h2", "h3", "h4", "code", "quote", "alert", "bullet", "task", "table", "hr", "p"
	text    string
	label   string     // bullet marker, code lang, or alert kind ("NOTE", "TIP", "IMPORTANT", "WARNING", "CAUTION")
	checked bool       // task checkbox state
	lineIdx int        // 0-based source line index for clickable task toggle
	rows    [][]string // parsed table rows
}

func parseMarkdownBlocks(src string) []mdBlock {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	var out []mdBlock
	var para []string
	flushPara := func() {
		if len(para) > 0 {
			out = append(out, mdBlock{kind: "p", text: cleanInlineMD(strings.Join(para, "\n"))})
			para = nil
		}
	}

	inCode := false
	codeLang := ""
	var codeLines []string

	// Parse optional YAML frontmatter at the top of Markdown files (---\n...\n---).
	startIdx := 0
	if len(lines) >= 3 && strings.TrimSpace(lines[0]) == "---" {
		for j := 1; j < len(lines) && j < 32; j++ {
			if strings.TrimSpace(lines[j]) == "---" {
				var fmLines []string
				for k := 1; k < j; k++ {
					if t := strings.TrimSpace(lines[k]); t != "" {
						fmLines = append(fmLines, t)
					}
				}
				if len(fmLines) > 0 {
					out = append(out, mdBlock{kind: "quote", text: strings.Join(fmLines, "\n")})
				}
				startIdx = j + 1
				break
			}
		}
	}

	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inCode {
				flushPara()
				inCode = true
				codeLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				codeLines = nil
			} else {
				out = append(out, mdBlock{kind: "code", label: codeLang, text: strings.Join(codeLines, "\n")})
				inCode = false
				codeLang = ""
				codeLines = nil
			}
			continue
		}
		if inCode {
			codeLines = append(codeLines, line)
			continue
		}
		if trimmed == "" {
			flushPara()
			continue
		}
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			flushPara()
			out = append(out, mdBlock{kind: "hr"})
			continue
		}
		// Markdown table block (lines containing | with a separator row like | --- | --- |).
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && i+1 < len(lines) && isMarkdownTableSep(lines[i+1]) {
			flushPara()
			var rows [][]string
			rows = append(rows, splitMarkdownTableRow(trimmed))
			i++ // skip separator line
			for i+1 < len(lines) {
				nextT := strings.TrimSpace(lines[i+1])
				if !strings.HasPrefix(nextT, "|") {
					break
				}
				i++
				rows = append(rows, splitMarkdownTableRow(nextT))
			}
			out = append(out, mdBlock{kind: "table", rows: rows})
			continue
		}
		// GitHub Alert block: > [!NOTE] / [!TIP] / [!IMPORTANT] / [!WARNING] / [!CAUTION]
		if strings.HasPrefix(trimmed, "> [!") {
			if closeB := strings.IndexByte(trimmed, ']'); closeB > 4 {
				alertKind := strings.ToUpper(strings.TrimSpace(trimmed[4:closeB]))
				flushPara()
				var body []string
				if rest := strings.TrimSpace(trimmed[closeB+1:]); rest != "" {
					body = append(body, cleanInlineMD(rest))
				}
				for i+1 < len(lines) {
					nt := strings.TrimSpace(lines[i+1])
					if !strings.HasPrefix(nt, ">") {
						break
					}
					i++
					body = append(body, cleanInlineMD(strings.TrimSpace(strings.TrimPrefix(nt, ">"))))
				}
				out = append(out, mdBlock{kind: "alert", label: alertKind, text: strings.Join(body, "\n")})
				continue
			}
		}
		// Task list checkbox: - [ ] or - [x] / * [ ] / * [x]
		if strings.HasPrefix(trimmed, "- [ ] ") || strings.HasPrefix(trimmed, "* [ ] ") {
			flushPara()
			out = append(out, mdBlock{kind: "task", checked: false, lineIdx: i, text: cleanInlineMD(strings.TrimSpace(trimmed[6:]))})
			continue
		}
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") ||
			strings.HasPrefix(trimmed, "* [x] ") || strings.HasPrefix(trimmed, "* [X] ") {
			flushPara()
			out = append(out, mdBlock{kind: "task", checked: true, lineIdx: i, text: cleanInlineMD(strings.TrimSpace(trimmed[6:]))})
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "#### "):
			flushPara()
			out = append(out, mdBlock{kind: "h4", text: cleanInlineMD(strings.TrimSpace(trimmed[5:]))})
		case strings.HasPrefix(trimmed, "### "):
			flushPara()
			out = append(out, mdBlock{kind: "h3", text: cleanInlineMD(strings.TrimSpace(trimmed[4:]))})
		case strings.HasPrefix(trimmed, "## "):
			flushPara()
			out = append(out, mdBlock{kind: "h2", text: cleanInlineMD(strings.TrimSpace(trimmed[3:]))})
		case strings.HasPrefix(trimmed, "# "):
			flushPara()
			out = append(out, mdBlock{kind: "h1", text: cleanInlineMD(strings.TrimSpace(trimmed[2:]))})
		case strings.HasPrefix(trimmed, ">"):
			flushPara()
			var qLines []string
			qLines = append(qLines, cleanInlineMD(strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))))
			for i+1 < len(lines) {
				nt := strings.TrimSpace(lines[i+1])
				if !strings.HasPrefix(nt, ">") || strings.HasPrefix(nt, "> [!") {
					break
				}
				i++
				qLines = append(qLines, cleanInlineMD(strings.TrimSpace(strings.TrimPrefix(nt, ">"))))
			}
			out = append(out, mdBlock{kind: "quote", text: strings.Join(qLines, "\n")})
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ "):
			flushPara()
			out = append(out, mdBlock{kind: "bullet", label: "•", text: cleanInlineMD(strings.TrimSpace(trimmed[2:]))})
		default:
			if dot := strings.Index(trimmed, ". "); dot > 0 && dot <= 3 {
				allDigits := true
				for k := 0; k < dot; k++ {
					if trimmed[k] < '0' || trimmed[k] > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					flushPara()
					out = append(out, mdBlock{kind: "bullet", label: trimmed[:dot+1], text: cleanInlineMD(strings.TrimSpace(trimmed[dot+2:]))})
					continue
				}
			}
			para = append(para, line)
		}
	}
	if inCode && len(codeLines) > 0 {
		out = append(out, mdBlock{kind: "code", label: codeLang, text: strings.Join(codeLines, "\n")})
	}
	flushPara()
	return out
}

func isMarkdownTableSep(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.Contains(t, "|") || !strings.Contains(t, "-") {
		return false
	}
	for _, r := range t {
		if r != '|' && r != '-' && r != ':' && r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

func splitMarkdownTableRow(line string) []string {
	t := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(t, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = cleanInlineMD(strings.TrimSpace(p))
	}
	return out
}

func countDocChars(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' && r != '\n' && r != '\r' && r != '\t' {
			n++
		}
	}
	return n
}

func (a *App) docHasUnsavedChanges() bool {
	return a.previewDocDraft != a.previewDocText
}

func (a *App) saveDocumentPreview(asset model.Asset) {
	if a.lib == nil || asset.ID == "" {
		return
	}
	updated, err := a.lib.SaveTextAsset(asset.ID, a.previewDocDraft)
	if err != nil {
		a.notify(err.Error())
		return
	}
	a.previewDocText = a.previewDocDraft
	a.updateAssetInMemory(updated)
	a.notify(a.t("doc.saved"))
}

func (a *App) toggleMarkdownTaskLine(asset model.Asset, lineIdx int) {
	normalized := strings.ReplaceAll(a.previewDocDraft, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if lineIdx < 0 || lineIdx >= len(lines) {
		return
	}
	line := lines[lineIdx]
	switch {
	case strings.Contains(line, "[ ] "):
		lines[lineIdx] = strings.Replace(line, "[ ] ", "[x] ", 1)
	case strings.Contains(line, "[x] "):
		lines[lineIdx] = strings.Replace(line, "[x] ", "[ ] ", 1)
	case strings.Contains(line, "[X] "):
		lines[lineIdx] = strings.Replace(line, "[X] ", "[ ] ", 1)
	default:
		return
	}
	a.previewDocDraft = strings.Join(lines, "\n")
	a.saveDocumentPreview(asset)
}

func cleanInlineMD(s string) string {
	for {
		openB := strings.IndexByte(s, '[')
		if openB < 0 {
			break
		}
		closeB := strings.Index(s[openB:], "](")
		if closeB < 0 {
			break
		}
		closeB += openB
		closeP := strings.IndexByte(s[closeB+2:], ')')
		if closeP < 0 {
			break
		}
		closeP += closeB + 2
		label := s[openB+1 : closeB]
		s = s[:openB] + label + s[closeP+1:]
	}
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}

// renderDocumentStage renders a Liubai-inspired borderless writing & reading desk
// directly on the canvas — zero outer module frame, generous breathing whitespace,
// Read / Split / Edit modes, Paragraph Focus dimming, and direct disk saving.
func (a *App) renderDocumentStage(c *gui.Context, asset model.Asset) {
	if a.previewLoading {
		gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.Spinner(c)
			gui.Text(c, a.t("preview.loading")).FontSize(13).TextColor(a.pal.inkSecondary)
		})
		return
	}
	if a.previewDocErr != "" {
		gui.Column(c).Center().Gap(10).Children(func() {
			gui.Icon(c, iconFileText).Size(32, 32).TextColor(a.pal.inkMuted)
			gui.Text(c, a.previewDocErr).FontSize(12).TextColor(a.pal.inkSecondary)
		})
		return
	}

	mode := a.previewDocMode
	if mode == "" {
		mode = "read"
	}

	renderEditorPane := func(key string, centered bool) {
		gui.Scroll(c.Key(key)).Fill().Children(func() {
			wrap := gui.Column(c).FillWidth().Padding(36, 44, 80, 44)
			if centered {
				wrap.AlignItems(gui.Center)
			}
			wrap.Children(func() {
				edCol := gui.Column(c).FillWidth()
				if centered {
					edCol.MaxWidth(740)
				}
				edCol.Children(func() {
					gui.TextAreaBase(c.Key("doc-editor-"+asset.ID), &a.previewDocDraft).
						Placeholder(a.t("doc.empty")).
						FillWidth().
						MinHeight(560).
						FontSize(14.5).
						LineHeight(1.85).
						TextColor(a.pal.ink).
						Background(gui.Transparent)
				})
			})
		})
	}

	renderReadPane := func(key string, centered bool) {
		services := c.Services()
		gui.Scroll(c.Key(key)).Fill().Children(func() {
			wrap := gui.Column(c).FillWidth().Padding(36, 44, 84, 44)
			if centered {
				wrap.AlignItems(gui.Center)
			}
			wrap.Children(func() {
				col := gui.Column(c).FillWidth().Gap(14)
				if centered {
					col.MaxWidth(740)
				}
				col.Children(func() {
					content := a.previewDocDraft
					if strings.TrimSpace(content) == "" {
						gui.ButtonBase(c.Key("doc-empty-edit")).
							FillWidth().
							Padding(12, 0).
							OnClick(func() { a.previewDocMode = "edit" }).
							Children(func() {
								gui.Text(c, a.t("doc.empty")).FontSize(14).TextColor(a.pal.inkMuted)
							})
						return
					}

					if !a.previewDocMarkdown {
						gui.Text(c, content).FontSize(14.5).TextColor(a.pal.ink).LineHeight(1.85)
						return
					}

					blocks := parseMarkdownBlocks(content)
					for i, blk := range blocks {
						b := blk
						blkBox := gui.Box(c.Key(fmt.Sprintf("md-blk-%s-%d", key, i))).FillWidth()
						if a.previewDocFocus {
							targetOp := float32(0.36)
							if blkBox.Hovered() {
								targetOp = 1.0
							}
							blkBox.Opacity(blkBox.AnimateTo("fop", targetOp, animFast))
						}
						blkBox.Children(func() {
							switch b.kind {
							case "h1":
								gui.Column(c).FillWidth().Padding(10, 0, 6, 0).Gap(8).Children(func() {
									gui.Text(c, b.text).FontSize(26).Bold().TextColor(a.pal.ink).LineHeight(1.3)
									gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
								})
							case "h2":
								gui.Box(c).FillWidth().Padding(12, 0, 2, 0).Children(func() {
									gui.Text(c, b.text).FontSize(19.5).Bold().TextColor(a.pal.ink).LineHeight(1.35)
								})
							case "h3":
								gui.Box(c).FillWidth().Padding(6, 0, 0, 0).Children(func() {
									gui.Text(c, b.text).FontSize(16).Bold().TextColor(a.pal.ink).LineHeight(1.4)
								})
							case "h4":
								gui.Box(c).FillWidth().Padding(4, 0, 0, 0).Children(func() {
									gui.Text(c, b.text).FontSize(14.5).Bold().TextColor(a.pal.inkSecondary).LineHeight(1.45)
								})
							case "hr":
								gui.Box(c).FillWidth().Padding(10, 0).Children(func() {
									gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
								})
							case "quote":
								gui.Row(c).FillWidth().Padding(4, 0, 4, 2).Gap(14).Children(func() {
									gui.Box(c).Width(2.5).FillHeight().Radius(2).Background(a.pal.controlOn)
									gui.Text(c, b.text).FontSize(14.5).TextColor(a.pal.inkSecondary).LineHeight(1.8).Grow(1)
								})
							case "alert":
								accent := a.pal.controlOn
								switch b.label {
								case "NOTE":
									accent = gui.Hex("#38BDF8")
								case "TIP":
									accent = gui.Hex("#22C55E")
								case "IMPORTANT":
									accent = gui.Hex("#A855F7")
								case "WARNING":
									accent = gui.Hex("#F59E0B")
								case "CAUTION":
									accent = a.pal.danger
								}
								gui.Row(c).FillWidth().Padding(4, 0, 4, 2).Gap(14).Children(func() {
									gui.Box(c).Width(3).FillHeight().Radius(2).Background(accent)
									gui.Column(c).Grow(1).Gap(4).Children(func() {
										gui.Text(c, b.label).FontSize(11).Bold().TextColor(accent)
										if b.text != "" {
											gui.Text(c, b.text).FontSize(14).TextColor(a.pal.inkSecondary).LineHeight(1.75)
										}
									})
								})
							case "code":
								codeText := b.text
								codeWrap := gui.Column(c).
									FillWidth().
									Padding(14, 16).
									Gap(8).
									Radius(radiusControl).
									Background(a.pal.surface)
								codeWrap.Children(func() {
									gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
										lbl := b.label
										if lbl == "" {
											lbl = "code"
										}
										gui.Text(c, lbl).FontSize(10.5).Bold().TextColor(a.pal.inkMuted).Grow(1)
										a.miniHeaderButton(c, fmt.Sprintf("md-cp-%s-%d", key, i), iconCopy, a.t("doc.copyText"), false).OnClick(func() {
											services.WriteClipboard(codeText)
											a.notify(a.t("common.copied"))
										})
									})
									gui.Text(c, codeText).FontSize(13).TextColor(a.pal.ink).LineHeight(1.65)
								})
							case "task":
								lineIdx := b.lineIdx
								checked := b.checked
								gui.ButtonBase(c.Key(fmt.Sprintf("md-task-%s-%d", key, i))).
									FillWidth().
									Padding(2, 0).
									Gap(10).
									AlignItems(gui.Center).
									OnClick(func() { a.toggleMarkdownTaskLine(asset, lineIdx) }).
									Children(func() {
										chkBg := a.pal.surfaceMuted
										if checked {
											chkBg = a.pal.controlOn
										}
										gui.Box(c).Size(16, 16).Radius(4).Background(chkBg).Center().Children(func() {
											if checked {
												gui.Icon(c, iconCheck).Size(10, 10).TextColor(a.pal.controlGlyph)
											}
										})
										txtCol := a.pal.ink
										if checked {
											txtCol = a.pal.inkMuted
										}
										gui.Text(c, b.text).FontSize(14.5).TextColor(txtCol).LineHeight(1.75).Grow(1)
									})
							case "table":
								gui.Column(c).FillWidth().Gap(0).Children(func() {
									for rIdx, row := range b.rows {
										isHeader := rIdx == 0
										rBox := gui.Row(c).FillWidth().Padding(8, 6).Gap(12).BorderWidth(0, 0, 1, 0).BorderColor(a.chromeHairline())
										rBox.Children(func() {
											for _, cell := range row {
												txt := gui.Text(c, cell).FontSize(13.5).LineHeight(1.5).Grow(1)
												if isHeader {
													txt.Bold().TextColor(a.pal.ink)
												} else {
													txt.TextColor(a.pal.inkSecondary)
												}
											}
										})
									}
								})
							case "bullet":
								gui.Row(c).FillWidth().Padding(0, 0, 0, 4).Gap(10).AlignItems(gui.Start).Children(func() {
									gui.Text(c, b.label).FontSize(14.5).Bold().TextColor(a.pal.controlOn)
									gui.Text(c, b.text).FontSize(14.5).TextColor(a.pal.ink).LineHeight(1.8).Grow(1)
								})
							default:
								gui.Text(c, b.text).FontSize(14.5).TextColor(a.pal.ink).LineHeight(1.85)
							}
						})
					}
				})
			})
		})
	}

	switch mode {
	case "edit":
		renderEditorPane("doc-edit-scroll-"+asset.ID, true)
	case "split":
		gui.Row(c.Key("doc-split-"+asset.ID)).Fill().Children(func() {
			gui.Box(c.Key("doc-split-left")).Grow(1).FillHeight().Clip().Children(func() {
				renderEditorPane("doc-split-edit-"+asset.ID, false)
			})
			gui.Box(c).Width(1).FillHeight().Background(a.chromeHairline())
			gui.Box(c.Key("doc-split-right")).Grow(1).FillHeight().Clip().Children(func() {
				renderReadPane("doc-split-read-"+asset.ID, false)
			})
		})
	default: // "read"
		renderReadPane("doc-read-scroll-"+asset.ID, true)
	}
}
