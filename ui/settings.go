package ui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	gui "github.com/egoist/mygo/ui"

	"raglex/internal/model"
	"raglex/internal/updater"
)

const (
	settingsTabGeneral = iota
	settingsTabDatabase
	settingsTabImport
	settingsTabSharing
	settingsTabAI
	settingsTabPlugins
)

func (a *App) openSettings(tab int) {
	if tab >= settingsTabGeneral && tab <= settingsTabPlugins {
		a.settingsPrevTab = a.settingsTab
		a.settingsTab = tab
	}
	a.settingsOpen = true
	if a.lib != nil {
		a.aiIndexedCount = a.lib.VectorCount()
	}
	a.refreshLayaStatus()
	a.refreshDBStats()
}

func (a *App) setSettingsTab(tab int) {
	if tab == a.settingsTab {
		return
	}
	a.settingsPrevTab = a.settingsTab
	a.settingsTab = tab
	if tab == settingsTabDatabase && !a.dbStatsLoaded {
		a.refreshDBStats()
	}
}

func (a *App) refreshDBStats() {
	if a.lib == nil || a.dbStatsBusy {
		return
	}
	dbPath := a.lib.DB.Path()
	bakPath := a.lib.DB.BackupPath()
	filesDir := a.lib.DB.FilesDir()
	thumbsDir := a.lib.DB.ThumbsDir()
	totalBoards := 0
	if a.projects != nil {
		for _, p := range a.projectList {
			if boards, err := a.projects.Boards(p.ID); err == nil {
				totalBoards += len(boards)
			}
		}
	}
	compute := func() dbStatsInfo {
		var st dbStatsInfo
		st.TotalBoards = totalBoards
		if fi, err := os.Stat(dbPath); err == nil {
			st.DBBytes = fi.Size()
		}
		if fi, err := os.Stat(bakPath); err == nil {
			st.HasBackup = true
			st.BackupBytes = fi.Size()
			st.BackupTime = fi.ModTime()
		}
		st.FilesBytes, st.FilesCount = dirStats(filesDir)
		st.ThumbsBytes, st.ThumbsCount = dirStats(thumbsDir)
		return st
	}
	if a.win == nil {
		a.dbStats = compute()
		a.dbStatsLoaded = true
		return
	}
	a.dbStatsBusy = true
	win := a.win
	go func() {
		st := compute()
		win.Update(func() {
			a.dbStatsBusy = false
			if a.isClosed() {
				return
			}
			a.dbStats = st
			a.dbStatsLoaded = true
		})
	}()
}

func dirStats(root string) (int64, int) {
	var total int64
	var count int
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
			count++
		}
		return nil
	})
	return total, count
}

func (a *App) backupDatabaseNow() {
	if a.lib == nil {
		return
	}
	if a.win == nil {
		if err := a.lib.DB.Backup(); err != nil {
			a.status = err.Error()
			return
		}
		a.notify(a.t("settings.backupDone"))
		a.refreshDBStats()
		return
	}
	lib := a.lib
	win := a.win
	go func() {
		err := lib.DB.Backup()
		win.Update(func() {
			if a.isClosed() {
				return
			}
			if err != nil {
				a.notify(err.Error())
				return
			}
			a.notify(a.t("settings.backupDone"))
			a.refreshDBStats()
		})
	}()
}

func (a *App) exportDatabaseBackup() {
	if a.lib == nil || a.win == nil {
		return
	}
	win := a.win
	lib := a.lib
	title := a.t("settings.exportBackup")
	defName := fmt.Sprintf("raglex-backup-%s.db", time.Now().Format("20060102-1504"))
	go func() {
		dst, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      win,
			Title:       title,
			DefaultPath: defName,
		})
		if err != nil || dst == "" {
			return
		}
		err = lib.DB.BackupTo(dst)
		win.Update(func() {
			if a.isClosed() {
				return
			}
			if err != nil {
				a.status = err.Error()
				return
			}
			a.status = fmt.Sprintf(a.t("settings.exportBackupDone"), filepath.Base(dst))
		})
	}()
}

func (a *App) rebuildAllThumbnails() {
	if a.lib == nil {
		return
	}
	a.queued = map[string]bool{}
	count := 0
	for _, asset := range a.allAssets {
		if asset.Missing {
			continue
		}
		kind := model.MediaKind(asset.Mime)
		if kind != model.MediaImage && kind != model.MediaVideo {
			continue
		}
		if a.thumbs != nil {
			a.thumbs.Enqueue(asset.ID)
			a.queued[asset.ID] = true
		}
		count++
	}
	a.notify(fmt.Sprintf(a.t("settings.rebuildThumbsDone"), count))
}

func (a *App) cleanOrphanThumbs() {
	if a.lib == nil {
		return
	}
	thumbsDir := a.lib.DB.ThumbsDir()
	valid := make(map[string]bool, len(a.allAssets))
	for _, asset := range a.allAssets {
		if asset.ThumbnailPath != "" {
			valid[filepath.Clean(asset.ThumbnailPath)] = true
		}
	}
	entries, err := os.ReadDir(thumbsDir)
	if err != nil {
		a.status = err.Error()
		return
	}
	var removed int
	var freed int64
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		full := filepath.Clean(filepath.Join(thumbsDir, ent.Name()))
		if valid[full] {
			continue
		}
		if info, err := ent.Info(); err == nil {
			freed += info.Size()
		}
		if os.Remove(full) == nil {
			removed++
		}
	}
	a.notify(fmt.Sprintf(a.t("settings.cleanThumbsDone"), removed, formatBytes(freed)))
	a.refreshDBStats()
}

