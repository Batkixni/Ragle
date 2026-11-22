package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"
)

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

	m := gui.Modal(c.Key("preview-modal"), &a.previewOpen, func() {
		box := gui.Column(c).
			Width(860).
			Height(620).
			Radius(14).
			Clip().
			Background(a.pal.card).
			Border(1, a.pal.hairline).
			Shadow(0, 20, 48, 0, a.pal.shadow)

		box.OnShortcut(0, gui.KeyLeft, func() { a.stepPreview(-1) })
		box.OnShortcut(0, gui.KeyRight, func() { a.stepPreview(1) })

		box.Children(func() {
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
					a.syncInspectorSelection()
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
	_ = m
}

func (a *App) newProjectModal(c *gui.Context) {
	if !a.newProjOpen {
		return
	}
	t := c.Theme()
	gui.Modal(c.Key("new-proj-modal"), &a.newProjOpen, func() {
		gui.Column(c).
			Width(440).
			Padding(22, 24).
			Gap(14).
			Radius(14).
			Background(a.pal.card).
			Border(1, a.pal.hairline).
			Shadow(0, 16, 40, 0, a.pal.shadow).
			Children(func() {
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
