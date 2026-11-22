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
	a.confirm(c)
}

func (a *App) captureTheme(c *gui.Context) {
	t := c.Theme()
	a.dark = t.Dark
	a.accent = t.Accent
	a.muted = t.TextMuted
	a.line = t.Border
	if t.Dark {
		a.paper = gui.Hex("#1c1c1c")
		a.ink = gui.Hex("#f4f1ea")
	} else {
		a.paper = gui.Hex("#f4f1ea")
		a.ink = gui.Hex("#1c1c1c")
	}
}

func (a *App) welcome(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Fill().Center().Gap(18).Padding(32).Children(func() {
		gui.Text(c, "Ragle").FontSize(36).Bold()
		gui.Text(c, a.t("welcome.lead")).Width(460).TextColor(t.TextMuted)
		gui.PrimaryButton(c, a.t("welcome.choose")).OnClick(func() { a.chooseLibrary() })
		if a.canRestore {
			gui.Button(c, a.t("welcome.restore")).OnClick(func() { a.restoreLibrary() })
		}
		if a.status != "" {
			gui.Text(c, a.status).Width(460).TextColor(t.TextMuted)
		}
		a.langButtons(c)
	})
}

func (a *App) langButtons(c *gui.Context) {
	gui.Row(c).Gap(8).Children(func() {
		gui.Button(c.Key("lang-zh"), "中文").OnClick(func() { a.setLang(i18nZh) })
		gui.Button(c.Key("lang-en"), "EN").OnClick(func() { a.setLang(i18nEn) })
	})
}
