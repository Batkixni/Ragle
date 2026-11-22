package ui

import (
	"fmt"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
)

// buildTagContextMenu populates an Eagle-inspired rich right-click menu shared
// by both sidebar tags and the Tag Manager popup.
func (a *App) buildTagContextMenu(m *gui.Menu, tg model.Tag, services gui.Services) {
	tagID := tg.ID
	children := a.tagChildren[tagID]

	if m.Item(a.t("tags.addChild")).Chosen() {
		a.newChildParentID = tagID
		a.newChildTagName = ""
		a.tagManagerOpen = true
	}
	if m.Item(a.t("tags.edit")).Chosen() {
		a.openEditTagModal(tg)
	}

	// Move to parent submenu (cycle-safe).
	forbidden := a.tagDescendantSet(tagID)
	m.Submenu(a.t("tags.moveTo"), func(sm *gui.Menu) {
		if sm.Item(a.t("tags.noParent")).Radio(tg.ParentID == "").Chosen() {
			a.moveTagParent(tagID, "")
		}
		sm.Separator()
		for _, cand := range a.tags {
			cnd := cand
			if forbidden[cnd.ID] {
				continue
			}
			if sm.Item(cnd.Name).Radio(tg.ParentID == cnd.ID).Chosen() {
				a.moveTagParent(tagID, cnd.ID)
			}
		}
	})

	// Tag color submenu (8 curated studio colors + neutral default).
	m.Submenu(a.t("tags.color"), func(sm *gui.Menu) {
		if sm.Item(a.t("tags.colorDefault")).Radio(tg.Color == "").Chosen() {
			a.setTagColor(tagID, "")
		}
		sm.Separator()
		for _, opt := range tagColorPalette {
			ch := opt
			if sm.Item(a.t(ch.key)).Radio(strings.EqualFold(tg.Color, ch.hex)).Chosen() {
				a.setTagColor(tagID, ch.hex)
			}
		}
	})

	m.Separator()

	if len(children) > 0 {
		lbl := a.t("tags.expand")
		if a.expandedTags[tagID] {
			lbl = a.t("tags.collapse")
		}
		if m.Item(lbl).Chosen() {
			a.toggleTagExpand(tagID)
		}
	}
	if m.Item(a.t("tags.expandAll")).Chosen() {
		a.expandAllTags(true)
	}
	if m.Item(a.t("tags.collapseAll")).Chosen() {
		a.expandAllTags(false)
	}

	m.Separator()

	if m.Item(a.t("tags.copyName")).Chosen() {
		services.WriteClipboard(tg.Name)
		a.status = a.t("common.copied")
	}
	if !a.tagManagerOpen {
		if m.Item(a.t("tags.manage")).Chosen() {
			a.tagManagerOpen = true
		}
	}

	m.Separator()

	if m.Item(a.t("tag.delete")).Chosen() {
		a.deleteTagPrompt(tagID)
	}
}

