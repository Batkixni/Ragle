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
		a.libraryPercent = 73
	}
	gui.SplitPercent(c, &a.libraryPercent, func() {
		a.libraryMain(c)
	}, func() {
		a.inspector(c)
	}).Grow(1)
}

func (a *App) libraryMain(c *gui.Context) {
	t := c.Theme()
	dropZone := gui.Column(c.Key("library-dropzone")).Fill().Padding(6, 12, 10, 6).Gap(10).Background(a.pal.canvas)
	if files := dropZone.DroppedFiles(); len(files) > 0 {
		a.importPaths(files)
	}
	dragOver := dropZone.FileDragOver()
	if dragOver {
		dropZone.Background(a.pal.ring.Alpha(0.06))
	}

	dropZone.Children(func() {
		// Drag-and-drop import indicator banner.
		if dragOver {
			modeLbl := a.modeLabel(storageModes[a.storageMode])
			gui.Row(c.Key("dnd-banner")).
				Padding(12, 16).
				Radius(radiusPanel).
				Gap(8).
				AlignItems(gui.Center).
				Background(a.pal.ring.Alpha(0.14)).
				Transition(gui.ElementTransition{
					Animation: &arcEnterAnim,
					Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
					Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
				}).
				Children(func() {
					gui.Icon(c, iconImport).Size(16, 16).TextColor(a.pal.ring)
					gui.Text(c, fmt.Sprintf(a.t("library.dropHint"), modeLbl)).Bold().FontSize(13).TextColor(a.pal.ring)
				})
		}

		// Arc UI Sliding Pill Filter & Storage Bar (smooth collapse/expand).
		if a.filterBarOpen {
			a.filterBar(c)
		}

		// Project reference mode banner.
		if a.referenceMode && a.project.Name != "" {
			gui.Row(c.Key("ref-banner")).
				Padding(10, 16).
				Radius(radiusPanel).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.ring.Alpha(0.12)).
				Transition(gui.ElementTransition{
					Animation: &arcEnterAnim,
					Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
					Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
				}).
				Children(func() {
					gui.Icon(c, iconFolder).Size(15, 15).TextColor(a.pal.ring)
					gui.Text(c, fmt.Sprintf(a.t("library.referencing"), a.project.Name)).Bold().FontSize(12).TextColor(a.pal.ring)
					gui.Spacer(c)
					gui.PrimaryButton(c.Key("ref-selected-btn"), a.t("library.reference")).Radius(radiusControl).OnClick(func() {
						a.referenceSelected()
					})
					gui.Button(c.Key("ref-done-btn"), a.t("common.done")).Radius(radiusControl).OnClick(func() {
						a.referenceMode = false
						if a.projectID != "" {
							a.screen = screenProject
							a.syncNavSelection()
						}
					})
				})
		}

		// Smooth status toast strip.
		if a.status != "" {
			gui.Row(c.Key("status-toast")).
				Padding(6, 12).
				Radius(radiusControl).
				Background(a.pal.surface).
				AlignItems(gui.Center).
				Gap(8).
				Transition(gui.ElementTransition{
					Animation: &arcEnterAnim,
					Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
					Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
				}).
				Children(func() {
					gui.Text(c, a.status).FontSize(12).TextColor(t.TextMuted).Grow(1).SingleLine()
					a.iconButton(c, "status-dismiss", iconX, a.t("common.close"), false).OnClick(func() {
						a.status = ""
					})
				})
		}

		// Missing-assets quick relink banner.
		if a.navFilter == "missing" && len(a.assets) > 0 {
			gui.Row(c.Key("missing-banner")).
				Padding(10, 16).
				Radius(radiusPanel).
				Gap(8).
				AlignItems(gui.Center).
				Background(t.Danger.Alpha(0.12)).
				Children(func() {
					gui.Icon(c, iconAlert).Size(15, 15).TextColor(t.Danger)
					gui.Text(c, fmt.Sprintf("%s (%d)", a.t("shell.missing"), len(a.assets))).FontSize(12).Bold().TextColor(t.Danger)
					gui.Spacer(c)
					gui.Button(c.Key("relink-file-all"), a.t("inspector.relinkFile")).Radius(radiusControl).OnClick(func() {
						a.relinkMissing(false)
					})
					gui.Button(c.Key("relink-dir-all"), a.t("inspector.relink")).Radius(radiusControl).OnClick(func() {
						a.relinkMissing(true)
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

func (a *App) filterBar(c *gui.Context) {
	kinds := []string{
		a.t("library.kind.all"),
		a.t("library.kind.image"),
		a.t("library.kind.video"),
		a.t("library.kind.other"),
	}
	ratings := []string{"0", "★1+", "★2+", "★3+", "★4+", "★5"}
	modes := []string{
		a.t("library.mode.index"),
		a.t("library.mode.symlink"),
		a.t("library.mode.move"),
		a.t("library.mode.copy"),
	}

	gui.Row(c.Key("arc-filter-bar")).
		Padding(6, 10).
		Gap(10).
		Radius(radiusPanel).
		AlignItems(gui.Center).
		Background(a.pal.canvas).
		Wrap().
		Transition(gui.ElementTransition{
			Animation: &arcEnterAnim,
			Enter:     &gui.Motion{Y: -8, Opacity: 0, Collapse: true},
			Exit:      &gui.Motion{Y: -8, Opacity: 0, Collapse: true},
		}).
		Children(func() {
			// Arc UI sliding pill navigator for media format.
			a.arcPillTabs(c, "filter-kind", a.kindFilter, kinds, func(idx int) {
				a.kindFilter = idx
				a.applyFilters()
			})

			// Compact rating toggle pills.
			gui.Row(c).Gap(4).AlignItems(gui.Center).Children(func() {
				for i, lbl := range ratings {
					idx := i
					if idx == 0 {
						continue
					}
					active := a.minRating == idx
					a.tagChip(c, fmt.Sprintf("frate-%d", idx), lbl, active).OnClick(func() {
						if a.minRating == idx {
							a.minRating = 0
						} else {
							a.minRating = idx
						}
						a.applyFilters()
					})
				}
			})

			if a.hasActiveFilters() {
				a.toolButton(c, "fbar-clear", iconX, a.t("common.clear"), false).OnClick(func() {
					a.clearFilters()
				})
			}

			gui.Spacer(c)

			// Arc UI sliding pill navigator for storage strategy.
			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("library.mode")).FontSize(11).TextColor(a.pal.inkMuted)
				a.arcPillTabs(c, "storage-mode", a.storageMode, modes, func(idx int) {
					a.storageMode = idx
				})
			})
		})
}

func (a *App) libraryEmptyState(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Grow(1).Center().Gap(14).Padding(32).Children(func() {
		gui.Box(c).Size(58, 58).Radius(radiusPanel).Background(a.pal.surface).Center().Children(func() {
			gui.Icon(c, iconImage).Size(26, 26).TextColor(a.pal.inkMuted)
		})
		if a.hasActiveFilters() {
			gui.Text(c, a.t("library.noMatch")).FontSize(15).Bold()
			gui.Button(c.Key("empty-clear"), a.t("common.clear")).Radius(radiusControl).OnClick(func() { a.clearFilters() })
		} else {
			gui.Text(c, a.t("library.empty")).FontSize(16).Bold()
			gui.Text(c, a.t("library.emptyHint")).
				Width(380).
				TextAlign(gui.Center).
				FontSize(12).
				TextColor(t.TextMuted).
				LineHeight(1.55)
			gui.Row(c).Gap(10).Padding(6, 0).Children(func() {
				gui.PrimaryButton(c.Key("empty-import-files"), a.t("library.files")).Radius(radiusControl).OnClick(func() {
					a.startImport(false)
				})
				gui.Button(c.Key("empty-import-folder"), a.t("library.folder")).Radius(radiusControl).OnClick(func() {
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
	cellH := cellW + 34

	gv := gui.GridView(c.Key("asset-grid"), &a.grid, len(a.assets), cellW, cellH, func(i int) {
		asset := a.assets[i]
		a.assetCard(c, asset, i == a.assetIndex)
	}).
		Gap(16).
		Grow(1).
		OnChange(func() {
			a.syncInspectorSelection(false)
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

// assetCard renders an Arc UI frameless card: zero borders, surface-level fill
// transitions on hover, subtle physical lift (-2px), and soft elevation shadow.
func (a *App) assetCard(c *gui.Context, asset model.Asset, selected bool) {
	t := c.Theme()
	services := c.Services()
	name := a.assetName(asset)
	ext := a.assetExt(asset)

	// No .Drag() inside GridView cell so single-click selection & double-click
	// preview work 100% reliably.
	card := gui.Column(c).Fill().Padding(4).Gap(8)
	card.PopoverMenu(func(m *gui.Menu) {
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
			if m.Item(a.t("common.revealFile")).Chosen() {
				_ = revealLocalFile(asset.Path)
			}
		}
		if m.Item(a.t("common.copyPath")).Chosen() {
			services.WriteClipboard(asset.Path)
			a.status = a.t("common.copied")
		}
		if asset.Missing {
			if m.Item(a.t("inspector.relinkFile")).Chosen() {
				a.relinkMissing(false)
			}
			if m.Item(a.t("inspector.relink")).Chosen() {
				a.relinkMissing(true)
			}
		}
		m.Separator()
		if m.Item(a.t("library.delete")).Chosen() {
			a.deleteAssetPrompt(asset)
		}
	})

	hovered := card.Hovered()
	targetBg := a.pal.surface
	liftY := float32(0)
	shadowColor := a.pal.shadowSm
	if selected {
		targetBg = a.pal.surfaceMuted
		shadowColor = a.pal.shadowMd
	} else if hovered {
		targetBg = a.pal.surfaceMuted
		liftY = -2
		shadowColor = a.pal.shadowMd
	}
	bg := card.AnimateColor("card-bg", targetBg, arcColorAnim)
	card.AnimatePose(gui.Pose{Y: liftY}, arcSpring)

	card.Children(func() {
		// Frameless rounded media surface (Arc UI: no borders, surface fill + shadow).
		frame := gui.Box(c).
			Grow(1).
			FillWidth().
			Radius(radiusPanel).
			Clip().
			Background(bg).
			Shadow(0, 4, 14, 0, shadowColor).
			Center()
		frame.Children(func() {
			if bm := a.bitmaps[asset.ID]; bm != nil {
				gui.Image(c, bm).Fit(gui.Contain).Grow(1).Padding(6).Radius(radiusControl)
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

		// Clean caption footer.
		gui.Row(c).Gap(6).AlignItems(gui.Center).Padding(0, 4).Children(func() {
			nameColor := t.Text
			if selected {
				nameColor = a.pal.ring
			}
			gui.Text(c, name).Grow(1).FontSize(12).Bold().TextColor(nameColor).SingleLine()
			if asset.Missing {
				a.pillBadge(c, a.t("library.missing"), t.Danger, t.Danger.Alpha(0.14))
			} else if asset.Rating > 0 {
				gui.Text(c, fmt.Sprintf("★%d", asset.Rating)).FontSize(11).Bold().TextColor(a.pal.gold).SingleLine()
			} else {
				gui.Text(c, ext).FontSize(10).TextColor(a.pal.inkMuted).SingleLine()
			}
		})
	})
}

func (a *App) inspector(c *gui.Context) {
	t := c.Theme()
	services := c.Services()
	asset, ok := a.selectedAsset()
	a.syncInspectorSelection(false)

	gui.Box(c).Fill().Padding(6, 12, 10, 6).Background(a.pal.canvas).Children(func() {
		gui.Column(c.Key("arc-inspector")).
			Fill().
			Radius(radiusPanel).
			Clip().
			Background(a.pal.surface).
			Transition(gui.ElementTransition{
				Animation: &arcEnterAnim,
				Enter:     &gui.Motion{X: 20, Opacity: 0},
			}).
			Children(func() {
				if !ok {
					gui.Column(c).Fill().Center().Padding(24).Gap(12).Children(func() {
						gui.Box(c).Size(46, 46).Radius(radiusControl).Background(a.pal.surfaceMuted).Center().Children(func() {
							gui.Icon(c, iconPanelRight).Size(22, 22).TextColor(a.pal.inkMuted)
						})
						gui.Text(c, a.t("inspector.none")).
							FontSize(12).
							TextColor(t.TextMuted).
							TextAlign(gui.Center).
							LineHeight(1.55)
					})
					return
				}

				gui.Scroll(c).Fill().Children(func() {
					gui.Column(c).Padding(14, 16).Gap(11).Children(func() {
						// Frameless clickable hero preview with gentle Arc hover scale.
						hero := gui.ButtonBase(c.Key("insp-hero-" + asset.ID)).
							FillWidth().
							Height(156).
							Radius(radiusControl).
							Clip().
							Background(a.pal.surfaceRaised).
							Shadow(0, 4, 14, 0, a.pal.shadowSm).
							Center().
							Label(a.t("library.preview")).
							Tooltip(a.t("library.preview")).
							OnClick(func() {
								a.openPreview(asset)
							})
						heroScale := float32(1)
						if hero.Pressed() {
							heroScale = 0.985
						} else if hero.Hovered() {
							heroScale = 1.015
						}
						hero.AnimatePose(gui.Pose{Scale: heroScale}, arcSpring)
						hero.Children(func() {
							if bm := a.bitmaps[asset.ID]; bm != nil {
								gui.Image(c, bm).Fit(gui.Contain).Grow(1).Padding(6).Radius(radiusControl)
							} else {
								gui.Column(c).Center().Gap(6).Children(func() {
									ic := iconFile
									switch model.MediaKind(asset.Mime) {
									case model.MediaVideo:
										ic = iconVideo
									case model.MediaImage:
										ic = iconImage
									}
									gui.Icon(c, ic).Size(28, 28).TextColor(a.pal.inkFaint)
									gui.Text(c, a.assetExt(asset)).Bold().TextColor(a.pal.inkMuted)
								})
							}
						})

						// Quick action bar right below preview.
						gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
							a.toolButton(c, "insp-preview-btn", iconEye, a.t("library.preview"), false).
								Grow(1).
								OnClick(func() { a.openPreview(asset) })
							if !asset.Missing && asset.Path != "" {
								a.iconButton(c, "insp-open-btn", iconExternal, a.t("common.openFile"), false).OnClick(func() {
									_ = openLocalFile(asset.Path)
								})
								a.iconButton(c, "insp-reveal-btn", iconFolder, a.t("common.revealFile"), false).OnClick(func() {
									_ = revealLocalFile(asset.Path)
								})
							}
							a.iconButton(c, "insp-copy-path", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
								services.WriteClipboard(asset.Path)
								a.status = a.t("common.copied")
							})
						})

						// Title + frameless pill badges.
						gui.Column(c).Gap(7).Children(func() {
							gui.Text(c, a.assetName(asset)).FontSize(14).Bold().LineHeight(1.3)
							gui.Row(c).Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
								a.pillBadge(c, a.assetExt(asset), t.Text, a.pal.surfaceMuted)
								a.pillBadge(c, a.modeLabel(asset.StorageMode), a.pal.inkSecondary, a.pal.surfaceMuted)
								if asset.Width > 0 && asset.Height > 0 {
									a.pillBadge(c, fmt.Sprintf("%d × %d", asset.Width, asset.Height), a.pal.inkSecondary, a.pal.surfaceMuted)
								}
								if asset.Missing {
									a.pillBadge(c, a.t("library.missing"), t.Danger, t.Danger.Alpha(0.16))
								}
							})
						})

						// Missing file alert & relink CTAs.
						if asset.Missing {
							gui.Column(c).
								Padding(12).
								Gap(8).
								Radius(radiusControl).
								Background(t.Danger.Alpha(0.12)).
								Children(func() {
									gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
										gui.Icon(c, iconAlert).Size(14, 14).TextColor(t.Danger)
										gui.Text(c, a.t("library.missing")).FontSize(12).Bold().TextColor(t.Danger)
									})
									gui.Row(c).Gap(6).Children(func() {
										gui.Button(c.Key("insp-relink-file"), a.t("inspector.relinkFile")).Grow(1).Radius(radiusControl).OnClick(func() {
											a.relinkMissing(false)
										})
										gui.Button(c.Key("insp-relink-dir"), a.t("inspector.relink")).Grow(1).Radius(radiusControl).OnClick(func() {
											a.relinkMissing(true)
										})
									})
								})
						}

						// Interactive Rating.
						gui.Column(c).Gap(6).Children(func() {
							a.sectionLabel(c, a.t("library.rating"))
							gui.Rating(c.Key("rating-"+asset.ID), &a.inspectorRating, 5).OnChange(func() {
								if err := a.lib.SetRating(asset.ID, a.inspectorRating); err == nil {
									a.reload()
								}
							})
						})

						// Wrapping frameless tag pills + inline quick-add.
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

						// Used in Projects / References.
						gui.Column(c).Gap(8).Padding(12).Radius(radiusControl).Background(a.pal.surfaceRaised).Children(func() {
							gui.Row(c).AlignItems(gui.Center).Children(func() {
								a.sectionLabel(c, a.t("inspector.usedIn"))
								gui.Spacer(c)
								if a.inspectorPlaces > 0 {
									gui.Text(c, fmt.Sprintf(a.t("inspector.placements"), a.inspectorPlaces)).FontSize(10).TextColor(a.pal.inkMuted)
								}
							})
							if len(a.inspectorProjects) == 0 {
								gui.Text(c, a.t("inspector.usedNone")).FontSize(11).TextColor(a.pal.inkMuted)
							} else {
								gui.Row(c).Gap(6).Wrap().Children(func() {
									for _, p := range a.inspectorProjects {
										proj := p
										a.tagChip(c, "insp-used-"+proj.ID, proj.Name, true).OnClick(func() {
											a.openProject(proj.ID)
										})
									}
								})
							}
							if len(a.projectList) > 0 {
								gui.PopoverMenuButton(c.Key("insp-add-proj"), "+ "+a.t("library.addToProject"), func(m *gui.Menu) {
									for _, p := range a.projectList {
										proj := p
										if m.Item(proj.Name).Chosen() {
											a.referenceAssetToProject(asset.ID, proj.ID)
										}
									}
								}).FillWidth()
							}
						})

						// Frameless metadata card.
						gui.Column(c).Gap(6).Padding(12).Radius(radiusControl).Background(a.pal.surfaceRaised).Children(func() {
							a.sectionLabel(c, a.t("inspector.details"))
							if asset.Width > 0 && asset.Height > 0 {
								a.metaRow(c, a.t("inspector.dimensions"), fmt.Sprintf("%d × %d px", asset.Width, asset.Height))
								a.metaRow(c, a.t("inspector.aspect"), aspectRatioLabel(asset.Width, asset.Height))
							}
							if a.inspectorFileSize > 0 {
								a.metaRow(c, a.t("inspector.size"), formatBytes(a.inspectorFileSize))
							}
							if asset.Mime != "" {
								a.metaRow(c, a.t("library.mime"), asset.Mime)
							}
							a.metaRow(c, a.t("library.storage"), a.modeLabel(asset.StorageMode))
							if len(asset.Hash) >= 12 {
								a.metaRow(c, a.t("inspector.hash"), asset.Hash[:12]+"…")
							}
							if !asset.CreatedAt.IsZero() {
								a.metaRow(c, a.t("inspector.added"), asset.CreatedAt.Local().Format("2006-01-02 15:04"))
							}
							gui.Box(c).Padding(8, 10).Radius(8).Background(a.pal.surface).Children(func() {
								gui.Text(c, asset.Path).FontSize(11).TextColor(a.pal.inkSecondary).LineHeight(1.4)
							})
						})

						gui.Button(c.Key("insp-delete"), a.t("library.delete")).
							FillWidth().
							Radius(radiusControl).
							TextColor(t.Danger).
							OnClick(func() { a.deleteAssetPrompt(asset) })
					})
				})
			})
	})
}
