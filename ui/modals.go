package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	gui "github.com/egoist/mygo/ui"

	"raglex/internal/config"
	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/sources"
	"raglex/internal/watcher"
)

func (a *App) styledDialog(c *gui.Context, key string, open *bool, w, h float32, fn func(panel gui.Element)) {
	gui.DialogBase(c.Key(key), open, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.56))
		panel.
			Radius(radiusSurface).
			Clip().
			Background(a.popoverSurface()).
			Border(1, a.chromeHairline()).
			Shadow(0, 22, 54, 0, a.pal.shadowFloating)
		if w > 0 {
			panel.Width(w)
		}
		if h > 0 {
			panel.Height(h)
		}
		applyBlurReveal(panel, key, 0)
		fn(panel)
	})
}

// quickLookModal is the 1st-Space floating Quick Look pop-up overlay.
// Pressing Space a second time (or clicking Full View) promotes directly to
// the full-stage inline viewer without reloading media.
func (a *App) quickLookModal(c *gui.Context) {
	if !a.quickLookOpen {
		return
	}
	asset, ok := a.assetByID[a.previewAssetID]
	if !ok {
		a.closePreview()
		return
	}
	t := c.Theme()
	services := c.Services()
	isVideo := model.MediaKind(asset.Mime) == model.MediaVideo
	isSVG := model.IsSVG(asset.Mime, asset.Path)
	isFont := model.IsFont(asset.Mime, asset.Path)
	isAudio := model.IsAudio(asset.Mime, asset.Path)
	isPDF := model.IsPDF(asset.Mime, asset.Path)

	a.styledDialog(c, "quicklook-modal", &a.quickLookOpen, 920, 660, func(panel gui.Element) {
		panel.OnShortcut(0, gui.KeyLeft, func() { a.stepPreview(-1) })
		panel.OnShortcut(0, gui.KeyRight, func() { a.stepPreview(1) })
		panel.OnShortcut(0, gui.KeySpace, func() {
			a.promoteQuickLookToFullPreview()
		})

		gui.Column(c).Fill().Children(func() {
			// Header bar.
			gui.Row(c).
				Height(52).
				Padding(0, 18).
				Gap(8).
				AlignItems(gui.Center).
				Children(func() {
					gui.Text(c, a.assetName(asset)).Bold().FontSize(14).Grow(1).SingleLine()
					if asset.Duration > 0 {
						a.pillBadge(c, formatDuration(asset.Duration), t.TextMuted, a.pal.surface)
					}
					if asset.Width > 0 && asset.Height > 0 {
						a.pillBadge(c, fmt.Sprintf("%d × %d", asset.Width, asset.Height), t.TextMuted, a.pal.surface)
					}
					a.pillBadge(c, a.assetExt(asset), t.Text, a.pal.surface)
					if len(a.assets) > 1 {
						a.iconButton(c, "ql-prev-img", iconChevronLeft, a.t("preview.prev"), false).OnClick(func() {
							a.stepPreview(-1)
						})
						a.iconButton(c, "ql-next-img", iconChevronRgt, a.t("preview.next"), false).OnClick(func() {
							a.stepPreview(1)
						})
					}
					a.toolButton(c, "ql-full-view", iconMaximize, a.t("preview.fullView"), false).OnClick(func() {
						a.promoteQuickLookToFullPreview()
					})
					if !asset.Missing && asset.Path != "" {
						a.iconButton(c, "ql-open", iconExternal, a.t("common.openFile"), false).OnClick(func() {
							_ = openLocalFile(asset.Path)
						})
					}
					a.iconButton(c, "ql-copy", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
						services.WriteClipboard(asset.Path)
						a.notify(a.t("common.copied"))
					})
					a.iconButton(c, "ql-close", iconX, a.t("common.close"), false).OnClick(func() {
						a.closePreview()
					})
				})

			// Artwork / Native Format stage.
			stageBg := a.pal.canvas
			if a.dark {
				stageBg = gui.Hex("#0B0B0D")
			}
			gui.Column(c).Grow(1).FillWidth().Background(stageBg).Center().Padding(18).Gap(12).Children(func() {
				switch {
				case isSVG:
					a.renderSVGStage(c, asset)
				case isFont:
					a.renderFontStage(c, asset)
				case isAudio:
					a.renderAudioStage(c, asset)
				case isPDF:
					a.renderPDFStage(c, asset)
				case a.previewVideo != nil:
					gui.Box(c).Grow(1).FillWidth().Center().Children(func() {
						gui.Video(c.Key("ql-video-"+asset.ID), a.previewVideo).
							Fit(gui.Contain).
							Fill().
							Radius(8)
					})
				case a.previewLoading:
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Spinner(c)
						gui.Text(c, a.t("preview.loading")).TextColor(t.TextMuted)
					})
				case isVideo:
					gui.Column(c).Center().Gap(10).Children(func() {
						if bm := a.bitmaps[asset.ID]; bm != nil {
							gui.Image(c, bm).Fit(gui.Contain).MaxHeight(380).Radius(8)
						} else {
							gui.Icon(c, iconVideo).Size(32, 32).TextColor(a.pal.inkMuted)
						}
						if a.previewVideoErr != "" {
							gui.Text(c, a.previewVideoErr).FontSize(12).TextColor(a.pal.inkSecondary).TextAlign(gui.Center)
							if !a.ffmpeg {
								dlLabel := a.t("settings.ffmpegDownload")
								if a.ffmpegInstalling {
									dlLabel = fmt.Sprintf(a.t("settings.ffmpegDownloading"), a.ffmpegInstallProgress)
								}
								a.arcButton(c, "ql-install-ff", dlLabel, true).OnClick(func() {
									a.installFFmpegAsync()
								})
							}
						}
					})
				default:
					bm := a.previewBitmap
					if bm == nil {
						bm = a.bitmaps[asset.ID]
					}
					if bm != nil {
						gui.Image(c, bm).Fit(gui.Contain).Grow(1).Radius(6)
					} else {
						gui.Column(c).Center().Gap(8).Children(func() {
							gui.Icon(c, iconFile).Size(40, 40).TextColor(a.pal.inkFaint)
							gui.Text(c, a.assetExt(asset)).FontSize(16).Bold().TextColor(a.pal.inkMuted)
						})
					}
				}
			})

			// Footer: rating + applied tinted tags + Space-to-promote hint.
			gui.Row(c).
				Padding(12, 18).
				Gap(12).
				AlignItems(gui.Center).
				Children(func() {
					a.syncInspectorSelection(false)
					if a.inspectorAssetID == asset.ID {
						gui.Rating(c.Key("ql-rating-"+asset.ID), &a.inspectorRating, 5).OnChange(func() {
							if err := a.lib.SetRating(asset.ID, a.inspectorRating); err == nil {
								a.reload()
							}
						})
					}
					gui.Row(c).Gap(6).Grow(1).AlignItems(gui.Center).Children(func() {
						for _, tg := range a.assetTags[asset.ID] {
							a.tintedTagChip(c, "ql-tag-"+tg.ID, tg, nil)
						}
					})
					gui.Text(c, a.t("preview.quickLookHint")).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
				})
		})
	})
}

