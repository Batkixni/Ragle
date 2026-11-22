package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"
)

const onboardingTotalSteps = 5

func (a *App) openOnboarding() {
	a.onboardingPrevStep = 0
	a.onboardingStep = 0
	a.onboardingOpen = true
}

func (a *App) setOnboardingStep(step int) {
	if step < 0 {
		step = 0
	}
	if step >= onboardingTotalSteps {
		step = onboardingTotalSteps - 1
	}
	if step == a.onboardingStep {
		return
	}
	a.onboardingPrevStep = a.onboardingStep
	a.onboardingStep = step
}

func (a *App) finishOnboarding() {
	a.onboardingOpen = false
	a.onboardingDone = true
	a.syncConfigFromState()
}

func (a *App) onboardingModal(c *gui.Context) {
	if !a.onboardingOpen {
		return
	}
	openProxy := a.onboardingOpen
	gui.DialogBase(c.Key("onboarding-modal"), &openProxy, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.62))
		panel.
			Width(780).
			Height(560).
			Radius(radiusSurface).
			Clip().
			Background(a.pal.canvas).
			Shadow(0, 24, 60, 0, a.pal.shadowFloating)
		applyBlurReveal(panel, "onboarding-modal", 0)

		panel.OnShortcut(0, gui.KeyLeft, func() {
			a.setOnboardingStep(a.onboardingStep - 1)
		})
		panel.OnShortcut(0, gui.KeyRight, func() {
			if a.onboardingStep < onboardingTotalSteps-1 {
				a.setOnboardingStep(a.onboardingStep + 1)
			}
		})
		panel.OnShortcut(0, gui.KeyEnter, func() {
			if a.onboardingStep < onboardingTotalSteps-1 {
				a.setOnboardingStep(a.onboardingStep + 1)
			} else {
				a.finishOnboarding()
			}
		})

		gui.Column(c).Fill().Children(func() {
			// Top bar: brand emblem + progress pills + language toggle + close.
			gui.Row(c).
				Height(56).
				Padding(0, 22).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					if bm := a.brandLogo(); bm != nil {
						gui.Image(c, bm).Size(20, 20).Fit(gui.Contain)
					}
					gui.Text(c, "Ragle · "+a.t("onboarding.open")).FontSize(13).Bold()
					gui.Spacer(c)

					// Quick language switch right in onboarding so users can read in zh-Hant or EN.
					sel := 0
					if a.lang == i18nEn {
						sel = 1
					}
					a.arcPillTabs(c, "ob-lang", sel, []string{"中文", "EN"}, func(idx int) {
						if idx == 0 {
							a.setLang(i18nZh)
						} else {
							a.setLang(i18nEn)
						}
					})

					a.iconButton(c, "ob-close", iconX, a.t("onboarding.skip"), false).OnClick(func() {
						a.finishOnboarding()
					})
				})

			// Step content area with directional horizontal Arc glide + soft de-blur.
			gui.Box(c).Grow(1).FillWidth().Clip().Children(func() {
				dirX := float32(18)
				if a.onboardingStep < a.onboardingPrevStep {
					dirX = -18
				}
				body := gui.Column(c.Key(fmt.Sprintf("ob-step-%d", a.onboardingStep))).
					Fill().
					Padding(26, 30, 18, 30).
					Gap(20)
				applyPanelSwitch(body, a.onboardingStep, dirX, 0)
				body.Children(func() {
					switch a.onboardingStep {
					case 0:
						a.onboardingFeatureStep(c, "s1", []struct {
							icon  *gui.SVG
							title string
							desc  string
						}{
							{iconDatabase, a.t("onboarding.s1.c1.title"), a.t("onboarding.s1.c1.desc")},
							{iconFolder, a.t("onboarding.s1.c2.title"), a.t("onboarding.s1.c2.desc")},
							{iconShare, a.t("onboarding.s1.c3.title"), a.t("onboarding.s1.c3.desc")},
						})
					case 1:
						a.onboardingFeatureStep(c, "s2", []struct {
							icon  *gui.SVG
							title string
							desc  string
						}{
							{iconLayoutBento, a.t("onboarding.s2.c1.title"), a.t("onboarding.s2.c1.desc")},
							{iconLayoutMasonry, a.t("onboarding.s2.c2.title"), a.t("onboarding.s2.c2.desc")},
							{iconLibrary, a.t("onboarding.s2.c3.title"), a.t("onboarding.s2.c3.desc")},
						})
					case 2:
						a.onboardingFeatureStep(c, "s3", []struct {
							icon  *gui.SVG
							title string
							desc  string
						}{
							{iconCheckSquare, a.t("onboarding.s3.c1.title"), a.t("onboarding.s3.c1.desc")},
							{iconFolder, a.t("onboarding.s3.c2.title"), a.t("onboarding.s3.c2.desc")},
							{iconTag, a.t("onboarding.s3.c3.title"), a.t("onboarding.s3.c3.desc")},
						})
					case 3:
						a.onboardingFeatureStep(c, "s4", []struct {
							icon  *gui.SVG
							title string
							desc  string
						}{
							{iconBoard, a.t("onboarding.s4.c1.title"), a.t("onboarding.s4.c1.desc")},
							{iconPlay, a.t("onboarding.s4.c2.title"), a.t("onboarding.s4.c2.desc")},
							{iconPalette, a.t("onboarding.s4.c3.title"), a.t("onboarding.s4.c3.desc")},
						})
					default:
						a.onboardingShortcutsStep(c)
					}
				})
			})

			// Footer bar: Skip | clickable progress pills | Back & Next / Start.
			gui.Row(c).
				Height(64).
				Padding(0, 24).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					a.toolButton(c, "ob-skip", nil, a.t("onboarding.skip"), false).OnClick(func() {
						a.finishOnboarding()
					})

					gui.Spacer(c)

					// Clickable step indicator dots.
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						for i := 0; i < onboardingTotalSteps; i++ {
							idx := i
							active := a.onboardingStep == idx
							w := float32(8)
							bg := a.pal.surfaceActive
							if active {
								w = 24
								bg = a.pal.controlOn
							}
							dot := gui.ButtonBase(c.Key(fmt.Sprintf("ob-dot-%d", idx))).
								Height(8).
								Radius(radiusPill).
								Label(fmt.Sprintf("%d", idx+1)).
								OnClick(func() {
									a.setOnboardingStep(idx)
								})
							aw := dot.AnimateTo("w", w, springSnappy)
							abg := dot.AnimateColor("bg", bg, animFast)
							dot.Width(aw).Background(abg)
						}
					})

					gui.Spacer(c)

					if a.onboardingStep > 0 {
						a.arcButton(c, "ob-prev", a.t("onboarding.prev"), false).OnClick(func() {
							a.setOnboardingStep(a.onboardingStep - 1)
						})
					}
					if a.onboardingStep < onboardingTotalSteps-1 {
						a.arcButton(c, "ob-next", a.t("onboarding.next"), true).OnClick(func() {
							a.setOnboardingStep(a.onboardingStep + 1)
						})
					} else {
						a.arcButton(c, "ob-start", a.t("onboarding.start"), true).OnClick(func() {
							a.finishOnboarding()
						})
					}
				})
		})
	})
	if !openProxy && a.onboardingOpen {
		a.finishOnboarding()
	}
}

