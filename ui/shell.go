package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

func (a *App) shell(c *gui.Context) {
	if a.sidebarWidth < 204 {
		a.sidebarWidth = 240
	} else if a.sidebarWidth > 340 {
		a.sidebarWidth = 340
	}
	if a.cardSize < 128 || a.cardSize > 300 {
		a.cardSize = 184
	}
	gui.Box(c).Fill().Background(a.pal.canvas).Children(func() {
		if a.screen == screenBoard {
			a.boardView(c)
			return
		}

		renderMain := func() {
			gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
				if a.previewOpen {
					a.inlinePreviewView(c)
					return
				}
				switch a.screen {
				case screenProject:
					a.projectView(c)
				default:
					a.workspaceBar(c)
					a.libraryView(c)
				}
			})
		}

		if !a.sidebarOpen {
			renderMain()
		} else {
			gui.Split(c, &a.sidebarWidth, func() {
				a.sidebar(c)
			}, renderMain).Fill()
		}

		// Floating bottom-corner re-open buttons when panels are collapsed
		// (keeps top bar clean and matches bottom-anchored collapse controls).
		if !a.sidebarOpen {
			gui.Box(c.Key("reopen-sidebar-wrap")).
				Absolute().
				Left(14).
				Bottom(14).
				Children(func() {
					gui.Box(c).
						Radius(radiusPill).
						Background(a.pal.surfaceRaised).
						Shadow(0, 6, 18, 0, a.pal.shadowRaised).
						Padding(2).
						Children(func() {
							a.iconButton(c, "bottom-open-sidebar", iconPanelLeft, a.t("sidebar.toggle"), false).OnClick(func() {
								a.sidebarOpen = true
							})
						})
				})
		}
		if !a.inspectorOpen && (a.screen == screenLibrary || a.previewOpen) {
			gui.Box(c.Key("reopen-inspector-wrap")).
				Absolute().
				Right(14).
				Bottom(14).
				Children(func() {
					gui.Box(c).
						Radius(radiusPill).
						Background(a.pal.surfaceRaised).
						Shadow(0, 6, 18, 0, a.pal.shadowRaised).
						Padding(2).
						Children(func() {
							a.iconButton(c, "bottom-open-inspector", iconPanelRight, a.t("inspector.toggle"), false).OnClick(func() {
								a.inspectorOpen = true
							})
						})
				})
		}
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
	case "recent":
		return a.t("shell.recent")
	case "random":
		return a.t("shell.random")
	case "missing":
		return a.t("shell.missing")
	}
	if a.tagID != "" {
		if tg, ok := a.tagByID[a.tagID]; ok {
			return "# " + tg.Name
		}
	}
	return a.t("shell.allAssets")
}