func formatDuration(sec float64) string {
	total := int(sec + 0.5)
	if total < 0 {
		total = 0
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func (a *App) newProjectModal(c *gui.Context) {
	if !a.newProjOpen {
		return
	}
	t := c.Theme()
	a.styledDialog(c, "new-proj-modal", &a.newProjOpen, 450, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("project.createTitle")).FontSize(16).Bold().Grow(1)
				a.iconButton(c, "np-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.newProjOpen = false
				})
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("project.new"))
				gui.TextInput(c.Key("np-name"), &a.newProjectName).
					Placeholder(a.t("project.namePlaceholder")).
					AutoFocus().
					FillWidth().
					OnSubmit(func() { a.createProjectFromModal() })
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("project.description"))
				gui.TextInput(c.Key("np-desc"), &a.newProjectDesc).
					Placeholder(a.t("project.descPlaceholder")).
					FillWidth()
			})

			gui.Column(c).Gap(8).Children(func() {
				a.sectionLabel(c, a.t("project.pickTags"))
				if len(a.tags) == 0 {
					gui.Text(c, a.t("project.needTag")).FontSize(12).TextColor(t.TextMuted)
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						gui.TextInput(c.Key("np-inline-tag"), &a.newTagName).
							Placeholder(a.t("tag.new")).
							Grow(1).
							OnSubmit(func() {
								a.createTag()
								if a.tagID != "" {
									a.newProjTags[a.tagID] = true
								}
							})
						gui.Button(c.Key("np-inline-tag-btn"), a.t("tag.create")).OnClick(func() {
							a.createTag()
							if a.tagID != "" {
								a.newProjTags[a.tagID] = true
							}
						})
					})
				} else {
					gui.Row(c).Gap(6).Wrap().Children(func() {
						for _, tag := range a.tags {
							tg := tag
							active := a.newProjTags[tg.ID]
							a.tagChip(c, "np-tag-"+tg.ID, tg.Name, active).OnClick(func() {
								a.newProjTags[tg.ID] = !active
							})
						}
					})
				}
			})

			gui.Divider(c)
			gui.Row(c).Gap(8).Justify(gui.End).Children(func() {
				gui.Button(c.Key("np-cancel"), a.t("common.cancel")).OnClick(func() {
					a.newProjOpen = false
				})
				gui.PrimaryButton(c.Key("np-submit"), a.t("project.create")).OnClick(func() {
					a.createProjectFromModal()
				})
			})
		})
	})
}

