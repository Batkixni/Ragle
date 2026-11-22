package ui

import (
	"fmt"
	"path/filepath"
	"strconv"

	gui "github.com/egoist/mygo/ui"
)

func (a *App) shell(c *gui.Context) {
	if a.sidebarWidth <= 0 {
		a.sidebarWidth = 244
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

func (a *App) sidebar(c *gui.Context) {
	t := c.Theme()
	// Frameless Arc UI surface panel nested with airy outer breathing room.
	gui.Box(c).Fill().Padding(10, 6, 10, 10).Background(a.pal.canvas).Children(func() {
		gui.Column(c).
			Fill().
			Radius(radiusPanel).
			Clip().
			Background(a.pal.surface).
			Children(func() {
				// Top brand & library switcher header (52px, aligned with floating workspaceBar).
				gui.Row(c).
					Height(52).
					Padding(0, 14).
					Gap(9).
					AlignItems(gui.Center).
					Children(func() {
						if bm := a.brandLogo(); bm != nil {
							gui.Box(c).
								Size(26, 26).
								Radius(8).
								Background(a.pal.surfaceRaised).
								Shadow(0, 2, 6, 0, a.pal.shadowSm).
								Center().
								Children(func() {
									gui.Image(c, bm).Size(18, 18).Fit(gui.Contain)
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
						gear.Background(gear.AnimateColor("bg", gearBg, arcColorAnim)).
							TextColor(t.TextMuted).
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

				// Main navigation tree with icons and live count badges.
				sb := gui.Sidebar(c.Key("nav-sidebar"), &a.selectedNav, func() {
					gui.SidebarItem(c, "tag-all", iconLibrary, a.t("shell.allAssets")).Children(func() {
						if a.allAssetsCount > 0 {
							gui.Badge(c, strconv.Itoa(a.allAssetsCount))
						}
					})
					gui.SidebarItem(c, "filter-untagged", iconInbox, a.t("shell.untagged")).Children(func() {
						if a.untaggedCount > 0 {
							gui.Badge(c, strconv.Itoa(a.untaggedCount))
						}
					})
					if a.missingCount > 0 {
						gui.SidebarItem(c, "filter-missing", iconAlert, a.t("shell.missing")).Children(func() {
							gui.Badge(c, strconv.Itoa(a.missingCount))
						})
					}

					gui.SidebarSection(c, a.t("shell.tags"), nil, func() {
						for _, tag := range a.tags {
							tg := tag
							item := gui.SidebarItem(c, "tag:"+tg.ID, iconTag, tg.Name)
							item.PopoverMenu(func(m *gui.Menu) {
								if m.Item(a.t("tag.delete")).Chosen() {
									a.deleteTagPrompt(tg.ID)
								}
							})
							item.Children(func() {
								if cnt := a.tagCounts[tg.ID]; cnt > 0 {
									gui.Badge(c, strconv.Itoa(cnt))
								}
							})
						}
					})

					gui.SidebarSection(c, a.t("shell.projects"), nil, func() {
						for _, p := range a.projectList {
							proj := p
							gui.SidebarItem(c, "project:"+proj.ID, iconFolder, proj.Name)
						}
					})
				}).Grow(1)

				if sb.Changed() || a.selectedNav != a.lastNav {
					a.handleNav(a.selectedNav)
				}

				// Smooth animated inline tag creator.
				if a.newTagOpen {
					gui.Row(c.Key("inline-new-tag-row")).
						Padding(8, 10).
						Gap(6).
						AlignItems(gui.Center).
						Transition(gui.ElementTransition{
							Animation: &arcEnterAnim,
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

				// Compact frameless footer actions.
				gui.Column(c).
					Padding(10, 12).
					Gap(8).
					Children(func() {
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

// workspaceBar is an Arc UI floating frameless navbar capsule with specular rim
// light and soft ambient shadow.
func (a *App) workspaceBar(c *gui.Context) {
	t := c.Theme()
	gui.Box(c).Padding(10, 12, 4, 6).Children(func() {
		gui.Row(c.Key("floating-navbar")).
			Height(52).
			Padding(0, 16).
			Gap(10).
			Radius(radiusPanel).
			AlignItems(gui.Center).
			Background(a.pal.surfaceRaised).
			Shadow(0, 6, 22, 0, a.pal.shadowSm).
			InsetShadow(0, 1, 1, 0, a.pal.rimLight).
			Children(func() {
				if a.screen == screenProject {
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						a.iconButton(c, "proj-back-lib", iconArrowLeft, a.t("shell.library"), false).OnClick(func() {
							a.handleNav("tag-all")
						})
						gui.Text(c, a.project.Name).Bold().FontSize(15).SingleLine()
						a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), len(a.projectAssets)), t.TextMuted, a.pal.surface)
					})
					gui.Spacer(c)
					if a.status != "" {
						gui.Text(c, a.status).MaxWidth(280).FontSize(12).TextColor(t.TextMuted).SingleLine()
					}
					a.toolButton(c, "top-share-proj", iconShare, a.t("share.send"), false).OnClick(func() {
						a.shareOpen = true
					})
					a.toolButton(c, "top-recv-pair", iconLink, a.t("share.receiveByCode"), false).OnClick(func() {
						a.pairRecvOpen = true
					})
					return
				}

				// Library view: Refto search-first header + Arc floating controls.
				gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Text(c, a.currentViewTitle()).Bold().FontSize(14).SingleLine().MaxWidth(150)
					a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), len(a.assets)), t.TextMuted, a.pal.surface)
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
					filterLabel = filterLabel + " ●"
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
					gui.Slider(c.Key("card-zoom"), &a.cardSize, 128, 300).Width(76).OnChange(func() {
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