func (a *App) settingsModal(c *gui.Context) {
	if !a.settingsOpen {
		return
	}
	gui.DialogBase(c.Key("settings-modal"), &a.settingsOpen, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.56))
		panel.
			Width(800).
			Height(620).
			Radius(radiusSurface).
			Clip().
			Background(a.popoverSurface()).
			Border(1, a.chromeHairline()).
			Shadow(0, 22, 54, 0, a.pal.shadowFloating)
		applyBlurReveal(panel, "settings-modal", 0)

		tabs := []struct {
			id    int
			icon  *gui.SVG
			label string
		}{
			{settingsTabGeneral, iconSliders, a.t("settings.tab.general")},
			{settingsTabDatabase, iconDatabase, a.t("settings.tab.database")},
			{settingsTabImport, iconImport, a.t("settings.tab.import")},
			{settingsTabSharing, iconShare, a.t("settings.tab.sharing")},
			{settingsTabAI, iconSparkle, a.t("settings.tab.ai")},
			{settingsTabPlugins, iconPalette, a.t("settings.tab.plugins")},
		}

		gui.Row(c).Fill().Children(func() {
			// Left category sidebar with Arc gliding pill highlight.
			navBg := a.pal.surface
			if a.glassActive {
				if a.dark {
					navBg = gui.Hex("#131317")
				} else {
					navBg = gui.Hex("#F4F4F6")
				}
			}
			gui.Column(c).
				Width(198).
				FillHeight().
				Background(navBg).
				Padding(18, 12, 16, 12).
				Gap(14).
				Children(func() {
					gui.Text(c, a.t("settings.title")).FontSize(15).Bold().Padding(0, 10)

					navTrack := gui.Column(c.Key("settings-nav-track")).FillWidth().Gap(3)
					var activeBtn gui.Element
					var hasActive bool
					navTrack.Children(func() {
						for _, tab := range tabs {
							tb := tab
							active := a.settingsTab == tb.id
							btn := gui.ButtonBase(c.Key(fmt.Sprintf("stab-%d", tb.id))).
								FillWidth().
								Padding(9, 12).
								Gap(10).
								Radius(radiusControl).
								AlignItems(gui.Center).
								OnClick(func() {
									a.setSettingsTab(tb.id)
								})
							targetFg := a.pal.inkSecondary
							if active || btn.Hovered() {
								targetFg = a.pal.ink
							}
							fg := btn.AnimateColor("fg", targetFg, animFast)
							if !active {
								hbg := gui.Transparent
								if btn.Hovered() {
									hbg = a.pal.surfaceMuted
								}
								btn.Background(btn.AnimateColor("hbg", hbg, animFast))
							}
							btn.Children(func() {
								gui.Icon(c, tb.icon).Size(15, 15).TextColor(fg)
								gui.Text(c, tb.label).FontSize(13).Bold().TextColor(fg).Grow(1).SingleLine()
							})
							if active {
								activeBtn = btn
								hasActive = true
							}
						}
					})
					attachGlidingPill(navTrack, activeBtn, hasActive, radiusControl, a.pal.pillActive, a.pal.shadowResting)

					gui.Spacer(c)

					gui.Column(c).Padding(0, 10).Gap(2).Children(func() {
						gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
							gui.Text(c, "Raglex v"+updater.Version).FontSize(11).Bold().TextColor(a.pal.inkMuted).SingleLine()
							if a.updateAvailable || a.updateInstalled {
								gui.Box(c).Size(7, 7).Radius(radiusPill).Background(gui.Hex("#10B981"))
							}
						})
						gui.Text(c, a.t("shell.localFirst")).FontSize(10).TextColor(a.pal.inkFaint).SingleLine()
					})
				})

			// Right content area: header + directional animated scrollable cards + footer.
			rightBg := a.pal.canvas
			if a.glassActive {
				rightBg = a.popoverSurface()
			}
			gui.Column(c).Grow(1).FillHeight().Background(rightBg).Children(func() {
				// Top header with clear title + human-friendly 1-line subtitle.
				activeTitle := tabs[0].label
				activeSub := a.t("settings.sub.general")
				subKeys := map[int]string{
					settingsTabGeneral:  "settings.sub.general",
					settingsTabDatabase: "settings.sub.database",
					settingsTabImport:   "settings.sub.import",
					settingsTabSharing:  "settings.sub.sharing",
					settingsTabAI:       "settings.sub.ai",
					settingsTabPlugins:  "settings.sub.plugins",
				}
				for _, tb := range tabs {
					if tb.id == a.settingsTab {
						activeTitle = tb.label
						break
					}
				}
				if sk, ok := subKeys[a.settingsTab]; ok {
					activeSub = a.t(sk)
				}
				gui.Row(c).
					Padding(16, 22, 12, 22).
					Gap(12).
					AlignItems(gui.Center).
					BorderWidth(0, 0, 1, 0).
					BorderColor(a.chromeHairline()).
					Children(func() {
						gui.Column(c).Grow(1).Gap(2).Children(func() {
							gui.Text(c, activeTitle).FontSize(16).Bold()
							gui.Text(c, activeSub).FontSize(11.5).TextColor(a.pal.inkSecondary).SingleLine()
						})
						a.iconButton(c, "settings-close-x", iconX, a.t("common.close"), false).OnClick(func() {
							a.settingsOpen = false
						})
					})

				// Scrollable grouped cards with directional Arc panel transition.
				gui.Scroll(c.Key("settings-scroll")).Grow(1).FillWidth().Children(func() {
					dirY := float32(10)
					if a.settingsTab < a.settingsPrevTab {
						dirY = -10
					}
					body := gui.Column(c.Key(fmt.Sprintf("settings-body-%d", a.settingsTab))).
						FillWidth().
						Padding(14, 22, 20, 22).
						Gap(14)
					applyPanelSwitch(body, a.settingsTab, 0, dirY)
					body.Children(func() {
						switch a.settingsTab {
						case settingsTabGeneral:
							a.settingsGeneralTab(c)
						case settingsTabDatabase:
							a.settingsDatabaseTab(c)
						case settingsTabImport:
							a.settingsImportTab(c)
						case settingsTabSharing:
							a.settingsSharingTab(c)
						case settingsTabAI:
							a.settingsAITab(c)
						case settingsTabPlugins:
							a.settingsPluginsTab(c)
						}
					})
				})

				// Footer bar.
				gui.Row(c).
					Padding(12, 22).
					Gap(10).
					AlignItems(gui.Center).
					BorderWidth(1, 0, 0, 0).
					BorderColor(a.chromeHairline()).
					Children(func() {
						if a.status != "" {
							gui.Text(c, a.status).FontSize(11).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
						} else {
							gui.Spacer(c)
						}
						a.arcButton(c, "settings-done", a.t("common.done"), true).OnClick(func() {
							a.syncConfigFromState()
							a.settingsOpen = false
						})
					})
			})
		})
	})
}

