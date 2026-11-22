package ui

import (
	"path/filepath"

	gui "github.com/egoist/mygo/ui"
)

func (a *App) shell(c *gui.Context) {
	t := c.Theme()
	if a.sidebarWidth <= 0 {
		a.sidebarWidth = 220
	}
	gui.Column(c).Fill().Background(t.Background).Children(func() {
		a.topBar(c)
		if a.pairRecvOpen {
			a.receivePairBar(c)
		}
		if a.screen == screenBoard {
			a.boardView(c)
		} else {
			gui.Split(c, &a.sidebarWidth, func() {
				a.sidebar(c)
			}, func() {
				switch a.screen {
				case screenProject:
					a.projectView(c)
				default:
					a.libraryView(c)
				}
			}).Grow(1)
		}
	})
}

func (a *App) topBar(c *gui.Context) {
	t := c.Theme()
	title := filepath.Base(a.cfg.LibraryPath)
	if title == "" || title == "." {
		title = "Ragle"
	}
	gui.Row(c).Padding(10, 16).Gap(12).AlignItems(gui.Center).Background(t.Surface).Border(0, t.Border).Children(func() {
		gui.Text(c, title).Bold().FontSize(15)
		if a.screen == screenProject || a.screen == screenBoard {
			gui.Text(c, "/").TextColor(t.TextMuted)
			gui.Text(c, a.project.Name).Bold().TextColor(t.Accent)
		}
		if a.screen == screenBoard {
			gui.Text(c, "/").TextColor(t.TextMuted)
			gui.Text(c, a.boardName).TextColor(t.TextMuted)
			gui.Button(c, a.t("board.leave")).OnClick(func() {
				a.flushBoard(true)
				a.session = nil
				a.boardID = ""
				a.screen = screenProject
				a.syncNavSelection()
			})
		}
		gui.Box(c).Grow(1)
		if a.status != "" {
			gui.Text(c, a.status).MaxWidth(360).TextColor(t.TextMuted).FontSize(12).SingleLine()
		}
		if a.screen != screenBoard {
			gui.Button(c.Key("top-recv-pair"), a.t("share.receiveByCode")).OnClick(func() {
				a.pairRecvOpen = !a.pairRecvOpen
			})
		}
		a.langButtons(c)
	})
}

func (a *App) sidebar(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Fill().Background(t.Surface).Children(func() {
		sb := gui.Sidebar(c, &a.selectedNav, func() {
			gui.SidebarItem(c, "library", nil, a.t("shell.library"))
			gui.SidebarSection(c, a.t("shell.tags"), nil, func() {
				gui.SidebarItem(c, "tag-all", nil, a.t("shell.all"))
				for _, tag := range a.tags {
					tid := tag.ID
					gui.SidebarItem(c, "tag:"+tid, nil, tag.Name)
				}
			})
			gui.SidebarSection(c, a.t("shell.projects"), nil, func() {
				for _, p := range a.projectList {
					pid := p.ID
					gui.SidebarItem(c, "project:"+pid, nil, p.Name)
				}
			})
		}).Grow(1)

		if sb.Changed() || a.selectedNav != a.lastNav {
			a.handleNav(a.selectedNav)
		}

		gui.Divider(c)
		gui.Column(c).Padding(10).Gap(8).Children(func() {
			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				gui.TextInput(c.Key("new-tag"), &a.newTagName).Placeholder(a.t("tag.new")).Grow(1)
				gui.Button(c, "+").OnClick(func() { a.createTag() })
			})
			if a.tagID != "" {
				gui.Button(c, a.t("tag.delete")).OnClick(func() {
					id := a.tagID
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
				})
			}
			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				gui.TextInput(c.Key("new-project"), &a.newProjectName).Placeholder(a.t("project.new")).Grow(1)
				gui.Button(c, "+").OnClick(func() { a.createProject() })
			})
		})
	})
}