// arcNavItem renders a stationary sidebar row with optional colored icon,
// crisp vector expand/collapse chevron, right-click menu, and drag-drop target.
// The active pill glides smoothly underneath all labels.
func (a *App) arcNavItem(
	c *gui.Context,
	id string,
	icon *gui.SVG,
	iconTint *gui.Color,
	label string,
	count int,
	depth int,
	hasChildren bool,
	expanded bool,
	onToggleExpand func(),
	menu func(m *gui.Menu),
	onDrop func([]string),
) (gui.Element, bool) {
	active := a.selectedNav == id
	indent := float32(depth * 18)

	btn := gui.ButtonBase(c.Key("nav-"+id)).
		FillWidth().
		Padding(7, 10, 7, 8+indent).
		Gap(7).
		Radius(radiusControl).
		AlignItems(gui.Center).
		OnClick(func() {
			a.handleNav(id)
		})
	if menu != nil {
		btn.PopoverMenu(menu)
	}

	dropHover := false
	if onDrop != nil {
		if _, over := gui.DragOver[assetBatchDrag](btn); over {
			dropHover = true
		}
		if drag, ok := gui.Drop[assetBatchDrag](btn); ok && len(drag.IDs) > 0 {
			onDrop(drag.IDs)
		}
	}

	targetFg := a.pal.inkSecondary
	if active || btn.Hovered() || dropHover {
		targetFg = a.pal.ink
	}
	fg := btn.AnimateColor("fg", targetFg, animFast)

	if dropHover {
		btn.Background(a.pal.surfaceActive).Border(1.5, a.pal.controlOn)
	} else if !active {
		hoverBg := gui.Transparent
		if btn.Hovered() {
			hoverBg = a.pal.surfaceMuted
		}
		btn.Background(btn.AnimateColor("hbg", hoverBg, animFast))
	}

	btn.Children(func() {
		if hasChildren && onToggleExpand != nil {
			chevIcon := iconChevronRgt
			if expanded {
				chevIcon = iconChevronDown
			}
			chevBtn := gui.ButtonBase(c.Key("nav-exp-"+id)).
				Size(20, 20).
				Center().
				Radius(6).
				OnClick(onToggleExpand)
			cbg := gui.Transparent
			cfg := a.pal.inkSecondary
			if chevBtn.Hovered() || chevBtn.Pressed() {
				cbg = a.pal.surfaceActive
				cfg = a.pal.ink
			}
			chevBtn.Background(chevBtn.AnimateColor("cbg", cbg, animFast)).
				TextColor(chevBtn.AnimateColor("cfg", cfg, animFast)).
				Children(func() {
					gui.Icon(c, chevIcon).Size(12, 12)
				})
		} else if depth > 0 {
			gui.Box(c).Size(10, 1)
		}
		if icon != nil {
			ic := fg
			if iconTint != nil {
				ic = *iconTint
			}
			gui.Icon(c, icon).Size(15, 15).TextColor(ic)
		}
		gui.Text(c, label).FontSize(13).Bold().TextColor(fg).Grow(1).SingleLine()
		if a.showSidebarCounts && count > 0 {
			gui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
		}
	})
	return btn, active
}