func (a *App) settingsGroup(c *gui.Context, title string, content func()) {
	cardBg := a.pal.surface
	if a.glassActive {
		if a.dark {
			cardBg = gui.RGBA(255, 255, 255, 0.04)
		} else {
			cardBg = gui.RGBA(0, 0, 0, 0.03)
		}
	}
	gui.Column(c).
		FillWidth().
		Padding(14, 16).
		Gap(11).
		Radius(radiusPanel).
		Background(cardBg).
		Border(1, a.chromeHairline()).
		Children(func() {
			a.sectionLabel(c, title)
			content()
		})
}

func (a *App) settingsSwitchRow(c *gui.Context, key, label string, val *bool, onChange func()) {
	gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
		gui.Text(c, label).FontSize(13).TextColor(a.pal.ink).Grow(1).LineHeight(1.35)
		a.arcSwitch(c, key, val, onChange)
	})
}

func (a *App) settingsSwitchRowDesc(c *gui.Context, key, label, desc string, val *bool, onChange func()) {
	gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
		gui.Column(c).Grow(1).Gap(2).Children(func() {
			gui.Text(c, label).FontSize(13).TextColor(a.pal.ink).LineHeight(1.3)
			if desc != "" {
				gui.Text(c, desc).FontSize(11).TextColor(a.pal.inkMuted).LineHeight(1.35)
			}
		})
		a.arcSwitch(c, key, val, onChange)
	})
}

