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
	"raglex/internal/plugin"
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
			// Left category sidebar separated by a crisp 1px hairline.
			navBg := a.pal.surface
			if a.glassActive {
				if a.dark {
					navBg = gui.Hex("#141417")
				} else {
					navBg = gui.Hex("#F5F5F7")
				}
			}
			gui.Column(c).
				Width(188).
				FillHeight().
				Background(navBg).
				BorderWidth(0, 1, 0, 0).
				BorderColor(a.chromeHairline()).
				Padding(18, 10, 14, 10).
				Gap(12).
				Children(func() {
					gui.Text(c, a.t("settings.title")).FontSize(14).Bold().Padding(0, 10)

					navTrack := gui.Column(c.Key("settings-nav-track")).FillWidth().Gap(2)
					var activeBtn gui.Element
					var hasActive bool
					navTrack.Children(func() {
						for _, tab := range tabs {
							tb := tab
							active := a.settingsTab == tb.id
							btn := gui.ButtonBase(c.Key(fmt.Sprintf("stab-%d", tb.id))).
								FillWidth().
								Padding(8, 10).
								Gap(10).
								Radius(8).
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
									hbg = a.settingsRowHoverBg()
								}
								btn.Background(btn.AnimateColor("hbg", hbg, animFast))
							}
							btn.Children(func() {
								gui.Icon(c, tb.icon).Size(15, 15).TextColor(fg)
								gui.Text(c, tb.label).FontSize(13).TextColor(fg).Grow(1).SingleLine()
							})
							if active {
								activeBtn = btn
								hasActive = true
							}
						}
					})
					attachGlidingPill(navTrack, activeBtn, hasActive, 8, a.pal.pillActive, a.pal.shadowResting)

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

			// Right content area: unified flat list layout matching our context menu aesthetic.
			rightBg := a.popoverSurface()
			gui.Column(c).Grow(1).FillHeight().Background(rightBg).Children(func() {
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
					Padding(16, 20, 12, 20).
					Gap(12).
					AlignItems(gui.Center).
					BorderWidth(0, 0, 1, 0).
					BorderColor(a.chromeHairline()).
					Children(func() {
						gui.Column(c).Grow(1).Gap(2).Children(func() {
							gui.Text(c, activeTitle).FontSize(15).Bold()
							gui.Text(c, activeSub).FontSize(11.5).TextColor(a.pal.inkMuted).SingleLine()
						})
						a.iconButton(c, "settings-close-x", iconX, a.t("common.close"), false).OnClick(func() {
							a.settingsOpen = false
						})
					})

				// Scrollable flat divider-separated list (zero boxed module cards).
				gui.Scroll(c.Key("settings-scroll")).Grow(1).FillWidth().Children(func() {
					dirY := float32(8)
					if a.settingsTab < a.settingsPrevTab {
						dirY = -8
					}
					body := gui.Column(c.Key(fmt.Sprintf("settings-body-%d", a.settingsTab))).
						FillWidth().
						Padding(10, 14, 18, 14).
						Gap(2)
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
					Padding(10, 20).
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

func (a *App) settingsRowHoverBg() gui.Color {
	if a.dark {
		return gui.RGBA(255, 255, 255, 0.045)
	}
	return gui.RGBA(0, 0, 0, 0.035)
}

func (a *App) settingsDivider(c *gui.Context) {
	gui.Box(c).FillWidth().Height(1).Margin(8, 10).Background(a.chromeHairline())
}

func (a *App) helpTip(c *gui.Context, tip string) {
	if strings.TrimSpace(tip) == "" {
		return
	}
	gui.Box(c).
		Size(15, 15).
		Radius(radiusPill).
		Background(a.pal.surfaceMuted).
		Center().
		Tooltip(tip).
		Children(func() {
			gui.Text(c, "?").FontSize(9.5).Bold().TextColor(a.pal.inkMuted).SingleLine()
		})
}

// settingsSection renders a flat, borderless section separated by crisp 1px
// hairlines (matching the clean list grammar of our right-click context menu).
func (a *App) settingsSection(c *gui.Context, title, tip string, topDivider bool, content func()) {
	if topDivider {
		a.settingsDivider(c)
	}
	gui.Row(c).FillWidth().Padding(4, 10, 4, 10).Gap(6).AlignItems(gui.Center).Children(func() {
		gui.Text(c, title).FontSize(11).Bold().TextColor(a.pal.inkMuted).SingleLine()
		a.helpTip(c, tip)
	})
	content()
}

// settingsItemRow renders a single flat, subtly hoverable setting row:
// [15px icon]  [<=8-char label] [?]        [right-aligned control / value]
func (a *App) settingsItemRow(c *gui.Context, key string, ic *gui.SVG, label, tip string, right func()) {
	row := gui.Row(c.Key("srow-"+key)).
		FillWidth().
		Padding(7, 10).
		Gap(10).
		Radius(8).
		AlignItems(gui.Center)
	if row.Hovered() {
		row.Background(a.settingsRowHoverBg())
	}
	row.Children(func() {
		gui.Row(c).Grow(1).Gap(10).AlignItems(gui.Center).Children(func() {
			if ic != nil {
				gui.Icon(c, ic).Size(15, 15).TextColor(a.pal.inkSecondary)
			}
			gui.Text(c, label).FontSize(13).TextColor(a.pal.ink).SingleLine()
			a.helpTip(c, tip)
		})
		if right != nil {
			right()
		}
	})
}

func (a *App) settingsGroup(c *gui.Context, title string, content func()) {
	a.settingsSection(c, title, "", false, content)
}

func (a *App) settingsSwitchRow(c *gui.Context, key, label string, val *bool, onChange func()) {
	a.settingsSwitchRowTip(c, key, nil, label, "", val, onChange)
}

func (a *App) settingsIconSwitchRow(c *gui.Context, key string, ic *gui.SVG, label string, val *bool, onChange func()) {
	a.settingsSwitchRowTip(c, key, ic, label, "", val, onChange)
}

func (a *App) settingsSwitchRowTip(c *gui.Context, key string, ic *gui.SVG, label, tip string, val *bool, onChange func()) {
	a.settingsItemRow(c, key, ic, label, tip, func() {
		a.arcSwitch(c, key, val, onChange)
	})
}

func (a *App) settingsGeneralTab(c *gui.Context) {
	// 1. 外觀風格
	a.settingsSection(c, a.t("pref.grpLook"), "", false, func() {
		a.settingsItemRow(c, "theme", iconMoon, a.t("pref.theme"), "", func() {
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
		a.settingsItemRow(c, "preset-head", iconPalette, a.t("pref.preset"), "", func() {
			a.toolButton(c, "pref-theme-reset", iconRefresh, a.t("settings.resetTheme"), false).OnClick(func() {
				a.resetCustomTheme()
			})
		})
		gui.Row(c).FillWidth().Padding(0, 10, 4, 35).Children(func() {
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

		a.settingsSwitchRowTip(c, "pref-glass-mode", iconLayers, a.t("pref.glass"), a.t("settings.glassDesc"), &a.glassMode, func() {
			a.syncWindowVibrancy()
			a.syncConfigFromState()
		})
		if a.glassMode {
			a.settingsItemRow(c, "glass-density", iconContrast, a.t("pref.glassDensity"), "", func() {
				gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
					gui.Text(c, fmt.Sprintf("%d%%", int(a.glassOpacity*100+0.5))).FontSize(12).TextColor(a.pal.inkMuted)
					gui.Slider(c.Key("pref-glass-opacity"), &a.glassOpacity, 0.30, 0.96).Width(156).OnChange(func() {
						a.syncConfigFromState()
					})
				})
			})
		}

		if a.themePreset == "custom" {
			gui.Column(c).FillWidth().Padding(4, 10, 4, 35).Gap(6).Children(func() {
				a.themeColorRow(c, "pref-col-canvas", a.t("settings.colorCanvas"), &a.customCanvas, a.pal.canvas, []string{"#111111", "#18191C", "#090A0D", "#FFFFFF", "#F6F3EE"})
				a.themeColorRow(c, "pref-col-surface", a.t("settings.colorSurface"), &a.customSurface, a.pal.surface, []string{"#18181B", "#222428", "#12141A", "#F4F4F5", "#EDE8DF"})
				a.themeColorRow(c, "pref-col-accent", a.t("settings.colorAccent"), &a.customAccent, a.pal.controlOn, []string{"#EDEDED", "#18181B", "#FF5C33", "#E11D48", "#10B981"})
				a.themeColorRow(c, "pref-col-text", a.t("settings.colorText"), &a.customText, a.pal.ink, []string{"#FAFAFA", "#E4E4E7", "#18181B", "#1F1B17"})
			})
		}

		a.settingsItemRow(c, "lang", iconGlobe, a.t("pref.lang"), "", func() {
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

	// 2. 畫廊顯示
	a.settingsSection(c, a.t("pref.grpGrid"), "", true, func() {
		a.settingsItemRow(c, "layout", iconLibrary, a.t("pref.layout"), "", func() {
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

		a.settingsItemRow(c, "sort", iconSort, a.t("pref.sort"), "", func() {
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

		a.settingsItemRow(c, "card-size", iconMaximize, a.t("pref.cardSize"), "", func() {
			gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
				gui.Text(c, fmt.Sprintf("%d px", int(a.cardSize))).FontSize(12).TextColor(a.pal.inkMuted)
				gui.Slider(c.Key("pref-card-size"), &a.cardSize, 128, 300).Width(156).OnChange(func() {
					a.syncConfigFromState()
				})
			})
		})

		a.settingsIconSwitchRow(c, "pref-sb-counts", iconPanelLeft, a.t("pref.sbCounts"), &a.showSidebarCounts, func() {
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-tag-colors", iconTag, a.t("pref.tagColors"), &a.showTagColors, func() {
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-card-caption", iconType, a.t("pref.cardTitle"), &a.showCardCaption, func() {
			a.syncConfigFromState()
		})
	})

	// 3. 引導與更新
	services := c.Services()
	a.settingsSection(c, a.t("pref.grpUpdate"), "", true, func() {
		a.settingsItemRow(c, "tour", iconHelp, a.t("pref.tour"), "", func() {
			a.toolButton(c, "pref-reopen-ob", iconExternal, a.t("onboarding.reopen"), false).OnClick(func() {
				a.settingsOpen = false
				a.openOnboarding()
			})
		})
		a.settingsItemRow(c, "upd-ver", iconClock, a.t("update.current"), "", func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, "v"+updater.Version).FontSize(12).TextColor(a.pal.inkMuted)
				if a.updateInstalled {
					a.arcButton(c, "pref-upd-restart", a.t("update.restartBtn"), true).OnClick(func() {
						a.restartToApplyUpdate()
					})
				} else if a.updateAvailable && a.updateRelease != nil && a.updateRelease.DownloadURL != "" {
					a.arcButton(c, "pref-upd-install", a.t("update.updateAndRestart"), true).
						Disabled(a.updateApplying).
						OnClick(func() {
							a.updateAutoRestart = true
							a.updatePromptOpen = true
							a.applyUpdateNow()
						})
				} else {
					a.toolButton(c, "pref-upd-check", iconRefresh, a.t("update.checkBtn"), false).
						Disabled(a.updateChecking || a.updateApplying).
						OnClick(func() { a.checkForUpdatesNow() })
				}
				if a.updateRelease != nil && a.updateRelease.HTMLURL != "" {
					relURL := a.updateRelease.HTMLURL
					a.iconButton(c, "pref-upd-notes", iconExternal, a.t("update.openRelease"), false).OnClick(func() {
						services.OpenURL(relURL)
					})
				}
			})
		})
		if a.updateMsg != "" {
			msgCol := a.pal.inkMuted
			if a.updateErr != "" {
				msgCol = a.pal.danger
			}
			gui.Text(c, a.updateMsg).FontSize(11.5).TextColor(msgCol).Padding(0, 10, 4, 35)
		}
		a.settingsSwitchRowTip(c, "pref-upd-auto", iconRefresh, a.t("pref.autoUpd"), a.t("pref.autoUpdTip"), &a.autoCheckUpdates, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowTip(c, "pref-upd-beta", iconSparkle, a.t("pref.betaUpd"), a.t("pref.betaUpdTip"), &a.updateBetaChannel, func() {
			a.syncConfigFromState()
		})
	})
}

func (a *App) settingsAITab(c *gui.Context) {
	services := c.Services()

	// 1. AI 助手連線 (MCP)
	a.settingsSection(c, a.t("pref.grpMCP"), a.t("pref.mcpTip"), false, func() {
		dotCol := a.pal.inkFaint
		statusText := a.t("pref.mcpOff")
		if a.localSrv != nil && a.localSrv.Running() {
			dotCol = gui.Hex("#10B981")
			statusText = a.t("pref.mcpReady")
		}
		a.settingsItemRow(c, "mcp-run", iconLink, statusText, "", func() {
			gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
				gui.Box(c).Size(7, 7).Radius(radiusPill).Background(dotCol)
				a.arcSwitch(c, "pref-mcp-srv-run", &a.serverRunning, func() {
					a.toggleBuiltInServer()
				})
			})
		})
		a.settingsItemRow(c, "mcp-cfg", iconCopy, a.t("pref.grpMCP"), "", func() {
			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				a.arcButton(c, "pref-ai-copy-mcp", a.t("pref.copyCfg"), true).OnClick(func() {
					services.WriteClipboard(a.buildMCPConfigJSON())
					a.notify(a.t("server.mcpCopied"))
				})
				a.toolButton(c, "pref-ai-copy-tok", iconLock, a.t("pref.advToken"), false).OnClick(func() {
					services.WriteClipboard(a.ensureServerToken())
					a.notify(a.t("common.copied"))
				})
			})
		})
	})

	// 2. 以圖搜圖
	a.settingsSection(c, a.t("pref.grpSimilar"), a.t("ai.embedDesc"), true, func() {
		a.settingsIconSwitchRow(c, "pref-ai-embed-en", iconEye, a.t("pref.simEnable"), &a.embedEnabled, func() {
			a.syncConfigFromState()
		})
		a.settingsItemRow(c, "embed-idx", iconRefresh, fmt.Sprintf(a.t("ai.embedStatus"), a.aiIndexedCount, a.allAssetsCount), "", func() {
			a.toolButton(c, "pref-ai-reindex", iconRefresh, a.t("ai.reindex"), false).
				Disabled(a.aiIndexBusy || !a.embedEnabled).
				OnClick(func() { a.reindexVisualEmbeddings() })
		})
	})

	// 3. AI 自動標籤
	a.settingsSection(c, a.t("pref.grpAutoTag"), a.t("ai.layaBuiltInDesc"), true, func() {
		a.settingsIconSwitchRow(c, "pref-ai-laya-en", iconSparkle, a.t("pref.layaEnable"), &a.layaEnabled, func() {
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
			a.settingsItemRow(c, "laya-state", iconCheck, stateLbl, "", func() {
				gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
					gui.Box(c).Size(7, 7).Radius(radiusPill).Background(dotCol)
					a.arcButton(c, "pref-laya-deploy", a.t("ai.layaDeploy"), true).
						Disabled(a.layaDeployBusy).
						OnClick(func() { a.deployLayaInApp() })
					if a.layaStatus.Running {
						a.toolButton(c, "pref-laya-stop", nil, a.t("ai.layaStop"), false).
							OnClick(func() { a.stopLayaInApp() })
					} else if a.layaStatus.BinaryInstalled {
						a.toolButton(c, "pref-laya-start", nil, a.t("ai.layaStart"), false).
							Disabled(a.layaDeployBusy).
							OnClick(func() { a.startLayaInApp() })
					}
					a.iconButton(c, "pref-laya-refresh", iconRefresh, a.t("settings.recalc"), false).OnClick(func() {
						a.refreshLayaStatus()
					})
				})
			})

			if a.layaDeployBusy || a.layaDeployText != "" {
				gui.Column(c).FillWidth().Padding(4, 10, 6, 35).Gap(6).Children(func() {
					gui.Text(c, a.layaDeployText).FontSize(11.5).TextColor(a.pal.inkSecondary)
					if a.layaDeployBusy {
						pct := a.layaDeployPct
						if pct < 0.04 {
							pct = 0.04
						} else if pct > 1 {
							pct = 1
						}
						gui.Box(c).FillWidth().Height(3).Radius(radiusPill).Background(a.pal.surfaceActive).Children(func() {
							gui.Box(c).FillHeight().WidthPercent(float32(pct * 100)).Radius(radiusPill).Background(a.pal.controlOn)
						})
					}
				})
			}

			advBtn := gui.ButtonBase(c.Key("pref-laya-adv")).
				FillWidth().
				Padding(7, 10).
				Gap(10).
				Radius(8).
				AlignItems(gui.Center).
				OnClick(func() {
					a.settingsAdvAIOpen = !a.settingsAdvAIOpen
				})
			if advBtn.Hovered() {
				advBtn.Background(a.settingsRowHoverBg())
			}
			advBtn.Children(func() {
				gui.Icon(c, iconSliders).Size(15, 15).TextColor(a.pal.inkSecondary)
				gui.Text(c, a.t("pref.advModel")).FontSize(13).TextColor(a.pal.ink).Grow(1).SingleLine()
				chev := iconChevronRgt
				if a.settingsAdvAIOpen {
					chev = iconChevronDown
				}
				gui.Icon(c, chev).Size(14, 14).TextColor(a.pal.inkMuted)
			})

			if a.settingsAdvAIOpen {
				models := []string{"laya:multilingual", "laya", "winnow:e4b"}
				selMod := 0
				for i, m := range models {
					if strings.EqualFold(a.layaModel, m) {
						selMod = i
						break
					}
				}
				gui.Row(c).FillWidth().Padding(2, 10, 4, 35).Children(func() {
					a.arcPillTabs(c, "pref-laya-preset", selMod, []string{"Laya 多語系", "Laya EN", "Winnow 4B"}, func(idx int) {
						a.layaModel = models[idx]
						a.syncConfigFromState()
					})
				})
				a.settingsItemRow(c, "laya-url", iconGlobe, a.t("laya.url"), "", func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						gui.TextInput(c.Key("pref-laya-url"), &a.layaURL).
							Placeholder("http://127.0.0.1:11435").
							Width(220).
							OnSubmit(func() { a.syncConfigFromState() })
						a.toolButton(c, "pref-laya-test", iconSparkle, a.t("laya.test"), false).OnClick(func() {
							a.syncConfigFromState()
							a.testLayaConnection()
						})
					})
				})
			}
		}
	})
}

func (a *App) settingsDatabaseTab(c *gui.Context) {
	services := c.Services()
	rootPath := a.cfg.LibraryPath
	if a.lib != nil {
		rootPath = a.lib.DB.Root()
	}

	st := a.dbStats
	totalBoards := st.TotalBoards + len(a.standaloneBoards)

	// Flat inline overview strip (zero boxy tiles).
	gui.Row(c).FillWidth().Padding(4, 10, 6, 10).Gap(16).AlignItems(gui.Center).Children(func() {
		statItem := func(ic *gui.SVG, label string, count int) {
			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				gui.Icon(c, ic).Size(13, 13).TextColor(a.pal.inkMuted)
				gui.Text(c, fmt.Sprintf("%d", count)).FontSize(13).Bold().TextColor(a.pal.ink)
				gui.Text(c, label).FontSize(11.5).TextColor(a.pal.inkMuted).SingleLine()
			})
		}
		statItem(iconLibrary, a.t("settings.kpiAssets"), a.allAssetsCount)
		statItem(iconTag, a.t("settings.kpiTags"), len(a.tags))
		statItem(iconFolder, a.t("settings.kpiProjects"), len(a.projectList))
		statItem(iconBoard, a.t("settings.kpiBoards"), totalBoards)
	})

	// 1. 資料庫位置
	a.settingsSection(c, a.t("pref.grpDbLoc"), "", true, func() {
		a.settingsItemRow(c, "db-loc", iconFolder, rootPath, "", func() {
			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				if rootPath != "" {
					a.iconButton(c, "pref-db-open", iconExternal, a.t("settings.openFolder"), false).OnClick(func() {
						_ = openLocalFile(rootPath)
					})
					a.iconButton(c, "pref-db-copy", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
						services.WriteClipboard(rootPath)
						a.status = a.t("common.copied")
					})
				}
				a.toolButton(c, "pref-db-switch", iconFolder, a.t("shell.switchLib"), false).OnClick(func() {
					a.settingsOpen = false
					a.chooseLibrary()
				})
			})
		})
	})

	// 2. 備份與還原
	a.settingsSection(c, a.t("pref.grpBackup"), "", true, func() {
		bakLabel := a.t("settings.backupNone")
		if a.dbStats.HasBackup {
			bakLabel = fmt.Sprintf("%s · %s", formatBytes(a.dbStats.BackupBytes), a.dbStats.BackupTime.Local().Format("2006-01-02 15:04"))
		}
		a.settingsKVRow(c, "bak-status", iconClock, a.t("settings.backupStatus"), bakLabel)
		a.settingsSwitchRowTip(c, "pref-bak-del", iconLock, a.t("pref.bakDel"), a.t("pref.bakDelTip"), &a.backupBeforeDelete, func() {
			a.syncConfigFromState()
		})
		a.settingsItemRow(c, "bak-actions", iconDatabase, a.t("pref.grpBackup"), "", func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				a.arcButton(c, "pref-db-bak-now", a.t("settings.backupNow"), true).OnClick(func() {
					a.backupDatabaseNow()
				})
				a.toolButton(c, "pref-db-export-bak", iconExport, a.t("settings.exportBackup"), false).OnClick(func() {
					a.exportDatabaseBackup()
				})
			})
		})
	})

	// 3. 容量與維護
	a.settingsSection(c, a.t("pref.grpMaint"), "", true, func() {
		a.settingsKVRow(c, "db-file", iconDatabase, a.t("pref.dbFile"), fmt.Sprintf("%s  (v%s)", formatBytes(st.DBBytes), model.SchemaVersion))
		a.settingsKVRow(c, "db-files", iconFolder, a.t("pref.dbFiles"), fmt.Sprintf("%s  (%d)", formatBytes(st.FilesBytes), st.FilesCount))
		a.settingsKVRow(c, "db-thumbs", iconLibrary, a.t("pref.dbThumbs"), fmt.Sprintf("%s  (%d)", formatBytes(st.ThumbsBytes), st.ThumbsCount))
		ffText := a.t("settings.ffmpegMissing")
		if a.ffmpeg {
			ffText = a.t("settings.ffmpegOK")
		}
		a.settingsKVRow(c, "db-ff", iconSliders, a.t("pref.ffmpeg"), ffText)
		gui.Row(c).FillWidth().Padding(6, 10).Gap(8).Wrap().Children(func() {
			relinkLbl := a.t("settings.relinkMissing")
			if a.missingCount > 0 {
				relinkLbl = fmt.Sprintf("%s (%d)", relinkLbl, a.missingCount)
			}
			a.toolButton(c, "pref-db-relink", iconLink, relinkLbl, false).OnClick(func() {
				a.relinkMissing(true)
			})
			a.toolButton(c, "pref-db-thumbs", iconRefresh, a.t("settings.rebuildThumbs"), false).OnClick(func() {
				a.rebuildAllThumbnails()
			})
			a.toolButton(c, "pref-db-clean-thumbs", iconTrash, a.t("settings.cleanThumbs"), false).OnClick(func() {
				a.cleanOrphanThumbs()
			})
			a.toolButton(c, "pref-db-recalc", iconRefresh, a.t("settings.recalc"), false).OnClick(func() {
				a.refreshDBStats()
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

	// 1. 檔案存放方式
	a.settingsSection(c, a.t("pref.grpImpMode"), a.t("pref.impModeTip"), false, func() {
		a.settingsItemRow(c, "imp-mode", iconImport, a.t("pref.grpImpMode"), "", func() {
			a.arcPillTabs(c, "pref-storage-mode", a.storageMode, modes, func(idx int) {
				a.setStorageMode(idx)
			})
		})
		desc := modeDescs[0]
		if a.storageMode >= 0 && a.storageMode < len(modeDescs) {
			desc = modeDescs[a.storageMode]
		}
		gui.Text(c, desc).FontSize(11.5).TextColor(a.pal.inkMuted).LineHeight(1.45).Padding(0, 10, 4, 35)
	})

	// 2. 匯入行為
	a.settingsSection(c, a.t("pref.grpImpRules"), "", true, func() {
		a.settingsSwitchRowTip(c, "pref-imp-autoselect", iconCheck, a.t("pref.impSel"), a.t("settings.impAutoSelectDesc"), &a.autoSelectImported, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowTip(c, "pref-imp-autotag", iconTag, a.t("pref.impTag"), a.t("settings.impAutoTagDesc"), &a.autoTagOnImport, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowTip(c, "pref-imp-hidden", iconEye, a.t("pref.impHidden"), a.t("settings.impHiddenDesc"), &a.ignoreHiddenOnImport, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowTip(c, "pref-imp-recursive", iconFolder, a.t("pref.impSub"), a.t("settings.impRecursiveDesc"), &a.importRecursive, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowTip(c, "pref-imp-dup", iconCopy, a.t("pref.impDup"), a.t("settings.impDupDesc"), &a.notifyDuplicates, func() {
			a.syncConfigFromState()
		})
	})
}

func (a *App) settingsSharingTab(c *gui.Context) {
	services := c.Services()

	// 1. 接收朋友專案
	a.settingsSection(c, a.t("pref.grpRecv"), a.t("pref.recvTip"), false, func() {
		lanText := a.t("settings.lanOff")
		if node := a.shareNode(); node != nil && node.DiscoveryOK() {
			lanText = a.t("settings.lanOK")
		}
		a.settingsKVRow(c, "lan-state", iconGlobe, a.t("pref.lanState"), lanText)
		a.settingsItemRow(c, "recv-code", iconLink, a.t("pref.grpRecv"), "", func() {
			a.arcButton(c, "pref-recv-code", a.t("settings.receiveByCodeBtn"), true).OnClick(func() {
				a.settingsOpen = false
				a.pairRecvOpen = true
			})
		})
	})

	// 2. 雲端與手機同步
	a.settingsSection(c, a.t("pref.grpSync"), a.t("pref.syncTip"), true, func() {
		gui.Row(c).FillWidth().Padding(4, 10).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.Icon(c, iconRefresh).Size(15, 15).TextColor(a.pal.inkSecondary)
			gui.TextInput(c.Key("pref-sync-url"), &a.syncServerURL).
				Placeholder("http://127.0.0.1:41595").
				Grow(1).
				OnChange(func() { a.syncConfigFromState() })
			gui.TextInput(c.Key("pref-sync-tok"), &a.syncToken).
				Placeholder(a.t("pref.advToken")).
				Width(128).
				OnChange(func() { a.syncConfigFromState() })
			btnLbl := a.t("sync.now")
			if a.syncBusy {
				btnLbl = a.t("sync.syncing")
			}
			a.arcButton(c, "pref-sync-now", btnLbl, true).
				Disabled(a.syncBusy || strings.TrimSpace(a.syncServerURL) == "").
				OnClick(func() { a.runCloudSyncNow() })
		})
		if a.syncStatusMsg != "" {
			gui.Text(c, a.syncStatusMsg).FontSize(11.5).TextColor(a.pal.inkMuted).Padding(0, 10, 4, 35)
		}
	})

	// 3. 本機分享設定 (+ collapsible 進階連線設定)
	a.settingsSection(c, a.t("pref.grpShareLocal"), "", true, func() {
		a.settingsItemRow(c, "dev-name", iconType, a.t("pref.devName"), a.t("pref.devNameTip"), func() {
			gui.TextInput(c.Key("pref-device-name"), &a.deviceName).
				Placeholder("My Studio PC").
				Width(210).
				OnChange(func() { a.syncConfigFromState() })
		})
		a.settingsSwitchRowTip(c, "pref-share-files", iconFolder, a.t("pref.sendFiles"), a.t("pref.sendFilesTip"), &a.shareFiles, func() {
			a.syncConfigFromState()
		})
		a.settingsSwitchRowTip(c, "pref-srv-run", iconGlobe, a.t("pref.webSrv"), a.t("pref.webSrvTip"), &a.serverRunning, func() {
			a.toggleBuiltInServer()
		})
		if a.localSrv != nil && a.localSrv.Running() {
			localURL := a.localSrv.LocalURL()
			lanURL := a.localSrv.LANURL()
			gui.Row(c).FillWidth().Padding(2, 10, 4, 35).Gap(8).Wrap().AlignItems(gui.Center).Children(func() {
				a.toolButton(c, "pref-srv-open", iconExternal, a.t("server.openWeb"), false).OnClick(func() {
					services.OpenURL(localURL)
				})
				a.toolButton(c, "pref-srv-mobile", iconExternal, a.t("sync.openMobile"), false).OnClick(func() {
					services.OpenURL(localURL + "/mobile")
				})
				if lanURL != "" && a.serverBindAll {
					a.toolButton(c, "pref-srv-lan-copy", iconLink, a.t("server.copyLAN"), false).OnClick(func() {
						services.WriteClipboard(lanURL)
						a.notify(a.t("common.copied"))
					})
				}
			})
		}

		// Collapsible Advanced Network settings (menu-style chevron row).
		advBtn := gui.ButtonBase(c.Key("pref-adv-conn-toggle")).
			FillWidth().
			Padding(7, 10).
			Gap(10).
			Radius(8).
			AlignItems(gui.Center).
			OnClick(func() {
				a.settingsAdvShareOpen = !a.settingsAdvShareOpen
			})
		if advBtn.Hovered() {
			advBtn.Background(a.settingsRowHoverBg())
		}
		advBtn.Children(func() {
			gui.Icon(c, iconSliders).Size(15, 15).TextColor(a.pal.inkSecondary)
			gui.Text(c, a.t("pref.advConn")).FontSize(13).TextColor(a.pal.ink).Grow(1).SingleLine()
			chev := iconChevronRgt
			if a.settingsAdvShareOpen {
				chev = iconChevronDown
			}
			gui.Icon(c, chev).Size(14, 14).TextColor(a.pal.inkMuted)
		})
		if a.settingsAdvShareOpen {
			a.settingsItemRow(c, "adv-relay", iconLink, a.t("pref.advRelay"), a.t("pref.advRelayTip"), func() {
				gui.TextInput(c.Key("pref-signal-url"), &a.signalURL).
					Placeholder("http://127.0.0.1:41595").
					Width(240).
					OnChange(func() {
						a.signalURL = strings.TrimSpace(a.signalURL)
						a.syncConfigFromState()
					})
			})
			a.settingsItemRow(c, "adv-port", iconSliders, a.t("pref.advPort"), "", func() {
				gui.TextInput(c.Key("pref-srv-port"), &a.serverPortStr).
					Placeholder("41595").
					Width(96).
					OnSubmit(func() { a.syncConfigFromState() })
			})
			a.settingsIconSwitchRow(c, "pref-srv-lan", iconGlobe, a.t("pref.advLAN"), &a.serverBindAll, func() {
				a.syncConfigFromState()
				if a.localSrv != nil && a.localSrv.Running() {
					a.stopBuiltInServer()
					a.startBuiltInServer()
				}
			})
			a.settingsIconSwitchRow(c, "pref-srv-auto", iconClock, a.t("pref.advAuto"), &a.serverAutoStart, func() {
				a.syncConfigFromState()
			})
			a.settingsItemRow(c, "adv-token", iconLock, a.t("pref.advToken"), "", func() {
				gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Text(c, a.ensureServerToken()).FontSize(11).TextColor(a.pal.inkMuted).MaxWidth(180).SingleLine()
					a.toolButton(c, "pref-srv-tok", iconCopy, a.t("server.copyToken"), false).OnClick(func() {
						services.WriteClipboard(a.ensureServerToken())
						a.notify(a.t("common.copied"))
					})
				})
			})
		}
	})
}

func (a *App) settingsPluginsTab(c *gui.Context) {
	services := c.Services()
	if !a.pluginMarketLoaded && !a.pluginMarketLoading {
		a.pluginMarket = plugin.BuiltinStarterCatalog()
		a.refreshPluginMarketplace()
	}

	installedList := []plugin.Installed(nil)
	if a.plugins != nil {
		installedList = a.plugins.List()
	}

	// Top flat switcher bar.
	gui.Row(c).FillWidth().Padding(4, 10, 6, 10).Gap(8).AlignItems(gui.Center).Children(func() {
		a.arcPillTabs(c, "plug-subtabs", a.pluginSubTab, []string{
			fmt.Sprintf("%s (%d)", a.t("plugins.tabInstalled"), len(installedList)),
			fmt.Sprintf("%s (%d)", a.t("plugins.tabMarket"), len(a.pluginMarket)),
		}, func(idx int) {
			a.pluginSubTab = idx
		})
		gui.Spacer(c)
		a.toolButton(c, "plug-sdk-guide", iconExternal, a.t("plugins.marketGuide"), false).OnClick(func() {
			services.OpenURL("https://github.com/Batkixni/Raglex/blob/main/docs/PLUGIN_SDK.md")
		})
	})

	if a.pluginSubTab == 0 {
		a.settingsSection(c, a.t("plugins.section"), "", true, func() {
			if a.plugins != nil {
				a.settingsItemRow(c, "plug-dir", iconFolder, a.plugins.Dir(), "", func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						a.iconButton(c, "plug-open-dir", iconExternal, a.t("plugins.openDir"), false).OnClick(func() {
							_ = openLocalFile(a.plugins.Dir())
						})
						a.iconButton(c, "plug-reload", iconRefresh, a.t("plugins.reload"), false).OnClick(func() {
							_ = a.plugins.Reload()
							a.notify(fmt.Sprintf(a.t("plugins.reloaded"), len(a.plugins.List())))
						})
					})
				})
			}
			if len(installedList) == 0 {
				gui.Column(c).FillWidth().Padding(8, 10).Gap(10).Children(func() {
					gui.Text(c, a.t("plugins.empty")).FontSize(12).TextColor(a.pal.inkMuted).LineHeight(1.5)
					gui.Row(c).Children(func() {
						a.arcButton(c, "plug-go-market", a.t("plugins.tabMarket"), true).OnClick(func() {
							a.pluginSubTab = 1
						})
					})
				})
				return
			}
			for i, inst := range installedList {
				p := inst
				enabled := p.Enabled
				if i > 0 {
					gui.Box(c).FillWidth().Height(1).Margin(2, 10).Background(a.chromeHairline())
				}
				row := gui.Row(c.Key("plug-row-"+p.Manifest.ID)).
					FillWidth().
					Padding(8, 10).
					Gap(10).
					Radius(8).
					AlignItems(gui.Center)
				if row.Hovered() {
					row.Background(a.settingsRowHoverBg())
				}
				row.Children(func() {
					gui.Icon(c, iconLayers).Size(15, 15).TextColor(a.pal.inkSecondary)
					gui.Column(c).Grow(1).Gap(2).Children(func() {
						gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
							gui.Text(c, p.Manifest.Name).FontSize(13).Bold().TextColor(a.pal.ink)
							if p.Manifest.Version != "" {
								gui.Text(c, "v"+p.Manifest.Version).FontSize(10.5).TextColor(a.pal.inkMuted)
							}
						})
						if p.Manifest.Description != "" {
							gui.Text(c, p.Manifest.Description).FontSize(11.5).TextColor(a.pal.inkMuted).LineHeight(1.4)
						}
					})
					a.arcSwitch(c, "plug-en-"+p.Manifest.ID, &enabled, func() {
						a.plugins.SetEnabled(p.Manifest.ID, enabled)
					})
					a.iconButton(c, "plug-del-"+p.Manifest.ID, iconTrash, a.t("plugins.uninstall"), false).OnClick(func() {
						a.uninstallPlugin(p.Manifest.ID, p.Manifest.Name)
					})
				})
			}
		})
		return
	}

	// Community & Official Plugin Marketplace (flat divider-separated list).
	a.settingsSection(c, a.t("plugins.tabMarket"), a.t("plugins.submitHint"), true, func() {
		gui.Row(c).FillWidth().Padding(4, 10, 8, 10).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.SearchField(c.Key("plug-market-search"), &a.pluginMarketSearch).
				Placeholder(a.t("plugins.marketSearch")).
				Grow(1)
			a.toolButton(c, "plug-market-sync", iconRefresh, a.t("plugins.marketRefresh"), false).
				Disabled(a.pluginMarketLoading).
				OnClick(func() { a.refreshPluginMarketplace() })
		})

		if a.pluginMarketErr != "" {
			gui.Text(c, a.t("plugins.marketOfflineHint")).FontSize(11).TextColor(a.pal.inkMuted).Padding(0, 10, 6, 10)
		}

		q := strings.ToLower(strings.TrimSpace(a.pluginMarketSearch))
		shown := 0
		for _, item := range a.pluginMarket {
			entry := item
			if q != "" && !strings.Contains(strings.ToLower(entry.Name+" "+entry.Description+" "+entry.Author+" "+entry.ID+" "+entry.Category), q) {
				continue
			}
			if shown > 0 {
				gui.Box(c).FillWidth().Height(1).Margin(2, 10).Background(a.chromeHairline())
			}
			shown++
			instVer, installed := a.plugins.IsInstalled(entry.ID)
			needsUpdate := installed && entry.Version != "" && instVer != entry.Version
			busy := a.pluginMarketBusyID == entry.ID

			row := gui.Row(c.Key("mkt-row-"+entry.ID)).
				FillWidth().
				Padding(9, 10).
				Gap(10).
				Radius(8).
				AlignItems(gui.Center)
			if row.Hovered() {
				row.Background(a.settingsRowHoverBg())
			}
			row.Children(func() {
				gui.Icon(c, iconSparkle).Size(15, 15).TextColor(a.pal.inkSecondary)
				gui.Column(c).Grow(1).Gap(2).Children(func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Wrap().Children(func() {
						gui.Text(c, entry.Name).FontSize(13).Bold().TextColor(a.pal.ink)
						if entry.Official {
							a.pillBadge(c, a.t("plugins.officialBadge"), gui.Hex("#10B981"), gui.RGBA(16, 185, 129, 0.14))
						}
						gui.Text(c, "v"+entry.Version).FontSize(10.5).TextColor(a.pal.inkMuted)
					})
					gui.Text(c, entry.Description).FontSize(11.5).TextColor(a.pal.inkMuted).LineHeight(1.4)
				})

				if busy {
					a.pillBadge(c, a.t("plugins.installing"), a.pal.inkSecondary, a.pal.surfaceRaised)
				} else if installed && !needsUpdate {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						a.pillBadge(c, "✓ "+a.t("plugins.installedBadge"), gui.Hex("#10B981"), gui.RGBA(16, 185, 129, 0.14))
						a.iconButton(c, "mkt-del-"+entry.ID, iconTrash, a.t("plugins.uninstall"), false).OnClick(func() {
							a.uninstallPlugin(entry.ID, entry.Name)
						})
					})
				} else {
					btnLbl := a.t("plugins.install")
					if needsUpdate {
						btnLbl = a.t("plugins.update")
					}
					a.arcButton(c, "mkt-inst-"+entry.ID, btnLbl, true).
						Disabled(a.pluginMarketBusyID != "").
						OnClick(func() { a.installMarketplacePlugin(entry) })
				}
			})
		}
		if shown == 0 {
			gui.Text(c, a.t("plugins.marketEmpty")).FontSize(12).TextColor(a.pal.inkMuted).Padding(8, 10)
		}
	})
}

func (a *App) settingsKVRow(c *gui.Context, key string, ic *gui.SVG, label, value string) {
	a.settingsItemRow(c, key, ic, label, "", func() {
		gui.Text(c, value).FontSize(12).TextColor(a.pal.inkMuted).SingleLine()
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