func (a *App) editProjectModal(c *gui.Context) {
	if !a.editProjOpen || a.projectID == "" {
		return
	}
	a.styledDialog(c, "edit-proj-modal", &a.editProjOpen, 440, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("project.edit")).FontSize(16).Bold().Grow(1)
				a.iconButton(c, "ep-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.editProjOpen = false
				})
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("project.new"))
				gui.TextInput(c.Key("ep-name"), &a.newProjectName).
					AutoFocus().
					FillWidth().
					OnSubmit(func() { a.saveProjectFromModal() })
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("project.description"))
				gui.TextInput(c.Key("ep-desc"), &a.newProjectDesc).
					Placeholder(a.t("project.descPlaceholder")).
					FillWidth().
					OnSubmit(func() { a.saveProjectFromModal() })
			})

			if len(a.tags) > 0 {
				gui.Column(c).Gap(8).Children(func() {
					a.sectionLabel(c, a.t("project.pickTags"))
					gui.Row(c).Gap(6).Wrap().Children(func() {
						for _, tag := range a.tags {
							tg := tag
							active := a.newProjTags[tg.ID]
							a.coloredTagChip(c, "ep-tag-"+tg.ID, tg, active).OnClick(func() {
								a.newProjTags[tg.ID] = !active
							})
						}
					})
				})
			}

			gui.Divider(c)
			gui.Row(c).Gap(8).Justify(gui.End).Children(func() {
				gui.Button(c.Key("ep-cancel"), a.t("common.cancel")).OnClick(func() {
					a.editProjOpen = false
				})
				gui.PrimaryButton(c.Key("ep-submit"), a.t("common.save")).OnClick(func() {
					a.saveProjectFromModal()
				})
			})
		})
	})
}

func (a *App) editTagModal(c *gui.Context) {
	if !a.editTagOpen || a.editTagID == "" {
		return
	}
	a.styledDialog(c, "edit-tag-modal", &a.editTagOpen, 440, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("tags.edit")).FontSize(16).Bold().Grow(1)
				a.iconButton(c, "et-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.editTagOpen = false
				})
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("tag.new"))
				gui.TextInput(c.Key("et-name"), &a.editTagName).
					AutoFocus().
					FillWidth().
					OnSubmit(func() { a.saveEditTag() })
			})

			// Parent tag selector (excludes self & own descendants to prevent cycles).
			forbidden := a.tagDescendantSet(a.editTagID)
			parentLabel := a.t("tags.noParent")
			if a.editTagParentID != "" {
				if p, ok := a.tagByID[a.editTagParentID]; ok {
					parentLabel = p.Name
				}
			}
			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("tags.parent"))
				gui.PopoverMenuButton(c.Key("et-parent"), parentLabel, func(m *gui.Menu) {
					if a.menuRadio(m, a.t("tags.noParent"), a.editTagParentID == "").Chosen() {
						a.editTagParentID = ""
					}
					m.Separator()
					for _, tg := range a.tags {
						cand := tg
						if forbidden[cand.ID] {
							continue
						}
						if a.menuRadio(m, cand.Name, a.editTagParentID == cand.ID).Chosen() {
							a.editTagParentID = cand.ID
						}
					}
				}, a.popoverMenuBtnOpts()).FillWidth()
			})

			gui.Column(c).Gap(8).Children(func() {
				a.sectionLabel(c, a.t("tags.color"))
				a.tagColorDots(c, "et-col", a.editTagColor, func(hex string) {
					a.editTagColor = hex
				})
			})

			gui.Divider(c)
			gui.Row(c).Gap(8).Justify(gui.End).Children(func() {
				a.arcButton(c, "et-cancel", a.t("common.cancel"), false).OnClick(func() {
					a.editTagOpen = false
				})
				a.arcButton(c, "et-submit", a.t("common.save"), true).OnClick(func() {
					a.saveEditTag()
				})
			})
		})
	})
}