func (a *App) settingsGeneralTab(c *gui.Context) {
	// Appearance & Frosted Glass group.
	a.settingsGroup(c, a.t("settings.appearance"), func() {
		gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("shell.theme")).FontSize(13).Grow(1)
			sel := 0
			switch a.themeMode {
			case "light":
				sel = 1
			case "dark":
				sel = 2
			}
			modes := []string{"auto", "light", "dark"}
			a.arcPillTabs(c, "pref-theme", sel, []string{
				a.t("shell.themeAuto"),
				a.t("shell.themeLight"),
				a.t("shell.themeDark"),
			}, func(idx int) {
				a.setThemeMode(modes[idx])
			})
		})

		presetIDs := []string{"", "graphite", "warm", "midnight", "glass", "custom"}
		presetSel := 0
		for i, id := range presetIDs {
			if a.themePreset == id {
				presetSel = i
				break
			}
		}
		gui.Column(c).FillWidth().Gap(8).Children(func() {
			gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("settings.themePreset")).FontSize(13).Grow(1)
				a.toolButton(c, "pref-theme-reset", iconRefresh, a.t("settings.resetTheme"), false).OnClick(func() {
					a.resetCustomTheme()
				})
			})
			a.arcPillTabs(c, "pref-theme-preset", presetSel, []string{
				a.t("settings.preset.default"),
				a.t("settings.preset.graphite"),
				a.t("settings.preset.warm"),
				a.t("settings.preset.midnight"),
				a.t("settings.preset.glass"),
				a.t("settings.preset.custom"),
			}, func(idx int) {
				if presetIDs[idx] == "custom" {
					a.themePreset = "custom"
					if a.customCanvas == "" {
						a.customCanvas = colorHex(a.pal.canvas)
					}
					if a.customSurface == "" {
						a.customSurface = colorHex(a.pal.surface)
					}
					if a.customAccent == "" {
						a.customAccent = colorHex(a.pal.controlOn)
					}
					if a.customText == "" {
						a.customText = colorHex(a.pal.ink)
					}
					a.syncConfigFromState()
				} else {
					a.applyThemePreset(presetIDs[idx])
				}
			})
		})

		a.settingsSwitchRowDesc(c, "pref-glass-mode", a.t("settings.glassMode"), a.t("settings.glassDesc"), &a.glassMode, func() {
			a.syncWindowVibrancy()
			a.syncConfigFromState()
		})
		if a.glassMode {
			gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("settings.glassOpacity")).FontSize(13).Grow(1)
				gui.Text(c, fmt.Sprintf("%d%%", int(a.glassOpacity*100+0.5))).FontSize(12).Bold().TextColor(a.pal.inkSecondary)
				gui.Slider(c.Key("pref-glass-opacity"), &a.glassOpacity, 0.30, 0.96).Width(160).OnChange(func() {
					a.syncConfigFromState()
				})
			})
		}

		// Only expand hex color pickers when user explicitly chooses Custom preset.
		if a.themePreset == "custom" {
			gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
			a.themeColorRow(c, "pref-col-canvas", a.t("settings.colorCanvas"), &a.customCanvas, a.pal.canvas, []string{"#111111", "#18191C", "#090A0D", "#FFFFFF", "#F6F3EE"})
			a.themeColorRow(c, "pref-col-surface", a.t("settings.colorSurface"), &a.customSurface, a.pal.surface, []string{"#18181B", "#222428", "#12141A", "#F4F4F5", "#EDE8DF"})
			a.themeColorRow(c, "pref-col-accent", a.t("settings.colorAccent"), &a.customAccent, a.pal.controlOn, []string{"#EDEDED", "#18181B", "#FF5C33", "#E11D48", "#10B981"})
			a.themeColorRow(c, "pref-col-text", a.t("settings.colorText"), &a.customText, a.pal.ink, []string{"#FAFAFA", "#E4E4E7", "#18181B", "#1F1B17"})
		} else {
			gui.Text(c, a.t("settings.customColorsToggle")).FontSize(11).TextColor(a.pal.inkMuted)
		}

		gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
		gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("shell.language")).FontSize(13).Grow(1)
			sel := 0
			if a.lang == i18nEn {
				sel = 1
			}
			a.arcPillTabs(c, "pref-lang", sel, []string{"繁體中文", "English"}, func(idx int) {
				if idx == 0 {
					a.setLang(i18nZh)
				} else {
					a.setLang(i18nEn)
				}
			})
		})
	})

	// Workspace display group.
	a.settingsGroup(c, a.t("settings.workspace"), func() {
		gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("settings.defaultLayout")).FontSize(13).Grow(1)
			sel := 0
			switch a.layoutMode {
			case layoutMasonry:
				sel = 1
			case layoutGrid:
				sel = 2
			}
			modes := []string{layoutBento, layoutMasonry, layoutGrid}
			a.arcPillTabs(c, "pref-layout", sel, []string{
				a.t("library.layout.bento"),
				a.t("library.layout.masonry"),
				a.t("library.layout.grid"),
			}, func(idx int) {
				a.setLayoutMode(modes[idx])
			})
		})

		gui.Column(c).FillWidth().Gap(6).Children(func() {
			gui.Text(c, a.t("settings.defaultSort")).FontSize(13)
			a.arcPillTabs(c, "pref-sort", a.sortBy, []string{
				a.t("library.sortNewest"),
				a.t("library.sortOldest"),
				a.t("library.sortName"),
				a.t("library.sortRating"),
			}, func(idx int) {
				a.sortBy = idx
				a.applyFilters()
				a.syncConfigFromState()
			})
		})

		gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("settings.cardSize")).FontSize(13).Grow(1)
			gui.Text(c, fmt.Sprintf("%d px", int(a.cardSize))).FontSize(12).TextColor(a.pal.inkSecondary)
			gui.Slider(c.Key("pref-card-size"), &a.cardSize, 128, 300).Width(160).OnChange(func() {
				a.syncConfigFromState()
			})
		})

		gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
		a.settingsSwitchRow(c, "pref-sb-counts", a.t("settings.sidebarCounts"), &a.showSidebarCounts, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRow(c, "pref-tag-colors", a.t("settings.tagColors"), &a.showTagColors, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRow(c, "pref-card-caption", a.t("settings.cardCaption"), &a.showCardCaption, func() {
			a.syncConfigFromState()
		})

		gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
		gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("onboarding.open")).FontSize(13).Grow(1)
			a.toolButton(c, "pref-reopen-ob", iconHelp, a.t("onboarding.reopen"), false).OnClick(func() {
				a.settingsOpen = false
				a.openOnboarding()
			})
		})
	})

	// Software updates (GitHub Releases Auto-Updater).
	services := c.Services()
	a.settingsGroup(c, a.t("update.section"), func() {
		gui.Row(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("update.current")).FontSize(13).Grow(1)
			a.pillBadge(c, "v"+updater.Version, a.pal.ink, a.pal.surfaceMuted)
		})
		a.settingsSwitchRow(c, "pref-upd-auto", a.t("update.autoCheck"), &a.autoCheckUpdates, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRow(c, "pref-upd-beta", a.t("update.betaChannel"), &a.updateBetaChannel, func() {
			a.syncConfigFromState()
		})

		if a.updateMsg != "" {
			msgCol := a.pal.inkSecondary
			if a.updateInstalled || a.updateAvailable {
				msgCol = a.pal.ink
			}
			if a.updateErr != "" {
				msgCol = a.pal.danger
			}
			gui.Text(c, a.updateMsg).FontSize(12).Bold().TextColor(msgCol).LineHeight(1.4)
		}

		if a.updateApplying {
			pct := float32(a.updatePct)
			if pct < 4 {
				pct = 4
			} else if pct > 100 {
				pct = 100
			}
			gui.Box(c).FillWidth().Height(6).Radius(radiusPill).Clip().Background(a.pal.surfaceMuted).Children(func() {
				gui.Box(c).WidthPercent(pct).FillHeight().Radius(radiusPill).Background(a.pal.controlOn)
			})
		}

		gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Wrap().Children(func() {
			if a.updateInstalled {
				a.arcButton(c, "pref-upd-restart", a.t("update.restartBtn"), true).OnClick(func() {
					a.restartToApplyUpdate()
				})
			} else if a.updateAvailable && a.updateRelease != nil && a.updateRelease.DownloadURL != "" {
				a.arcButton(c, "pref-upd-install", a.t("update.installBtn"), true).
					Disabled(a.updateApplying).
					OnClick(func() { a.applyUpdateNow() })
			}
			a.arcButton(c, "pref-upd-check", a.t("update.checkBtn"), !a.updateAvailable && !a.updateInstalled).
				Disabled(a.updateChecking || a.updateApplying).
				OnClick(func() { a.checkForUpdatesNow() })
			if a.updateRelease != nil && a.updateRelease.HTMLURL != "" {
				relURL := a.updateRelease.HTMLURL
				a.toolButton(c, "pref-upd-notes", iconExternal, a.t("update.openRelease"), false).OnClick(func() {
					services.OpenURL(relURL)
				})
			}
		})
	})
}

