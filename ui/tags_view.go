package ui

import (
	"fmt"
	"strconv"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
)

// buildTagContextMenu populates a rich right-click menu shared
// by both sidebar tags and the Tag Manager popup.
func (a *App) buildTagContextMenu(m *gui.Menu, tg model.Tag, services gui.Services) {
	tagID := tg.ID
	children := a.tagChildren[tagID]

	if a.menuItem(m, iconPlus, a.t("tags.addChild"), "", false).Chosen() {
		a.newChildParentID = tagID
		a.newChildTagName = ""
		a.tagManagerOpen = true
	}
	if a.menuItem(m, iconEdit, a.t("tags.edit"), "", false).Chosen() {
		a.openEditTagModal(tg)
	}

	siblings := a.tagChildren[tg.ParentID]
	if len(siblings) > 1 {
		idx := -1
		for i, s := range siblings {
			if s.ID == tagID {
				idx = i
				break
			}
		}
		if idx > 0 {
			if a.menuItem(m, iconChevronUp, a.t("common.moveUp"), "", false).Chosen() {
				a.moveTagOrder(tagID, -1)
			}
		}
		if idx >= 0 && idx < len(siblings)-1 {
			if a.menuItem(m, iconChevronDown, a.t("common.moveDown"), "", false).Chosen() {
				a.moveTagOrder(tagID, 1)
			}
		}
	}

	// Move to parent submenu (cycle-safe).
	forbidden := a.tagDescendantSet(tagID)
	a.menuSub(m, iconFolder, a.t("tags.moveTo"), func(sm *gui.Menu) {
		if a.menuRadio(sm, a.t("tags.noParent"), tg.ParentID == "").Chosen() {
			a.moveTagParent(tagID, "")
		}
		sm.Separator()
		for _, cand := range a.tags {
			cnd := cand
			if forbidden[cnd.ID] {
				continue
			}
			if a.menuRadio(sm, cnd.Name, tg.ParentID == cnd.ID).Chosen() {
				a.moveTagParent(tagID, cnd.ID)
			}
		}
	})

	// Tag color submenu (8 curated studio colors + neutral default).
	a.menuSub(m, iconPalette, a.t("tags.color"), func(sm *gui.Menu) {
		if a.menuRadio(sm, a.t("tags.colorDefault"), tg.Color == "").Chosen() {
			a.setTagColor(tagID, "")
		}
		sm.Separator()
		for _, opt := range tagColorPalette {
			ch := opt
			if a.menuRadio(sm, a.t(ch.key), strings.EqualFold(tg.Color, ch.hex)).Chosen() {
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
		if a.menuItem(m, iconChevronDown, lbl, "", false).Chosen() {
			a.toggleTagExpand(tagID)
		}
	}
	if a.menuItem(m, iconChevronDown, a.t("tags.expandAll"), "", false).Chosen() {
		a.expandAllTags(true)
	}
	if a.menuItem(m, iconChevronRgt, a.t("tags.collapseAll"), "", false).Chosen() {
		a.expandAllTags(false)
	}

	m.Separator()

	if a.menuItem(m, iconCopy, a.t("tags.copyName"), "", false).Chosen() {
		services.WriteClipboard(tg.Name)
		a.status = a.t("common.copied")
	}
	if !a.tagManagerOpen {
		if a.menuItem(m, iconTag, a.t("tags.manage"), "", false).Chosen() {
			a.tagManagerOpen = true
		}
	}

	m.Separator()

	if a.menuItem(m, iconTrash, a.t("tag.delete"), "Del", true).Chosen() {
		a.deleteTagPrompt(tagID)
	}
}

// tagManagerModal renders a sleek, flat divider-separated Tag Manager matching
// our Preferences & Watch Folders design language, with full-row expand/collapse,
// smooth spring drag-and-drop ordering, and crisp hierarchical guide lines.
func (a *App) tagManagerModal(c *gui.Context) {
	if !a.tagManagerOpen {
		return
	}
	a.ensureTagTree()
	services := c.Services()

	gui.DialogBase(c.Key("tag-manager-modal"), &a.tagManagerOpen, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.56))
		panel.
			Width(660).
			Height(550).
			Radius(radiusSurface).
			Clip().
			Background(a.popoverSurface()).
			Border(1, a.chromeHairline()).
			Shadow(0, 22, 54, 0, a.pal.shadowFloating)
		applyBlurReveal(panel, "tag-manager-modal", 0)

		gui.Column(c).Fill().Children(func() {
			// Sleek unified header separated by a crisp 1px hairline.
			gui.Row(c).
				Height(54).
				Padding(0, 18).
				Gap(8).
				AlignItems(gui.Center).
				BorderWidth(0, 0, 1, 0).
				BorderColor(a.chromeHairline()).
				Children(func() {
					gui.Icon(c, iconTag).Size(15, 15).TextColor(a.pal.ink)
					gui.Text(c, a.t("tags.manage")).FontSize(14.5).Bold().SingleLine()
					if a.showSidebarCounts {
						a.pillBadge(c, strconv.Itoa(len(a.tags)), a.pal.inkSecondary, a.pal.surfaceMuted)
					}

					gui.Spacer(c)

					gui.SearchField(c.Key("tm-search"), &a.tagSearch).
						Placeholder(a.t("tags.search")).
						Width(176)

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

			// Compact flat quick-create bar with refined color dots.
			gui.Row(c).
				Padding(10, 18).
				Gap(10).
				AlignItems(gui.Center).
				BorderWidth(0, 0, 1, 0).
				BorderColor(a.chromeHairline()).
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

			// Scrollable flat divider-separated hierarchical tree.
			gui.Scroll(c.Key("tm-scroll")).Grow(1).FillWidth().Children(func() {
				gui.Column(c).FillWidth().Padding(8, 14, 14, 14).Gap(2).Children(func() {
					roots := a.tagChildren[""]
					q := strings.ToLower(strings.TrimSpace(a.tagSearch))
					if len(roots) == 0 {
						gui.Column(c).
							FillWidth().
							Padding(48, 24).
							Gap(10).
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
						if shown > 0 {
							gui.Box(c).FillWidth().Height(1).Margin(3, 8).Background(a.chromeHairline())
						}
						shown++
						a.tagManagerRow(c, root, 0, q, services)
					}
					if shown == 0 {
						gui.Box(c).FillWidth().Padding(32).Center().Children(func() {
							gui.Text(c, a.t("library.noMatch")).FontSize(12).TextColor(a.pal.inkSecondary)
						})
					}
				})
			})

			// Minimal footer separated by a crisp 1px top hairline.
			gui.Row(c).
				Height(50).
				Padding(0, 18).
				Gap(10).
				AlignItems(gui.Center).
				BorderWidth(1, 0, 0, 0).
				BorderColor(a.chromeHairline()).
				Children(func() {
					gui.Text(c, fmt.Sprintf(a.t("tags.summary"), len(a.tags), len(a.tagChildren[""]))).
						FontSize(11.5).
						TextColor(a.pal.inkMuted).
						Grow(1).
						SingleLine()
					a.arcButton(c, "tm-done", a.t("common.done"), true).OnClick(func() {
						a.tagManagerOpen = false
					})
				})
		})
	})
}

func (a *App) tagColorDots(c *gui.Context, keyPrefix, selected string, onPick func(string)) {
	gui.Row(c).Gap(5).AlignItems(gui.Center).Children(func() {
		// Neutral (clear color) dot.
		noneOn := selected == ""
		noneBtn := gui.ButtonBase(c.Key(keyPrefix+"-none")).
			Size(14, 14).
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
					Size(14, 14).
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
	indent := float32(depth * 22)

	iconColor := a.pal.inkSecondary
	if col, ok := a.effectiveTagColor(tg.ID); ok {
		iconColor = col
	}

	guideCol := gui.RGBA(255, 255, 255, 0.14)
	if !a.dark {
		guideCol = gui.RGBA(0, 0, 0, 0.12)
	}

	gui.Column(c.Key("tm-node-" + tg.ID)).
		FillWidth().
		Gap(2).
		Transition(gui.ElementTransition{
			Animation: &springSnappy,
			Position:  true,
		}).
		Children(func() {
			row := gui.ButtonBase(c.Key("tm-row-"+tg.ID)).
				FillWidth().
				Padding(7, 8, 7, 8+indent).
				Gap(8).
				Radius(8).
				AlignItems(gui.Center).
				OnClick(func() {
					if hasChildren {
						a.toggleTagExpand(tg.ID)
					} else {
						a.tagManagerOpen = false
						a.handleNav("tag:" + tg.ID)
					}
				})

			row.Drag(tagReorderDrag{ID: tg.ID}).
				PopoverMenu(func(m *gui.Menu) {
					a.buildTagContextMenu(m, tg, services)
				}, a.popoverMenuOpts())
			if d, over := gui.DragOver[tagReorderDrag](row); over && d.ID != tg.ID {
				a.reorderTagLive(d.ID, tg.ID)
			}
			if _, ok := gui.Drop[tagReorderDrag](row); ok {
				a.lastReorderPair = ""
			}

			dragging := row.Dragging()
			targetOp := float32(1.0)
			if dragging {
				targetOp = 0.24
			}
			if op := row.AnimateTo("drag-op", targetOp, animFast); op < 0.995 {
				row.Opacity(op)
			}

			bg := gui.Transparent
			if dragging {
				bg = a.pal.surfaceMuted
			} else if row.Hovered() {
				bg = a.settingsRowHoverBg()
			}
			row.Background(row.AnimateColor("bg", bg, animFast))

			row.Children(func() {
				if hasChildren {
					chevBtn := gui.ButtonBase(c.Key("tm-tog-"+tg.ID)).
						Size(22, 22).
						Center().
						Radius(6).
						Tooltip(a.t("tags.expand")).
						OnClick(func() { a.toggleTagExpand(tg.ID) })
					cbg := gui.Transparent
					cfg := a.pal.inkMuted
					if chevBtn.Hovered() || chevBtn.Pressed() {
						cbg = a.pal.surfaceMuted
						cfg = a.pal.ink
					}
					chevRotTarget := float32(0)
					if expanded {
						chevRotTarget = 90
					}
					chevRot := chevBtn.AnimateTo("chev-rot", chevRotTarget, springSnappy)
					chevBtn.Background(chevBtn.AnimateColor("cbg", cbg, animFast)).
						TextColor(chevBtn.AnimateColor("cfg", cfg, animFast)).
						Children(func() {
							gui.Icon(c, iconChevronRgt).Size(12, 12).Rotate(chevRot)
						})
				} else {
					gui.Box(c).Size(22, 22)
				}

				gui.Icon(c, iconTag).Size(14, 14).TextColor(iconColor)

				nameTxt := gui.Text(c, tg.Name).FontSize(13).TextColor(a.pal.ink).Grow(1).SingleLine()
				if depth == 0 {
					nameTxt.Bold()
				}

				if hasChildren && !expanded {
					a.pillBadge(c, fmt.Sprintf("+%d", len(children)), a.pal.inkMuted, a.pal.surfaceMuted)
				}
				if a.showSidebarCounts {
					if cnt := a.tagDisplayCount(tg.ID); cnt > 0 {
						gui.Text(c, strconv.Itoa(cnt)).
							FontSize(11.5).
							TextColor(a.pal.inkMuted).
							Padding(0, 4).
							SingleLine()
					}
				}

				// Compact quiet monochrome actions on the right.
				gui.Row(c).Gap(2).AlignItems(gui.Center).Children(func() {
					a.miniHeaderButton(c, "tm-view-"+tg.ID, iconEye, a.t("tags.viewAssets"), false).OnClick(func() {
						a.tagManagerOpen = false
						a.handleNav("tag:" + tg.ID)
					})
					a.miniHeaderButton(c, "tm-addch-"+tg.ID, iconPlus, a.t("tags.addChild"), a.newChildParentID == tg.ID).OnClick(func() {
						if a.newChildParentID == tg.ID {
							a.newChildParentID = ""
							a.newChildTagName = ""
						} else {
							a.newChildParentID = tg.ID
							a.newChildTagName = ""
							a.expandedTags[tg.ID] = true
						}
					})
					a.miniHeaderButton(c, "tm-edit-"+tg.ID, iconEdit, a.t("tags.edit"), false).OnClick(func() {
						a.openEditTagModal(tg)
					})
					a.miniHeaderButton(c, "tm-del-"+tg.ID, iconTrash, a.t("tag.delete"), false).OnClick(func() {
						a.deleteTagPrompt(tg.ID)
					})
				})
			})

			if a.newChildParentID == tg.ID {
				gui.Row(c.Key("tm-inline-ch-"+tg.ID)).
					FillWidth().
					Padding(6, 10, 6, 36+indent).
					Gap(8).
					Radius(8).
					Background(a.settingsRowHoverBg()).
					AlignItems(gui.Center).
					Transition(gui.ElementTransition{
						Animation: &springSmooth,
						Enter:     &gui.Motion{Y: -4, Opacity: 0, Collapse: true},
						Exit:      &gui.Motion{Y: -4, Opacity: 0, Collapse: true},
					}).
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
				lineX := indent + 19
				chCol := gui.Column(c.Key("tm-ch-" + tg.ID)).
					FillWidth().
					Gap(2).
					Transition(gui.ElementTransition{
						Animation: &springSnappy,
						Position:  true,
						Enter:     &gui.Motion{Y: -4, Opacity: 0, Collapse: true},
						Exit:      &gui.Motion{Y: -4, Opacity: 0, Collapse: true},
					}).
					Draw(func(p *gui.Painter, r gui.Rect) {
						if r.H > 6 {
							p.Fill(gui.Rect{X: r.X + lineX, Y: r.Y + 2, W: 1, H: r.H - 6}, guideCol, 0.5)
						}
					})
				chCol.Children(func() {
					for _, ch := range children {
						if q != "" && !a.tagBranchMatches(ch.ID, q) {
							continue
						}
						a.tagManagerRow(c, ch, depth+1, q, services)
					}
				})
			}
		})
}
