package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"
)

const onboardingTotalSteps = 5

func (a *App) openOnboarding() {
	a.onboardingPrevStep = 0
	a.onboardingStep = 0
	a.onboardingTried = [onboardingTotalSteps]bool{}
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

// onboardingModal renders a sleek, non-blocking Interactive Guided Tour coach
// card floating cleanly at the bottom-center of the live workspace so users can
// see the real Raglex interface and hands-on try every feature step-by-step.
func (a *App) onboardingModal(c *gui.Context) {
	if !a.onboardingOpen {
		return
	}
	// Temporarily tuck away while a child modal (Settings / Go To / Tag Manager / Web Import) is open.
	if a.settingsOpen || a.gotoOpen || a.tagManagerOpen || a.webImportOpen {
		return
	}

	step := a.onboardingStep
	if step < 0 || step >= onboardingTotalSteps {
		step = 0
	}

	gui.Row(c.Key("onboarding-tour-wrap")).
		Absolute().
		Left(0).
		Right(0).
		Bottom(24).
		Justify(gui.Center).
		PassThrough().
		Children(func() {
			card := gui.Column(c.Key("onboarding-modal")).
				Width(560).
				Radius(16).
				Clip().
				Background(a.popoverSurface()).
				Border(1, a.chromeHairline()).
				Shadow(0, 20, 52, 0, a.pal.shadowFloating).
				Transition(gui.ElementTransition{
					Animation: &springSmooth,
					Enter:     &gui.Motion{Y: 20, Opacity: 0},
					Exit:      &gui.Motion{Y: 16, Opacity: 0},
				})
			applyBlurReveal(card, "onboarding-modal", 0)

			card.Children(func() {
				// Compact header: brand + Interactive Guide badge + progress pills + lang + close.
				headerBg := a.pal.surface
				if a.glassActive {
					if a.dark {
						headerBg = gui.Hex("#131317")
					} else {
						headerBg = gui.Hex("#F4F4F6")
					}
				}
				gui.Row(c).
					FillWidth().
					Padding(10, 14, 10, 16).
					Gap(8).
					AlignItems(gui.Center).
					Background(headerBg).
					BorderWidth(0, 0, 1, 0).
					BorderColor(a.chromeHairline()).
					Children(func() {
						if bm := a.brandLogo(); bm != nil {
							gui.Image(c, bm).Size(18, 18).Fit(gui.Contain)
						}
						gui.Text(c, "Raglex").FontSize(12.5).Bold()
						a.pillBadge(c, a.t("tour.badge"), a.pal.inkSecondary, a.pal.surfaceMuted)
						if a.onboardingTried[step] {
							a.pillBadge(c, "✓ "+a.t("tour.tried"), gui.Hex("#10B981"), gui.RGBA(16, 185, 129, 0.14))
						}
						gui.Spacer(c)

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

						a.iconButton(c, "ob-close", iconX, a.t("tour.skip"), false).
							Size(26, 26).
							OnClick(func() {
								a.finishOnboarding()
							})
					})

				// Step body with smooth horizontal slide & live hands-on action buttons.
				dirX := float32(14)
				if a.onboardingStep < a.onboardingPrevStep {
					dirX = -14
				}
				body := gui.Column(c.Key(fmt.Sprintf("ob-step-%d", step))).
					FillWidth().
					Padding(16, 20, 14, 20).
					Gap(12)
				applyPanelSwitch(body, step, dirX, 0)
				body.Children(func() {
					titleKey := fmt.Sprintf("tour.s%d.title", step+1)
					descKey := fmt.Sprintf("tour.s%d.desc", step+1)
					gui.Text(c, a.t(titleKey)).FontSize(16).Bold().LetterSpacing(-0.2)
					gui.Text(c, a.t(descKey)).
						FontSize(12.5).
						TextColor(a.pal.inkSecondary).
						LineHeight(1.55)

					// Hands-on live interactive actions for each step!
					gui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(gui.Center).Children(func() {
						switch step {
						case 0:
							a.arcButton(c, "ob-try-import", a.t("tour.s1.act1"), true).OnClick(func() {
								a.onboardingTried[0] = true
								a.startImport(false)
							})
							a.toolButton(c, "ob-try-web", iconLink, a.t("tour.s1.act2"), false).OnClick(func() {
								a.onboardingTried[0] = true
								a.webImportOpen = true
								a.webImportURL = ""
							})
						case 1:
							a.arcButton(c, "ob-try-select", a.t("tour.s2.act1"), true).OnClick(func() {
								a.onboardingTried[1] = true
								a.closePreview()
								a.screen = screenLibrary
								if len(a.assets) > 0 {
									a.selectCard(0, 0)
								}
							})
							if len(a.assets) > 0 {
								a.toolButton(c, "ob-try-preview", iconImage, a.t("tour.s2.act2"), false).OnClick(func() {
									a.onboardingTried[1] = true
									a.openPreview(a.assets[0])
								})
							}
						case 2:
							a.arcButton(c, "ob-try-goto", a.t("tour.s3.act1"), true).OnClick(func() {
								a.onboardingTried[2] = true
								a.openGotoModal()
							})
							a.toolButton(c, "ob-try-tags", iconTag, a.t("tour.s3.act2"), false).OnClick(func() {
								a.onboardingTried[2] = true
								a.tagManagerOpen = true
							})
						case 3:
							a.arcButton(c, "ob-try-board", a.t("tour.s4.act1"), true).OnClick(func() {
								a.onboardingTried[3] = true
								a.closePreview()
								if len(a.standaloneBoards) > 0 {
									a.openBoard(a.standaloneBoards[0])
									return
								}
								if a.projects != nil {
									if b, err := a.projects.CreateStandaloneBoard("靈感白板"); err == nil {
										a.reload()
										a.openBoard(b)
									}
								}
							})
						case 4:
							a.toolButton(c, "ob-theme-dark", iconMoon, a.t("tour.s5.themeDark"), a.dark && !a.glassMode).OnClick(func() {
								a.onboardingTried[4] = true
								a.glassMode = false
								a.themePreset = ""
								a.setThemeMode("dark")
								a.syncWindowVibrancy()
								a.syncConfigFromState()
							})
							a.toolButton(c, "ob-theme-light", iconSun, a.t("tour.s5.themeLight"), !a.dark && !a.glassMode).OnClick(func() {
								a.onboardingTried[4] = true
								a.glassMode = false
								a.themePreset = ""
								a.setThemeMode("light")
								a.syncWindowVibrancy()
								a.syncConfigFromState()
							})
							a.toolButton(c, "ob-theme-glass", iconSparkle, a.t("tour.s5.themeGlass"), a.glassMode).OnClick(func() {
								a.onboardingTried[4] = true
								a.applyThemePreset("glass")
							})
							a.toolButton(c, "ob-open-mcp", iconSliders, a.t("tour.s5.openMCP"), false).OnClick(func() {
								a.onboardingTried[4] = true
								a.finishOnboarding()
								a.openSettings(settingsTabAI)
							})
						}
					})
				})

				// Footer bar: Step progress indicator + Back / Next / Finish.
				gui.Row(c).
					FillWidth().
					Padding(10, 16).
					Gap(8).
					AlignItems(gui.Center).
					BorderWidth(1, 0, 0, 0).
					BorderColor(a.chromeHairline()).
					Children(func() {
						gui.Text(c, fmt.Sprintf(a.t("tour.stepOf"), step+1, onboardingTotalSteps)).
							FontSize(11).
							Bold().
							TextColor(a.pal.inkMuted).
							SingleLine()

						// Clickable step dots.
						gui.Row(c).Gap(5).Padding(0, 6).AlignItems(gui.Center).Children(func() {
							for i := 0; i < onboardingTotalSteps; i++ {
								idx := i
								active := step == idx
								w := float32(7)
								bg := a.pal.surfaceActive
								if active {
									w = 20
									bg = a.pal.controlOn
								} else if a.onboardingTried[idx] {
									bg = gui.Hex("#10B981")
								}
								dot := gui.ButtonBase(c.Key(fmt.Sprintf("ob-dot-%d", idx))).
									Height(7).
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

						if step > 0 {
							a.arcButton(c, "ob-prev", a.t("tour.prev"), false).OnClick(func() {
								a.setOnboardingStep(step - 1)
							})
						} else {
							a.toolButton(c, "ob-skip", nil, a.t("tour.skip"), false).OnClick(func() {
								a.finishOnboarding()
							})
						}

						if step < onboardingTotalSteps-1 {
							a.arcButton(c, "ob-next", a.t("tour.next"), true).OnClick(func() {
								a.setOnboardingStep(step + 1)
							})
						} else {
							a.arcButton(c, "ob-start", a.t("tour.done"), true).OnClick(func() {
								a.finishOnboarding()
							})
						}
					})
			})
		})
}