func (a *App) settingsAITab(c *gui.Context) {
	services := c.Services()

	// 1. Prominent MCP AI Agent Integration card (17 tools).
	a.settingsGroup(c, a.t("mcp.section"), func() {
		gui.Text(c, a.t("mcp.desc")).FontSize(12).TextColor(a.pal.inkSecondary).LineHeight(1.45)

		dotCol := a.pal.inkFaint
		statusText := a.t("mcp.stopped")
		if a.localSrv != nil && a.localSrv.Running() {
			dotCol = gui.Hex("#10B981")
			statusText = fmt.Sprintf(a.t("mcp.running"), a.localSrv.LocalURL())
		}
		gui.Row(c).FillWidth().Padding(9, 12).Gap(8).Radius(radiusControl).Background(a.pal.surfaceMuted).AlignItems(gui.Center).Children(func() {
			gui.Box(c).Size(8, 8).Radius(radiusPill).Background(dotCol)
			gui.Text(c, statusText).FontSize(12).Bold().TextColor(a.pal.ink).Grow(1).SingleLine()
			a.arcSwitch(c, "pref-mcp-srv-run", &a.serverRunning, func() {
				a.toggleBuiltInServer()
			})
		})

		gui.Text(c, a.t("mcp.toolsSummary")).FontSize(11).TextColor(a.pal.inkMuted).LineHeight(1.45)

		gui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(gui.Center).Children(func() {
			a.arcButton(c, "pref-ai-copy-mcp", a.t("server.copyMCP"), true).OnClick(func() {
				services.WriteClipboard(a.buildMCPConfigJSON())
				a.notify(a.t("server.mcpCopied"))
			})
			a.toolButton(c, "pref-ai-copy-tok", iconCopy, a.t("server.copyToken"), false).OnClick(func() {
				services.WriteClipboard(a.ensureServerToken())
				a.notify(a.t("common.copied"))
			})
		})
		gui.Text(c, a.t("mcp.stepHint")).FontSize(11).TextColor(a.pal.inkMuted).LineHeight(1.4)
	})

	// 2. Built-in Visual Embedding Model card.
	a.settingsGroup(c, a.t("ai.embedSection"), func() {
		a.settingsSwitchRowDesc(c, "pref-ai-embed-en", a.t("ai.embedEnable"), a.t("ai.embedDesc"), &a.embedEnabled, func() {
			a.syncConfigFromState()
		})
		gui.Row(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
			gui.Text(c, fmt.Sprintf(a.t("ai.embedStatus"), a.aiIndexedCount, a.allAssetsCount)).
				FontSize(12).
				Bold().
				TextColor(a.pal.ink).
				Grow(1)
			a.arcButton(c, "pref-ai-reindex", a.t("ai.reindex"), false).
				Disabled(a.aiIndexBusy || !a.embedEnabled).
				OnClick(func() { a.reindexVisualEmbeddings() })
		})
	})

	// 3. Built-in Laya Decision Engine with one-click in-app deployment & runtime management.
	a.settingsGroup(c, a.t("laya.section"), func() {
		a.settingsSwitchRowDesc(c, "pref-ai-laya-en", a.t("ai.layaEnable"), a.t("ai.layaBuiltInDesc"), &a.layaEnabled, func() {
			a.syncConfigFromState()
		})

		if a.layaEnabled {
			stateLbl := a.t("ai.layaStateNotInstalled")
			dotCol := a.pal.inkFaint
			if a.layaStatus.Running {
				stateLbl = a.t("ai.layaStateRunning")
				dotCol = gui.Hex("#10B981")
			} else if a.layaStatus.BinaryInstalled {
				stateLbl = a.t("ai.layaStateInstalled")
				dotCol = a.pal.inkSecondary
			}
			gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Box(c).Size(8, 8).Radius(radiusPill).Background(dotCol)
				gui.Text(c, stateLbl).FontSize(12).Bold().TextColor(a.pal.ink).Grow(1)
				a.iconButton(c, "pref-laya-refresh", iconRefresh, a.t("settings.recalc"), false).OnClick(func() {
					a.refreshLayaStatus()
				})
			})
			if len(a.layaStatus.InstalledModels) > 0 {
				gui.Text(c, fmt.Sprintf(a.t("ai.layaModelsInstalled"), strings.Join(a.layaStatus.InstalledModels, ", "))).
					FontSize(11).
					TextColor(a.pal.inkMuted)
			}

			models := []string{"laya:multilingual", "laya", "winnow:e4b"}
			selMod := 0
			for i, m := range models {
				if strings.EqualFold(a.layaModel, m) {
					selMod = i
					break
				}
			}
			gui.Column(c).FillWidth().Gap(6).Children(func() {
				gui.Text(c, a.t("ai.layaModelPreset")).FontSize(12).TextColor(a.pal.inkSecondary)
				a.arcPillTabs(c, "pref-laya-preset", selMod, []string{"Laya 多語系 (CPU)", "Laya EN", "Winnow 4B"}, func(idx int) {
					a.layaModel = models[idx]
					a.syncConfigFromState()
				})
			})

			if a.layaDeployBusy || a.layaDeployText != "" {
				gui.Column(c).FillWidth().Padding(10, 12).Gap(6).Radius(radiusControl).Background(a.pal.surfaceRaised).Children(func() {
					gui.Text(c, a.layaDeployText).FontSize(12).TextColor(a.pal.ink)
					if a.layaDeployBusy {
						pct := a.layaDeployPct
						if pct < 0.04 {
							pct = 0.04
						} else if pct > 1 {
							pct = 1
						}
						gui.Box(c).FillWidth().Height(4).Radius(radiusPill).Background(a.pal.surfaceMuted).Children(func() {
							gui.Box(c).FillHeight().WidthPercent(float32(pct * 100)).Radius(radiusPill).Background(a.pal.controlOn)
						})
					}
				})
			}

			gui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(gui.Center).Children(func() {
				a.arcButton(c, "pref-laya-deploy", a.t("ai.layaDeploy"), true).
					Disabled(a.layaDeployBusy).
					OnClick(func() { a.deployLayaInApp() })
				if a.layaStatus.Running {
					a.arcButton(c, "pref-laya-stop", a.t("ai.layaStop"), false).
						OnClick(func() { a.stopLayaInApp() })
				} else if a.layaStatus.BinaryInstalled {
					a.arcButton(c, "pref-laya-start", a.t("ai.layaStart"), false).
						Disabled(a.layaDeployBusy).
						OnClick(func() { a.startLayaInApp() })
				}
				a.toolButton(c, "pref-laya-test", iconSparkle, a.t("laya.test"), false).OnClick(func() {
					a.syncConfigFromState()
					a.testLayaConnection()
				})
			})

			gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("laya.url")).FontSize(12).TextColor(a.pal.inkSecondary).Width(130)
				gui.TextInput(c.Key("pref-laya-url"), &a.layaURL).
					Placeholder("http://127.0.0.1:11435").
					Grow(1).
					OnSubmit(func() { a.syncConfigFromState() })
			})
		}
	})
}