func (a *App) webImportModal(c *gui.Context) {
	if !a.webImportOpen {
		return
	}
	a.styledDialog(c, "web-import", &a.webImportOpen, 540, 0, func(panel gui.Element) {
		panel.Padding(18, 20).Children(func() {
			gui.Column(c).FillWidth().Gap(8).Children(func() {
				// Clean single-purpose header for Web Sources.
				gui.Row(c).FillWidth().Padding(0, 4, 6, 4).Gap(10).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconGlobe).Size(17, 17).TextColor(a.pal.ink)
					gui.Text(c, a.t("webimport.title")).FontSize(15).Bold().SingleLine()
					gui.Spacer(c)
					a.iconButton(c, "web-imp-close", iconX, a.t("common.close"), false).OnClick(func() {
						if !a.webImportBusy {
							a.webImportOpen = false
						}
					})
				})

				gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())

				// Multi-Platform Web Sources (YouTube, oEmbed/JSON-LD, RSS/Atom, Direct Images).
				gui.Column(c).FillWidth().Padding(6, 4, 2, 4).Gap(10).Children(func() {
					gui.Text(c, a.t("webimport.hint")).
						FontSize(12).
						TextColor(a.pal.inkSecondary).
						LineHeight(1.45)

					// Supported source badges strip.
					gui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
						a.pillBadge(c, "YouTube", a.pal.inkSecondary, a.pal.surfaceMuted)
						a.pillBadge(c, "oEmbed / JSON-LD", a.pal.inkSecondary, a.pal.surfaceMuted)
						a.pillBadge(c, "RSS / Atom / JSON Feed", a.pal.inkSecondary, a.pal.surfaceMuted)
						a.pillBadge(c, a.t("webimport.badge"), a.pal.inkSecondary, a.pal.surfaceMuted)
					})

					gui.TextArea(c.Key("web-imp-url"), &a.webImportURL).
						Placeholder(a.t("webimport.placeholder")).
						FillWidth().
						Height(108)

					// Live zero-network source detection summary as user pastes links.
					urls := sources.ParseURLList(a.webImportURL)
					if len(urls) > 0 {
						counts := map[string]int{}
						for _, u := range urls {
							counts[sources.DetectSourceKind(u)]++
						}
						gui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
							gui.Text(c, fmt.Sprintf(a.t("webimport.detected"), len(urls))).
								FontSize(11.5).
								Bold().
								TextColor(a.pal.ink)
							if n := counts["youtube"]; n > 0 {
								a.pillBadge(c, fmt.Sprintf("YouTube ×%d", n), gui.Hex("#FF5C33"), gui.RGBA(255, 92, 51, 0.14))
							}
							if n := counts["feed"]; n > 0 {
								a.pillBadge(c, fmt.Sprintf("RSS/Feed ×%d", n), gui.Hex("#38BDF8"), gui.RGBA(56, 189, 248, 0.14))
							}
							if n := counts["image"]; n > 0 {
								a.pillBadge(c, fmt.Sprintf("Image ×%d", n), gui.Hex("#10B981"), gui.RGBA(16, 185, 129, 0.14))
							}
							if n := counts["web"]; n > 0 {
								a.pillBadge(c, fmt.Sprintf("Media/Web ×%d", n), a.pal.inkSecondary, a.pal.surfaceMuted)
							}
						})
					}
				})

				gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())

				// Flat toggle options (matching Preferences style).
				a.settingsSwitchRowTip(c, "web-imp-autotag", iconTag, a.t("webimport.autoTag"), a.t("webimport.autoTagTip"), &a.webImportAutoTag, nil)
				a.settingsSwitchRowTip(c, "web-imp-autostack", iconLayers, a.t("webimport.autoStack"), a.t("webimport.autoStackTip"), &a.webImportAutoStack, nil)

				if a.webImportBusy {
					prog := a.webImportProgress
					if prog == "" {
						prog = a.t("webimport.fetching")
					}
					gui.Row(c).Padding(2, 10).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Text(c, prog).FontSize(12).TextColor(a.pal.inkSecondary).SingleLine()
					})
				}

				gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())

				gui.Row(c).FillWidth().Padding(6, 4, 0, 4).Gap(8).Justify(gui.End).Children(func() {
					a.arcButton(c, "web-imp-cancel", a.t("common.cancel"), false).OnClick(func() {
						if !a.webImportBusy {
							a.webImportOpen = false
						}
					})
					lbl := a.t("webimport.action")
					if a.webImportBusy {
						lbl = a.t("webimport.fetching")
					}
					a.arcButton(c, "web-imp-go", lbl, true).
						Disabled(a.webImportBusy || len(sources.ParseURLList(a.webImportURL)) == 0).
						OnClick(func() {
							a.startWebImport()
						})
				})
			})
		})
	})
}

