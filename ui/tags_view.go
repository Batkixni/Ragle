package ui

import (
	"fmt"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

// tagsView renders the dedicated Tag Management page with hierarchical
// parent/child tags, search, inline root/child creation, editing, and deletion.
func (a *App) tagsView(c *gui.Context) {
	a.ensureTagTree()
	gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
		// Unified frameless top bar.
		gui.Box(c).Padding(10, 12, 4, 8).Children(func() {
			gui.Row(c.Key("tags-topbar")).
				Height(48).
				Padding(0, 10, 0, 16).
				Gap(10).
				Radius(radiusPanel).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Row(c).Gap(8).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
						gui.Icon(c, iconTag).Size(15, 15).TextColor(a.pal.ink)
						gui.Text(c, a.t("tags.manage")).Bold().FontSize(15).SingleLine()
						a.pillBadge(c, fmt.Sprintf("%d", len(a.tags)), a.pal.inkSecondary, a.pal.surfaceMuted)
					})

					gui.Box(c).Grow(1).FillHeight().DragWindow()

					gui.SearchField(c.Key("tags-search"), &a.tagSearch).
						Placeholder(a.t("tags.search")).
						Width(220)

					gui.Box(c).Grow(1).FillHeight().DragWindow()

					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						gui.TextInput(c.Key("tags-new-root"), &a.newRootTagName).
							Placeholder(a.t("tags.addRoot")).
							Width(170).
							OnSubmit(func() {
								a.createTagWithParent(a.newRootTagName, "")
								a.newRootTagName = ""
							})
						a.arcButton(c, "tags-add-root-btn", "+ "+a.t("tags.addRoot"), true).OnClick(func() {
							a.createTagWithParent(a.newRootTagName, "")
							a.newRootTagName = ""
						})
					})

					gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
					a.windowControls(c)
				})
		})

		gui.Scroll(c.Key("tags-scroll")).Grow(1).FillWidth().Background(a.pal.canvas).Children(func() {
			col := gui.Column(c.Key("tags-body")).FillWidth().Padding(12, 20, 28, 12).Gap(14)
			applyPanelSwitch(col, "tags-manager", 0, 6)
			col.Children(func() {
				if a.status != "" {
					gui.Row(c.Key("tags-status")).
						Padding(6, 12).
						Radius(radiusControl).
						Background(a.pal.surface).
						AlignItems(gui.Center).
						Gap(8).
						Children(func() {
							gui.Text(c, a.status).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
							a.iconButton(c, "tags-status-x", iconX, a.t("common.close"), false).OnClick(func() {
								a.status = ""
							})
						})
				}

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
							gui.Icon(c, iconTag).Size(26, 26).TextColor(a.pal.inkFaint)
							gui.Text(c, a.t("tags.empty")).FontSize(15).Bold()
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
					a.tagManagerCard(c, root, q)
				}
				if shown == 0 {
					gui.Column(c).
						FillWidth().
						Padding(32).
						Gap(8).
						Radius(radiusPanel).
						Background(a.pal.surface).
						Center().
						Children(func() {
							gui.Text(c, a.t("library.noMatch")).FontSize(13).TextColor(a.pal.inkSecondary)
						})
				}
			})
		})
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

func (a *App) tagManagerCard(c *gui.Context, root model.Tag, q string) {
	children := a.tagChildren[root.ID]
	total := a.tagDisplayCount(root.ID)

	gui.Column(c.Key("tm-card-" + root.ID)).
		FillWidth().
		Padding(14, 16).
		Gap(10).
		Radius(radiusPanel).
		Background(a.pal.surface).
		Children(func() {
			// Parent tag header row.
			gui.Row(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
				gui.Box(c).Size(32, 32).Radius(radiusControl).Background(a.pal.surfaceRaised).Center().Children(func() {
					gui.Icon(c, iconTag).Size(15, 15).TextColor(a.pal.ink)
				})
				gui.Column(c).Grow(1).Gap(2).Children(func() {
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Text(c, root.Name).FontSize(14).Bold().SingleLine()
						a.pillBadge(c, fmt.Sprintf(a.t("common.itemsCount"), total), a.pal.inkSecondary, a.pal.surfaceMuted)
						if len(children) > 0 {
							a.pillBadge(c, fmt.Sprintf(a.t("tags.childCount"), len(children)), a.pal.inkMuted, a.pal.surfaceMuted)
						}
					})
				})

				a.toolButton(c, "tm-view-"+root.ID, iconEye, a.t("tags.viewAssets"), false).OnClick(func() {
					a.handleNav("tag:" + root.ID)
				})
				a.toolButton(c, "tm-addch-"+root.ID, iconPlus, a.t("tags.addChild"), a.newChildParentID == root.ID).OnClick(func() {
					if a.newChildParentID == root.ID {
						a.newChildParentID = ""
						a.newChildTagName = ""
					} else {
						a.newChildParentID = root.ID
						a.newChildTagName = ""
					}
				})
				a.iconButton(c, "tm-edit-"+root.ID, iconEdit, a.t("tags.edit"), false).OnClick(func() {
					a.openEditTagModal(root)
				})
				a.iconButton(c, "tm-del-"+root.ID, iconTrash, a.t("tag.delete"), false).OnClick(func() {
					a.deleteTagPrompt(root.ID)
				})
			})

			// Inline add-child input when active on this parent.
			if a.newChildParentID == root.ID {
				a.inlineAddChildRow(c, root.ID)
			}

			// Nested descendant tag rows.
			if len(children) > 0 {
				gui.Column(c).FillWidth().Padding(4, 0, 0, 14).Gap(6).Children(func() {
					for _, ch := range children {
						a.tagManagerChildRows(c, ch, 1, q)
					}
				})
			}
		})
}

