package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

func (a *App) projectView(c *gui.Context) {
	t := c.Theme()
	gui.Scroll(c).Grow(1).FillHeight().Background(a.pal.canvas).Children(func() {
		gui.Column(c).Padding(24, 28).Gap(24).Children(func() {
			// Project header card.
			gui.Column(c).
				Padding(20, 22).
				Gap(14).
				Radius(12).
				Background(a.pal.card).
				Border(1, a.pal.hairline).
				Shadow(0, 2, 10, 0, a.pal.shadow).
				Children(func() {
					gui.Row(c).Gap(12).AlignItems(gui.Center).Children(func() {
						gui.Box(c).Size(40, 40).Radius(10).Background(a.pal.ring.Alpha(0.12)).Center().Children(func() {
							gui.Icon(c, iconFolder).Size(20, 20).TextColor(a.pal.ring)
						})
						gui.Column(c).Grow(1).Gap(3).Children(func() {
							gui.Text(c, a.project.Name).FontSize(20).Bold()
							if a.project.Description != "" {
								gui.Text(c, a.project.Description).FontSize(13).TextColor(t.TextMuted)
							}
						})
						gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
							gui.PrimaryButton(c.Key("proj-from-lib"), a.t("project.fromLibrary")).OnClick(func() {
								a.referenceMode = true
								a.screen = screenLibrary
							})
							gui.Button(c.Key("proj-edit-btn"), a.t("project.edit")).OnClick(func() {
								a.openEditProjectModal()
							})
							gui.Button(c.Key("proj-share-btn"), a.t("share.send")).OnClick(func() {
								a.shareOpen = true
							})
							a.iconButton(c, "proj-del-btn", iconTrash, a.t("project.delete"), false).OnClick(func() {
								id := a.projectID
								a.ask(a.t("project.delete"), a.t("project.deleteNote"), a.t("common.delete"), func() {
									if a.boardProject == id {
										a.session = nil
										a.boardID = ""
										a.boardProject = ""
									}
									if err := a.projects.Delete(id); err != nil {
										a.status = err.Error()
										return
									}
									a.projectID = ""
									a.project = model.Project{}
									a.screen = screenLibrary
									a.reload()
								})
							})
						})
					})

					// Wrapping project tag pills.
					if len(a.tags) > 0 {
						gui.Row(c).Gap(6).Wrap().AlignItems(gui.Center).Children(func() {
							gui.Text(c, a.t("library.tags")+":").FontSize(11).TextColor(a.pal.inkMuted)
							for _, tag := range a.tags {
								tg := tag
								isOn := a.projectTags[tg.ID]
								a.tagChip(c, "ptag-"+tg.ID, tg.Name, isOn).OnClick(func() {
									if err := a.projects.SetTag(a.projectID, tg.ID, !isOn); err != nil {
										a.status = err.Error()
										return
									}
									a.reload()
								})
							}
						})
					}
				})

			// Boards section.
			gui.Column(c).Gap(12).Children(func() {
				gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
					gui.Text(c, a.t("project.boards")).FontSize(15).Bold()
					a.pillBadge(c, fmt.Sprintf("%d", len(a.boards)), t.TextMuted, a.pal.subtle)
					gui.Spacer(c)
					gui.TextInput(c.Key("new-board"), &a.newBoardName).
						Placeholder(a.t("project.newBoard")).
						Width(200).
						OnSubmit(func() { a.createBoard() })
					gui.PrimaryButton(c.Key("create-board-btn"), "+ "+a.t("project.newBoard")).OnClick(func() {
						a.createBoard()
					})
				})

				if len(a.boards) == 0 {
					gui.Column(c).
						FillWidth().
						Padding(28).
						Gap(6).
						Radius(12).
						Background(a.pal.card).
						Border(1, a.pal.hairline).
						Center().
						Children(func() {
							gui.Icon(c, iconBoard).Size(24, 24).TextColor(a.pal.inkFaint)
							gui.Text(c, a.t("project.emptyBoards")).FontSize(13).Bold()
							gui.Text(c, a.t("project.emptyBoardsHint")).FontSize(12).TextColor(t.TextMuted)
						})
				} else {
					gui.Row(c).Gap(14).Wrap().Children(func() {
						for _, b := range a.boards {
							board := b
							a.boardCard(c, board)
						}
					})
				}
			})

			// Referenced assets visual grid.
			gui.Column(c).Gap(12).Children(func() {
				gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
					gui.Text(c, a.t("project.assets")).FontSize(15).Bold()
					a.pillBadge(c, fmt.Sprintf("%d", len(a.projectAssets)), t.TextMuted, a.pal.subtle)
				})

				if len(a.projectAssets) == 0 {
					gui.Column(c).
						FillWidth().
						Padding(28).
						Gap(6).
						Radius(12).
						Background(a.pal.card).
						Border(1, a.pal.hairline).
						Center().
						Children(func() {
							gui.Icon(c, iconImage).Size(24, 24).TextColor(a.pal.inkFaint)
							gui.Text(c, a.t("project.emptyAssets")).FontSize(13).Bold()
							gui.Text(c, a.t("project.emptyAssetsHint")).FontSize(12).TextColor(t.TextMuted)
						})
				} else {
					gui.Row(c).Gap(14).Wrap().Children(func() {
						for _, asset := range a.projectAssets {
							item := asset
							a.projectAssetCard(c, item)
						}
					})
				}
			})
		})
	})
}