func (a *App) startWebImport() {
	if a.webImportBusy || a.lib == nil {
		return
	}
	urls := sources.ParseURLList(a.webImportURL)
	if len(urls) == 0 {
		return
	}
	a.webImportBusy = true
	a.webImportProgress = a.t("webimport.fetching")
	win := a.win
	lib := a.lib
	autoTag := a.webImportAutoTag
	autoStack := a.webImportAutoStack
	var activeTagIDs []string
	if a.autoTagOnImport && a.tagID != "" && a.tagID != "untagged" {
		activeTagIDs = []string{a.tagID}
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		res, err := lib.ImportSourcesFromURLs(ctx, urls, library.SourceImportOptions{
			TagIDs:              activeTagIDs,
			AutoPlatformTag:     autoTag,
			AutoStackMultiImage: autoStack,
			OnProgress: func(done, total int, label string) {
				if win == nil {
					return
				}
				win.Update(func() {
					if !a.isClosed() && total > 1 {
						a.webImportProgress = fmt.Sprintf(a.t("webimport.progress"), done+1, total)
					}
				})
			},
		})
		finish := func() {
			a.webImportBusy = false
			a.webImportProgress = ""
			if err != nil && len(res.Assets) == 0 {
				a.notify(fmt.Sprintf(a.t("webimport.failed"), err.Error()))
				return
			}
			a.webImportOpen = false
			a.webImportURL = ""
			if a.thumbs != nil {
				for i, ast := range res.Assets {
					if i < 6 {
						a.thumbs.Prioritize(ast.ID)
					}
				}
			}
			a.reload()
			if a.autoSelectImported && len(res.Assets) > 0 {
				firstID := res.Assets[0].ID
				for i, it := range a.assets {
					if it.ID == firstID {
						a.assetIndex = i
						a.selectedIDs = map[string]bool{firstID: true}
						break
					}
				}
			}
			switch {
			case len(res.Assets) == 1 && res.StackCount == 0:
				a.notify(fmt.Sprintf(a.t("webimport.done"), a.assetName(res.Assets[0])))
			case res.StackCount > 0:
				a.notify(fmt.Sprintf(a.t("webimport.batchDone"), len(res.Assets), res.StackCount))
			default:
				a.notify(fmt.Sprintf(a.t("webimport.batchDoneSimple"), len(res.Assets)))
			}
		}
		if win != nil && !a.isClosed() {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}

// watchFoldersModal renders a dedicated, clean Watch Folders manager opened
// directly from the "+ Import" menu right next to "Import Folder".
func (a *App) watchFoldersModal(c *gui.Context) {
	if !a.watchFoldersOpen {
		return
	}
	a.styledDialog(c, "watch-folders-modal", &a.watchFoldersOpen, 520, 0, func(panel gui.Element) {
		panel.Padding(18, 20).Children(func() {
			gui.Column(c).FillWidth().Gap(8).Children(func() {
				gui.Row(c).FillWidth().Padding(0, 4, 6, 4).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconRefresh).Size(16, 16).TextColor(a.pal.ink)
					gui.Text(c, a.t("watch.section")).FontSize(15).Bold().SingleLine()
					a.helpTip(c, a.t("watch.tip"))
					gui.Spacer(c)
					a.iconButton(c, "wf-modal-close", iconX, a.t("common.close"), false).OnClick(func() {
						a.watchFoldersOpen = false
					})
				})

				gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
				a.watchFoldersPanel(c, "modal-wf")
				gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())

				gui.Row(c).FillWidth().Padding(6, 4, 0, 4).Gap(8).AlignItems(gui.Center).Children(func() {
					a.toolButton(c, "wf-open-imp-pref", iconSettings, a.t("settings.tab.import"), false).OnClick(func() {
						a.watchFoldersOpen = false
						a.openSettings(settingsTabImport)
					})
					gui.Spacer(c)
					a.arcButton(c, "wf-modal-done", a.t("common.done"), true).OnClick(func() {
						a.watchFoldersOpen = false
					})
				})
			})
		})
	})
}

