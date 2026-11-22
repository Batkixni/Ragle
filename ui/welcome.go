package ui

import gui "github.com/egoist/mygo/ui"

// View builds the window. Tester calls it without a running app.
func (a *App) View(c *gui.Context) {
	a.captureTheme(c)
	if a.lib == nil || a.screen == screenWelcome {
		a.welcome(c)
	} else {
		a.shell(c)
	}
	a.previewModal(c)
	a.newProjectModal(c)
	a.shareModal(c)
	a.receivePairModal(c)
	a.confirm(c)
}

func (a *App) captureTheme(c *gui.Context) {
	sysDark := c.Theme().Dark
	dark := sysDark
	switch a.themeMode {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	theme, pal := makeTheme(dark)
	c.SetTheme(theme)
	a.dark = dark
	a.pal = pal
	a.paper = pal.paper
	a.ink = pal.paperInk
	a.accent = pal.ring
	a.muted = pal.inkMuted
	a.line = pal.hairline
}

func (a *App) welcome(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Fill().Background(a.pal.canvas).Center().Padding(32).Children(func() {
		gui.Column(c).
			Width(460).
			Padding(36, 36).
			Gap(20).
			Radius(16).
			Background(a.pal.card).
			Border(1, a.pal.hairline).
			Shadow(0, 12, 32, 0, a.pal.shadow).
			AlignItems(gui.Center).
			Children(func() {
				if bm := a.brandLogo(); bm != nil {
					gui.Box(c).Size(64, 64).Radius(14).Clip().Center().Children(func() {
						gui.Image(c, bm).Size(64, 64).Fit(gui.Contain)
					})
				}
				gui.Column(c).Gap(6).AlignItems(gui.Center).Children(func() {
					gui.Text(c, "Ragle").FontSize(28).Bold().LetterSpacing(-0.4)
					gui.Text(c, a.t("shell.localFirst")).FontSize(12).TextColor(a.pal.inkFaint)
				})
				gui.Text(c, a.t("welcome.lead")).
					Width(380).
					TextAlign(gui.Center).
					TextColor(t.TextMuted).
					LineHeight(1.5)

				gui.Column(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
					gui.PrimaryButton(c.Key("welcome-choose"), a.t("welcome.choose")).
						FillWidth().
						Padding(10, 18).
						OnClick(func() { a.chooseLibrary() })
					if a.canRestore {
						gui.Button(c.Key("welcome-restore"), a.t("welcome.restore")).
							FillWidth().
							OnClick(func() { a.restoreLibrary() })
					}
				})

				if a.status != "" {
					gui.Box(c).FillWidth().Padding(8, 12).Radius(8).Background(a.pal.subtle).Children(func() {
						gui.Text(c, a.status).FontSize(12).TextColor(t.TextMuted).TextAlign(gui.Center)
					})
				}

				gui.Divider(c)
				gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
					a.langButtons(c)
					gui.Spacer(c)
					a.themeQuickToggle(c)
				})
			})
	})
}

func (a *App) langButtons(c *gui.Context) {
	gui.Row(c).Gap(4).Children(func() {
		a.tagChip(c, "lang-zh", "中文", a.lang == i18nZh).OnClick(func() { a.setLang(i18nZh) })
		a.tagChip(c, "lang-en", "EN", a.lang == i18nEn).OnClick(func() { a.setLang(i18nEn) })
	})
}

func (a *App) themeQuickToggle(c *gui.Context) {
	gui.Row(c).Gap(4).Children(func() {
		a.tagChip(c, "theme-auto", a.t("shell.themeAuto"), a.themeMode == "auto").OnClick(func() { a.setThemeMode("auto") })
		a.tagChip(c, "theme-light", a.t("shell.themeLight"), a.themeMode == "light").OnClick(func() { a.setThemeMode("light") })
		a.tagChip(c, "theme-dark", a.t("shell.themeDark"), a.themeMode == "dark").OnClick(func() { a.setThemeMode("dark") })
	})
}