func (a *App) sidebar(c *gui.Context) {
	a.ensureTagTree()
	services := c.Services()
	sf := strings.ToLower(strings.TrimSpace(a.sidebarFilter))

	gui.Box(c).Fill().Padding(10, 4, 10, 10).Background(a.pal.canvas).Children(func() {
		gui.Column(c).
			Fill().
			Radius(radiusPanel).
			Clip().
			Background(a.pal.surface).
			Children(func() {
				// Clean top brand header (draggable frameless window region; utilities live at bottom).
				gui.Row(c).
					Height(44).
					Padding(4, 10, 0, 14).
					Gap(8).
					AlignItems(gui.Center).
					DragWindow().
					Children(func() {
						if bm := a.brandLogo(); bm != nil {
							gui.Image(c, bm).Size(20, 20).Fit(gui.Contain)
						}
						gui.Text(c, a.libraryTitle()).Bold().FontSize(13).Grow(1).SingleLine()
					})

				// Clean sidebar quick-filter + Ctrl+T Go to launcher right below header.
				gui.Row(c).Padding(6, 10, 6, 10).Gap(6).AlignItems(gui.Center).Children(func() {
					gui.SearchField(c.Key("sidebar-filter"), &a.sidebarFilter).
						Placeholder(a.t("library.search")).
						Grow(1)
					a.iconButton(c, "sb-goto-btn", iconCommand, a.t("goto.title")+" (Ctrl+T)", a.gotoOpen).OnClick(func() {
						a.openGotoModal()
					})
				})

				// Scrollable navigation tree with gliding pill highlight.
				gui.Scroll(c.Key("nav-scroll")).Grow(1).FillWidth().Children(func() {
					track := gui.Column(c.Key("sidebar-nav-track")).Padding(4, 10, 12, 10).Gap(2)
					var activeBtn gui.Element
					var hasActive bool
					addNav := func(id string, icon *gui.SVG, tint *gui.Color, label string, count int, depth int, hasChildren, expanded bool, onToggle func(), menu func(m *gui.Menu), onDrop func([]string)) {
						btn, active := a.arcNavItem(c, id, icon, tint, label, count, depth, hasChildren, expanded, onToggle, menu, onDrop)
						if active {
							activeBtn = btn
							hasActive = true
						}
					}

					guideCol := gui.RGBA(255, 255, 255, 0.22)
					if !a.dark {
						guideCol = gui.RGBA(24, 24, 27, 0.18)
					}

					var renderTagNodes func(list []model.Tag, depth int)
					renderTagNodes = func(list []model.Tag, depth int) {
						for _, tag := range list {
							tg := tag
							if sf != "" && !a.tagBranchMatches(tg.ID, sf) {
								continue
							}
							children := a.tagChildren[tg.ID]
							hasCh := len(children) > 0
							expanded := a.expandedTags[tg.ID] || sf != ""
							tagID := tg.ID
							var tint *gui.Color
							if col, ok := a.effectiveTagColor(tagID); ok {
								cc := col
								tint = &cc
							}
							addNav(
								"tag:"+tagID,
								iconTag,
								tint,
								tg.Name,
								a.tagDisplayCount(tagID),
								depth,
								hasCh,
								expanded,
								func() { a.toggleTagExpand(tagID) },
								func(m *gui.Menu) { a.buildTagContextMenu(m, tg, services) },
								func(ids []string) { a.batchApplyTag(ids, tagID) },
							)
							if hasCh && expanded {
								// Wrap nested children in a container that draws ONE continuous
								// neutral white guide line from top to bottom (no broken segments).
								lineX := float32(depth*18) + 18
								chCol := gui.Column(c.Key("tag-ch-" + tagID)).
									FillWidth().
									Gap(2).
									Draw(func(p *gui.Painter, r gui.Rect) {
										if r.H > 6 {
											p.Fill(gui.Rect{X: r.X + lineX, Y: r.Y + 2, W: 1, H: r.H - 6}, guideCol, 0.5)
										}
									})
								chCol.Children(func() {
									renderTagNodes(children, depth+1)
								})
							}
						}
					}

					track.Children(func() {
						addNav("tag-all", iconLibrary, nil, a.t("shell.allAssets"), a.allAssetsCount, 0, false, false, nil, nil, nil)
						addNav("filter-untagged", iconInbox, nil, a.t("shell.untagged"), a.untaggedCount, 0, false, false, nil, nil, nil)
						addNav("filter-recent", iconClock, nil, a.t("shell.recent"), 0, 0, false, false, nil, nil, nil)
						addNav("filter-random", iconShuffle, nil, a.t("shell.random"), 0, 0, false, false, nil, nil, nil)
						if a.missingCount > 0 {
							addNav("filter-missing", iconAlert, nil, a.t("shell.missing"), a.missingCount, 0, false, false, nil, nil, nil)
						}
						addNav("manage-tags", iconTag, nil, a.t("tags.manage"), len(a.tags), 0, false, false, nil, nil, nil)

						// Boards section header with inline "+" creator button.
						gui.Row(c).FillWidth().Padding(14, 4, 4, 10).AlignItems(gui.Center).Children(func() {
							a.sectionLabel(c, a.t("shell.boards"))
							gui.Spacer(c)
							a.miniHeaderButton(c, "sb-add-board", iconPlus, a.t("board.newStandalone"), a.newStandaloneBoardOpen).OnClick(func() {
								a.newStandaloneBoardOpen = !a.newStandaloneBoardOpen
							})
						})

						if a.newStandaloneBoardOpen {
							gui.Row(c.Key("inline-new-board-row")).
								Padding(4, 4, 6, 4).
								Gap(4).
								AlignItems(gui.Center).
								Transition(gui.ElementTransition{
									Animation: &springSmooth,
									Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
									Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
								}).
								Children(func() {
									gui.TextInput(c.Key("new-standalone-board"), &a.newStandaloneBoardName).
										Placeholder(a.t("board.newStandalone")).
										AutoFocus().
										Grow(1).
										OnSubmit(func() { a.createStandaloneBoard() })
									a.iconButton(c, "new-sb-ok", iconCheck, a.t("project.addBoard"), true).OnClick(func() {
										a.createStandaloneBoard()
									})
									a.iconButton(c, "new-sb-x", iconX, a.t("common.cancel"), false).OnClick(func() {
										a.newStandaloneBoardOpen = false
										a.newStandaloneBoardName = ""
									})
								})
						}

						for _, sb := range a.standaloneBoards {
							board := sb
							if sf != "" && !strings.Contains(strings.ToLower(board.Name), sf) {
								continue
							}
							bID := board.ID
							addNav("board:"+bID, iconBoard, nil, board.Name, a.boardCounts[bID], 0, false, false, nil, func(m *gui.Menu) {
								if len(a.projectList) > 0 {
									m.Submenu(a.t("board.moveToProject"), func(sm *gui.Menu) {
										for _, p := range a.projectList {
											proj := p
											if sm.Item(proj.Name).Chosen() {
												a.moveBoardToProject(bID, proj.ID)
											}
										}
									})
									m.Separator()
								}
								if m.Item(a.t("board.deleteBoard")).Chosen() {
									a.deleteStandaloneBoardPrompt(board)
								}
							}, func(ids []string) {
								a.sendAssetsToBoard(ids, bID)
							})
						}

						// Projects section header with inline "+" creator button.
						gui.Row(c).FillWidth().Padding(14, 4, 4, 10).AlignItems(gui.Center).Children(func() {
							a.sectionLabel(c, a.t("shell.projects"))
							gui.Spacer(c)
							a.miniHeaderButton(c, "sb-add-proj", iconPlus, a.t("project.new"), false).OnClick(func() {
								a.openNewProjectModal()
							})
						})

						for _, p := range a.projectList {
							proj := p
							if sf != "" && !strings.Contains(strings.ToLower(proj.Name), sf) {
								continue
							}
							projID := proj.ID
							addNav("project:"+projID, iconFolder, nil, proj.Name, 0, 0, false, false, nil, nil, func(ids []string) {
								a.batchReferenceToProject(ids, projID)
							})
						}

						// Tags section header with inline "+" creator button.
						gui.Row(c).FillWidth().Padding(14, 4, 4, 10).AlignItems(gui.Center).Children(func() {
							a.sectionLabel(c, a.t("shell.tags"))
							gui.Spacer(c)
							a.miniHeaderButton(c, "sb-add-tag", iconPlus, a.t("tag.new"), a.newTagOpen).OnClick(func() {
								a.newTagOpen = !a.newTagOpen
							})
						})

						if a.newTagOpen {
							gui.Row(c.Key("inline-new-tag-row")).
								Padding(4, 4, 6, 4).
								Gap(4).
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

						if len(a.tags) > 0 {
							renderTagNodes(a.tagChildren[""], 0)
						}
					})
					attachGlidingPill(track, activeBtn, hasActive, radiusControl, a.pal.pillActive, a.pal.shadowResting)
				})

				// Bottom utility footer inside the sidebar (Settings, Help, and Collapse Sidebar).
				gui.Row(c).
					Height(44).
					Padding(0, 10).
					Gap(4).
					AlignItems(gui.Center).
					Children(func() {
						a.iconButton(c, "sb-settings", iconSettings, a.t("settings.open"), a.settingsOpen).OnClick(func() {
							a.openSettings(a.settingsTab)
						})
						a.iconButton(c, "sb-help-guide", iconHelp, a.t("onboarding.open"), a.onboardingOpen).OnClick(func() {
							a.openOnboarding()
						})
						gui.Spacer(c)
						a.iconButton(c, "sb-collapse-btn", iconPanelLeft, a.t("sidebar.toggle"), false).OnClick(func() {
							a.sidebarOpen = false
						})
					})
			})
	})
}

func (a *App) miniHeaderButton(c *gui.Context, key string, icon *gui.SVG, tip string, active bool) gui.Element {
	btn := gui.ButtonBase(c.Key(key)).Size(22, 22).Center().Radius(6).Label(tip).Tooltip(tip)
	bg := gui.Transparent
	fg := a.pal.inkMuted
	if active {
		bg = a.pal.surfaceMuted
		fg = a.pal.ink
	} else if btn.Hovered() || btn.Pressed() {
		bg = a.pal.surfaceMuted
		fg = a.pal.ink
	}
	btn.Background(btn.AnimateColor("bg", bg, animFast)).
		TextColor(btn.AnimateColor("fg", fg, animFast)).
		Children(func() {
			gui.Icon(c, icon).Size(13, 13)
		})
	return btn
}

func (a *App) deleteTagPrompt(id string) {
	a.ask(a.t("tag.delete"), a.t("tag.deleteNote"), a.t("common.delete"), func() {
		if err := a.lib.DeleteTag(id); err != nil {
			a.notify(err.Error())
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

// workspaceBar is the responsive frameless Library top bar: adapts smoothly
// across wide/medium/narrow window widths so window controls never overflow.
func (a *App) workspaceBar(c *gui.Context) {
	leftPad := float32(8)
	if !a.sidebarOpen {
		leftPad = 12
	}
	gui.Box(c).Padding(10, 12, 4, leftPad).Children(func() {
		bar := gui.Row(c.Key("floating-navbar")).
			Height(48).
			Padding(0, 10, 0, 10).
			Gap(6).
			Radius(radiusPanel).
			Clip().
			AlignItems(gui.Center).
			Background(a.pal.surface)

		barW := bar.LayoutBounds().W
		compact := barW > 0 && barW < 920
		tight := barW > 0 && barW < 740

		searchW := float32(220)
		titleMaxW := float32(140)
		if tight {
			searchW = 112
			titleMaxW = 78
		} else if compact {
			searchW = 152
			titleMaxW = 104
		}

		bar.Children(func() {
			gui.Row(c).Gap(8).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
				gui.Text(c, a.currentViewTitle()).Bold().FontSize(14).SingleLine().MaxWidth(titleMaxW)
				if !tight {
					a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), len(a.assets)), a.pal.inkSecondary, a.pal.surfaceMuted)
				}
			})

			gui.Box(c).Grow(1).FillHeight().DragWindow()

			gui.SearchField(c.Key("library-search"), &a.searchQuery).
				Bind(&a.searchHandle).
				Placeholder(a.t("library.search")).
				Label(a.t("library.search")).
				Width(searchW).
				OnChange(func() {
					a.applyFilters()
				})

			gui.Box(c).Grow(1).FillHeight().DragWindow()

			filterActive := a.filterBarOpen || a.kindFilter != 0 || a.minRating != 0
			if compact {
				a.iconButton(c, "top-filter", iconFilter, a.t("library.filter"), filterActive).OnClick(func() {
					a.filterBarOpen = !a.filterBarOpen
				})
			} else {
				filterLabel := a.t("library.filter")
				if a.kindFilter != 0 || a.minRating != 0 {
					filterLabel = filterLabel + " ·"
				}
				a.toolButton(c, "top-filter", iconFilter, filterLabel, filterActive).OnClick(func() {
					a.filterBarOpen = !a.filterBarOpen
				})
			}

			layoutIcon := iconLayoutBento
			switch a.layoutMode {
			case layoutMasonry:
				layoutIcon = iconLayoutMasonry
			case layoutGrid:
				layoutIcon = iconLibrary
			}
			layoutBtn := gui.ButtonBase(c.Key("top-layout")).
				Radius(radiusControl).
				AlignItems(gui.Center).
				Label(a.t("library.layout")).
				Tooltip(a.t("library.layout"))
			if compact {
				layoutBtn.Size(32, 32).Center().Radius(radiusPill)
			} else {
				layoutBtn.Padding(6, 10).Gap(5)
			}
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
					if !compact {
						gui.Text(c, a.t("library.layout")).FontSize(12).Bold().SingleLine()
					}
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

			a.iconButton(c, "top-multiselect", iconCheckSquare, a.t("batch.multiMode"), a.multiSelectMode || len(a.selectedIDs) > 1).OnClick(func() {
				a.multiSelectMode = !a.multiSelectMode
				if !a.multiSelectMode && len(a.selectedIDs) > 1 {
					a.selectedIDs = map[string]bool{}
				}
			})

			if !compact {
				gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconZoomIn).Size(13, 13).TextColor(a.pal.inkMuted)
					gui.Slider(c.Key("card-zoom"), &a.cardSize, 128, 300).Width(64).OnChange(func() {
						a.syncConfigFromState()
					})
				})
			}

			gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)

			a.windowControls(c)
		})
	})
}
