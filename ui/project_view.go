package ui

import (
	"fmt"
	"time"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

// projectView renders a single unified top header (merged navbar + project hero,
// zero duplicate titles/buttons, integrated frameless window controls) above
// the scrollable Boards and Referenced Assets workspace.
func (a *App) projectView(c *gui.Context) {
	gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
		// Unified Project Header (top navbar + hero merged into one calm panel).
		gui.Box(c).Padding(10, 12, 4, 8).Children(func() {
			hdr := gui.Column(c.Key("proj-unified-header")).
				FillWidth().
				Radius(radiusPanel).
				Background(a.pal.surface)
			hdr.Children(func() {
				// Primary bar (48px — exact visual alignment with sidebar header & library bar).
				gui.Row(c).
					Height(48).
					Padding(0, 10, 0, 12).
					Gap(8).
					AlignItems(gui.Center).
					Children(func() {
						a.iconButton(c, "proj-back-lib", iconArrowLeft, a.t("shell.library"), false).OnClick(func() {
							a.handleNav("tag-all")
						})

						gui.Row(c).Gap(10).FillHeight().AlignItems(gui.Center).DragWindow().Children(func() {
							gui.Text(c, a.project.Name).FontSize(15).Bold().LetterSpacing(-0.2).SingleLine().MaxWidth(260)
							gui.Text(c, fmt.Sprintf(a.t("project.summary"), len(a.boards), len(a.projectAssets))).
								FontSize(11).
								TextColor(a.pal.inkMuted).
								SingleLine()
						})

						gui.Box(c).Grow(1).FillHeight().DragWindow()

						// Each project action appears once — zero duplication.
						a.arcButton(c, "proj-from-lib", a.t("project.fromLibrary"), true).OnClick(func() {
							a.referenceMode = true
							a.screen = screenLibrary
						})
						a.toolButton(c, "proj-edit-btn", iconEdit, a.t("project.edit"), false).OnClick(func() {
							a.openEditProjectModal()
						})
						a.toolButton(c, "proj-share-btn", iconShare, a.t("share.send"), false).OnClick(func() {
							a.shareOpen = true
						})
						a.iconButton(c, "proj-del-btn", iconTrash, a.t("project.delete"), false).OnClick(func() {
							id := a.projectID
							a.ask(a.t("project.delete"), a.t("project.deleteNote"), a.t("common.delete"), func() {
								if a.backupBeforeDelete && a.lib != nil {
									_ = a.lib.DB.Backup()
								}
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

						gui.Box(c).Width(1).Height(18).Background(a.pal.surfaceMuted)
						a.windowControls(c)
					})

				// Row 2: Project description on its own dedicated line.
				if a.project.Description != "" {
					gui.Row(c).
						FillWidth().
						Padding(2, 16, 6, 16).
						AlignItems(gui.Center).
						Children(func() {
							gui.Text(c, a.project.Description).
								FontSize(12).
								TextColor(a.pal.inkSecondary).
								LineHeight(1.45)
						})
				}

				// Row 3: Interactive project tags on their own dedicated row.
				if len(a.tags) > 0 {
					gui.Row(c).
						FillWidth().
						Padding(4, 16, 12, 16).
						Gap(6).
						Wrap().
						AlignItems(gui.Center).
						Children(func() {
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
		})

		// Scrollable project workspace.
		gui.Scroll(c.Key("proj-scroll")).Grow(1).FillWidth().Background(a.pal.canvas).Children(func() {
			col := gui.Column(c.Key("proj-view-" + a.projectID)).Padding(12, 20, 28, 12).Gap(24)
			applyPanelSwitch(col, a.projectID, 0, 6)
			col.Children(func() {
				if a.status != "" {
					gui.Row(c.Key("proj-status-toast")).
						Padding(6, 12).
						Radius(radiusControl).
						Background(a.pal.surface).
						AlignItems(gui.Center).
						Gap(8).
						Children(func() {
							gui.Text(c, a.status).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
							a.iconButton(c, "proj-status-dismiss", iconX, a.t("common.close"), false).OnClick(func() {
								a.status = ""
							})
						})
				}

				// Section 1: Boards.
				gui.Column(c).Gap(12).Children(func() {
					gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Text(c, a.t("project.boards")).FontSize(15).Bold()
						a.pillBadge(c, fmt.Sprintf("%d", len(a.boards)), a.pal.inkSecondary, a.pal.surface)
						gui.Spacer(c)
						gui.TextInput(c.Key("new-board"), &a.newBoardName).
							Placeholder(a.t("project.newBoard")).
							Width(196).
							OnSubmit(func() { a.createBoard() })
						a.arcButton(c, "create-board-btn", "+ "+a.t("project.newBoard"), true).OnClick(func() {
							a.createBoard()
						})
					})

					if len(a.boards) == 0 {
						gui.Column(c).
							FillWidth().
							Padding(28).
							Gap(8).
							Radius(radiusPanel).
							Background(a.pal.surface).
							Center().
							Children(func() {
								gui.Icon(c, iconBoard).Size(22, 22).TextColor(a.pal.inkFaint)
								gui.Text(c, a.t("project.emptyBoards")).FontSize(14).Bold()
								gui.Text(c, a.t("project.emptyBoardsHint")).FontSize(12).TextColor(a.pal.inkSecondary)
							})
					} else {
						gui.Row(c).Gap(14).Wrap().Children(func() {
							for i, b := range a.boards {
								a.boardCard(c, i, b)
							}
						})
					}
				})

				// Section 2: Referenced Assets.
				gui.Column(c).Gap(12).Children(func() {
					gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Text(c, a.t("project.assets")).FontSize(15).Bold()
						a.pillBadge(c, fmt.Sprintf("%d", len(a.projectAssets)), a.pal.inkSecondary, a.pal.surface)
					})

					if len(a.projectAssets) == 0 {
						gui.Column(c).
							FillWidth().
							Padding(28).
							Gap(8).
							Radius(radiusPanel).
							Background(a.pal.surface).
							Center().
							Children(func() {
								gui.Icon(c, iconImage).Size(22, 22).TextColor(a.pal.inkFaint)
								gui.Text(c, a.t("project.emptyAssets")).FontSize(14).Bold()
								gui.Text(c, a.t("project.emptyAssetsHint")).FontSize(12).TextColor(a.pal.inkSecondary)
							})
					} else {
						gui.Row(c).Gap(16).Wrap().Children(func() {
							for i, asset := range a.projectAssets {
								a.projectAssetCard(c, i, asset)
							}
						})
					}
				})
			})
		})
	})
}

func (a *App) boardCard(c *gui.Context, index int, board model.Board) {
	count := a.boardCounts[board.ID]

	card := gui.Column(c.Key("board-card-" + board.ID)).
		Width(228).
		Padding(14, 16).
		Gap(12).
		Radius(radiusPanel)

	card.PopoverMenu(func(m *gui.Menu) {
		if m.Item(a.t("project.openBoard")).Chosen() {
			a.openBoard(board)
		}
		m.Separator()
		if m.Item(a.t("project.deleteBoard")).Chosen() {
			a.deleteBoardPrompt(board)
		}
	})

	targetBg := a.pal.surface
	liftY := float32(0)
	shadow := a.pal.shadowResting
	if card.Hovered() {
		targetBg = a.pal.surfaceMuted
		liftY = -2
		shadow = a.pal.shadowRaised
	}
	card.Background(card.AnimateColor("bg", targetBg, animFast)).
		Shadow(0, 4, 14, 0, shadow).
		AnimatePose(gui.Pose{Y: liftY}, springGentle)
	applyCardBlurFade(card, board.ID, time.Duration(min(index, 6))*arcStaggerItem)

	card.Children(func() {
		gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
			gui.Box(c).Size(34, 34).Radius(radiusControl).Background(a.pal.surfaceRaised).Center().Children(func() {
				gui.Icon(c, iconBoard).Size(16, 16).TextColor(a.pal.ink)
			})
			gui.Column(c).Grow(1).Gap(2).Children(func() {
				gui.Text(c, board.Name).FontSize(13).Bold().SingleLine()
				gui.Text(c, fmt.Sprintf(a.t("project.boardItems"), count)).FontSize(11).TextColor(a.pal.inkMuted)
			})
		})
		gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
			a.arcButton(c, "open-b-"+board.ID, a.t("project.openBoard"), false).Grow(1).OnClick(func() {
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

func (a *App) projectAssetCard(c *gui.Context, index int, item model.Asset) {
	services := c.Services()
	name := a.assetName(item)

	card := gui.Column(c.Key("pref-" + item.ID)).
		Width(176).
		Radius(radiusPanel).
		Clip()

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

	hovered := card.Hovered()
	targetBg := a.pal.surface
	liftY := float32(0)
	imgScale := float32(1)
	shadow := a.pal.shadowResting
	if hovered {
		targetBg = a.pal.surfaceMuted
		liftY = -3
		imgScale = 1.04
		shadow = a.pal.shadowRaised
	}
	card.Background(card.AnimateColor("bg", targetBg, animFast)).
		Shadow(0, 4, 14, 0, shadow).
		AnimatePose(gui.Pose{Y: liftY}, springGentle)
	applyCardBlurFade(card, item.ID, time.Duration(min(index, 6))*arcStaggerItem)

	card.Children(func() {
		thumb := gui.ButtonBase(c.Key("pref-open-" + item.ID)).
			Size(176, 132).
			Clip().
			Background(a.pal.surfaceMuted).
			Center().
			OnClick(func() { a.openPreview(item) })
		thumb.Children(func() {
			if bm := a.bitmaps[item.ID]; bm != nil {
				gui.Image(c, bm).Fit(gui.Cover).Fill().AnimatePose(gui.Pose{Scale: imgScale}, springGentle)
			} else {
				gui.Text(c, a.assetExt(item)).FontSize(11).Bold().TextColor(a.pal.inkMuted)
			}
		})

		gui.Row(c).Gap(4).AlignItems(gui.Center).Padding(7, 10).Children(func() {
			gui.Text(c, name).Grow(1).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine()
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