// watchFoldersPanel renders the flat divider-separated Watch Folders manager
// shared between the dedicated Watch Folders modal and Preferences -> Import.
func (a *App) watchFoldersPanel(c *gui.Context, prefix string) {
	gui.Column(c).FillWidth().Gap(4).Children(func() {
		gui.Row(c).FillWidth().Padding(4, 10).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.TextInput(c.Key(prefix+"-tag"), &a.watchNewTag).
				Placeholder(a.t("watch.tagPlaceholder")).
				Grow(1)
			a.arcButton(c, prefix+"-add", a.t("watch.add"), true).OnClick(func() {
				a.addWatchFolder()
			})
			if len(a.watchFolders) > 0 {
				a.toolButton(c, prefix+"-scan", iconRefresh, a.t("watch.scan"), false).
					Disabled(a.watchScanning).
					OnClick(func() { a.scanWatchFoldersNow() })
			}
		})

		if len(a.watchFolders) == 0 {
			gui.Text(c, a.t("watch.empty")).
				FontSize(12).
				TextColor(a.pal.inkMuted).
				LineHeight(1.45).
				Padding(6, 10)
			return
		}

		for i := range a.watchFolders {
			idx := i
			wf := a.watchFolders[idx]
			if i > 0 {
				gui.Box(c).FillWidth().Height(1).Margin(2, 10).Background(a.chromeHairline())
			}
			en := wf.Enabled
			name := filepath.Base(wf.Path)
			if name == "" || name == "." {
				name = wf.Path
			}
			row := gui.Row(c.Key(fmt.Sprintf("%s-row-%d", prefix, idx))).
				FillWidth().
				Padding(7, 10).
				Gap(10).
				Radius(8).
				AlignItems(gui.Center)
			if row.Hovered() {
				row.Background(a.settingsRowHoverBg())
			}
			row.Children(func() {
				gui.Icon(c, iconFolder).Size(15, 15).TextColor(a.pal.inkSecondary)
				gui.Column(c).Grow(1).Gap(2).Children(func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						gui.Text(c, name).FontSize(13).Bold().TextColor(a.pal.ink).SingleLine()
						if wf.TagName != "" {
							a.pillBadge(c, "#"+wf.TagName, a.pal.inkSecondary, a.pal.surfaceMuted)
						}
					})
					gui.Text(c, wf.Path).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
				})
				a.arcSwitch(c, fmt.Sprintf("%s-en-%d", prefix, idx), &en, func() {
					a.toggleWatchFolder(idx, en)
				})
				a.iconButton(c, fmt.Sprintf("%s-del-%d", prefix, idx), iconTrash, a.t("watch.remove"), false).OnClick(func() {
					a.removeWatchFolder(idx)
				})
			})
		}
	})
}

func (a *App) effectiveClipperInboxDir() string {
	dir := strings.TrimSpace(a.clipperInboxDir)
	if dir == "" {
		dir = config.DefaultClipperInboxDir()
	}
	return filepath.Clean(dir)
}