func (a *App) boardCard(c *gui.Context, board model.Board) {
	t := c.Theme()
	count := a.boardCounts[board.ID]

	card := gui.Column(c.Key("board-card-" + board.ID)).
		Width(224).
		Padding(14, 16).
		Gap(10).
		Radius(12).
		Background(a.pal.card).
		Border(1, a.pal.hairline).
		Shadow(0, 2, 8, 0, a.pal.shadow)

	card.PopoverMenu(func(m *gui.Menu) {
		if m.Item(a.t("project.openBoard")).Chosen() {
			a.openBoard(board)
		}
		m.Separator()
		if m.Item(a.t("project.deleteBoard")).Chosen() {
			a.deleteBoardPrompt(board)
		}
	})

	card.Children(func() {
		gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
			gui.Box(c).Size(32, 32).Radius(8).Background(a.pal.subtle).Center().Children(func() {
				gui.Icon(c, iconBoard).Size(16, 16).TextColor(t.Text)
			})
			gui.Column(c).Grow(1).Gap(2).Children(func() {
				gui.Text(c, board.Name).FontSize(13).Bold().SingleLine()
				gui.Text(c, fmt.Sprintf(a.t("project.boardItems"), count)).FontSize(11).TextColor(a.pal.inkMuted)
			})
		})
		gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
			gui.Button(c.Key("open-b-"+board.ID), a.t("project.openBoard")).Grow(1).OnClick(func() {
				a.openBoard(board)
			})
			a.iconButton(c, "del-b-"+board.ID, iconTrash, a.t("project.deleteBoard"), false).OnClick(func() {
				a.deleteBoardPrompt(board)
			})
		})
	})
}

func (a *App) deleteBoardPrompt(board model.Board) {
	a.ask(a.t("project.deleteBoard"), a.t("project.deleteBoardNote"), a.t("common.delete"), func() {
		if a.boardID == board.ID {
			a.session = nil
			a.boardID = ""
		}
		if err := a.projects.DeleteBoard(board.ID); err != nil {
			a.status = err.Error()
			return
		}
		a.reload()
	})
}

func (a *App) projectAssetCard(c *gui.Context, item model.Asset) {
	t := c.Theme()
	services := c.Services()
	name := a.assetName(item)

	card := gui.Column(c.Key("pref-" + item.ID)).
		Width(164).
		Gap(6)

	card.PopoverMenu(func(m *gui.Menu) {
		if m.Item(a.t("library.preview")).Chosen() {
			a.openPreview(item)
		}
		if !item.Missing && item.Path != "" {
			if m.Item(a.t("common.openFile")).Chosen() {
				_ = openLocalFile(item.Path)
			}
			if m.Item(a.t("common.revealFile")).Chosen() {
				_ = revealLocalFile(item.Path)
			}
		}
		if m.Item(a.t("common.copyPath")).Chosen() {
			services.WriteClipboard(item.Path)
			a.status = a.t("common.copied")
		}
		m.Separator()
		if m.Item(a.t("project.unreference")).Chosen() {
			a.unreferencePrompt(item)
		}
	})

	card.Children(func() {
		thumb := gui.ButtonBase(c.Key("pref-open-" + item.ID)).
			Size(164, 132).
			Radius(10).
			Clip().
			Background(a.pal.card).
			Border(1, a.pal.hairline).
			Shadow(0, 2, 8, 0, a.pal.shadow).
			Center().
			OnClick(func() { a.openPreview(item) })
		thumb.Children(func() {
			if bm := a.bitmaps[item.ID]; bm != nil {
				gui.Image(c, bm).Fit(gui.Contain).Grow(1).Padding(4).Radius(8)
			} else {
				gui.Text(c, a.assetExt(item)).FontSize(11).Bold().TextColor(a.pal.inkMuted)
			}
		})

		gui.Row(c).Gap(4).AlignItems(gui.Center).Padding(0, 2).Children(func() {
			gui.Text(c, name).Grow(1).FontSize(12).TextColor(t.Text).SingleLine()
			a.iconButton(c, "unref-"+item.ID, iconX, a.t("project.unreference"), false).OnClick(func() {
				a.unreferencePrompt(item)
			})
		})
	})
}

func (a *App) unreferencePrompt(item model.Asset) {
	a.ask(a.t("project.unreference"), a.t("project.unreferenceNote"), a.t("common.remove"), func() {
		a.dropAssetFromSession(item.ID)
		if err := a.projects.Unreference(a.projectID, item.ID); err != nil {
			a.status = err.Error()
			return
		}
		a.reload()
	})
}

func (a *App) createBoard() {
	name := trim(a.newBoardName)
	if name == "" {
		name = a.t("project.newBoard")
	}
	b, err := a.projects.CreateBoard(a.projectID, name)
	if err != nil {
		a.status = err.Error()
		return
	}
	a.newBoardName = ""
	a.openBoard(b)
}

func (a *App) dropAssetFromSession(assetID string) {
	if a.session == nil || a.boardProject != a.projectID {
		return
	}
	var drop []string
	for _, item := range a.session.Items {
		if item.Kind == model.KindImage && item.AssetID == assetID {
			drop = append(drop, item.ID)
		}
	}
	if len(drop) == 0 {
		return
	}
	a.session.Delete(drop)
	a.flushBoard(true)
}
