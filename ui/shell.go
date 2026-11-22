package ui

import (
	"fmt"
	"path/filepath"
	"strconv"

	gui "github.com/egoist/mygo/ui"
)

func (a *App) shell(c *gui.Context) {
	if a.sidebarWidth <= 0 {
		a.sidebarWidth = 236
	}
	if a.cardSize < 128 || a.cardSize > 300 {
		a.cardSize = 184
	}
	gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
		c.OnShortcut(gui.Cmd, gui.KeyF, func() {
			if a.screen == screenLibrary {
				a.searchHandle.Focus(c)
			}
		})
		c.OnShortcut(gui.Cmd, gui.KeyI, func() {
			if a.screen == screenLibrary {
				a.startImport(false)
			}
		})

		if a.screen == screenBoard {
			a.boardView(c)
			return
		}

		gui.Split(c, &a.sidebarWidth, func() {
			a.sidebar(c)
		}, func() {
			gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
				a.workspaceBar(c)
				switch a.screen {
				case screenProject:
					a.projectView(c)
				default:
					a.libraryView(c)
				}
			})
		}).Fill()
	})
}

func (a *App) libraryTitle() string {
	title := filepath.Base(a.cfg.LibraryPath)
	if title == "" || title == "." {
		return "Ragle"
	}
	return title
}

func (a *App) currentViewTitle() string {
	if a.screen == screenProject {
		return a.project.Name
	}
	switch a.navFilter {
	case "untagged":
		return a.t("shell.untagged")
	case "missing":
		return a.t("shell.missing")
	}
	if a.tagID != "" {
		for _, tg := range a.tags {
			if tg.ID == a.tagID {
				return "# " + tg.Name
			}
		}
	}
	return a.t("shell.allAssets")
}

// arcNavItem is a frameless monochrome sidebar item whose active highlight
// smoothly morphs between items using springMorph (420ms, zero bounce).
func (a *App) arcNavItem(c *gui.Context, id string, icon *gui.SVG, label string, count int, menu func(m *gui.Menu)) {
	active := a.selectedNav == id
	btn := gui.ButtonBase(c.Key("nav-" + id)).
		FillWidth().
		Radius(radiusControl).
		OnClick(func() {
			a.handleNav(id)
		})
	if menu != nil {
		btn.PopoverMenu(menu)
	}
	scale := float32(1)
	if btn.Pressed() {
		scale = 0.98
	}
	btn.AnimatePose(gui.Pose{Scale: scale}, springResponsive)

	targetFg := a.pal.inkSecondary
	if active || btn.Hovered() {
		targetFg = a.pal.ink
	}
	fg := btn.AnimateColor("fg", targetFg, animFast)

	content := func() {
		if icon != nil {
			gui.Icon(c, icon).Size(15, 15).TextColor(fg)
		}
		gui.Text(c, label).FontSize(13).Bold().TextColor(fg).Grow(1).SingleLine()
		if count > 0 {
			gui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
		}
	}

	btn.Children(func() {
		if active {
			gui.Row(c.Key("nav-pill-" + id)).
				FillWidth().
				Padding(8, 12).
				Gap(10).
				Radius(radiusControl).
				AlignItems(gui.Center).
				Background(a.pal.surfaceRaised).
				Shadow(0, 1, 3, 0, a.pal.shadowResting).
				Transition(gui.ElementTransition{
					Animation: &springMorph,
					Match:     pillMatchKey{group: "sidebar-nav"},
				}).
				Children(content)
		} else {
			hoverBg := gui.Transparent
			if btn.Hovered() {
				hoverBg = a.pal.surfaceMuted
			}
			bg := btn.AnimateColor("hbg", hoverBg, animFast)
			gui.Row(c).
				FillWidth().
				Padding(8, 12).
				Gap(10).
				Radius(radiusControl).
				AlignItems(gui.Center).
				Background(bg).
				Children(content)
		}
	})
}

