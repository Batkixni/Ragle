package ui

import (
	"fmt"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

func (a *App) libraryView(c *gui.Context) {
	if !a.inspectorOpen {
		a.libraryMain(c)
		return
	}
	if a.libraryPercent <= 40 || a.libraryPercent >= 90 {
		a.libraryPercent = 74
	}
	gui.SplitPercent(c, &a.libraryPercent, func() {
		a.libraryMain(c)
	}, func() {
		a.inspector(c)
	}).Grow(1)
}

func (a *App) libraryMain(c *gui.Context) {
	t := c.Theme()
	dropZone := gui.Column(c.Key("library-dropzone")).Fill().Padding(14, 18).Gap(12).Background(a.pal.canvas)
	if files := dropZone.DroppedFiles(); len(files) > 0 {
		a.importPaths(files)
	}
	dragOver := dropZone.FileDragOver()
	if dragOver {
		dropZone.Border(2, a.pal.ring).Background(a.pal.ring.Alpha(0.05))
	}

	dropZone.Children(func() {
		// Drag-and-drop import indicator banner.
		if dragOver {
			modeLbl := a.modeLabel(storageModes[a.storageMode])
			gui.Row(c).
				Padding(10, 14).
				Radius(10).
				Gap(8).
				AlignItems(gui.Center).
				Background(a.pal.ring.Alpha(0.14)).
				Border(1, a.pal.ring.Alpha(0.45)).
				Children(func() {
					gui.Icon(c, iconImport).Size(16, 16).TextColor(a.pal.ring)
					gui.Text(c, fmt.Sprintf(a.t("library.dropHint"), modeLbl)).Bold().FontSize(13).TextColor(a.pal.ring)
				})
		}

		// Project reference mode banner.
		if a.referenceMode && a.project.Name != "" {
			gui.Row(c).
				Padding(8, 14).
				Radius(10).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.ring.Alpha(0.12)).
				Border(1, a.pal.ring.Alpha(0.30)).
				Children(func() {
					gui.Icon(c, iconFolder).Size(15, 15).TextColor(a.pal.ring)
					gui.Text(c, fmt.Sprintf(a.t("library.referencing"), a.project.Name)).Bold().FontSize(12).TextColor(a.pal.ring)
					gui.Spacer(c)
					gui.PrimaryButton(c.Key("ref-selected-btn"), a.t("library.reference")).OnClick(func() {
						a.referenceSelected()
					})
					gui.Button(c.Key("ref-done-btn"), a.t("common.done")).OnClick(func() {
						a.referenceMode = false
						if a.projectID != "" {
							a.screen = screenProject
							a.syncNavSelection()
						}
					})
				})
		}

		// Compact active filter & status strip (hidden when clean & idle).
		if a.hasActiveFilters() || a.status != "" {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Wrap().Children(func() {
				if a.kindFilter != 0 {
					kinds := []string{"", a.t("library.kind.image"), a.t("library.kind.video"), a.t("library.kind.other")}
					a.tagChip(c, "chip-kind", kinds[a.kindFilter]+" ×", true).OnClick(func() {
						a.kindFilter = 0
						a.applyFilters()
					})
				}
				if a.minRating > 0 {
					a.tagChip(c, "chip-rating", fmt.Sprintf("★ %d+ ×", a.minRating), true).OnClick(func() {
						a.minRating = 0
						a.applyFilters()
					})
				}
				if a.hasActiveFilters() {
					a.toolButton(c, "chip-clear", iconX, a.t("common.clear"), false).OnClick(func() {
						a.clearFilters()
					})
				}
				gui.Spacer(c)
				if a.status != "" {
					gui.Text(c, a.status).FontSize(12).TextColor(t.TextMuted).SingleLine().MaxWidth(420)
				}
			})
		}

		// Missing-assets quick relink banner when viewing Missing section.
		if a.navFilter == "missing" && len(a.assets) > 0 {
			gui.Row(c).
				Padding(10, 14).
				Radius(10).
				Gap(10).
				AlignItems(gui.Center).
				Background(t.Danger.Alpha(0.10)).
				Border(1, t.Danger.Alpha(0.28)).
				Children(func() {
					gui.Icon(c, iconAlert).Size(15, 15).TextColor(t.Danger)
					gui.Text(c, fmt.Sprintf("%s (%d)", a.t("shell.missing"), len(a.assets))).FontSize(12).Bold().TextColor(t.Danger)
					gui.Spacer(c)
					gui.Button(c.Key("relink-all-btn"), a.t("inspector.relink")).OnClick(func() {
						a.relinkMissing()
					})
				})
		}

		// Gallery grid or refined empty state.
		if len(a.assets) == 0 {
			a.libraryEmptyState(c)
		} else {
			a.assetGrid(c)
		}
	})
}

