package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

func (a *App) styledDialog(c *gui.Context, key string, open *bool, w, h float32, fn func(panel gui.Element)) {
	gui.DialogBase(c.Key(key), open, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.56))
		panel.
			Radius(radiusSurface).
			Clip().
			Background(a.pal.surfaceRaised).
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

func (a *App) previewModal(c *gui.Context) {
	if !a.previewOpen {
		if a.previewVideo != nil {
			a.previewVideo.Close()
			a.previewVideo = nil
		}
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

	a.styledDialog(c, "preview-modal", &a.previewOpen, 900, 660, func(panel gui.Element) {
		panel.OnShortcut(0, gui.KeyLeft, func() { a.stepPreview(-1) })
		panel.OnShortcut(0, gui.KeyRight, func() { a.stepPreview(1) })
		panel.OnShortcut(0, gui.KeySpace, func() {
			if a.previewVideo != nil {
				if a.previewVideo.Playing() {
					a.previewVideo.Pause()
				} else {
					a.previewVideo.Play()
				}
			}
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
						a.iconButton(c, "prev-img", iconChevronLeft, a.t("preview.prev"), false).OnClick(func() {
							a.stepPreview(-1)
						})
						a.iconButton(c, "next-img", iconChevronRgt, a.t("preview.next"), false).OnClick(func() {
							a.stepPreview(1)
						})
					}
					if !asset.Missing && asset.Path != "" {
						a.iconButton(c, "prev-open", iconExternal, a.t("common.openFile"), false).OnClick(func() {
							_ = openLocalFile(asset.Path)
						})
						a.iconButton(c, "prev-reveal", iconFolder, a.t("common.revealFile"), false).OnClick(func() {
							_ = revealLocalFile(asset.Path)
						})
					}
					a.iconButton(c, "prev-copy", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
						services.WriteClipboard(asset.Path)
						a.status = a.t("common.copied")
					})
					a.iconButton(c, "prev-close", iconX, a.t("common.close"), false).OnClick(func() {
						a.closePreview()
					})
				})

			// Artwork / Video stage.
			stageBg := a.pal.canvas
			if a.dark {
				stageBg = gui.Hex("#0B0B0D")
			}
			gui.Column(c).Grow(1).FillWidth().Background(stageBg).Center().Padding(18).Gap(12).Children(func() {
				switch {
				case a.previewVideo != nil:
					gui.Box(c).Grow(1).FillWidth().Center().Children(func() {
						gui.Video(c.Key("prev-video-"+asset.ID), a.previewVideo).
							Fit(gui.Contain).
							Fill().
							Radius(8)
					})
					// Monochrome playback controls bar.
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
							if isVideo && !asset.Missing && asset.Path != "" {
								a.toolButton(c, "prev-vid-sys", iconExternal, a.t("preview.openSystem"), false).OnClick(func() {
									_ = openLocalFile(asset.Path)
								})
							}
						})
				case a.previewLoading:
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Spinner(c)
						gui.Text(c, a.t("preview.loading")).TextColor(t.TextMuted)
					})
				case isVideo:
					// Fallback when ffmpeg is missing or codec failed to decode.
					gui.Column(c).Center().Gap(12).Children(func() {
						if bm := a.bitmaps[asset.ID]; bm != nil {
							gui.Image(c, bm).Fit(gui.Contain).MaxHeight(340).Radius(8)
						} else {
							gui.Box(c).Size(64, 64).Radius(radiusPanel).Background(a.pal.surface).Center().Children(func() {
								gui.Icon(c, iconVideo).Size(30, 30).TextColor(a.pal.inkMuted)
							})
						}
						if a.previewVideoErr != "" {
							gui.Text(c, a.previewVideoErr).
								Width(420).
								TextAlign(gui.Center).
								FontSize(12).
								TextColor(a.pal.inkSecondary).
								LineHeight(1.5)
						}
						if !asset.Missing && asset.Path != "" {
							a.arcButton(c, "prev-vid-open-sys", a.t("preview.openSystem"), true).OnClick(func() {
								_ = openLocalFile(asset.Path)
							})
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

			// Footer: rating + tags + path.
			gui.Row(c).
				Padding(12, 18).
				Gap(12).
				AlignItems(gui.Center).
				Children(func() {
					a.syncInspectorSelection(false)
					if a.inspectorAssetID == asset.ID {
						gui.Rating(c.Key("prev-rating-"+asset.ID), &a.inspectorRating, 5).OnChange(func() {
							if err := a.lib.SetRating(asset.ID, a.inspectorRating); err == nil {
								a.reload()
							}
						})
					}
					gui.Row(c).Gap(6).Grow(1).AlignItems(gui.Center).Children(func() {
						for _, tg := range a.assetTags[asset.ID] {
							a.pillBadge(c, "#"+tg.Name, a.pal.inkSecondary, a.pal.surface)
						}
					})
					gui.Text(c, asset.Path).FontSize(11).TextColor(a.pal.inkFaint).MaxWidth(320).SingleLine()
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