func (a *App) pickClipperInboxDir() {
	if a.win == nil {
		return
	}
	win := a.win
	title := a.t("ext.grpOffline")
	go func() {
		dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     title,
			Directory: true,
		})
		if err != nil || len(dirs) == 0 || strings.TrimSpace(dirs[0]) == "" {
			return
		}
		clean := filepath.Clean(dirs[0])
		win.Update(func() {
			if a.isClosed() {
				return
			}
			a.clipperInboxDir = clean
			a.syncConfigFromState()
			a.syncWatcherRules()
			a.notify(fmt.Sprintf(a.t("ext.offlineSet"), clean))
			a.scanWatchFoldersNow()
		})
	}()
}

func (a *App) syncWatcherRules() {
	if a.watch == nil {
		return
	}
	rules := make([]watcher.WatchRule, 0, len(a.watchFolders)+1)
	for _, wf := range a.watchFolders {
		mode := wf.StorageMode
		if mode == "" {
			mode = model.StorageIndex
		}
		rules = append(rules, watcher.WatchRule{
			Path:        wf.Path,
			TagName:     wf.TagName,
			StorageMode: mode,
			Recursive:   wf.Recursive,
			Enabled:     wf.Enabled,
		})
	}
	// Always watch the Offline Browser Clipper Drop Folder so images/sources
	// saved while Raglex was closed auto-import seamlessly!
	if inbox := a.effectiveClipperInboxDir(); inbox != "" {
		_ = os.MkdirAll(inbox, 0o755)
		mode := model.StorageIndex
		if a.storageMode >= 0 && a.storageMode < len(storageModes) {
			mode = storageModes[a.storageMode]
		}
		rules = append(rules, watcher.WatchRule{
			Path:           inbox,
			StorageMode:    mode,
			Recursive:      false,
			Enabled:        true,
			IsClipperInbox: true,
		})
	}
	a.watch.SetWatchRules(rules)
}

func (a *App) addWatchFolder() {
	if a.win == nil {
		return
	}
	win := a.win
	title := a.t("watch.add")
	tagName := strings.TrimSpace(a.watchNewTag)
	rec := a.importRecursive
	go func() {
		dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     title,
			Directory: true,
		})
		if err != nil || len(dirs) == 0 || strings.TrimSpace(dirs[0]) == "" {
			return
		}
		clean := filepath.Clean(dirs[0])
		win.Update(func() {
			if a.isClosed() {
				return
			}
			for i, existing := range a.watchFolders {
				if strings.EqualFold(filepath.Clean(existing.Path), clean) {
					a.watchFolders[i].Enabled = true
					if tagName != "" {
						a.watchFolders[i].TagName = tagName
					}
					a.syncConfigFromState()
					a.syncWatcherRules()
					a.scanWatchFoldersNow()
					return
				}
			}
			a.watchFolders = append(a.watchFolders, config.WatchFolder{
				Path:        clean,
				TagName:     tagName,
				StorageMode: model.StorageIndex,
				Recursive:   rec,
				Enabled:     true,
			})
			a.watchNewTag = ""
			a.syncConfigFromState()
			a.syncWatcherRules()
			a.notify(fmt.Sprintf(a.t("watch.added"), filepath.Base(clean)))
			a.scanWatchFoldersNow()
		})
	}()
}

func (a *App) toggleWatchFolder(idx int, on bool) {
	if idx < 0 || idx >= len(a.watchFolders) {
		return
	}
	a.watchFolders[idx].Enabled = on
	a.syncConfigFromState()
	a.syncWatcherRules()
}

func (a *App) removeWatchFolder(idx int) {
	if idx < 0 || idx >= len(a.watchFolders) {
		return
	}
	a.watchFolders = append(a.watchFolders[:idx], a.watchFolders[idx+1:]...)
	a.syncConfigFromState()
	a.syncWatcherRules()
}

func (a *App) scanWatchFoldersNow() {
	if a.watch == nil || a.watchScanning {
		return
	}
	a.watchScanning = true
	w := a.watch
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		added, _ := w.ScanWatchRules(ctx)
		finish := func() {
			a.watchScanning = false
			if a.isClosed() {
				return
			}
			if added > 0 {
				a.reload()
			}
			a.notify(fmt.Sprintf(a.t("watch.scanned"), added))
		}
		if win != nil && !a.isClosed() {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}