func (a *App) sidebar(c *gui.Context) {
	gui.Box(c).Fill().Padding(10, 4, 10, 10).Background(a.pal.canvas).Children(func() {
		gui.Column(c).
			Fill().
			Radius(radiusPanel).
			Clip().
			Background(a.pal.surface).
			Children(func() {
				// Top brand & library switcher header (52px).
				gui.Row(c).
					Height(52).
					Padding(0, 14).
					Gap(10).
					AlignItems(gui.Center).
					Children(func() {
						if bm := a.brandLogo(); bm != nil {
							gui.Box(c).
								Size(26, 26).
								Radius(8).
								Background(a.pal.surfaceRaised).
								Center().
								Children(func() {
									gui.Image(c, bm).Size(17, 17).Fit(gui.Contain)
								})
						}
						gui.Text(c, a.libraryTitle()).Bold().FontSize(13).Grow(1).SingleLine()
						gear := gui.ButtonBase(c.Key("lib-menu")).
							Size(30, 30).
							Center().
							Radius(radiusPill).
							Label(a.t("shell.theme")).
							Tooltip(a.t("shell.switchLib"))
						gearBg := gui.Transparent
						if gear.Hovered() || gear.Pressed() {
							gearBg = a.pal.surfaceMuted
						}
						gear.Background(gear.AnimateColor("bg", gearBg, animFast)).
							TextColor(a.pal.inkSecondary).
							PopoverMenuButton(func(m *gui.Menu) {
								if m.Item(a.t("shell.switchLib")).Chosen() {
									a.chooseLibrary()
								}
								if m.Item(a.t("share.receiveByCode")).Chosen() {
									a.pairRecvOpen = true
								}
								m.Separator()
								m.Submenu(a.t("shell.theme"), func(sm *gui.Menu) {
									if sm.Item(a.t("shell.themeAuto")).Radio(a.themeMode == "auto").Chosen() {
										a.setThemeMode("auto")
									}
									if sm.Item(a.t("shell.themeLight")).Radio(a.themeMode == "light").Chosen() {
										a.setThemeMode("light")
									}
									if sm.Item(a.t("shell.themeDark")).Radio(a.themeMode == "dark").Chosen() {
										a.setThemeMode("dark")
									}
								})
								m.Submenu(a.t("shell.language"), func(sm *gui.Menu) {
									if sm.Item("繁體中文").Radio(a.lang == i18nZh).Chosen() {
										a.setLang(i18nZh)
									}
									if sm.Item("English").Radio(a.lang == i18nEn).Chosen() {
										a.setLang(i18nEn)
									}
								})
							}).
							Children(func() {
								gui.Icon(c, iconSettings).Size(15, 15)
							})
					})

				// Arc UI frameless scrollable navigation with shared-layout morphing highlight.
				gui.Scroll(c.Key("nav-scroll")).Grow(1).FillWidth().Children(func() {
					gui.Column(c).Padding(4, 10, 12, 10).Gap(2).Children(func() {
						a.arcNavItem(c, "tag-all", iconLibrary, a.t("shell.allAssets"), a.allAssetsCount, nil)
						a.arcNavItem(c, "filter-untagged", iconInbox, a.t("shell.untagged"), a.untaggedCount, nil)
						if a.missingCount > 0 {
							a.arcNavItem(c, "filter-missing", iconAlert, a.t("shell.missing"), a.missingCount, nil)
						}

						if len(a.tags) > 0 {
							gui.Box(c).Padding(14, 10, 4, 10).Children(func() {
								a.sectionLabel(c, a.t("shell.tags"))
							})
							for _, tag := range a.tags {
								tg := tag
								a.arcNavItem(c, "tag:"+tg.ID, iconTag, tg.Name, a.tagCounts[tg.ID], func(m *gui.Menu) {
									if m.Item(a.t("tag.delete")).Chosen() {
										a.deleteTagPrompt(tg.ID)
									}
								})
							}
						}

						if len(a.projectList) > 0 {
							gui.Box(c).Padding(14, 10, 4, 10).Children(func() {
								a.sectionLabel(c, a.t("shell.projects"))
							})
							for _, p := range a.projectList {
								proj := p
								a.arcNavItem(c, "project:"+proj.ID, iconFolder, proj.Name, 0, nil)
							}
						}
					})
				})

				// Smooth critically-damped inline tag creator.
				if a.newTagOpen {
					gui.Row(c.Key("inline-new-tag-row")).
						Padding(8, 10).
						Gap(6).
						AlignItems(gui.Center).
						Transition(gui.ElementTransition{
							Animation: &springSmooth,
							Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
							Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
						}).
						Children(func() {
							gui.TextInput(c.Key("new-tag"), &a.newTagName).
								Placeholder(a.t("tag.new")).
								AutoFocus().
								Grow(1).
								OnSubmit(func() { a.createTag() })
							a.iconButton(c, "new-tag-ok", iconCheck, a.t("tag.create"), true).OnClick(func() {
								a.createTag()
							})
							a.iconButton(c, "new-tag-x", iconX, a.t("common.cancel"), false).OnClick(func() {
								a.newTagOpen = false
								a.newTagName = ""
							})
						})
				}

				// Quiet bottom actions.
				gui.Column(c).Padding(10, 12).Gap(8).Children(func() {
					gui.Row(c).Gap(6).Children(func() {
						a.toolButton(c, "sb-add-tag", iconPlus, a.t("tag.new"), a.newTagOpen).
							Grow(1).
							OnClick(func() { a.newTagOpen = !a.newTagOpen })
						a.toolButton(c, "sb-add-proj", iconFolder, a.t("project.new"), false).
							Grow(1).
							OnClick(func() { a.openNewProjectModal() })
					})
					if a.tagID != "" {
						a.toolButton(c, "sb-del-tag", iconTrash, a.t("tag.delete"), false).
							FillWidth().
							OnClick(func() { a.deleteTagPrompt(a.tagID) })
					}
					gui.Row(c).AlignItems(gui.Center).Padding(2, 4).Children(func() {
						gui.Text(c, a.t("shell.localFirst")).FontSize(10).TextColor(a.pal.inkFaint).Grow(1).SingleLine()
						a.iconButton(c, "sb-recv-pair", iconLink, a.t("share.receiveByCode"), false).OnClick(func() {
							a.pairRecvOpen = true
						})
					})
				})
			})
	})
}