func (a *App) libraryEmptyState(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Grow(1).Center().Gap(12).Padding(32).Children(func() {
		gui.Box(c).Size(52, 52).Radius(14).Background(a.pal.subtle).Center().Children(func() {
			gui.Icon(c, iconImage).Size(24, 24).TextColor(a.pal.inkMuted)
		})
		if a.hasActiveFilters() {
			gui.Text(c, a.t("library.noMatch")).FontSize(15).Bold()
			gui.Button(c.Key("empty-clear"), a.t("common.clear")).OnClick(func() { a.clearFilters() })
		} else {
			gui.Text(c, a.t("library.empty")).FontSize(15).Bold()
			gui.Text(c, a.t("library.emptyHint")).
				Width(380).
				TextAlign(gui.Center).
				FontSize(12).
				TextColor(t.TextMuted).
				LineHeight(1.5)
			gui.Row(c).Gap(8).Padding(6, 0).Children(func() {
				gui.PrimaryButton(c.Key("empty-import-files"), a.t("library.files")).OnClick(func() {
					a.startImport(false)
				})
				gui.Button(c.Key("empty-import-folder"), a.t("library.folder")).OnClick(func() {
					a.startImport(true)
				})
			})
		}
	})
}

func (a *App) assetGrid(c *gui.Context) {
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	a.grid.Selected = &a.assetIndex
	a.grid.Key = func(i int) any {
		if i < 0 || i >= len(a.assets) {
			return i
		}
		return a.assets[i].ID
	}
	a.grid.Label = func(i int) string {
		if i < 0 || i >= len(a.assets) {
			return ""
		}
		return a.assetName(a.assets[i])
	}

	cellW := float32(a.cardSize)
	cellH := cellW + 30

	gv := gui.GridView(c.Key("asset-grid"), &a.grid, len(a.assets), cellW, cellH, func(i int) {
		asset := a.assets[i]
		a.assetCard(c, asset, i == a.assetIndex)
	}).
		Gap(14).
		Grow(1).
		OnChange(func() {
			a.syncInspectorSelection()
		}).
		OnSubmit(func() {
			if asset, ok := a.selectedAsset(); ok {
				a.openPreview(asset)
			}
		})

	gv.OnShortcut(0, gui.KeySpace, func() {
		if asset, ok := a.selectedAsset(); ok {
			a.openPreview(asset)
		}
	})
	gv.OnShortcut(0, gui.KeyDelete, func() {
		if asset, ok := a.selectedAsset(); ok {
			a.deleteAssetPrompt(asset)
		}
	})
}

func (a *App) assetCard(c *gui.Context, asset model.Asset, selected bool) {
	t := c.Theme()
	services := c.Services()
	name := a.assetName(asset)
	ext := a.assetExt(asset)

	card := gui.Column(c).Fill().Gap(6).Drag(assetDrag{ID: asset.ID})
	card.ContextMenu(func(m *gui.Menu) {
		if m.Item(a.t("library.preview")).Chosen() {
			a.openPreview(asset)
		}
		if len(a.projectList) > 0 {
			m.Submenu(a.t("library.addToProject"), func(sm *gui.Menu) {
				for _, p := range a.projectList {
					proj := p
					if sm.Item(proj.Name).Chosen() {
						a.referenceAssetToProject(asset.ID, proj.ID)
					}
				}
			})
		}
		m.Submenu(a.t("library.rating"), func(sm *gui.Menu) {
			for r := 0; r <= 5; r++ {
				rating := r
				lbl := "0"
				if rating > 0 {
					lbl = strings.Repeat("★", rating)
				}
				if sm.Item(lbl).Radio(asset.Rating == rating).Chosen() {
					if err := a.lib.SetRating(asset.ID, rating); err == nil {
						a.reload()
					}
				}
			}
		})
		m.Separator()
		if !asset.Missing && asset.Path != "" {
			if m.Item(a.t("common.openFile")).Chosen() {
				_ = openLocalFile(asset.Path)
			}
		}
		if m.Item(a.t("common.copyPath")).Chosen() {
			services.WriteClipboard(asset.Path)
			a.status = a.t("common.copied")
		}
		if asset.Missing {
			if m.Item(a.t("inspector.relink")).Chosen() {
				a.relinkMissing()
			}
		}
		m.Separator()
		if m.Item(a.t("library.delete")).Chosen() {
			a.deleteAssetPrompt(asset)
		}
	})

	card.Children(func() {
		// Artwork frame: clean rounded surface, crisp ring on selection.
		frame := gui.Box(c).
			Grow(1).
			FillWidth().
			Radius(10).
			Clip().
			Background(a.pal.card).
			Shadow(0, 2, 8, 0, a.pal.shadow).
			Center()
		if selected {
			frame.Border(2, a.pal.ring)
		} else {
			frame.Border(1, a.pal.hairline)
		}
		frame.Children(func() {
			if bm := a.bitmaps[asset.ID]; bm != nil {
				gui.Image(c, bm).Fit(gui.Contain).Grow(1).Padding(4).Radius(8)
			} else {
				gui.Column(c).Center().Gap(6).Children(func() {
					ic := iconFile
					switch model.MediaKind(asset.Mime) {
					case model.MediaVideo:
						ic = iconVideo
					case model.MediaImage:
						ic = iconImage
					}
					gui.Icon(c, ic).Size(24, 24).TextColor(a.pal.inkFaint)
					gui.Text(c, ext).FontSize(11).Bold().TextColor(a.pal.inkMuted)
				})
			}
		})

		// Clean caption footer: single-line filename + compact meta badge.
		gui.Row(c).Gap(6).AlignItems(gui.Center).Padding(0, 2).Children(func() {
			nameColor := t.Text
			if selected {
				nameColor = a.pal.ring
			}
			gui.Text(c, name).Grow(1).FontSize(12).TextColor(nameColor).SingleLine()
			if asset.Missing {
				a.pillBadge(c, a.t("library.missing"), t.Danger, t.Danger.Alpha(0.14))
			} else if asset.Rating > 0 {
				gui.Text(c, fmt.Sprintf("★%d", asset.Rating)).FontSize(11).Bold().TextColor(a.pal.gold).SingleLine()
			} else {
				gui.Text(c, ext).FontSize(10).TextColor(a.pal.inkFaint).SingleLine()
			}
		})
	})
}

