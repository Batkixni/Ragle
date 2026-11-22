package ui

import gui "github.com/egoist/mygo/ui"

// View builds the window. Tester calls it without a running app.
func (a *App) View(c *gui.Context) {
	a.captureTheme(c)
	// Register window-wide shortcuts at the root scope (raw.parent == raw.root)
	// so Ctrl+A, Ctrl+F, Ctrl+I, Ctrl+,, Space, Delete, Arrows, and 0-5 work
	// even when no specific widget holds keyboard focus.
	a.registerWindowShortcuts(c)
	if a.lib == nil || a.screen == screenWelcome {
		a.welcome(c)
	} else {
		a.shell(c)
	}
	a.gotoModal(c)
	a.settingsModal(c)
	a.onboardingModal(c)
	a.tagManagerModal(c)
	a.renameModal(c)
	a.exportModal(c)
	a.newProjectModal(c)
	a.editProjectModal(c)
	a.editTagModal(c)
	a.shareModal(c)
	a.receivePairModal(c)
	a.confirm(c)
	a.toastOverlay(c)
}

// toastOverlay renders a sleek auto-dismissing floating toast pill at the
// bottom-center of the window so import/backup/batch notices never shift layout.
func (a *App) toastOverlay(c *gui.Context) {
	if !a.toastVisible || a.toastMsg == "" {
		return
	}
	gui.Row(c.Key("floating-toast-wrap")).
		Absolute().
		Left(0).
		Right(0).
		Bottom(24).
		Justify(gui.Center).
		PassThrough().
		Children(func() {
			gui.Row(c.Key("floating-toast")).
				Padding(10, 14, 10, 16).
				Gap(10).
				Radius(radiusPill).
				Background(a.pal.surfaceRaised).
				Shadow(0, 12, 32, 0, a.pal.shadowFloating).
				AlignItems(gui.Center).
				Transition(gui.ElementTransition{
					Animation: &springSmooth,
					Enter:     &gui.Motion{Y: 16, Opacity: 0},
					Exit:      &gui.Motion{Y: 10, Opacity: 0},
				}).
				Children(func() {
					gui.Box(c).Size(8, 8).Radius(radiusPill).Background(a.pal.controlOn)
					gui.Text(c, a.toastMsg).FontSize(12).Bold().TextColor(a.pal.ink).MaxWidth(440).SingleLine()
					a.iconButton(c, "toast-dismiss", iconX, a.t("common.close"), false).OnClick(func() {
						a.toastVisible = false
						a.status = ""
					})
				})
		})
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
	theme, pal, resolvedDark := makeThemeWithOverrides(dark, themeOverrides{
		preset:  a.themePreset,
		canvas:  a.customCanvas,
		surface: a.customSurface,
		accent:  a.customAccent,
		text:    a.customText,
	})
	c.SetTheme(theme)
	a.dark = resolvedDark
	a.pal = pal
	a.paper = pal.paper
	a.ink = pal.paperInk
	a.accent = pal.controlOn
	a.muted = pal.inkMuted
	a.line = pal.surfaceMuted
}

func (a *App) welcome(c *gui.Context) {
	gui.Column(c).Fill().Background(a.pal.canvas).Children(func() {
		// Integrated frameless window top bar.
		gui.Row(c.Key("welcome-titlebar")).
			Height(42).
			Padding(0, 12, 0, 16).
			AlignItems(gui.Center).
			Children(func() {
				gui.Row(c).Grow(1).FillHeight().DragWindow().AlignItems(gui.Center).Gap(8).Children(func() {
					if bm := a.brandLogo(); bm != nil {
						gui.Image(c, bm).Size(15, 15).Fit(gui.Contain)
					}
					gui.Text(c, "Ragle").FontSize(12).Bold().TextColor(a.pal.inkMuted)
				})
				a.windowControls(c)
			})

		gui.Column(c).Grow(1).FillWidth().Center().Padding(0, 32, 32, 32).Children(func() {
			card := gui.Column(c.Key("welcome-card")).
				Width(468).
				Padding(40, 40).
				Gap(22).
				Radius(radiusSurface).
				Background(a.pal.surface).
				Shadow(0, 12, 36, 0, a.pal.shadowRaised).
				AlignItems(gui.Center)
			applyBlurReveal(card, "welcome", 0)
			card.Children(func() {
				if bm := a.appIcon(); bm != nil {
					gui.Image(c, bm).Size(68, 68).Fit(gui.Contain)
				}
				gui.Column(c).Gap(6).AlignItems(gui.Center).Children(func() {
					gui.Text(c, "Ragle").FontSize(28).Bold().LetterSpacing(-0.5)
					gui.Text(c, a.t("shell.localFirst")).FontSize(12).TextColor(a.pal.inkMuted)
				})
				gui.Text(c, a.t("welcome.lead")).
					Width(380).
					TextAlign(gui.Center).
					TextColor(a.pal.inkSecondary).
					LineHeight(1.6)

				gui.Column(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
					a.arcButton(c, "welcome-choose", a.t("welcome.choose"), true).
						FillWidth().
						Padding(11, 18).
						OnClick(func() { a.chooseLibrary() })
					if a.canRestore {
						a.arcButton(c, "welcome-restore", a.t("welcome.restore"), false).
							FillWidth().
							OnClick(func() { a.restoreLibrary() })
					}
				})

				if a.status != "" {
					gui.Box(c).FillWidth().Padding(10, 14).Radius(radiusControl).Background(a.pal.surfaceMuted).Children(func() {
						gui.Text(c, a.status).FontSize(12).TextColor(a.pal.inkSecondary).TextAlign(gui.Center)
					})
				}

				gui.Row(c).FillWidth().AlignItems(gui.Center).Padding(6, 0, 0, 0).Children(func() {
					a.langButtons(c)
					gui.Spacer(c)
					a.themeQuickToggle(c)
				})
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
