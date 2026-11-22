package ui

import (
	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

func (a *App) projectView(c *gui.Context) {
	t := c.Theme()
	gui.Scroll(c).Grow(1).FillHeight().Children(func() {
		gui.Column(c).Padding(16).Gap(12).Children(func() {
			gui.Text(c, a.project.Name).FontSize(22).Bold()
			if a.project.Description != "" {
				gui.Text(c, a.project.Description).TextColor(t.TextMuted)
			}
			gui.Row(c).Gap(8).Children(func() {
				gui.Button(c, a.t("project.fromLibrary")).OnClick(func() {
					a.referenceMode = true
					a.screen = screenLibrary
				})
				gui.Button(c, a.t("share.send")).OnClick(func() { a.shareOpen = !a.shareOpen })
				gui.Button(c, a.t("project.delete")).OnClick(func() {
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
			if a.shareOpen {
				a.sharePanel(c)
			}
			gui.Text(c, a.t("library.tags")).Bold()
			owned := map[string]bool{}
			if ids, err := a.projects.TagIDs(a.projectID); err == nil {
				for _, id := range ids {
					owned[id] = true
				}
			}
			gui.Row(c).Gap(6).Children(func() {
				for _, tag := range a.tags {
					id := tag.ID
					label := tag.Name
					if owned[id] {
						label = "● " + label
					}
					gui.Button(c.Key("ptag-"+id), label).OnClick(func() {
						if err := a.projects.SetTag(a.projectID, id, !owned[id]); err != nil {
							a.status = err.Error()
							return
						}
						a.reload()
					})
				}
			})
			gui.Text(c, a.t("project.boards")).Bold()
			gui.Row(c).Gap(8).Children(func() {
				gui.TextInput(c.Key("new-board"), &a.newBoardName).Placeholder(a.t("project.newBoard")).Width(180)
				gui.Button(c, a.t("project.newBoard")).OnClick(func() { a.createBoard() })
			})
			if len(a.boards) == 0 {
				gui.Text(c, a.t("project.emptyBoards")).TextColor(t.TextMuted)
			}
			for _, b := range a.boards {
				board := b
				gui.Row(c.Key("board-row-" + board.ID)).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Text(c, board.Name).Width(200)
					gui.Button(c, a.t("project.openBoard")).OnClick(func() { a.openBoard(board) })
					gui.Button(c, a.t("project.deleteBoard")).OnClick(func() {
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
					})
				})
			}
			gui.Text(c, a.t("project.assets")).Bold()
			if len(a.projectAssets) == 0 {
				gui.Text(c, a.t("project.emptyAssets")).TextColor(t.TextMuted)
			}
			for _, asset := range a.projectAssets {
				item := asset
				gui.Row(c.Key("pref-" + item.ID)).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Text(c, a.assetName(item)).Width(240).Drag(assetDrag{ID: item.ID})
					gui.Button(c, a.t("project.unreference")).OnClick(func() {
						a.ask(a.t("project.unreference"), a.t("project.unreferenceNote"), a.t("common.remove"), func() {
							a.dropAssetFromSession(item.ID)
							if err := a.projects.Unreference(a.projectID, item.ID); err != nil {
								a.status = err.Error()
								return
							}
							a.reload()
						})
					})
				})
			}
		})
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