func (a *App) settingsDatabaseTab(c *gui.Context) {
	services := c.Services()
	rootPath := a.cfg.LibraryPath
	if a.lib != nil {
		rootPath = a.lib.DB.Root()
	}

	// Clean compact 4-tile KPI summary row so library scale is obvious at a glance.
	st := a.dbStats
	totalBoards := st.TotalBoards + len(a.standaloneBoards)
	gui.Row(c).FillWidth().Gap(8).Children(func() {
		kpiTile := func(label string, count int) {
			tileBg := a.pal.surface
			if a.glassActive {
				if a.dark {
					tileBg = gui.RGBA(255, 255, 255, 0.04)
				} else {
					tileBg = gui.RGBA(0, 0, 0, 0.03)
				}
			}
			gui.Row(c).
				Grow(1).
				Padding(9, 12).
				Gap(6).
				Radius(radiusControl).
				Background(tileBg).
				Border(1, a.chromeHairline()).
				AlignItems(gui.Center).
				Children(func() {
					gui.Text(c, fmt.Sprintf("%d", count)).FontSize(15).Bold().TextColor(a.pal.ink)
					gui.Text(c, label).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
				})
		}
		kpiTile(a.t("settings.kpiAssets"), a.allAssetsCount)
		kpiTile(a.t("settings.kpiTags"), len(a.tags))
		kpiTile(a.t("settings.kpiProjects"), len(a.projectList))
		kpiTile(a.t("settings.kpiBoards"), totalBoards)
	})

	// Library Location card.
	a.settingsGroup(c, a.t("settings.dbLocation"), func() {
		gui.Box(c).FillWidth().Padding(8, 12).Radius(radiusControl).Background(a.pal.surfaceMuted).Children(func() {
			gui.Text(c, rootPath).FontSize(12).TextColor(a.pal.ink).LineHeight(1.35)
		})
		gui.Row(c).Gap(8).Wrap().Children(func() {
			if rootPath != "" {
				a.arcButton(c, "pref-db-open", a.t("settings.openFolder"), false).OnClick(func() {
					_ = openLocalFile(rootPath)
				})
				a.arcButton(c, "pref-db-copy", a.t("common.copyPath"), false).OnClick(func() {
					services.WriteClipboard(rootPath)
					a.status = a.t("common.copied")
				})
			}
			a.arcButton(c, "pref-db-switch", a.t("shell.switchLib"), true).OnClick(func() {
				a.settingsOpen = false
				a.chooseLibrary()
			})
		})
	})

	// Backup & Safety card (above the fold for instant access).
	a.settingsGroup(c, a.t("settings.dbBackup"), func() {
		bakLabel := a.t("settings.backupNone")
		if a.dbStats.HasBackup {
			bakLabel = fmt.Sprintf("%s · %s", formatBytes(a.dbStats.BackupBytes), a.dbStats.BackupTime.Local().Format("2006-01-02 15:04"))
		}
		a.settingsKVRow(c, a.t("settings.backupStatus"), bakLabel)
		a.settingsSwitchRow(c, "pref-bak-del", a.t("settings.backupBeforeDel"), &a.backupBeforeDelete, func() {
			a.syncConfigFromState()
		})
		gui.Row(c).Gap(8).Wrap().Children(func() {
			a.arcButton(c, "pref-db-bak-now", a.t("settings.backupNow"), true).OnClick(func() {
				a.backupDatabaseNow()
			})
			a.arcButton(c, "pref-db-export-bak", a.t("settings.exportBackup"), false).OnClick(func() {
				a.exportDatabaseBackup()
			})
		})
	})

	// Storage & Statistics card.
	a.settingsGroup(c, a.t("settings.dbStorage"), func() {
		a.settingsKVRow(c, a.t("settings.dbFile"), fmt.Sprintf("%s  (raglex.db · v%s)", formatBytes(st.DBBytes), model.SchemaVersion))
		a.settingsKVRow(c, a.t("settings.dbFilesDir"), fmt.Sprintf("%s  (%d)", formatBytes(st.FilesBytes), st.FilesCount))
		a.settingsKVRow(c, a.t("settings.dbThumbsDir"), fmt.Sprintf("%s  (%d)", formatBytes(st.ThumbsBytes), st.ThumbsCount))
		gui.Row(c).Justify(gui.End).Children(func() {
			a.toolButton(c, "pref-db-recalc", iconRefresh, a.t("settings.recalc"), false).OnClick(func() {
				a.refreshDBStats()
			})
		})
	})

	// Maintenance & Repair card.
	a.settingsGroup(c, a.t("settings.dbMaint"), func() {
		ffText := a.t("settings.ffmpegMissing")
		if a.ffmpeg {
			ffText = a.t("settings.ffmpegOK")
		}
		a.settingsKVRow(c, a.t("settings.ffmpegStatus"), ffText)
		gui.Row(c).Gap(8).Wrap().Children(func() {
			relinkLbl := a.t("settings.relinkMissing")
			if a.missingCount > 0 {
				relinkLbl = fmt.Sprintf("%s (%d)", relinkLbl, a.missingCount)
			}
			a.arcButton(c, "pref-db-relink", relinkLbl, false).OnClick(func() {
				a.relinkMissing(true)
			})
			a.arcButton(c, "pref-db-thumbs", a.t("settings.rebuildThumbs"), false).OnClick(func() {
				a.rebuildAllThumbnails()
			})
			a.arcButton(c, "pref-db-clean-thumbs", a.t("settings.cleanThumbs"), false).OnClick(func() {
				a.cleanOrphanThumbs()
			})
		})
	})
}