// tagManagerModal renders the sleek popup modal for organizing parent/child
// tags, colors, renaming, reparenting, and jumping to tagged assets.
func (a *App) tagManagerModal(c *gui.Context) {
	if !a.tagManagerOpen {
		return
	}
	a.ensureTagTree()
	services := c.Services()

	gui.DialogBase(c.Key("tag-manager-modal"), &a.tagManagerOpen, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.58))
		panel.
			Width(720).
			Height(560).
			Radius(radiusSurface).
			Clip().
			Background(a.pal.canvas).
			Shadow(0, 22, 54, 0, a.pal.shadowFloating)
		applyBlurReveal(panel, "tag-manager-modal", 0)

		gui.Column(c).Fill().Children(func() {
			// Header row.
			gui.Row(c).
				Height(56).
				Padding(0, 20).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Icon(c, iconTag).Size(16, 16).TextColor(a.pal.ink)
					gui.Text(c, a.t("tags.manage")).FontSize(15).Bold()
					a.pillBadge(c, fmt.Sprintf("%d", len(a.tags)), a.pal.inkSecondary, a.pal.surfaceMuted)

					gui.Spacer(c)

					gui.SearchField(c.Key("tm-search"), &a.tagSearch).
						Placeholder(a.t("tags.search")).
						Width(190)

					a.toolButton(c, "tm-exp-all", nil, a.t("tags.expandAll"), false).OnClick(func() {
						a.expandAllTags(true)
					})
					a.toolButton(c, "tm-col-all", nil, a.t("tags.collapseAll"), false).OnClick(func() {
						a.expandAllTags(false)
					})
					a.iconButton(c, "tm-close", iconX, a.t("common.close"), false).OnClick(func() {
						a.tagManagerOpen = false
					})
				})

			// Quick root tag creator + color dot picker.
			gui.Row(c).
				Padding(12, 20, 8, 20).
				Gap(8).
				AlignItems(gui.Center).
				Children(func() {
					gui.TextInput(c.Key("tm-new-root"), &a.newRootTagName).
						Placeholder(a.t("tags.addRoot")).
						Grow(1).
						OnSubmit(func() {
							a.createTagWithParentAndColor(a.newRootTagName, "", a.newRootTagColor)
							a.newRootTagName = ""
						})
					if a.showTagColors {
						a.tagColorDots(c, "tm-root-col", a.newRootTagColor, func(hex string) {
							a.newRootTagColor = hex
						})
					}
					a.arcButton(c, "tm-add-root-btn", "+ "+a.t("tag.create"), true).OnClick(func() {
						a.createTagWithParentAndColor(a.newRootTagName, "", a.newRootTagColor)
						a.newRootTagName = ""
					})
				})

			// Scrollable hierarchical tree.
			gui.Scroll(c.Key("tm-scroll")).Grow(1).FillWidth().Children(func() {
				gui.Column(c).FillWidth().Padding(4, 20, 16, 20).Gap(6).Children(func() {
					roots := a.tagChildren[""]
					q := strings.ToLower(strings.TrimSpace(a.tagSearch))
					if len(roots) == 0 {
						gui.Column(c).
							FillWidth().
							Padding(40).
							Gap(10).
							Radius(radiusPanel).
							Background(a.pal.surface).
							Center().
							Children(func() {
								gui.Icon(c, iconTag).Size(24, 24).TextColor(a.pal.inkFaint)
								gui.Text(c, a.t("tags.empty")).FontSize(14).Bold()
								gui.Text(c, a.t("tags.emptyHint")).
									FontSize(12).
									TextColor(a.pal.inkSecondary).
									TextAlign(gui.Center)
							})
						return
					}

					shown := 0
					for _, root := range roots {
						if q != "" && !a.tagBranchMatches(root.ID, q) {
							continue
						}
						shown++
						a.tagManagerRow(c, root, 0, q, services)
					}
					if shown == 0 {
						gui.Box(c).FillWidth().Padding(28).Center().Children(func() {
							gui.Text(c, a.t("library.noMatch")).FontSize(12).TextColor(a.pal.inkSecondary)
						})
					}
				})
			})

			// Compact footer.
			gui.Row(c).
				Height(52).
				Padding(0, 20).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Text(c, fmt.Sprintf(a.t("tags.summary"), len(a.tags), len(a.tagChildren[""]))).
						FontSize(11).
						TextColor(a.pal.inkMuted).
						Grow(1)
					a.arcButton(c, "tm-done", a.t("common.done"), true).OnClick(func() {
						a.tagManagerOpen = false
					})
				})
		})
	})
}

func (a *App) tagColorDots(c *gui.Context, keyPrefix, selected string, onPick func(string)) {
	gui.Row(c).Gap(4).AlignItems(gui.Center).Children(func() {
		// Neutral (clear color) dot.
		noneOn := selected == ""
		noneBtn := gui.ButtonBase(c.Key(keyPrefix+"-none")).
			Size(16, 16).
			Radius(radiusPill).
			Background(a.pal.surfaceMuted).
			Tooltip(a.t("tags.colorDefault")).
			OnClick(func() { onPick("") })
		if noneOn {
			noneBtn.Border(1.5, a.pal.controlOn)
		}
		for i, opt := range tagColorPalette {
			ch := opt
			if col, ok := parseHexColor(ch.hex); ok {
				on := strings.EqualFold(selected, ch.hex)
				dot := gui.ButtonBase(c.Key(fmt.Sprintf("%s-%d", keyPrefix, i))).
					Size(16, 16).
					Radius(radiusPill).
					Background(col).
					Tooltip(a.t(ch.key)).
					OnClick(func() { onPick(ch.hex) })
				if on {
					dot.Border(2, a.pal.ink)
				}
			}
		}
	})
}

func (a *App) tagBranchMatches(id, q string) bool {
	if strings.Contains(strings.ToLower(a.tagByID[id].Name), q) {
		return true
	}
	for _, ch := range a.tagChildren[id] {
		if a.tagBranchMatches(ch.ID, q) {
			return true
		}
	}
	return false
}

