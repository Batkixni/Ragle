package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"
)

func (a *App) styledDialog(c *gui.Context, key string, open *bool, w, h float32, fn func(panel gui.Element)) {
	gui.DialogBase(c.Key(key), open, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.52))
		panel.
			Radius(14).
			Clip().
			Background(a.pal.card).
			Border(1, a.pal.hairline).
			Shadow(0, 20, 48, 0, a.pal.shadow)
		if w > 0 {
			panel.Width(w)
		}
		if h > 0 {
			panel.Height(h)
		}
		fn(panel)
	})
}

func (a *App) previewModal(c *gui.Context) {
	if !a.previewOpen {
		return
	}
	asset, ok := a.assetByID[a.previewAssetID]
	if !ok {
		a.previewOpen = false
		return
	}
	t := c.Theme()
	services := c.Services()

	a.styledDialog(c, "preview-modal", &a.previewOpen, 880, 640, func(panel gui.Element) {
		panel.OnShortcut(0, gui.KeyLeft, func() { a.stepPreview(-1) })
		panel.OnShortcut(0, gui.KeyRight, func() { a.stepPreview(1) })

		gui.Column(c).Fill().Children(func() {
			// Header bar.
			gui.Row(c).
				Height(48).
				Padding(0, 16).
				Gap(8).
				AlignItems(gui.Center).
				BorderWidth(0, 0, 1, 0).
				BorderColor(a.pal.hairline).
				Children(func() {
					gui.Text(c, a.assetName(asset)).Bold().FontSize(14).Grow(1).SingleLine()
					if asset.Width > 0 && asset.Height > 0 {
						a.pillBadge(c, fmt.Sprintf("%d × %d", asset.Width, asset.Height), t.TextMuted, a.pal.subtle)
					}
					a.pillBadge(c, a.assetExt(asset), t.Text, a.pal.subtle)
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
						a.previewOpen = false
					})
				})

			// Artwork stage.
			stageBg := a.pal.canvas
			if a.dark {
				stageBg = gui.Hex("#0B0B0D")
			}
			gui.Box(c).Grow(1).FillWidth().Background(stageBg).Center().Padding(20).Children(func() {
				bm := a.previewBitmap
				if bm == nil {
					bm = a.bitmaps[asset.ID]
				}
				if bm != nil {
					gui.Image(c, bm).Fit(gui.Contain).Grow(1).Radius(6)
				} else if a.previewLoading {
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Spinner(c)
						gui.Text(c, a.t("preview.loading")).TextColor(t.TextMuted)
					})
				} else {
					gui.Column(c).Center().Gap(8).Children(func() {
						gui.Icon(c, iconFile).Size(40, 40).TextColor(a.pal.inkFaint)
						gui.Text(c, a.assetExt(asset)).FontSize(16).Bold().TextColor(a.pal.inkMuted)
					})
				}
			})

			// Footer: rating + tags + path.
			gui.Row(c).
				Padding(10, 16).
				Gap(12).
				AlignItems(gui.Center).
				BorderWidth(1, 0, 0, 0).
				BorderColor(a.pal.hairline).
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
							a.pillBadge(c, "#"+tg.Name, a.pal.ring, a.pal.ring.Alpha(0.14))
						}
					})
					gui.Text(c, asset.Path).FontSize(11).TextColor(a.pal.inkFaint).MaxWidth(320).SingleLine()
				})
		})
	})
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