func (a *App) inspector(c *gui.Context) {
	t := c.Theme()
	services := c.Services()
	asset, ok := a.selectedAsset()
	a.syncInspectorSelection()

	gui.Column(c).
		Fill().
		Background(a.pal.sidebar).
		BorderWidth(0, 0, 0, 1).
		BorderColor(a.pal.hairline).
		Children(func() {
			if !ok {
				gui.Column(c).Fill().Center().Padding(24).Gap(10).Children(func() {
					gui.Icon(c, iconPanelRight).Size(26, 26).TextColor(a.pal.inkFaint)
					gui.Text(c, a.t("inspector.none")).
						FontSize(12).
						TextColor(t.TextMuted).
						TextAlign(gui.Center).
						LineHeight(1.5)
				})
				return
			}

			gui.Scroll(c).Fill().Children(func() {
				gui.Column(c).Padding(14, 16).Gap(12).Children(func() {
					// Clickable hero preview thumbnail.
					hero := gui.ButtonBase(c.Key("insp-hero-" + asset.ID)).
						FillWidth().
						Height(168).
						Radius(10).
						Clip().
						Background(a.pal.card).
						Border(1, a.pal.hairline).
						Center().
						Tooltip(a.t("library.preview")).
						OnClick(func() {
							a.openPreview(asset)
						})
					hero.Children(func() {
						if bm := a.bitmaps[asset.ID]; bm != nil {
							gui.Image(c, bm).Fit(gui.Contain).Grow(1).Padding(6).Radius(8)
						} else {
							gui.Column(c).Center().Gap(6).Children(func() {
								gui.Icon(c, iconFile).Size(28, 28).TextColor(a.pal.inkFaint)
								gui.Text(c, a.assetExt(asset)).Bold().TextColor(a.pal.inkMuted)
							})
						}
					})

					// Title + compact badge strip.
					gui.Column(c).Gap(8).Children(func() {
						gui.Text(c, a.assetName(asset)).FontSize(14).Bold().LineHeight(1.3)
						gui.Row(c).Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
							a.pillBadge(c, a.assetExt(asset), t.Text, a.pal.subtle)
							a.pillBadge(c, a.modeLabel(asset.StorageMode), t.TextMuted, a.pal.subtle)
							if asset.Width > 0 && asset.Height > 0 {
								a.pillBadge(c, fmt.Sprintf("%d × %d", asset.Width, asset.Height), t.TextMuted, a.pal.subtle)
							}
							if asset.Missing {
								a.pillBadge(c, a.t("library.missing"), t.Danger, t.Danger.Alpha(0.15))
							}
						})
					})

					// Missing file alert & relink CTA.
					if asset.Missing {
						gui.Column(c).
							Padding(10, 12).
							Gap(8).
							Radius(8).
							Background(t.Danger.Alpha(0.10)).
							Border(1, t.Danger.Alpha(0.28)).
							Children(func() {
								gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
									gui.Icon(c, iconAlert).Size(14, 14).TextColor(t.Danger)
									gui.Text(c, a.t("library.missing")).FontSize(12).Bold().TextColor(t.Danger)
								})
								gui.Button(c.Key("insp-relink"), a.t("inspector.relink")).FillWidth().OnClick(func() {
									a.relinkMissing()
								})
							})
					}

					// Interactive Rating (bound to persistent a.inspectorRating!).
					gui.Column(c).Gap(6).Children(func() {
						a.sectionLabel(c, a.t("library.rating"))
						gui.Rating(c.Key("rating-"+asset.ID), &a.inspectorRating, 5).OnChange(func() {
							if err := a.lib.SetRating(asset.ID, a.inspectorRating); err == nil {
								a.reload()
							}
						})
					})

					// Wrapping tag pills + quick-add tag field.
					gui.Column(c).Gap(8).Children(func() {
						a.sectionLabel(c, a.t("library.tags"))
						owned := map[string]bool{}
						for _, tg := range a.assetTags[asset.ID] {
							owned[tg.ID] = true
						}
						if len(a.tags) > 0 {
							gui.Row(c).Gap(6).Wrap().Children(func() {
								for _, tag := range a.tags {
									tg := tag
									isOn := owned[tg.ID]
									a.tagChip(c, "insp-tag-"+asset.ID+"-"+tg.ID, tg.Name, isOn).OnClick(func() {
										if err := a.lib.SetAssetTag(asset.ID, tg.ID, !isOn); err != nil {
											a.status = err.Error()
											return
										}
										a.reload()
									})
								}
							})
						}
						gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
							gui.TextInput(c.Key("insp-quick-tag"), &a.quickTagName).
								Placeholder(a.t("inspector.quickTag")).
								Grow(1).
								OnSubmit(func() { a.quickAddAssetTag(asset.ID) })
							a.iconButton(c, "insp-quick-tag-add", iconPlus, a.t("tag.create"), false).OnClick(func() {
								a.quickAddAssetTag(asset.ID)
							})
						})
					})

					gui.Divider(c)

					// Details metadata grid.
					gui.Column(c).Gap(6).Children(func() {
						a.sectionLabel(c, a.t("inspector.details"))
						if asset.Width > 0 && asset.Height > 0 {
							a.metaRow(c, a.t("inspector.dimensions"), fmt.Sprintf("%d × %d px", asset.Width, asset.Height))
						}
						if a.inspectorFileSize > 0 {
							a.metaRow(c, a.t("inspector.size"), formatBytes(a.inspectorFileSize))
						}
						if asset.Mime != "" {
							a.metaRow(c, a.t("library.mime"), asset.Mime)
						}
						a.metaRow(c, a.t("library.storage"), a.modeLabel(asset.StorageMode))
						if !asset.CreatedAt.IsZero() {
							a.metaRow(c, a.t("inspector.added"), asset.CreatedAt.Local().Format("2006-01-02 15:04"))
						}
						gui.Column(c).Gap(4).Padding(4, 0, 0, 0).Children(func() {
							gui.Row(c).AlignItems(gui.Center).Children(func() {
								gui.Text(c, a.t("library.path")).FontSize(11).TextColor(a.pal.inkMuted)
								gui.Spacer(c)
								a.iconButton(c, "insp-copy-path", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
									services.WriteClipboard(asset.Path)
									a.status = a.t("common.copied")
								})
							})
							gui.Box(c).Padding(6, 8).Radius(6).Background(a.pal.card).Border(1, a.pal.hairline).Children(func() {
								gui.Text(c, asset.Path).FontSize(11).TextColor(t.TextMuted).LineHeight(1.4)
							})
						})
					})

					gui.Divider(c)

					// Action buttons.
					gui.Column(c).Gap(8).Children(func() {
						a.sectionLabel(c, a.t("inspector.actions"))
						gui.Row(c).Gap(8).Children(func() {
							gui.Button(c.Key("insp-preview"), a.t("library.preview")).Grow(1).OnClick(func() {
								a.openPreview(asset)
							})
							if !asset.Missing && asset.Path != "" {
								gui.Button(c.Key("insp-open"), a.t("common.openFile")).Grow(1).OnClick(func() {
									_ = openLocalFile(asset.Path)
								})
							}
						})
						if len(a.projectList) > 0 {
							gui.PopoverMenuButton(c.Key("insp-add-proj"), a.t("library.addToProject"), func(m *gui.Menu) {
								for _, p := range a.projectList {
									proj := p
									if m.Item(proj.Name).Chosen() {
										a.referenceAssetToProject(asset.ID, proj.ID)
									}
								}
							}).FillWidth()
						}
						gui.Button(c.Key("insp-delete"), a.t("library.delete")).
							FillWidth().
							TextColor(t.Danger).
							OnClick(func() { a.deleteAssetPrompt(asset) })
					})
				})
			})
		})
}
