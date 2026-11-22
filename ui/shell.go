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
		c.OnShortcut(gui.Cmd, gui.KeyComma, func() {
			a.openSettings(a.settingsTab)
		})

		if a.screen == screenBoard {
			a.boardView(c)
			return
		}

		gui.Split(c, &a.sidebarWidth, func() {
			a.sidebar(c)
		}, func() {
			gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
				switch a.screen {
				case screenProject:
					// Project view owns its single unified top header (no duplicate navbar + hero).
					a.projectView(c)
				default:
					a.workspaceBar(c)
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

// arcNavItem renders a stationary monochrome sidebar item. The active highlight
// pill glides smoothly underneath all labels via attachGlidingPill on the parent
// track (zero text jitter or stretching).
func (a *App) arcNavItem(c *gui.Context, id string, icon *gui.SVG, label string, count int, menu func(m *gui.Menu)) (gui.Element, bool) {
	active := a.selectedNav == id
	btn := gui.ButtonBase(c.Key("nav-" + id)).
		FillWidth().
		Padding(8, 12).
		Gap(10).
		Radius(radiusControl).
		AlignItems(gui.Center).
		OnClick(func() {
			a.handleNav(id)
		})
	if menu != nil {
		btn.PopoverMenu(menu)
	}

	targetFg := a.pal.inkSecondary
	if active || btn.Hovered() {
		targetFg = a.pal.ink
	}
	fg := btn.AnimateColor("fg", targetFg, animFast)

	if !active {
		hoverBg := gui.Transparent
		if btn.Hovered() {
			hoverBg = a.pal.surfaceMuted
		}
		btn.Background(btn.AnimateColor("hbg", hoverBg, animFast))
	}

	btn.Children(func() {
		if icon != nil {
			gui.Icon(c, icon).Size(15, 15).TextColor(fg)
		}
		gui.Text(c, label).FontSize(13).Bold().TextColor(fg).Grow(1).SingleLine()
		if a.showSidebarCounts && count > 0 {
			gui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
		}
	})
	return btn, active
}

func (a *App) sidebar(c *gui.Context) {
	gui.Box(c).Fill().Padding(10, 4, 10, 10).Background(a.pal.canvas).Children(func() {
		gui.Column(c).
			Fill().
			Radius(radiusPanel).
			Clip().
			Background(a.pal.surface).
			Children(func() {
				// Top brand & library header (48px, draggable to move frameless window).
				gui.Row(c).
					Height(48).
					Padding(0, 10, 0, 14).
					Gap(8).
					AlignItems(gui.Center).
					Children(func() {
						gui.Row(c).
							Grow(1).
							FillHeight().
							Gap(10).
							AlignItems(gui.Center).
							DragWindow().
							Children(func() {
								if bm := a.brandLogo(); bm != nil {
									gui.Box(c).
										Size(24, 24).
										Radius(7).
										Background(a.pal.surfaceRaised).
										Center().
										Children(func() {
											gui.Image(c, bm).Size(15, 15).Fit(gui.Contain)
										})
								}
								gui.Text(c, a.libraryTitle()).Bold().FontSize(13).Grow(1).SingleLine()
							})
						a.iconButton(c, "sb-settings", iconSettings, a.t("settings.open"), a.settingsOpen).OnClick(func() {
							a.openSettings(a.settingsTab)
						})
					})

				// Arc UI frameless scrollable navigation with gliding pill highlight underneath labels.
				gui.Scroll(c.Key("nav-scroll")).Grow(1).FillWidth().Children(func() {
					track := gui.Column(c.Key("sidebar-nav-track")).Padding(4, 10, 12, 10).Gap(2)
					var activeBtn gui.Element
					var hasActive bool
					addNav := func(id string, icon *gui.SVG, label string, count int, menu func(m *gui.Menu)) {
						btn, active := a.arcNavItem(c, id, icon, label, count, menu)
						if active {
							activeBtn = btn
							hasActive = true
						}
					}
					track.Children(func() {
						addNav("tag-all", iconLibrary, a.t("shell.allAssets"), a.allAssetsCount, nil)
						addNav("filter-untagged", iconInbox, a.t("shell.untagged"), a.untaggedCount, nil)
						if a.missingCount > 0 {
							addNav("filter-missing", iconAlert, a.t("shell.missing"), a.missingCount, nil)
						}

						if len(a.tags) > 0 {
							gui.Box(c).Padding(14, 10, 4, 10).Children(func() {
								a.sectionLabel(c, a.t("shell.tags"))
							})
							for _, tag := range a.tags {
								tg := tag
								addNav("tag:"+tg.ID, iconTag, tg.Name, a.tagCounts[tg.ID], func(m *gui.Menu) {
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
								addNav("project:"+proj.ID, iconFolder, proj.Name, 0, nil)
							}
						}
					})
					attachGlidingPill(track, activeBtn, hasActive, radiusControl, a.pal.pillActive, a.pal.shadowResting)
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

// workspaceBar is the integrated frameless Library top bar: draggable empty
// chrome, search, filter/sort/import/zoom controls, and custom window controls.
func (a *App) workspaceBar(c *gui.Context) {
	gui.Box(c).Padding(10, 12, 4, 8).Children(func() {
		gui.Row(c.Key("floating-navbar")).
			Height(48).
			Padding(0, 10, 0, 16).
			Gap(8).
			Radius(radiusPanel).
			AlignItems(gui.Center).
			Background(a.pal.surface).
			Children(func() {
				gui.Row(c).Gap(8).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
					gui.Text(c, a.currentViewTitle()).Bold().FontSize(14).SingleLine().MaxWidth(150)
					a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), len(a.assets)), a.pal.inkSecondary, a.pal.surfaceMuted)
				})

				gui.Box(c).Grow(1).FillHeight().DragWindow()

				gui.SearchField(c.Key("library-search"), &a.searchQuery).
					Bind(&a.searchHandle).
					Placeholder(a.t("library.search")).
					Label(a.t("library.search")).
					Width(260).
					OnChange(func() {
						a.applyFilters()
					})

				gui.Box(c).Grow(1).FillHeight().DragWindow()

				filterLabel := a.t("library.filter")
				if a.kindFilter != 0 || a.minRating != 0 {
					filterLabel = filterLabel + " ·"
				}
				a.toolButton(c, "top-filter", iconFilter, filterLabel, a.filterBarOpen || a.kindFilter != 0 || a.minRating != 0).OnClick(func() {
					a.filterBarOpen = !a.filterBarOpen
				})

				layoutIcon := iconLayoutBento
				switch a.layoutMode {
				case layoutMasonry:
					layoutIcon = iconLayoutMasonry
				case layoutGrid:
					layoutIcon = iconLibrary
				}
				layoutBtn := gui.ButtonBase(c.Key("top-layout")).
					Padding(6, 10).
					Gap(5).
					Radius(radiusControl).
					AlignItems(gui.Center).
					Label(a.t("library.layout")).
					Tooltip(a.t("library.layout"))
				lbg := gui.Transparent
				lfg := a.pal.inkSecondary
				if layoutBtn.Pressed() {
					lbg = a.pal.surfaceActive
					lfg = a.pal.ink
				} else if layoutBtn.Hovered() {
					lbg = a.pal.surfaceMuted
					lfg = a.pal.ink
				}
				layoutBtn.Background(layoutBtn.AnimateColor("bg", lbg, animFast)).
					TextColor(layoutBtn.AnimateColor("fg", lfg, animFast)).
					PopoverMenuButton(func(m *gui.Menu) {
						if m.Item(a.t("library.layout.bento")).Radio(a.layoutMode == layoutBento).Chosen() {
							a.setLayoutMode(layoutBento)
						}
						if m.Item(a.t("library.layout.masonry")).Radio(a.layoutMode == layoutMasonry).Chosen() {
							a.setLayoutMode(layoutMasonry)
						}
						if m.Item(a.t("library.layout.grid")).Radio(a.layoutMode == layoutGrid).Chosen() {
							a.setLayoutMode(layoutGrid)
						}
					}).
					Children(func() {
						gui.Icon(c, layoutIcon).Size(14, 14)
						gui.Text(c, a.t("library.layout")).FontSize(12).Bold().SingleLine()
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
							a.syncConfigFromState()
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
					gui.Slider(c.Key("card-zoom"), &a.cardSize, 128, 300).Width(70).OnChange(func() {
						a.syncConfigFromState()
					})
				})

				a.iconButton(c, "top-toggle-inspector", iconPanelRight, a.t("inspector.toggle"), a.inspectorOpen).OnClick(func() {
					a.inspectorOpen = !a.inspectorOpen
				})

				gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)

				a.windowControls(c)
			})
	})
}