func (a *App) tagManagerRow(c *gui.Context, tg model.Tag, depth int, q string, services gui.Services) {
	children := a.tagChildren[tg.ID]
	hasChildren := len(children) > 0
	expanded := a.expandedTags[tg.ID] || q != ""
	indent := float32(depth * 20)

	iconColor := a.pal.inkSecondary
	if col, ok := a.effectiveTagColor(tg.ID); ok {
		iconColor = col
	}

	gui.Column(c.Key("tm-node-" + tg.ID)).FillWidth().Gap(4).Children(func() {
		row := gui.Row(c.Key("tm-row-"+tg.ID)).
			FillWidth().
			Padding(7, 10, 7, 10+indent).
			Gap(8).
			Radius(radiusControl).
			AlignItems(gui.Center)
		bg := a.pal.surface
		if depth > 0 {
			bg = a.pal.surfaceRaised
		}
		if row.Hovered() {
			bg = a.pal.surfaceMuted
		}
		row.Background(row.AnimateColor("bg", bg, animFast)).
			PopoverMenu(func(m *gui.Menu) {
				a.buildTagContextMenu(m, tg, services)
			})

		row.Children(func() {
			if hasChildren {
				chev := "▸"
				if expanded {
					chev = "▾"
				}
				gui.ButtonBase(c.Key("tm-tog-"+tg.ID)).
					Size(20, 20).
					Center().
					Radius(6).
					OnClick(func() { a.toggleTagExpand(tg.ID) }).
					Children(func() {
						gui.Text(c, chev).FontSize(11).TextColor(a.pal.inkMuted)
					})
			} else {
				gui.Box(c).Size(20, 20)
			}

			gui.Icon(c, iconTag).Size(14, 14).TextColor(iconColor)
			gui.Text(c, tg.Name).FontSize(13).Bold().Grow(1).SingleLine()

			a.pillBadge(c, fmt.Sprintf("%d", a.tagDisplayCount(tg.ID)), a.pal.inkSecondary, a.pal.surfaceMuted)

			a.iconButton(c, "tm-view-"+tg.ID, iconEye, a.t("tags.viewAssets"), false).OnClick(func() {
				a.tagManagerOpen = false
				a.handleNav("tag:" + tg.ID)
			})
			a.iconButton(c, "tm-addch-"+tg.ID, iconPlus, a.t("tags.addChild"), a.newChildParentID == tg.ID).OnClick(func() {
				if a.newChildParentID == tg.ID {
					a.newChildParentID = ""
					a.newChildTagName = ""
				} else {
					a.newChildParentID = tg.ID
					a.newChildTagName = ""
					a.expandedTags[tg.ID] = true
				}
			})
			a.iconButton(c, "tm-edit-"+tg.ID, iconEdit, a.t("tags.edit"), false).OnClick(func() {
				a.openEditTagModal(tg)
			})
			a.iconButton(c, "tm-del-"+tg.ID, iconTrash, a.t("tag.delete"), false).OnClick(func() {
				a.deleteTagPrompt(tg.ID)
			})
		})

		if a.newChildParentID == tg.ID {
			gui.Row(c.Key("tm-inline-ch-"+tg.ID)).
				FillWidth().
				Padding(6, 10, 6, 30+indent).
				Gap(8).
				Radius(radiusControl).
				Background(a.pal.surfaceMuted).
				AlignItems(gui.Center).
				Children(func() {
					gui.TextInput(c.Key("tm-ch-in-"+tg.ID), &a.newChildTagName).
						Placeholder(a.t("tags.addChild")).
						AutoFocus().
						Grow(1).
						OnSubmit(func() {
							a.createTagWithParent(a.newChildTagName, tg.ID)
							a.newChildTagName = ""
							a.newChildParentID = ""
						})
					a.arcButton(c, "tm-ch-ok-"+tg.ID, a.t("tag.create"), true).OnClick(func() {
						a.createTagWithParent(a.newChildTagName, tg.ID)
						a.newChildTagName = ""
						a.newChildParentID = ""
					})
					a.iconButton(c, "tm-ch-x-"+tg.ID, iconX, a.t("common.cancel"), false).OnClick(func() {
						a.newChildParentID = ""
						a.newChildTagName = ""
					})
				})
		}

		if hasChildren && expanded {
			for _, ch := range children {
				if q != "" && !a.tagBranchMatches(ch.ID, q) {
					continue
				}
				a.tagManagerRow(c, ch, depth+1, q, services)
			}
		}
	})
}