func (a *App) settingsImportTab(c *gui.Context) {
	modes := []string{
		a.t("library.mode.index"),
		a.t("library.mode.symlink"),
		a.t("library.mode.move"),
		a.t("library.mode.copy"),
	}
	modeDescs := []string{
		a.t("settings.modeDesc.index"),
		a.t("settings.modeDesc.symlink"),
		a.t("settings.modeDesc.move"),
		a.t("settings.modeDesc.copy"),
	}

	// Default storage mode card with highlighted plain-language explanation box.
	a.settingsGroup(c, a.t("settings.importMode"), func() {
		a.arcPillTabs(c, "pref-storage-mode", a.storageMode, modes, func(idx int) {
			a.setStorageMode(idx)
		})
		desc := modeDescs[0]
		if a.storageMode >= 0 && a.storageMode < len(modeDescs) {
			desc = modeDescs[a.storageMode]
		}
		gui.Box(c).FillWidth().Padding(10, 12).Radius(radiusControl).Background(a.pal.surfaceMuted).Children(func() {
			gui.Text(c, desc).FontSize(12).TextColor(a.pal.ink).LineHeight(1.45)
		})
	})

	// Import behavior switches with clear helper descriptions.
	a.settingsGroup(c, a.t("settings.importBehavior"), func() {
		a.settingsSwitchRowDesc(c, "pref-imp-autoselect", a.t("settings.autoSelect"), a.t("settings.impAutoSelectDesc"), &a.autoSelectImported, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowDesc(c, "pref-imp-autotag", a.t("settings.autoTag"), a.t("settings.impAutoTagDesc"), &a.autoTagOnImport, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowDesc(c, "pref-imp-hidden", a.t("settings.ignoreHidden"), a.t("settings.impHiddenDesc"), &a.ignoreHiddenOnImport, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowDesc(c, "pref-imp-recursive", a.t("settings.importRecursive"), a.t("settings.impRecursiveDesc"), &a.importRecursive, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowDesc(c, "pref-imp-dup", a.t("settings.notifyDup"), a.t("settings.impDupDesc"), &a.notifyDuplicates, func() {
			a.syncConfigFromState()
		})
	})

	// Formats & deduplication info card.
	a.settingsGroup(c, a.t("settings.importFormats"), func() {
		gui.Text(c, a.t("settings.importFormatsDesc")).FontSize(12).TextColor(a.pal.inkSecondary).LineHeight(1.5)
	})
}

func (a *App) settingsSharingTab(c *gui.Context) {
	services := c.Services()

	// Device & defaults card.
	a.settingsGroup(c, a.t("settings.shareDevice"), func() {
		gui.Column(c).FillWidth().Gap(6).Children(func() {
			gui.Text(c, a.t("settings.deviceName")).FontSize(13)
			gui.TextInput(c.Key("pref-device-name"), &a.deviceName).
				Placeholder(a.t("settings.deviceNameHint")).
				FillWidth().
				OnChange(func() { a.syncConfigFromState() })
			gui.Text(c, a.t("settings.deviceNameHint")).FontSize(11).TextColor(a.pal.inkMuted)
		})
		a.settingsSwitchRow(c, "pref-share-files", a.t("settings.shareFilesDefault"), &a.shareFiles, func() {
			a.syncConfigFromState()
		})
	})

	// Signaling server & LAN status card.
	a.settingsGroup(c, a.t("settings.shareSignal"), func() {
		lanText := a.t("settings.lanOff")
		if node := a.shareNode(); node != nil && node.DiscoveryOK() {
			lanText = a.t("settings.lanOK")
		}
		a.settingsKVRow(c, a.t("settings.lanStatus"), lanText)
		gui.Column(c).FillWidth().Gap(6).Children(func() {
			gui.Text(c, a.t("share.signalURL")).FontSize(13)
			gui.TextInput(c.Key("pref-signal-url"), &a.signalURL).
				Placeholder("http://127.0.0.1:8787").
				FillWidth().
				OnChange(func() {
					a.signalURL = strings.TrimSpace(a.signalURL)
					a.syncConfigFromState()
				})
		})
		gui.Text(c, a.t("settings.shareSignalDesc")).FontSize(12).TextColor(a.pal.inkSecondary).LineHeight(1.45)
		gui.Row(c).Padding(4, 0, 0, 0).Children(func() {
			a.arcButton(c, "pref-recv-code", a.t("settings.receiveByCodeBtn"), true).OnClick(func() {
				a.settingsOpen = false
				a.pairRecvOpen = true
			})
		})
	})

	// Stage 15 & 16: Built-in Server (Web Gallery, Companion API, MCP, Signaling).
	a.settingsGroup(c, a.t("server.section"), func() {
		a.settingsSwitchRow(c, "pref-srv-run", a.t("server.enable"), &a.serverRunning, func() {
			a.toggleBuiltInServer()
		})
		gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("server.port")).FontSize(12).TextColor(a.pal.inkSecondary).Width(130)
			gui.TextInput(c.Key("pref-srv-port"), &a.serverPortStr).
				Placeholder("41595").
				Width(96).
				OnSubmit(func() { a.syncConfigFromState() })
			gui.Spacer(c)
			a.toolButton(c, "pref-srv-mcp", iconCopy, a.t("server.copyMCP"), false).OnClick(func() {
				services.WriteClipboard(a.buildMCPConfigJSON())
				a.notify(a.t("server.mcpCopied"))
			})
		})
		a.settingsSwitchRow(c, "pref-srv-lan", a.t("server.bindAll"), &a.serverBindAll, func() {
			a.syncConfigFromState()
			if a.localSrv != nil && a.localSrv.Running() {
				a.stopBuiltInServer()
				a.startBuiltInServer()
			}
		})
		a.settingsSwitchRow(c, "pref-srv-auto", a.t("server.autoStart"), &a.serverAutoStart, func() {
			a.syncConfigFromState()
		})
		gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("server.token")).FontSize(12).TextColor(a.pal.inkSecondary).Width(130)
			gui.Text(c, a.ensureServerToken()).FontSize(11).TextColor(a.pal.inkMuted).Grow(1).SingleLine()
			a.toolButton(c, "pref-srv-tok", iconCopy, a.t("server.copyToken"), false).OnClick(func() {
				services.WriteClipboard(a.ensureServerToken())
				a.notify(a.t("common.copied"))
			})
		})
		if a.localSrv != nil && a.localSrv.Running() {
			localURL := a.localSrv.LocalURL()
			lanURL := a.localSrv.LANURL()
			gui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(gui.Center).Children(func() {
				a.arcButton(c, "pref-srv-open", a.t("server.openWeb"), true).OnClick(func() {
					services.OpenURL(localURL)
				})
				if lanURL != "" && a.serverBindAll {
					a.toolButton(c, "pref-srv-lan-copy", iconLink, a.t("server.copyLAN")+" ("+lanURL+")", false).OnClick(func() {
						services.WriteClipboard(lanURL)
						a.notify(a.t("common.copied"))
					})
				}
			})
		}
	})
}

