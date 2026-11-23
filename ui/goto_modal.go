package ui

import (
	"fmt"
	"strings"

	gui "github.com/egoist/mygo/ui"
)

type gotoEntry struct {
	id       string
	icon     *gui.SVG
	tint     *gui.Color
	label    string
	kindText string
	run      func()
}

func (a *App) openGotoModal() {
	a.gotoQuery = ""
	a.gotoIndex = 0
	a.gotoOpen = true
}

func (a *App) buildGotoEntries() []gotoEntry {
	q := strings.ToLower(strings.TrimSpace(a.gotoQuery))
	match := func(s string) bool {
		if q == "" {
			return true
		}
		return strings.Contains(strings.ToLower(s), q)
	}

	var out []gotoEntry

	// 1. Tags.
	for _, tg := range a.tags {
		tag := tg
		if !match(tag.Name) {
			continue
		}
		var tint *gui.Color
		if a.showTagColors && tag.Color != "" {
			if col, ok := parseHexColor(tag.Color); ok {
				cCopy := col
				tint = &cCopy
			}
		}
		out = append(out, gotoEntry{
			id:       "tag:" + tag.ID,
			icon:     iconTag,
			tint:     tint,
			label:    tag.Name,
			kindText: a.t("goto.kindTag"),
			run: func() {
				a.closePreview()
				a.handleNav("tag:" + tag.ID)
			},
		})
	}

	// 2. Projects.
	for _, p := range a.projectList {
		proj := p
		if !match(proj.Name + " " + proj.Description) {
			continue
		}
		out = append(out, gotoEntry{
			id:       "proj:" + proj.ID,
			icon:     iconFolder,
			label:    proj.Name,
			kindText: a.t("goto.kindProject"),
			run: func() {
				a.closePreview()
				a.openProject(proj.ID)
			},
		})
	}

	// 3. Boards.
	for _, sb := range a.standaloneBoards {
		b := sb
		if !match(b.Name) {
			continue
		}
		out = append(out, gotoEntry{
			id:       "board:" + b.ID,
			icon:     iconBoard,
			label:    b.Name,
			kindText: a.t("goto.kindBoard"),
			run: func() {
				a.closePreview()
				a.openBoard(b)
			},
		})
	}

	// 4. Quick Actions.
	actions := []struct {
		id    string
		icon  *gui.SVG
		label string
		run   func()
	}{
		{"act-import", iconImport, a.t("goto.actImport"), func() { a.startImport(false) }},
		{"act-new-board", iconBoard, a.t("goto.actNewBoard"), func() { a.createStandaloneBoard() }},
		{"act-new-proj", iconFolder, a.t("goto.actNewProject"), func() { a.openNewProjectModal() }},
		{"act-rename", iconEdit, a.t("goto.actBatchRename"), func() {
			if ids := a.selectedAssetIDs(); len(ids) > 0 {
				a.openRenameModal(ids)
			} else if len(a.assets) > 0 {
				a.openRenameModal([]string{a.assets[0].ID})
			}
		}},
		{"act-sim-img", iconImage, a.t("goto.actSearchImage"), func() { a.searchSimilarByFileDialog() }},
		{"act-auto-stack", iconLayers, a.t("stack.autoGroup"), func() { a.autoStackVersions() }},
		{"act-theme", iconPalette, a.t("goto.actToggleTheme"), func() {
			if a.dark {
				a.setThemeMode("light")
			} else {
				a.setThemeMode("dark")
			}
		}},
		{"act-sidebar", iconPanelLeft, a.t("goto.actToggleSidebar"), func() { a.sidebarOpen = !a.sidebarOpen }},
		{"act-inspector", iconPanelRight, a.t("goto.actToggleInspector"), func() { a.inspectorOpen = !a.inspectorOpen }},
		{"act-server", iconShare, a.t("goto.actToggleServer"), func() {
			a.serverRunning = !a.serverRunning
			a.toggleBuiltInServer()
		}},
		{"act-settings", iconSettings, a.t("goto.actSettings"), func() { a.openSettings(a.settingsTab) }},
	}
	for _, act := range actions {
		item := act
		if !match(item.label) {
			continue
		}
		out = append(out, gotoEntry{
			id:       item.id,
			icon:     item.icon,
			label:    item.label,
			kindText: a.t("goto.kindAction"),
			run:      item.run,
		})
	}

	// 5. Matching Assets (shown when user types a query so palette stays crisp when empty).
	if q != "" {
		addedAssets := 0
		for _, item := range a.allAssets {
			asset := item
			name := a.assetName(asset)
			if !match(name + " " + asset.Notes) {
				continue
			}
			out = append(out, gotoEntry{
				id:       "asset:" + asset.ID,
				icon:     iconImage,
				label:    name,
				kindText: a.t("goto.kindAsset"),
				run: func() {
					a.screen = screenLibrary
					a.openPreview(asset)
				},
			})
			addedAssets++
			if addedAssets >= 10 {
				break
			}
		}
	}

	return out
}

