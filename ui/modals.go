package ui

import (
	"context"
	"fmt"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
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
					if bm := a.bitmaps[asset.ID]; bm != nil {
						gui.Image(c, bm).Fit(gui.Contain).MaxHeight(420).Radius(8)
					} else {
						gui.Icon(c, iconVideo).Size(32, 32).TextColor(a.pal.inkMuted)
					}
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
					if m.Item(a.t("tags.noParent")).Radio(a.editTagParentID == "").Chosen() {
						a.editTagParentID = ""
					}
					m.Separator()
					for _, tg := range a.tags {
						cand := tg
						if forbidden[cand.ID] {
							continue
						}
						if m.Item(cand.Name).Radio(a.editTagParentID == cand.ID).Chosen() {
							a.editTagParentID = cand.ID
						}
					}
				}).FillWidth()
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
	a.styledDialog(c, "web-import", &a.webImportOpen, 480, 0, func(panel gui.Element) {
		panel.Padding(22).Children(func() {
			gui.Column(c).Gap(14).Children(func() {
				gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconGlobe).Size(18, 18).TextColor(a.pal.ink)
					gui.Text(c, a.t("webimport.title")).FontSize(16).Bold().Grow(1)
					a.iconButton(c, "web-imp-close", iconX, a.t("common.close"), false).OnClick(func() {
						if !a.webImportBusy {
							a.webImportOpen = false
						}
					})
				})
				gui.Text(c, a.t("webimport.hint")).
					FontSize(12).
					TextColor(a.pal.inkSecondary).
					LineHeight(1.5)
				gui.TextInput(c.Key("web-imp-url"), &a.webImportURL).
					Placeholder(a.t("webimport.placeholder")).
					FillWidth().
					OnSubmit(func() { a.startWebImport() })
				if a.webImportBusy {
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Text(c, a.t("webimport.fetching")).FontSize(12).TextColor(a.pal.inkSecondary)
					})
				}
				gui.Divider(c)
				gui.Row(c).Gap(8).Justify(gui.End).Children(func() {
					a.arcButton(c, "web-imp-cancel", a.t("common.cancel"), false).OnClick(func() {
						if !a.webImportBusy {
							a.webImportOpen = false
						}
					})
					lbl := a.t("webimport.action")
					if a.webImportBusy {
						lbl = a.t("webimport.fetching")
					}
					a.arcButton(c, "web-imp-go", lbl, true).OnClick(func() {
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
	rawURL := trim(a.webImportURL)
	if rawURL == "" {
		return
	}
	a.webImportBusy = true
	win := a.win
	lib := a.lib
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		asset, err := lib.ImportFromURL(ctx, rawURL, model.StorageCopy)
		finish := func() {
			a.webImportBusy = false
			if err != nil {
				a.notify(fmt.Sprintf(a.t("webimport.failed"), err.Error()))
				return
			}
			a.webImportOpen = false
			a.webImportURL = ""
			if a.thumbs != nil {
				a.thumbs.Prioritize(asset.ID)
			}
			a.reload()
			if a.autoSelectImported {
				for i, it := range a.assets {
					if it.ID == asset.ID {
						a.assetIndex = i
						a.selectedIDs = map[string]bool{asset.ID: true}
						break
					}
				}
			}
			a.notify(fmt.Sprintf(a.t("webimport.done"), a.assetName(asset)))
		}
		if win != nil && !a.isClosed() {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}