func (a *App) onboardingFeatureStep(c *gui.Context, prefix string, cards []struct {
	icon  *gui.SVG
	title string
	desc  string
}) {
	gui.Column(c).Gap(8).Children(func() {
		gui.Row(c).Children(func() {
			a.pillBadge(c, a.t("onboarding."+prefix+".badge"), a.pal.inkSecondary, a.pal.surface)
		})
		gui.Text(c, a.t("onboarding."+prefix+".title")).FontSize(24).Bold().LetterSpacing(-0.4)
		gui.Text(c, a.t("onboarding."+prefix+".lead")).
			FontSize(13).
			TextColor(a.pal.inkSecondary).
			LineHeight(1.55)
	})

	gui.Row(c).Grow(1).FillWidth().Gap(14).AlignItems(gui.Stretch).Children(func() {
		for i, card := range cards {
			cd := card
			gui.Column(c.Key(fmt.Sprintf("ob-%s-card-%d", prefix, i))).
				Grow(1).
				Padding(18, 18).
				Gap(12).
				Radius(radiusPanel).
				Background(a.pal.surface).
				Children(func() {
					gui.Box(c).Size(38, 38).Radius(radiusControl).Background(a.pal.surfaceRaised).Center().Children(func() {
						gui.Icon(c, cd.icon).Size(18, 18).TextColor(a.pal.ink)
					})
					gui.Text(c, cd.title).FontSize(14).Bold().LineHeight(1.35)
					gui.Text(c, cd.desc).FontSize(12).TextColor(a.pal.inkSecondary).LineHeight(1.55)
				})
		}
	})
}

func (a *App) onboardingShortcutsStep(c *gui.Context) {
	gui.Column(c).Gap(8).Children(func() {
		gui.Row(c).Children(func() {
			a.pillBadge(c, a.t("onboarding.s5.badge"), a.pal.inkSecondary, a.pal.surface)
		})
		gui.Text(c, a.t("onboarding.s5.title")).FontSize(24).Bold().LetterSpacing(-0.4)
		gui.Text(c, a.t("onboarding.s5.lead")).
			FontSize(13).
			TextColor(a.pal.inkSecondary).
			LineHeight(1.55)
	})

	left := []struct{ keys, desc string }{
		{"Ctrl + A", a.t("onboarding.k.selectAll")},
		{"Ctrl + F", a.t("onboarding.k.search")},
		{"Ctrl + I", a.t("onboarding.k.import")},
		{"Ctrl + ,", a.t("onboarding.k.settings")},
		{"Esc", a.t("onboarding.k.esc")},
	}
	right := []struct{ keys, desc string }{
		{"Space", a.t("onboarding.k.preview")},
		{"Delete", a.t("onboarding.k.delete")},
		{"← / →", a.t("onboarding.k.nav")},
		{"0 – 5", a.t("onboarding.k.rating")},
	}

	renderCol := func(items []struct{ keys, desc string }) {
		gui.Column(c).Grow(1).Gap(8).Children(func() {
			for _, it := range items {
				gui.Row(c).
					FillWidth().
					Padding(9, 12).
					Gap(10).
					Radius(radiusControl).
					Background(a.pal.surface).
					AlignItems(gui.Center).
					Children(func() {
						gui.Box(c).
							Padding(3, 8).
							Radius(7).
							Background(a.pal.surfaceMuted).
							Children(func() {
								gui.Text(c, it.keys).FontSize(11).Bold().TextColor(a.pal.ink).SingleLine()
							})
						gui.Text(c, it.desc).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
					})
			}
		})
	}

	gui.Row(c).Grow(1).FillWidth().Gap(14).AlignItems(gui.Start).Children(func() {
		renderCol(left)
		renderCol(right)
	})
}
