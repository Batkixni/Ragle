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
	a.editProjectModal(c)
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
	a.line = pal.surfaceMuted
}

func (a *App) welcome(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Fill().Background(a.pal.canvas).Center().Padding(32).Children(func() {
		gui.Column(c.Key("welcome-card")).
			Width(472).
			Padding(38, 38).
			Gap(22).
			Radius(radiusSurface).
			Background(a.pal.surface).
			Shadow(0, 16, 40, 0, a.pal.shadowMd).
			InsetShadow(0, 1, 1, 0, a.pal.rimLight).
			AlignItems(gui.Center).
			Transition(gui.ElementTransition{
				Animation: &arcEnterAnim,
				Enter:     &gui.Motion{Scale: 0.96, Y: 10, Opacity: 0},
			}).
			Children(func() {
				if bm := a.brandLogo(); bm != nil {
					gui.Box(c).
						Size(68, 68).
						Radius(radiusPanel).
						Background(a.pal.surfaceRaised).
						Shadow(0, 4, 14, 0, a.pal.shadowSm).
						Center().
						Children(func() {
							gui.Image(c, bm).Size(44, 44).Fit(gui.Contain)
						})
				}
				gui.Column(c).Gap(6).AlignItems(gui.Center).Children(func() {
					gui.Text(c, "Ragle").FontSize(28).Bold().LetterSpacing(-0.5)
					gui.Text(c, a.t("shell.localFirst")).FontSize(12).TextColor(a.pal.inkMuted)
				})
				gui.Text(c, a.t("welcome.lead")).
					Width(388).
					TextAlign(gui.Center).
					TextColor(t.TextMuted).
					LineHeight(1.55)

				gui.Column(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
					gui.PrimaryButton(c.Key("welcome-choose"), a.t("welcome.choose")).
						FillWidth().
						Padding(11, 18).
						Radius(radiusControl).
						OnClick(func() { a.chooseLibrary() })
					if a.canRestore {
						gui.Button(c.Key("welcome-restore"), a.t("welcome.restore")).
							FillWidth().
							Radius(radiusControl).
							OnClick(func() { a.restoreLibrary() })
					}
				})

				if a.status != "" {
					gui.Box(c).FillWidth().Padding(10, 14).Radius(radiusControl).Background(a.pal.surfaceMuted).Children(func() {
						gui.Text(c, a.status).FontSize(12).TextColor(t.TextMuted).TextAlign(gui.Center)
					})
				}

				gui.Row(c).FillWidth().AlignItems(gui.Center).Padding(4, 0, 0, 0).Children(func() {
					a.langButtons(c)
					gui.Spacer(c)
					a.themeQuickToggle(c)
				})
			})
	})
}

func (a *App) langButtons(c *gui.Context) {
	sel := 0
	if a.lang == i18nEn {
		sel = 1
	}
	a.arcPillTabs(c, "welcome-lang", sel, []string{"中文", "EN"}, func(idx int) {
		if idx == 0 {
			a.setLang(i18nZh)
		} else {
			a.setLang(i18nEn)
		}
	})
}

func (a *App) themeQuickToggle(c *gui.Context) {
	sel := 0
	switch a.themeMode {
	case "light":
		sel = 1
	case "dark":
		sel = 2
	}
	modes := []string{"auto", "light", "dark"}
	a.arcPillTabs(c, "welcome-theme", sel, []string{
		a.t("shell.themeAuto"),
		a.t("shell.themeLight"),
		a.t("shell.themeDark"),
	}, func(idx int) {
		a.setThemeMode(modes[idx])
	})
}
