package ui

import (
	"fmt"
	"strconv"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
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
	shellBg := a.pal.canvas
	if a.glassActive {
		shellBg = gui.Transparent
	}
	root := gui.Box(c.Key("shell-root")).Fill().Background(shellBg)

	targetSidebarW := float32(0)
	if a.sidebarOpen {
		targetSidebarW = a.sidebarWidth
	}
	var animSidebarW float32
	if a.sidebarResizing && a.sidebarOpen {
		animSidebarW = root.Jump("sidebar-w", a.sidebarWidth)
	} else {
		animSidebarW = root.AnimateTo("sidebar-w", targetSidebarW, springSidebar)
	}

	root.Children(func() {
		if a.screen == screenBoard {
			a.boardView(c)
			return
		}

		renderMain := func() {
			mainBg := a.pal.canvas
			if a.glassActive {
				mainBg = gui.Transparent
			}
			gui.Column(c).Fill().Background(mainBg).Children(func() {
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

		gui.Row(c.Key("shell-layout")).Fill().Children(func() {
			if animSidebarW > 0.5 {
				progress := animSidebarW / a.sidebarWidth
				if progress < 0 {
					progress = 0
				} else if progress > 1 {
					progress = 1
				}
				gui.Box(c.Key("sidebar-anim-clip")).
					Width(animSidebarW).
					FillHeight().
					Clip().
					Children(func() {
						slide := gui.Box(c.Key("sidebar-slide-wrap")).
							Absolute().
							Left(animSidebarW - a.sidebarWidth).
							Top(0).
							Bottom(0).
							Width(a.sidebarWidth)
						if progress < 0.995 {
							slide.Opacity(0.35 + 0.65*progress)
						}
						slide.Children(func() {
							a.sidebar(c)
						})
					})

				// Flush edge-to-edge resize seam when sidebar is expanded.
				if a.sidebarOpen && animSidebarW >= a.sidebarWidth-1.5 {
					grip := gui.Box(c.Key("sidebar-resize-grip")).
						Width(1).
						FillHeight().
						Background(a.chromeHairline()).
						Cursor(gui.CursorResizeEW).
						HandleInput(func(ev gui.InputEvent) bool {
							winX := a.sidebarGripX + ev.X
							switch ev.Kind {
							case gui.InputPointerDown:
								if ev.Button != 0 {
									return false
								}
								a.sidebarResizing = true
								a.sidebarDragStartX = winX
								a.sidebarDragStartW = a.sidebarWidth
								return true
							case gui.InputPointerMove:
								if !a.sidebarResizing {
									return false
								}
								w := a.sidebarDragStartW + (winX - a.sidebarDragStartX)
								if w < 204 {
									w = 204
								} else if w > 340 {
									w = 340
								}
								a.sidebarWidth = w
								return true
							case gui.InputPointerUp:
								if !a.sidebarResizing {
									return false
								}
								if ev.Canceled {
									a.sidebarWidth = a.sidebarDragStartW
								}
								a.sidebarResizing = false
								return true
							}
							return false
						})
					a.sidebarGripX = grip.Bounds().X
					if a.sidebarResizing || grip.Hovered() {
						grip.Width(3).Background(a.pal.surfaceActive)
					}
				}
			}

			gui.Box(c.Key("shell-main-slot")).Grow(1).FillHeight().Clip().Children(renderMain)
		})

		// Floating bottom-corner re-open buttons (hidden during Focus Mode for a pure canvas).
		if !a.focusMode && !a.sidebarOpen {
			gui.Box(c.Key("reopen-sidebar-wrap")).
				Absolute().
				Left(14).
				Bottom(14).
				Transition(gui.ElementTransition{
					Animation: &springSnappy,
					Enter:     &gui.Motion{X: -12, Scale: 0.9, Opacity: 0},
					Exit:      &gui.Motion{X: -12, Scale: 0.9, Opacity: 0},
				}).
				Children(func() {
					gui.Box(c).
						Radius(radiusPill).
						Background(a.popoverSurface()).
						Border(1, a.chromeHairline()).
						Shadow(0, 6, 18, 0, a.pal.shadowRaised).
						Padding(2).
						Children(func() {
							a.iconButton(c, "bottom-open-sidebar", iconPanelLeft, a.sidebarToggleTooltip(), false).OnClick(func() {
								a.sidebarOpen = true
							})
						})
				})
		}
		_, hasSel := a.selectedAsset()
		if !a.focusMode && !a.inspectorOpen && ((a.screen == screenLibrary && hasSel) || a.previewOpen) {
			gui.Box(c.Key("reopen-inspector-wrap")).
				Absolute().
				Right(14).
				Bottom(14).
				Transition(gui.ElementTransition{
					Animation: &springSnappy,
					Enter:     &gui.Motion{X: 12, Scale: 0.9, Opacity: 0},
					Exit:      &gui.Motion{X: 12, Scale: 0.9, Opacity: 0},
				}).
				Children(func() {
					gui.Box(c).
						Radius(radiusPill).
						Background(a.popoverSurface()).
						Border(1, a.chromeHairline()).
						Shadow(0, 6, 18, 0, a.pal.shadowRaised).
						Padding(2).
						Children(func() {
							a.iconButton(c, "bottom-open-inspector", iconPanelRight, a.t("inspector.toggle"), false).OnClick(func() {
								a.inspectorOpen = true
							})
						})
				})
		}

		// Whisper-quiet floating Focus Mode exit pill in top-right corner (never traps user).
		if a.focusMode {
			a.focusModeExitPill(c)
		}
	})
}

func (a *App) focusModeExitPill(c *gui.Context) {
	wrap := gui.Box(c.Key("focus-exit-wrap")).
		Absolute().
		Top(10).
		Right(12).
		Transition(gui.ElementTransition{
			Animation: &springSnappy,
			Enter:     &gui.Motion{Y: -8, Opacity: 0},
			Exit:      &gui.Motion{Y: -8, Opacity: 0},
		})
	targetOp := float32(0.32)
	if wrap.Hovered() {
		targetOp = 0.96
	}
	wrap.Opacity(wrap.AnimateTo("focus-op", targetOp, animFast)).
		Children(func() {
			gui.Row(c).
				Radius(radiusPill).
				Background(a.popoverSurface()).
				Border(1, a.chromeHairline()).
				Shadow(0, 8, 22, 0, a.pal.shadowRaised).
				Padding(4, 10).
				Gap(6).
				AlignItems(gui.Center).
				Children(func() {
					a.toolButton(c, "focus-exit-btn", iconRestore, a.t("view.focusExit"), false).
						Padding(2, 4).
						OnClick(func() {
							a.toggleFocusMode()
						})
				})
		})
}

func (a *App) inspectorShouldShow() bool {
	if !a.inspectorOpen {
		return false
	}
	if a.previewOpen {
		_, ok := a.assetByID[a.previewAssetID]
		return ok
	}
	_, ok := a.selectedAsset()
	return ok
}

// withAnimatedInspector renders mainContent alongside a smoothly sliding,
// resizable right Inspector panel (shared by Library and Inline Preview).
// The inspector only slides open when an asset is actively selected, keeping
// the browsing stage completely uncluttered when nothing is selected.
func (a *App) withAnimatedInspector(c *gui.Context, key string, mainContent func()) {
	if a.inspectorWidth < 260 {
		a.inspectorWidth = 316
	} else if a.inspectorWidth > 460 {
		a.inspectorWidth = 460
	}
	show := a.inspectorShouldShow()
	row := gui.Row(c.Key(key)).Grow(1).FillWidth()
	targetW := float32(0)
	if show {
		targetW = a.inspectorWidth
	}
	var animW float32
	if a.inspectorResizing && show {
		animW = row.Jump("inspector-w", a.inspectorWidth)
	} else {
		animW = row.AnimateTo("inspector-w", targetW, springSidebar)
	}

	row.Children(func() {
		gui.Box(c.Key(key + "-main")).Grow(1).FillHeight().Clip().Children(mainContent)

		if animW > 0.5 {
			if show && animW >= a.inspectorWidth-1.5 {
				grip := gui.Box(c.Key(key + "-resize-grip")).
					Width(1).
					FillHeight().
					Background(a.chromeHairline()).
					Cursor(gui.CursorResizeEW).
					HandleInput(func(ev gui.InputEvent) bool {
						winX := a.inspectorGripX + ev.X
						switch ev.Kind {
						case gui.InputPointerDown:
							if ev.Button != 0 {
								return false
							}
							a.inspectorResizing = true
							a.inspectorDragX = winX
							a.inspectorDragW = a.inspectorWidth
							return true
						case gui.InputPointerMove:
							if !a.inspectorResizing {
								return false
							}
							w := a.inspectorDragW - (winX - a.inspectorDragX)
							if w < 260 {
								w = 260
							} else if w > 460 {
								w = 460
							}
							a.inspectorWidth = w
							return true
						case gui.InputPointerUp:
							if !a.inspectorResizing {
								return false
							}
							if ev.Canceled {
								a.inspectorWidth = a.inspectorDragW
							}
							a.inspectorResizing = false
							return true
						}
						return false
					})
				a.inspectorGripX = grip.Bounds().X
				if a.inspectorResizing || grip.Hovered() {
					grip.Width(3).Background(a.pal.surfaceActive)
				}
			}

			progress := animW / a.inspectorWidth
			if progress < 0 {
				progress = 0
			} else if progress > 1 {
				progress = 1
			}
			gui.Box(c.Key(key + "-insp-clip")).
				Width(animW).
				FillHeight().
				Clip().
				Children(func() {
					slide := gui.Box(c.Key(key + "-insp-slide")).
						Absolute().
						Left(0).
						Top(0).
						Bottom(0).
						Width(a.inspectorWidth)
					if progress < 0.995 {
						slide.Opacity(0.35 + 0.65*progress)
					}
					slide.Children(func() {
						a.inspector(c)
					})
				})
		}
	})
}

func (a *App) libraryTitle() string {
	return "Raglex"
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
	isTag := strings.HasPrefix(id, "tag:")
	isProject := strings.HasPrefix(id, "project:")
	reorderable := isTag || isProject

	// Outer wrapper isolates assetBatchDrag drops from reorder drops (MyGo's
	// DragOver overwrites accepts per element) and drives smooth spring FLIP
	// movement whenever sibling order changes.
	wrap := gui.Box(c.Key("nav-wrap-" + id)).FillWidth()
	if reorderable {
		wrap.Transition(gui.ElementTransition{
			Animation: &springSnappy,
			Position:  true,
		})
	}

	dropHover := false
	if onDrop != nil {
		if _, over := gui.DragOver[assetBatchDrag](wrap); over {
			dropHover = true
		}
		if drag, ok := gui.Drop[assetBatchDrag](wrap); ok && len(drag.IDs) > 0 {
			onDrop(drag.IDs)
		}
	}

	var btn gui.Element
	wrap.Children(func() {
		btn = gui.ButtonBase(c.Key("nav-"+id)).
			FillWidth().
			Padding(7, 10, 7, 8+indent).
			Gap(7).
			Radius(radiusControl).
			AlignItems(gui.Center).
			OnClick(func() {
				a.handleNav(id)
			})
		if menu != nil {
			btn.PopoverMenu(menu, a.popoverMenuOpts())
		}

		if isTag {
			tagID := strings.TrimPrefix(id, "tag:")
			btn.Drag(tagReorderDrag{ID: tagID})
			if d, over := gui.DragOver[tagReorderDrag](btn); over && d.ID != tagID {
				a.reorderTagLive(d.ID, tagID)
			}
			if _, ok := gui.Drop[tagReorderDrag](btn); ok {
				a.lastReorderPair = ""
			}
		} else if isProject {
			projID := strings.TrimPrefix(id, "project:")
			btn.Drag(projectReorderDrag{ID: projID})
			if d, over := gui.DragOver[projectReorderDrag](btn); over && d.ID != projID {
				a.reorderProjectLive(d.ID, projID)
			}
			if _, ok := gui.Drop[projectReorderDrag](btn); ok {
				a.lastReorderPair = ""
			}
		}

		dragging := reorderable && btn.Dragging()
		targetOp := float32(1.0)
		if dragging {
			targetOp = 0.24
		}
		if op := btn.AnimateTo("drag-op", targetOp, animFast); op < 0.995 {
			btn.Opacity(op)
		}

		targetFg := a.pal.inkSecondary
		if active || btn.Hovered() || dropHover {
			targetFg = a.pal.ink
		}
		fg := btn.AnimateColor("fg", targetFg, animFast)

		if dropHover {
			btn.Background(a.pal.surfaceActive).Border(1, a.pal.controlOn)
		} else if dragging {
			btn.Background(a.pal.surfaceMuted)
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
	})
	return btn, active
}

func (a *App) sidebar(c *gui.Context) {
	a.ensureTagTree()
	services := c.Services()
	rawQ := strings.ToLower(strings.TrimSpace(a.searchQuery))
	sf := ""
	if rawQ != "" {
		matchedNav := false
		for _, sb := range a.standaloneBoards {
			if strings.Contains(strings.ToLower(sb.Name), rawQ) {
				matchedNav = true
				break
			}
		}
		if !matchedNav {
			for _, p := range a.projectList {
				if strings.Contains(strings.ToLower(p.Name), rawQ) {
					matchedNav = true
					break
				}
			}
		}
		if !matchedNav {
			for _, tg := range a.tags {
				if strings.Contains(strings.ToLower(tg.Name), rawQ) {
					matchedNav = true
					break
				}
			}
		}
		if matchedNav {
			sf = rawQ
		}
	}

	gui.Column(c).
		Fill().
		Clip().
		Background(a.pal.surface).
		Children(func() {
				// Sidebar top header:
				// - macOS: native traffic lights clearance (left) + account & settings (right)
				// - Windows/Linux: clean brand header (logo + title)
				gui.Row(c).
					Height(48).
					Padding(0, 10, 0, 14).
					Gap(6).
					AlignItems(gui.Center).
					DragWindow().
					Children(func() {
						if useMacChrome {
							a.macTrafficLightSpacer(c)
							gui.Box(c).Grow(1).FillHeight().DragWindow()
							a.macSidebarHeaderUtilities(c)
						} else {
							if bm := a.brandLogo(); bm != nil {
								gui.Image(c, bm).Size(20, 20).Fit(gui.Contain)
							}
							gui.Text(c, a.libraryTitle()).Bold().FontSize(13).Grow(1).SingleLine()
						}
					})

				// Single unified search bar (filters assets, projects & matching nav items) + Ctrl+T launcher.
				gui.Row(c).Padding(6, 10, 6, 10).Gap(6).AlignItems(gui.Center).Children(func() {
					sbSearch := gui.SearchField(c.Key("library-search"), &a.searchQuery).
						Bind(&a.searchHandle).
						Placeholder(a.t("library.search")).
						Label(a.t("library.search")).
						Grow(1).
						OnChange(func() {
							a.projSearchQuery = a.searchQuery
							a.applyFilters()
						})
					if a.glassActive {
						sbSearch.Border(1, a.chromeHairline())
					}
					a.iconButton(c, "sb-goto-btn", iconCommand, fmt.Sprintf("%s (%s)", a.t("goto.title"), a.shortcutLabel("quick_goto")), a.gotoOpen).OnClick(func() {
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
									Transition(gui.ElementTransition{
										Animation: &springSnappy,
										Position:  true,
									}).
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
									a.menuSub(m, iconFolder, a.t("board.moveToProject"), func(sm *gui.Menu) {
										for _, p := range a.projectList {
											proj := p
											if a.menuItem(sm, iconFolder, proj.Name, "", false).Chosen() {
												a.moveBoardToProject(bID, proj.ID)
											}
										}
									})
									m.Separator()
								}
								if a.menuItem(m, iconTrash, a.t("board.deleteBoard"), "Del", true).Chosen() {
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

						for i, p := range a.projectList {
							idx := i
							proj := p
							if sf != "" && !strings.Contains(strings.ToLower(proj.Name), sf) {
								continue
							}
							projID := proj.ID
							addNav("project:"+projID, iconFolder, nil, proj.Name, 0, 0, false, false, nil, func(m *gui.Menu) {
								moved := false
								if idx > 0 {
									moved = true
									if a.menuItem(m, iconChevronUp, a.t("common.moveUp"), "", false).Chosen() {
										a.moveProjectOrder(projID, -1)
									}
								}
								if idx < len(a.projectList)-1 {
									moved = true
									if a.menuItem(m, iconChevronDown, a.t("common.moveDown"), "", false).Chosen() {
										a.moveProjectOrder(projID, 1)
									}
								}
								if moved {
									m.Separator()
								}
								if a.menuItem(m, iconEdit, a.t("project.edit"), "", false).Chosen() {
									a.openProject(projID)
									a.openEditProjectModal()
								}
								if a.menuItem(m, iconTrash, a.t("project.delete"), "Del", true).Chosen() {
									a.deleteProjectPrompt(projID)
								}
							}, func(ids []string) {
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

				// Sidebar bottom footer:
				// - macOS: Raglex brand logo (bottom-left) + Collapse Sidebar (bottom-right)
				// - Windows/Linux: Account, Settings, Help (bottom-left) + Collapse Sidebar (bottom-right)
				if useMacChrome {
					gui.Row(c).
						Height(44).
						Padding(0, 10, 0, 14).
						Gap(8).
						AlignItems(gui.Center).
						BorderWidth(1, 0, 0, 0).
						BorderColor(a.chromeHairline()).
						Children(func() {
							if bm := a.brandLogo(); bm != nil {
								gui.Image(c, bm).Size(20, 20).Fit(gui.Contain)
							}
							gui.Text(c, a.libraryTitle()).Bold().FontSize(13).TextColor(a.pal.ink).Grow(1).SingleLine()
							if a.sidebarOpen {
								a.iconButton(c, "sb-collapse-btn", iconPanelLeft, a.sidebarToggleTooltip(), false).OnClick(func() {
									a.sidebarOpen = false
								})
							}
						})
				} else {
					gui.Row(c).
						Height(44).
						Padding(0, 10).
						Gap(4).
						AlignItems(gui.Center).
						Children(func() {
							acctTip := a.t("account.openSidebar")
							if a.accountEmail != "" {
								acctTip = a.accountEmail
							}
							a.iconButton(c, "sb-account", iconUser, acctTip, a.settingsOpen && a.settingsTab == settingsTabAccount).OnClick(func() {
								a.openSettings(settingsTabAccount)
							})
							a.iconButton(c, "sb-settings", iconSettings, a.settingsOpenTooltip(), a.settingsOpen && a.settingsTab != settingsTabAccount).OnClick(func() {
								a.openSettings(a.settingsTab)
							})
							a.iconButton(c, "sb-help-guide", iconHelp, a.t("onboarding.open"), a.onboardingOpen).OnClick(func() {
								a.openOnboarding()
							})
							gui.Spacer(c)
							if a.sidebarOpen {
								a.iconButton(c, "sb-collapse-btn", iconPanelLeft, a.sidebarToggleTooltip(), false).OnClick(func() {
									a.sidebarOpen = false
								})
							}
						})
				}
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

// workspaceBar is the responsive seamless edge-to-edge Library top bar:
// fuses cleanly with the window edge (no floating card gap or outer corner radius)
// and includes the Global Display Options Popover & Web URL import.
func (a *App) workspaceBar(c *gui.Context) {
	if a.focusMode {
		return
	}
	bar := gui.Row(c.Key("floating-navbar")).
		Height(48).
		FillWidth().
		Padding(0, 10, 0, 14).
		Gap(6).
		Clip().
		AlignItems(gui.Center).
		Background(a.pal.surface).
		BorderWidth(0, 0, 1, 0).
		BorderColor(a.chromeHairline())

	barW := bar.LayoutBounds().W
	compact := barW > 0 && barW < 920
	tight := barW > 0 && barW < 740

	titleMaxW := float32(200)
	if tight {
		titleMaxW = 110
	} else if compact {
		titleMaxW = 150
	}

	bar.Children(func() {
		if !a.sidebarOpen && a.macTrafficLightSpacer(c) {
			gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
		}
		gui.Row(c).Gap(8).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
			gui.Text(c, a.currentViewTitle()).Bold().FontSize(14).SingleLine().MaxWidth(titleMaxW)
			if !tight && a.showSidebarCounts {
				a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), len(a.assets)), a.pal.inkSecondary, a.pal.surfaceMuted)
			}
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
			Tooltip(a.t("library.layout")).
			OnClick(func() {
				a.displayPopoverOpen = !a.displayPopoverOpen
			})
		if compact {
			layoutBtn.Size(32, 32).Center().Radius(radiusPill)
		} else {
			layoutBtn.Padding(6, 10).Gap(5)
		}
		lbg := gui.Transparent
		lfg := a.pal.inkSecondary
		if a.displayPopoverOpen || layoutBtn.Pressed() {
			lbg = a.pal.surfaceActive
			lfg = a.pal.ink
		} else if layoutBtn.Hovered() {
			lbg = a.pal.surfaceMuted
			lfg = a.pal.ink
		}
		layoutBtn.Background(layoutBtn.AnimateColor("bg", lbg, animFast)).
			TextColor(layoutBtn.AnimateColor("fg", lfg, animFast)).
			Children(func() {
				gui.Icon(c, layoutIcon).Size(14, 14)
				if !compact {
					gui.Text(c, a.t("library.layout")).FontSize(12).Bold().SingleLine()
				}
			})

		// Global View Display Options Popover (Layout, Thumb Bg, Show Titles/Ext/Rating, Grayscale, Glass, Panels).
		gui.PopoverBase(c.Key("view-display-popover"), layoutBtn, &a.displayPopoverOpen, func(p gui.Element) {
			p.AttachTo(layoutBtn, gui.AnchorBottomRight, gui.AnchorTopRight).
				Margin(6, 0, 0, 0).
				Width(284).
				Padding(14).
				Radius(radiusPanel).
				Background(a.popoverSurface()).
				Border(1, a.chromeHairline()).
				Shadow(0, 14, 34, 0, a.pal.shadowFloating).
				Transition(gui.ElementTransition{
					Animation: &springSnappy,
					Enter:     &gui.Motion{Y: -6, Opacity: 0},
					Exit:      &gui.Motion{Y: -6, Opacity: 0},
				}).
				Children(func() {
					gui.Column(c).FillWidth().Gap(10).Children(func() {
						// 1. Layout Mode selector.
						a.sectionLabel(c, a.t("view.layoutMode"))
						gui.Row(c).FillWidth().Gap(4).Children(func() {
							modes := []struct {
								id    string
								icon  *gui.SVG
								label string
							}{
								{layoutBento, iconLayoutBento, a.t("library.layout.bento")},
								{layoutMasonry, iconLayoutMasonry, a.t("library.layout.masonry")},
								{layoutGrid, iconLibrary, a.t("library.layout.grid")},
							}
							for _, m := range modes {
								md := m
								active := a.layoutMode == md.id
								a.toolButton(c, "disp-lay-"+md.id, md.icon, md.label, active).
									Grow(1).
									Padding(6, 6).
									Center().
									OnClick(func() {
										a.setLayoutMode(md.id)
										a.displayPopoverOpen = false
									})
							}
						})

						// 2. Thumbnail Background switcher (None / Black / Gray / White / Checker).
						gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
						a.sectionLabel(c, a.t("view.thumbBg"))
						gui.Row(c).FillWidth().Gap(5).Children(func() {
							bgs := []struct {
								id    string
								label string
								col   gui.Color
							}{
								{"", a.t("view.thumbBgNone"), gui.Transparent},
								{"black", a.t("view.thumbBgBlack"), gui.Hex("#09090B")},
								{"gray", a.t("view.thumbBgGray"), gui.Hex("#6B7280")},
								{"white", a.t("view.thumbBgWhite"), gui.Hex("#F4F4F5")},
								{"checker", a.t("view.thumbBgChecker"), gui.Transparent},
							}
							for _, bgOpt := range bgs {
								opt := bgOpt
								active := a.thumbBg == opt.id
								b := gui.ButtonBase(c.Key("disp-bg-"+opt.id)).
									Grow(1).
									Padding(5, 4).
									Gap(4).
									Radius(radiusControl - 2).
									Center().
									Label(opt.label).
									Tooltip(opt.label).
									OnClick(func() { a.setThumbBg(opt.id) })
								bbg := a.pal.surface
								bfg := a.pal.inkSecondary
								if active {
									bbg = a.pal.controlOn
									bfg = a.pal.controlGlyph
								} else if b.Hovered() {
									bbg = a.pal.surfaceMuted
									bfg = a.pal.ink
								}
								b.Background(bbg).TextColor(bfg).Children(func() {
									sw := gui.Box(c).Size(12, 12).Radius(3).Clip().Border(1, a.chromeHairline())
									if opt.id == "checker" {
										sw.Draw(func(pt *gui.Painter, r gui.Rect) { a.paintCheckerboard(pt, r, 4) })
									} else if opt.id == "" {
										sw.Background(a.pal.surfaceMuted)
									} else {
										sw.Background(opt.col)
									}
									gui.Text(c, opt.label).FontSize(11).Bold().SingleLine()
								})
							}
						})

						// 3. Global Card Display Toggles (Show Title, Ext, Rating, Grayscale, Panels).
						gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
						switchRow := func(key, lbl string, val *bool, onChange func()) {
							gui.Row(c).FillWidth().Padding(2, 0).AlignItems(gui.Center).Children(func() {
								gui.Text(c, lbl).FontSize(12).TextColor(a.pal.ink).Grow(1).SingleLine()
								a.arcSwitch(c, key, val, onChange)
							})
						}
						switchRow("disp-sw-title", a.t("view.showTitle"), &a.showCardCaption, func() {
							a.layoutGen++
							a.syncConfigFromState()
						})
						switchRow("disp-sw-ext", a.t("view.showExt"), &a.showCardExt, func() {
							a.syncConfigFromState()
						})
						switchRow("disp-sw-rating", a.t("view.showRating"), &a.showCardRating, func() {
							a.syncConfigFromState()
						})
						switchRow("disp-sw-counts", a.t("pref.sbCounts"), &a.showSidebarCounts, func() {
							a.syncConfigFromState()
						})
						switchRow("disp-sw-gray", a.t("view.grayscale"), &a.grayscalePreview, func() {
							a.syncConfigFromState()
						})
						switchRow("disp-sw-glass", a.t("view.glassMode"), &a.glassMode, func() {
							a.syncWindowVibrancy()
							a.syncConfigFromState()
						})
						gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
						switchRow("disp-sw-sb", a.t("view.showSidebar"), &a.sidebarOpen, nil)
						switchRow("disp-sw-insp", a.t("view.showInspector"), &a.inspectorOpen, nil)
						focusVal := a.focusMode
						focusTitle := a.t("view.focusMode")
						if sc := a.shortcutLabel("focus_mode"); sc != "" {
							focusTitle = fmt.Sprintf("%s (%s)", focusTitle, sc)
						}
						switchRow("disp-sw-focus", focusTitle, &focusVal, func() {
							a.toggleFocusMode()
						})
					})
				})
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
				if a.menuRadio(m, opt.label, a.sortBy == opt.id).Chosen() {
					a.sortBy = opt.id
					a.applyFilters()
					a.syncConfigFromState()
				}
			}
		}, a.popoverMenuBtnOpts())

		gui.PopoverMenuButton(c.Key("top-import"), "+ "+a.t("library.import"), func(m *gui.Menu) {
			if a.menuItem(m, iconImport, a.t("library.files"), a.shortcutLabel("import_files"), false).Chosen() {
				a.startImport(false)
			}
			if a.menuItem(m, iconFolder, a.t("library.folder"), "", false).Chosen() {
				a.startImport(true)
			}
			watchLabel := a.t("watch.section")
			if len(a.watchFolders) > 0 {
				watchLabel = fmt.Sprintf("%s (%d)", watchLabel, len(a.watchFolders))
			}
			if a.menuItem(m, iconRefresh, watchLabel, "", false).Chosen() {
				a.watchFoldersOpen = true
			}
			if a.isPluginEnabled("raglex.library-migrator") {
				if a.menuItem(m, iconImport, a.t("plugins.migrateMenu"), "", false).Chosen() {
					a.startMigrateExternalLibrary()
				}
			}
			m.Separator()
			if a.menuItem(m, iconLink, a.t("webimport.menu"), a.shortcutLabel("web_import"), false).Chosen() {
				a.webImportOpen = true
				a.webImportURL = ""
			}
			if a.menuItem(m, iconShare, a.t("account.receiveMenu"), "", false).Chosen() {
				a.pairRecvOpen = true
			}
		}, a.popoverMenuBtnOpts())

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

		a.topRightChrome(c)
	})
}