func (a *App) inlineAddChildRow(c *gui.Context, parentID string) {
	gui.Row(c.Key("tm-inline-child-" + parentID)).
		FillWidth().
		Padding(6, 10).
		Gap(8).
		Radius(radiusControl).
		Background(a.pal.surfaceMuted).
		AlignItems(gui.Center).
		Children(func() {
			gui.TextInput(c.Key("tm-child-input-"+parentID), &a.newChildTagName).
				Placeholder(a.t("tags.addChild")).
				AutoFocus().
				Grow(1).
				OnSubmit(func() {
					a.createTagWithParent(a.newChildTagName, parentID)
					a.newChildTagName = ""
					a.newChildParentID = ""
				})
			a.arcButton(c, "tm-child-ok-"+parentID, a.t("tag.create"), true).OnClick(func() {
				a.createTagWithParent(a.newChildTagName, parentID)
				a.newChildTagName = ""
				a.newChildParentID = ""
			})
			a.iconButton(c, "tm-child-x-"+parentID, iconX, a.t("common.cancel"), false).OnClick(func() {
				a.newChildParentID = ""
				a.newChildTagName = ""
			})
		})
}

func (a *App) tagManagerChildRows(c *gui.Context, tg model.Tag, depth int, q string) {
	if q != "" && !a.tagBranchMatches(tg.ID, q) {
		return
	}
	indent := float32((depth - 1) * 18)
	gui.Column(c.Key("tm-ch-" + tg.ID)).FillWidth().Padding(0, 0, 0, indent).Gap(6).Children(func() {
		gui.Row(c).
			FillWidth().
			Padding(7, 12).
			Gap(8).
			Radius(radiusControl).
			Background(a.pal.surfaceRaised).
			AlignItems(gui.Center).
			Children(func() {
				gui.Icon(c, iconTag).Size(13, 13).TextColor(a.pal.inkSecondary)
				gui.Text(c, tg.Name).FontSize(13).Bold().Grow(1).SingleLine()
				a.pillBadge(c, fmt.Sprintf("%d", a.tagDisplayCount(tg.ID)), a.pal.inkSecondary, a.pal.surfaceMuted)
				a.iconButton(c, "tm-cview-"+tg.ID, iconEye, a.t("tags.viewAssets"), false).OnClick(func() {
					a.handleNav("tag:" + tg.ID)
				})
				a.iconButton(c, "tm-cadd-"+tg.ID, iconPlus, a.t("tags.addChild"), a.newChildParentID == tg.ID).OnClick(func() {
					if a.newChildParentID == tg.ID {
						a.newChildParentID = ""
						a.newChildTagName = ""
					} else {
						a.newChildParentID = tg.ID
						a.newChildTagName = ""
					}
				})
				a.iconButton(c, "tm-cedit-"+tg.ID, iconEdit, a.t("tags.edit"), false).OnClick(func() {
					a.openEditTagModal(tg)
				})
				a.iconButton(c, "tm-cdel-"+tg.ID, iconTrash, a.t("tag.delete"), false).OnClick(func() {
					a.deleteTagPrompt(tg.ID)
				})
			})
		if a.newChildParentID == tg.ID {
			a.inlineAddChildRow(c, tg.ID)
		}
		for _, sub := range a.tagChildren[tg.ID] {
			a.tagManagerChildRows(c, sub, depth+1, q)
		}
	})
}