func (a *App) gotoModal(c *gui.Context) {
	if !a.gotoOpen {
		return
	}
	entries := a.buildGotoEntries()
	if len(entries) == 0 {
		a.gotoIndex = 0
	} else {
		if a.gotoIndex < 0 {
			a.gotoIndex = len(entries) - 1
		} else if a.gotoIndex >= len(entries) {
			a.gotoIndex = 0
		}
	}

	execute := func(idx int) {
		if idx >= 0 && idx < len(entries) {
			fn := entries[idx].run
			a.gotoOpen = false
			if fn != nil {
				fn()
			}
		}
	}

	openProxy := a.gotoOpen
	gui.DialogBase(c.Key("goto-palette-modal"), &openProxy, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.52))
		panel.
			Width(540).
			MaxHeight(520).
			Radius(radiusSurface).
			Clip().
			Background(a.popoverSurface()).
			Border(1, a.chromeHairline()).
			Shadow(0, 24, 64, 0, a.pal.shadowFloating)
		applyBlurReveal(panel, "goto-modal", 0)

		panel.OnShortcut(0, gui.KeyDown, func() {
			if len(entries) > 0 {
				a.gotoIndex = (a.gotoIndex + 1) % len(entries)
			}
		})
		panel.OnShortcut(0, gui.KeyUp, func() {
			if len(entries) > 0 {
				a.gotoIndex = (a.gotoIndex - 1 + len(entries)) % len(entries)
			}
		})
		panel.OnShortcut(0, gui.KeyEnter, func() {
			execute(a.gotoIndex)
		})

		gui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
			// Clean Go To header matching reference screenshot.
			gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("goto.title")).FontSize(16).Bold().TextColor(a.pal.ink).Grow(1)
				a.iconButton(c, "goto-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.gotoOpen = false
				})
			})

			// Auto-focused search input with crisp #FF5C33 focus frame.
			gui.Box(c).
				FillWidth().
				Radius(radiusControl).
				Border(1.5, a.pal.controlOn).
				Background(a.pal.canvas).
				Padding(2).
				Children(func() {
					gui.TextInput(c.Key("goto-query"), &a.gotoQuery).
						Placeholder(a.t("goto.placeholder")).
						AutoFocus().
						FillWidth().
						OnChange(func() { a.gotoIndex = 0 }).
						OnSubmit(func() { execute(a.gotoIndex) })
				})

			// Scrollable results list.
			if len(entries) == 0 {
				gui.Box(c).FillWidth().Padding(24).Center().Children(func() {
					gui.Text(c, a.t("goto.empty")).FontSize(12).TextColor(a.pal.inkMuted)
				})
			} else {
				gui.Scroll(c.Key("goto-results-scroll")).MaxHeight(360).FillWidth().Children(func() {
					gui.Column(c).FillWidth().Gap(3).Children(func() {
						for i, ent := range entries {
							idx := i
							item := ent
							active := idx == a.gotoIndex
							btn := gui.ButtonBase(c.Key(fmt.Sprintf("goto-row-%s", item.id))).
								FillWidth().
								Padding(9, 12).
								Gap(10).
								Radius(10).
								AlignItems(gui.Center).
								OnClick(func() { execute(idx) })
							bg := gui.Transparent
							fg := a.pal.inkSecondary
							if active {
								bg = a.pal.surfaceMuted
								fg = a.pal.ink
							} else if btn.Hovered() {
								bg = a.pal.surfaceRaised
								fg = a.pal.ink
							}
							btn.Background(bg).Children(func() {
								icCol := fg
								if item.tint != nil {
									icCol = *item.tint
								}
								gui.Icon(c, item.icon).Size(15, 15).TextColor(icCol)
								gui.Text(c, item.label).FontSize(13).Bold().TextColor(fg).Grow(1).SingleLine()
								gui.Text(c, item.kindText).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
							})
						}
					})
				})
			}
		})
	})
	if !openProxy {
		a.gotoOpen = false
	}
}