func (a *App) deleteTagPrompt(id string) {
	a.ask(a.t("tag.delete"), a.t("tag.deleteNote"), a.t("common.delete"), func() {
		if err := a.lib.DeleteTag(id); err != nil {
			a.status = err.Error()
			return
		}
		if a.tagID == id {
			a.tagID = ""
			a.selectedNav = "tag-all"
			a.lastNav = "tag-all"
		}
		a.reload()
	})
}

// workspaceBar is a clean monochrome Arc floating navbar (zero glare, soft grounded shadow).
func (a *App) workspaceBar(c *gui.Context) {
	gui.Box(c).Padding(10, 12, 4, 8).Children(func() {
		gui.Row(c.Key("floating-navbar")).
			Height(52).
			Padding(0, 16).
			Gap(10).
			Radius(radiusPanel).
			AlignItems(gui.Center).
			Background(a.pal.surface).
			Children(func() {
				if a.screen == screenProject {
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						a.iconButton(c, "proj-back-lib", iconArrowLeft, a.t("shell.library"), false).OnClick(func() {
							a.handleNav("tag-all")
						})
						gui.Text(c, a.project.Name).Bold().FontSize(15).SingleLine()
						a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), len(a.projectAssets)), a.pal.inkSecondary, a.pal.surfaceMuted)
					})
					gui.Spacer(c)
					if a.status != "" {
						gui.Text(c, a.status).MaxWidth(280).FontSize(12).TextColor(a.pal.inkSecondary).SingleLine()
					}
					a.toolButton(c, "top-share-proj", iconShare, a.t("share.send"), false).OnClick(func() {
						a.shareOpen = true
					})
					a.toolButton(c, "top-recv-pair", iconLink, a.t("share.receiveByCode"), false).OnClick(func() {
						a.pairRecvOpen = true
					})
					return
				}

				gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Text(c, a.currentViewTitle()).Bold().FontSize(14).SingleLine().MaxWidth(150)
					a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), len(a.assets)), a.pal.inkSecondary, a.pal.surfaceMuted)
				})

				gui.Spacer(c)

				gui.SearchField(c.Key("library-search"), &a.searchQuery).
					Bind(&a.searchHandle).
					Placeholder(a.t("library.search")).
					Label(a.t("library.search")).
					Width(280).
					OnChange(func() {
						a.applyFilters()
					})

				gui.Spacer(c)

				filterLabel := a.t("library.filter")
				if a.kindFilter != 0 || a.minRating != 0 {
					filterLabel = filterLabel + " ·"
				}
				a.toolButton(c, "top-filter", iconFilter, filterLabel, a.filterBarOpen || a.kindFilter != 0 || a.minRating != 0).OnClick(func() {
					a.filterBarOpen = !a.filterBarOpen
				})

				gui.PopoverMenuButton(c.Key("top-sort"), a.t("library.sort"), func(m *gui.Menu) {
					opts := []struct {
						id    int
						label string
					}{
						{sortNewest, a.t("library.sortNewest")},
						{sortOldest, a.t("library.sortOldest")},
						{sortName, a.t("library.sortName")},
						{sortRating, a.t("library.sortRating")},
					}
					for _, o := range opts {
						opt := o
						if m.Item(opt.label).Radio(a.sortBy == opt.id).Chosen() {
							a.sortBy = opt.id
							a.applyFilters()
						}
					}
				})

				gui.PopoverMenuButton(c.Key("top-import"), "+ "+a.t("library.import"), func(m *gui.Menu) {
					if m.Item(a.t("library.files")).Shortcut(gui.Cmd, gui.KeyI).Chosen() {
						a.startImport(false)
					}
					if m.Item(a.t("library.folder")).Chosen() {
						a.startImport(true)
					}
				})

				gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconZoomIn).Size(13, 13).TextColor(a.pal.inkMuted)
					gui.Slider(c.Key("card-zoom"), &a.cardSize, 128, 300).Width(74).OnChange(func() {
						a.cfg.CardSize = a.cardSize
						a.saveConfig()
					})
				})

				a.iconButton(c, "top-toggle-inspector", iconPanelRight, a.t("inspector.toggle"), a.inspectorOpen).OnClick(func() {
					a.inspectorOpen = !a.inspectorOpen
				})
			})
	})
}