func (a *App) settingsPluginsTab(c *gui.Context) {
	a.settingsGroup(c, a.t("plugins.section"), func() {
		gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
			if a.plugins != nil {
				gui.Text(c, a.plugins.Dir()).FontSize(11).TextColor(a.pal.inkMuted).Grow(1).SingleLine()
				a.toolButton(c, "plug-open-dir", iconFolder, a.t("plugins.openDir"), false).OnClick(func() {
					_ = openLocalFile(a.plugins.Dir())
				})
				a.toolButton(c, "plug-reload", iconRefresh, a.t("plugins.reload"), false).OnClick(func() {
					_ = a.plugins.Reload()
					a.notify(fmt.Sprintf(a.t("plugins.reloaded"), len(a.plugins.List())))
				})
			}
		})
		if a.plugins == nil || len(a.plugins.List()) == 0 {
			gui.Text(c, a.t("plugins.empty")).FontSize(12).TextColor(a.pal.inkSecondary).LineHeight(1.55)
			return
		}
		for _, inst := range a.plugins.List() {
			p := inst
			enabled := p.Enabled
			gui.Row(c.Key("plug-row-"+p.Manifest.ID)).
				FillWidth().
				Padding(10, 12).
				Gap(10).
				Radius(radiusControl).
				Background(a.pal.surfaceRaised).
				AlignItems(gui.Center).
				Children(func() {
					gui.Column(c).Grow(1).Gap(3).Children(func() {
						gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
							gui.Text(c, p.Manifest.Name).FontSize(13).Bold().TextColor(a.pal.ink)
							if p.Manifest.Version != "" {
								gui.Text(c, "v"+p.Manifest.Version).FontSize(10).TextColor(a.pal.inkMuted)
							}
						})
						if p.Manifest.Description != "" {
							gui.Text(c, p.Manifest.Description).FontSize(11).TextColor(a.pal.inkSecondary).LineHeight(1.4)
						}
						if len(p.Manifest.Hooks) > 0 {
							gui.Text(c, strings.Join(p.Manifest.Hooks, " · ")).FontSize(10).TextColor(a.pal.inkMuted)
						}
					})
					a.arcSwitch(c, "plug-en-"+p.Manifest.ID, &enabled, func() {
						a.plugins.SetEnabled(p.Manifest.ID, enabled)
					})
				})
		}
	})
}

func (a *App) settingsKVRow(c *gui.Context, label, value string) {
	gui.Row(c).FillWidth().Gap(12).AlignItems(gui.Center).Children(func() {
		gui.Text(c, label).Width(156).FontSize(12).TextColor(a.pal.inkSecondary).SingleLine()
		gui.Text(c, value).Grow(1).FontSize(12).Bold().TextColor(a.pal.ink).SingleLine()
	})
}

func (a *App) themeColorRow(c *gui.Context, key, label string, val *string, preview gui.Color, swatches []string) {
	gui.Row(c).FillWidth().Gap(10).AlignItems(gui.Center).Children(func() {
		gui.Text(c, label).Width(112).FontSize(12).TextColor(a.pal.inkSecondary).SingleLine()
		gui.Box(c).Size(22, 22).Radius(6).Background(preview).Border(1, a.pal.surfaceActive)
		gui.TextInput(c.Key(key), val).
			Placeholder(colorHex(preview)).
			Width(96).
			OnChange(func() {
				a.themePreset = "custom"
				a.syncConfigFromState()
			})
		gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
			for i, sw := range swatches {
				hex := sw
				if col, ok := parseHexColor(hex); ok {
					dot := gui.ButtonBase(c.Key(fmt.Sprintf("%s-sw-%d", key, i))).
						Size(18, 18).
						Radius(radiusPill).
						Background(col).
						Border(1, a.pal.surfaceActive).
						Tooltip(hex).
						OnClick(func() {
							*val = hex
							a.themePreset = "custom"
							a.syncConfigFromState()
						})
					_ = dot
				}
			}
		})
	})
}

func colorHex(c gui.Color) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}
