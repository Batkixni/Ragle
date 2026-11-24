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

	"raglex/internal/config"
	"raglex/internal/model"
	"raglex/internal/plugin"
	"raglex/internal/updater"
)

const (
	settingsTabGeneral = iota
	settingsTabShortcuts
	settingsTabAccount
	settingsTabSync
	settingsTabDatabase
	settingsTabImport
	settingsTabExtension
	settingsTabSharing
	settingsTabAI
	settingsTabPlugins
)

func (a *App) openSettings(tab int) {
	if tab >= settingsTabGeneral && tab <= settingsTabPlugins {
		a.settingsPrevTab = a.settingsTab
		a.settingsTab = tab
	}
	a.recordingShortcutID = ""
	a.settingsOpen = true
	if a.lib != nil {
		a.aiIndexedCount = a.lib.VectorCount()
	}
	a.refreshLayaStatus()
	a.refreshDBStats()
	if a.settingsTab == settingsTabAccount {
		a.refreshAccountProfile()
	}
	if a.settingsTab == settingsTabSync && a.hasSyncConnection() {
		a.refreshCollabSpaces()
	}
}

func (a *App) setSettingsTab(tab int) {
	if tab == a.settingsTab {
		return
	}
	a.recordingShortcutID = ""
	a.settingsPrevTab = a.settingsTab
	a.settingsTab = tab
	if tab == settingsTabDatabase && !a.dbStatsLoaded {
		a.refreshDBStats()
	}
	if tab == settingsTabAccount {
		a.refreshAccountProfile()
	}
	if tab == settingsTabSync && a.hasSyncConnection() {
		a.refreshCollabSpaces()
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
	a.bitmapFailed = map[string]bool{}
	if a.thumbs != nil {
		a.thumbs.Reset()
	}
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
		a.recordingShortcutID = ""
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
			{settingsTabShortcuts, iconCommand, a.t("settings.tab.shortcuts")},
			{settingsTabAccount, iconUser, a.t("settings.tab.account")},
			{settingsTabSync, iconCloud, a.t("settings.tab.sync")},
			{settingsTabDatabase, iconDatabase, a.t("settings.tab.database")},
			{settingsTabImport, iconImport, a.t("settings.tab.import")},
			{settingsTabExtension, iconGlobe, a.t("settings.tab.extension")},
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
					settingsTabGeneral:   "settings.sub.general",
					settingsTabShortcuts: "settings.sub.shortcuts",
					settingsTabAccount:   "settings.sub.account",
					settingsTabSync:      "settings.sub.sync",
					settingsTabDatabase:  "settings.sub.database",
					settingsTabImport:    "settings.sub.import",
					settingsTabExtension: "settings.sub.extension",
					settingsTabSharing:   "settings.sub.sharing",
					settingsTabAI:        "settings.sub.ai",
					settingsTabPlugins:   "settings.sub.plugins",
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
							a.recordingShortcutID = ""
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
						case settingsTabShortcuts:
							a.settingsShortcutsTab(c)
						case settingsTabAccount:
							a.settingsAccountTab(c)
						case settingsTabSync:
							a.settingsSyncTab(c)
						case settingsTabDatabase:
							a.settingsDatabaseTab(c)
						case settingsTabImport:
							a.settingsImportTab(c)
						case settingsTabExtension:
							a.settingsExtensionTab(c)
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
							a.recordingShortcutID = ""
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
			switch a.lang {
			case i18nEn:
				sel = 1
			case i18nJa:
				sel = 2
			}
			a.arcPillTabs(c, "pref-lang", sel, []string{"繁體中文", "English", "日本語"}, func(idx int) {
				switch idx {
				case 0:
					a.setLang(i18nZh)
				case 1:
					a.setLang(i18nEn)
				case 2:
					a.setLang(i18nJa)
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

		a.settingsSwitchRowTip(c, "pref-sb-counts", iconPanelLeft, a.t("pref.sbCounts"), a.t("pref.sbCountsTip"), &a.showSidebarCounts, func() {
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-tag-colors", iconTag, a.t("pref.tagColors"), &a.showTagColors, func() {
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-card-caption", iconType, a.t("pref.cardTitle"), &a.showCardCaption, func() {
			a.syncConfigFromState()
		})
	})

	// 2.5 側邊欄頂部按鈕（全部素材固定顯示，其餘可個別開關）
	a.settingsSection(c, a.t("pref.grpSidebarBtns"), a.t("pref.grpSidebarBtnsTip"), true, func() {
		a.settingsItemRow(c, "pref-sb-btn-all", iconLibrary, a.t("pref.sbBtnAllLocked"), "", func() {
			a.pillBadge(c, "ON", a.pal.inkMuted, a.pal.surfaceMuted)
		})
		a.settingsIconSwitchRow(c, "pref-sb-btn-untagged", iconInbox, a.t("shell.untagged"), &a.showSidebarUntagged, func() {
			if !a.showSidebarUntagged && a.navFilter == "untagged" {
				a.handleNav("tag-all")
			}
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-sb-btn-recent", iconClock, a.t("shell.recent"), &a.showSidebarRecent, func() {
			if !a.showSidebarRecent && a.navFilter == "recent" {
				a.handleNav("tag-all")
			}
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-sb-btn-random", iconShuffle, a.t("shell.random"), &a.showSidebarRandom, func() {
			if !a.showSidebarRandom && a.navFilter == "random" {
				a.handleNav("tag-all")
			}
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-sb-btn-missing", iconAlert, a.t("shell.missing"), &a.showSidebarMissing, func() {
			if !a.showSidebarMissing && a.navFilter == "missing" {
				a.handleNav("tag-all")
			}
			a.syncConfigFromState()
		})
		a.settingsIconSwitchRow(c, "pref-sb-btn-tagmgr", iconTag, a.t("tags.manage"), &a.showSidebarTagMgr, func() {
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
					a.arcPillTabs(c, "pref-laya-preset", selMod, []string{a.t("ai.layaMultilingual"), "Laya EN", "Winnow 4B"}, func(idx int) {
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
		ffText := a.ffmpegStatusLabel()
		a.settingsItemRow(c, "db-ff", iconSliders, a.t("pref.ffmpeg"), "", func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, ffText).FontSize(12).TextColor(a.pal.inkSecondary).SingleLine()
				if !a.ffmpeg {
					dlLabel := a.t("settings.ffmpegDownload")
					if a.ffmpegInstalling {
						dlLabel = fmt.Sprintf(a.t("settings.ffmpegDownloading"), a.ffmpegInstallProgress)
					}
					a.arcButton(c, "pref-db-install-ff", dlLabel, true).OnClick(func() {
						a.installFFmpegAsync()
					})
				}
			})
		})
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

	// 3. 監控資料夾 (Watch Folders)
	a.settingsSection(c, a.t("watch.section"), a.t("watch.tip"), true, func() {
		a.watchFoldersPanel(c, "pref-wf")
	})
}

func (a *App) settingsExtensionTab(c *gui.Context) {
	services := c.Services()
	tok := a.ensureServerToken()
	addr := fmt.Sprintf("http://127.0.0.1:%d", a.serverPortValue())
	if a.localSrv != nil && a.localSrv.Running() {
		addr = a.localSrv.LocalURL()
	}

	// 1. 擴充連線 + 獨立置頂存取金鑰
	a.settingsSection(c, a.t("ext.grpConn"), a.t("ext.connTip"), false, func() {
		dotCol := a.pal.inkFaint
		statusText := a.t("ext.statusOff")
		if a.localSrv != nil && a.localSrv.Running() {
			dotCol = gui.Hex("#10B981")
			statusText = a.t("ext.statusOn")
		}
		a.settingsItemRow(c, "ext-run", iconGlobe, a.t("ext.grpConn"), "", func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Box(c).Size(7, 7).Radius(radiusPill).Background(dotCol)
				gui.Text(c, statusText).FontSize(12).TextColor(a.pal.inkMuted).SingleLine()
				a.arcSwitch(c, "pref-ext-srv-run", &a.serverRunning, func() {
					a.toggleBuiltInServer()
				})
			})
		})

		a.settingsItemRow(c, "ext-key", iconLock, a.t("pref.advToken"), a.t("ext.keyTip"), func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, tok).FontSize(12).TextColor(a.pal.inkSecondary).MaxWidth(210).SingleLine()
				a.arcButton(c, "pref-ext-copy-key", a.t("ext.copyKey"), true).OnClick(func() {
					services.WriteClipboard(tok)
					a.notify(a.t("common.copied"))
				})
			})
		})

		a.settingsItemRow(c, "ext-addr", iconLink, a.t("ext.addr"), a.t("ext.addrTip"), func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, addr).FontSize(12).TextColor(a.pal.inkMuted).SingleLine()
				a.iconButton(c, "pref-ext-copy-addr", iconCopy, a.t("common.copyPath"), false).OnClick(func() {
					services.WriteClipboard(addr)
					a.notify(a.t("common.copied"))
				})
			})
		})

		a.settingsIconSwitchRow(c, "pref-ext-auto", iconClock, a.t("pref.advAuto"), &a.serverAutoStart, func() {
			a.syncConfigFromState()
		})
	})

	// 1.5 離線收件資料夾（不開軟體也能存圖）
	a.settingsSection(c, a.t("ext.grpOffline"), a.t("ext.offlineTip"), true, func() {
		inbox := a.effectiveClipperInboxDir()
		a.settingsItemRow(c, "ext-offline-dir", iconInbox, filepath.Base(inbox), "", func() {
			gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
				a.toolButton(c, "pref-ext-inbox-pick", iconFolder, a.t("ext.offlineChange"), false).OnClick(func() {
					a.pickClipperInboxDir()
				})
				a.iconButton(c, "pref-ext-inbox-open", iconExternal, a.t("settings.openFolder"), false).OnClick(func() {
					_ = os.MkdirAll(inbox, 0o755)
					_ = openLocalFile(inbox)
				})
				a.iconButton(c, "pref-ext-inbox-sync", iconRefresh, a.t("ext.offlineSync"), false).
					Disabled(a.watchScanning).
					OnClick(func() { a.scanWatchFoldersNow() })
			})
		})
		gui.Text(c, inbox).
			FontSize(11).
			TextColor(a.pal.inkMuted).
			Padding(0, 10, 4, 35).
			SingleLine()
	})

	// 2. 安裝與步驟
	a.settingsSection(c, a.t("ext.grpInstall"), "", true, func() {
		a.settingsItemRow(c, "ext-chrome", iconExternal, a.t("ext.chrome"), a.t("ext.chromeTip"), func() {
			a.toolButton(c, "pref-ext-dl-chrome", iconExternal, a.t("ext.download"), false).OnClick(func() {
				services.OpenURL("https://github.com/Batkixni/Raglex/releases/latest")
			})
		})
		a.settingsItemRow(c, "ext-firefox", iconExternal, a.t("ext.firefox"), a.t("ext.firefoxTip"), func() {
			a.toolButton(c, "pref-ext-dl-ff", iconExternal, a.t("ext.download"), false).OnClick(func() {
				services.OpenURL("https://github.com/Batkixni/Raglex/releases/latest")
			})
		})
		gui.Text(c, a.t("ext.steps")).
			FontSize(11.5).
			TextColor(a.pal.inkMuted).
			LineHeight(1.5).
			Padding(6, 10, 2, 10)
	})
}

// settingsAccountTab provides Raglex Cloud Account Sign-In (using the default
// placeholder endpoint https://cloud.raglex.app) and a clean 1-click toggle to
// switch between Raglex Cloud and Self-Host sync servers.
func (a *App) settingsShortcutsTab(c *gui.Context) {
	hintKey := "shortcuts.hintWin"
	if useMacChrome {
		hintKey = "shortcuts.hintMac"
	}
	gui.Row(c).
		FillWidth().
		Padding(6, 10, 10, 10).
		Gap(10).
		AlignItems(gui.Center).
		Children(func() {
			gui.Text(c, a.t(hintKey)).FontSize(11.5).TextColor(a.pal.inkMuted).Grow(1)
			a.toolButton(c, "sc-reset-all", iconRefresh, a.t("shortcuts.resetAll"), false).OnClick(func() {
				a.resetAllShortcuts()
			})
		})

	categories := []struct {
		id       string
		titleKey string
	}{
		{"nav", "shortcuts.cat.nav"},
		{"edit", "shortcuts.cat.edit"},
		{"view", "shortcuts.cat.view"},
	}

	for _, cat := range categories {
		ct := cat
		a.settingsSection(c, a.t(ct.titleKey), "", true, func() {
			for _, def := range shortcutDefs {
				if def.Category != ct.id {
					continue
				}
				d := def
				recording := a.recordingShortcutID == d.ID
				isCustom := a.shortcutIsCustom(d.ID)
				lbl := a.shortcutLabel(d.ID)
				if lbl == "" {
					lbl = a.t("shortcuts.unbound")
				}

				row := gui.Row(c.Key("sc-row-" + d.ID)).
					FillWidth().
					Padding(7, 10).
					Gap(10).
					Radius(8).
					AlignItems(gui.Center)
				rbg := gui.Transparent
				if recording || row.Hovered() {
					rbg = a.settingsRowHoverBg()
				}
				row.Background(row.AnimateColor("rbg", rbg, animFast)).Children(func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Grow(1).Children(func() {
						gui.Text(c, a.t(d.LabelKey)).FontSize(12.5).TextColor(a.pal.ink).SingleLine()
						if isCustom {
							gui.Box(c).Size(5, 5).Radius(radiusPill).Background(a.pal.controlOn)
						}
					})

					if isCustom && !recording {
						a.miniHeaderButton(c, "sc-rst-"+d.ID, iconRefresh, a.t("shortcuts.resetOne"), false).OnClick(func() {
							a.resetShortcut(d.ID)
						})
					}

					if recording {
						recBtn := gui.ButtonBase(c.Key("sc-rec-" + d.ID)).
							Padding(5, 11).
							Radius(8).
							Background(a.pal.controlOn).
							TextColor(a.pal.controlGlyph).
							Border(1, a.pal.controlOn).
							Focusable().
							AutoFocus().
							Label(a.t("shortcuts.recording")).
							HandleInput(func(ev gui.InputEvent) bool {
								if ev.Kind != gui.InputKeyDown || ev.Repeat {
									return false
								}
								if ev.Key == gui.KeyEscape {
									a.recordingShortcutID = ""
									return true
								}
								k := ev.Key
								if k == gui.KeyUnknown {
									k = ev.PhysicalKey
								}
								if k == gui.KeyUnknown {
									return true
								}
								if a.setShortcut(d.ID, ev.Mods, k) {
									a.recordingShortcutID = ""
								}
								return true
							}).
							OnClick(func() {
								a.recordingShortcutID = ""
							})
						recBtn.Children(func() {
							gui.Text(c, a.t("shortcuts.recording")).FontSize(11.5).Bold().SingleLine()
						})
					} else {
						keyBtn := gui.ButtonBase(c.Key("sc-key-" + d.ID)).
							Padding(5, 11).
							Radius(8).
							Label(a.t(d.LabelKey) + " " + lbl).
							Tooltip(a.t("shortcuts.clickToBind")).
							OnClick(func() {
								a.recordingShortcutID = d.ID
							})
						kbg := a.pal.surfaceMuted
						kfg := a.pal.ink
						if lbl == a.t("shortcuts.unbound") {
							kfg = a.pal.inkMuted
						}
						if keyBtn.Hovered() {
							kbg = a.pal.surfaceActive
						}
						keyBtn.Background(keyBtn.AnimateColor("kbg", kbg, animFast)).
							TextColor(kfg).
							Border(1, a.chromeHairline()).
							Children(func() {
								gui.Text(c, lbl).FontSize(11.5).Bold().SingleLine()
							})
					}
				})
			}
		})
	}
}

func (a *App) settingsAccountTab(c *gui.Context) {
	providerIdx := 0
	if a.syncProvider == "selfhost" {
		providerIdx = 1
	}

	// 1. 同步服務模式切換（Raglex Cloud 預設 / Self-Host）
	a.settingsSection(c, a.t("account.provider"), a.t("account.providerTip"), false, func() {
		a.settingsItemRow(c, "sync-provider", iconCloud, a.t("account.provider"), "", func() {
			a.arcPillTabs(c, "pref-sync-provider", providerIdx, []string{
				a.t("account.cloud"),
				a.t("account.selfHost"),
			}, func(idx int) {
				if idx == 1 {
					a.syncProvider = "selfhost"
				} else {
					a.syncProvider = "cloud"
				}
				a.syncConfigFromState()
			})
		})

		if a.syncProvider != "selfhost" {
			a.settingsKVRow(c, "cloud-endpoint", iconLink, a.t("account.endpoint"), config.DefaultCloudEndpoint)
		}
	})

	// 2A. Raglex Cloud 帳號登入 / 狀態
	if a.syncProvider != "selfhost" {
		a.settingsSection(c, a.t("account.grpSignIn"), "", true, func() {
			if a.accountToken != "" && a.accountEmail != "" {
				initial := strings.ToUpper(a.accountEmail[:1])
				emailLbl := a.accountEmail
				if a.accountVerified {
					emailLbl += "  ·  " + a.t("account.verified")
				}
				a.settingsItemRow(c, "acct-signed", nil, emailLbl, "", func() {
					gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Box(c).Size(24, 24).Radius(radiusPill).Background(gui.Hex("#FF5C33")).Center().Children(func() {
							gui.Text(c, initial).FontSize(11).Bold().TextColor(gui.Hex("#FFFFFF"))
						})
						btnLbl := a.t("sync.now")
						if a.syncBusy {
							btnLbl = a.t("sync.syncing")
						}
						a.arcButton(c, "acct-sync-now", btnLbl, true).
							Disabled(a.syncBusy).
							OnClick(func() { a.runCloudSyncNow() })
						a.toolButton(c, "acct-signout", iconX, a.t("account.signOut"), false).OnClick(func() {
							a.signOutAccount()
						})
					})
				})

				// 20 GB Cloud Storage Quota Meter
				quota := a.accountQuotaBytes
				if quota == 0 {
					quota = 20 << 30
				}
				used := a.accountUsedBytes
				pct := float64(used) / float64(quota) * 100
				barPct := pct
				if barPct < 0 {
					barPct = 0
				}
				if barPct > 100 {
					barPct = 100
				}
				barCol := a.pal.controlOn
				if pct >= 90 {
					barCol = gui.Hex("#FF5C33")
				} else if pct >= 75 {
					barCol = gui.Hex("#F59E0B")
				}
				gui.Column(c.Key("acct-quota-meter")).FillWidth().Padding(6, 10, 8, 10).Gap(6).Children(func() {
					gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Icon(c, iconDatabase).Size(15, 15).TextColor(a.pal.inkSecondary)
						gui.Text(c, a.t("account.storage")).FontSize(13).TextColor(a.pal.ink).SingleLine()
						gui.Spacer(c)
						gui.Text(c, fmt.Sprintf(a.t("account.storageUsed"), formatBytes(int64(used)), formatBytes(int64(quota)), pct)).
							FontSize(11.5).TextColor(a.pal.inkMuted).SingleLine()
					})
					gui.Box(c).FillWidth().Height(5).Radius(radiusPill).Background(a.pal.surfaceMuted).Children(func() {
						if barPct > 0 {
							wPct := float32(barPct)
							if wPct < 1.5 {
								wPct = 1.5
							}
							gui.Box(c).FillHeight().WidthPercent(wPct).Radius(radiusPill).Background(barCol)
						}
					})
				})

				if !a.accountVerified {
					a.settingsItemRow(c, "acct-verify-row", iconCheck, a.t("account.verifyEmail"), "", func() {
						gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
							gui.TextInput(c.Key("pref-auth-verify"), &a.authVerifyCodeBuf).
								Placeholder(a.t("account.verifyCodePh")).
								Width(156).
								OnSubmit(func() { a.submitVerifyEmail() })
							a.arcButton(c, "pref-verify-ok", a.t("account.verifyBtn"), true).
								Disabled(a.authBusy).
								Padding(5, 10).
								OnClick(func() { a.submitVerifyEmail() })
							a.toolButton(c, "pref-verify-resend", iconRefresh, a.t("account.resendVerify"), false).
								Disabled(a.authBusy).
								OnClick(func() { a.resendVerifyEmail() })
						})
					})
				}
			} else if a.authResetMode {
				a.settingsItemRow(c, "acct-reset-email", iconUser, a.t("account.email"), "", func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						gui.TextInput(c.Key("pref-reset-email"), &a.authEmailBuf).
							Placeholder("you@example.com").
							Width(180).
							OnSubmit(func() { a.sendPasswordResetCode() })
						a.toolButton(c, "pref-reset-send", nil, a.t("account.sendReset"), false).
							Disabled(a.authBusy).
							OnClick(func() { a.sendPasswordResetCode() })
					})
				})
				a.settingsItemRow(c, "acct-reset-code", iconTag, a.t("account.resetCode"), "", func() {
					gui.TextInput(c.Key("pref-reset-code"), &a.authResetCodeBuf).
						Placeholder(a.t("account.verifyCodePh")).
						Width(240)
				})
				a.settingsItemRow(c, "acct-reset-pass", iconLock, a.t("account.newPassword"), "", func() {
					gui.TextInput(c.Key("pref-reset-pass"), &a.authResetNewPassBuf).
						Placeholder("••••••••").
						Password().
						Width(240).
						OnSubmit(func() { a.submitPasswordReset() })
				})
				gui.Row(c).FillWidth().Padding(8, 10, 4, 10).Gap(8).AlignItems(gui.Center).Justify(gui.End).Children(func() {
					a.toolButton(c, "pref-reset-back", nil, a.t("account.backToLogin"), false).OnClick(func() {
						a.authResetMode = false
						a.authStatusMsg = ""
					})
					a.arcButton(c, "pref-reset-submit", a.t("account.resetSubmit"), true).
						Disabled(a.authBusy).
						OnClick(func() { a.submitPasswordReset() })
				})
			} else {
				a.settingsItemRow(c, "acct-email", iconUser, a.t("account.email"), "", func() {
					gui.TextInput(c.Key("pref-auth-email"), &a.authEmailBuf).
						Placeholder("you@example.com").
						Width(240).
						OnSubmit(func() { a.submitCloudAuth(false) })
				})
				a.settingsItemRow(c, "acct-pass", iconLock, a.t("account.password"), "", func() {
					gui.TextInput(c.Key("pref-auth-pass"), &a.authPassBuf).
						Placeholder("••••••••").
						Password().
						Width(240).
						OnSubmit(func() { a.submitCloudAuth(false) })
				})
				a.settingsItemRow(c, "acct-invite", iconTag, a.t("account.inviteCode"), "", func() {
					gui.TextInput(c.Key("pref-auth-invite"), &a.authInviteBuf).
						Placeholder(a.t("account.invitePlaceholder")).
						Width(240).
						OnSubmit(func() { a.submitCloudAuth(true) })
				})
				gui.Row(c).FillWidth().Padding(8, 10, 4, 10).Gap(8).AlignItems(gui.Center).Children(func() {
					a.toolButton(c, "pref-auth-forgot", nil, a.t("account.forgotPass"), false).OnClick(func() {
						a.authResetMode = true
						a.authStatusMsg = ""
					})
					gui.Spacer(c)
					a.toolButton(c, "pref-auth-reg", nil, a.t("account.register"), false).
						Disabled(a.authBusy).
						OnClick(func() { a.submitCloudAuth(true) })
					signLbl := a.t("account.signIn")
					if a.authBusy {
						signLbl = a.t("account.signingIn")
					}
					a.arcButton(c, "pref-auth-login", signLbl, true).
						Disabled(a.authBusy).
						OnClick(func() { a.submitCloudAuth(false) })
				})
			}
			if a.authStatusMsg != "" {
				gui.Text(c, a.authStatusMsg).
					FontSize(11.5).
					TextColor(a.pal.inkMuted).
					LineHeight(1.45).
					Padding(4, 10, 2, 10)
			}
			if a.syncStatusMsg != "" {
				gui.Text(c, a.syncStatusMsg).
					FontSize(11.5).
					TextColor(a.pal.inkMuted).
					Padding(2, 10, 4, 10)
			}
		})
		return
	}

	// 2B. Self-Host 自架同步伺服器
	a.settingsSection(c, a.t("pref.grpSync"), a.t("pref.syncTip"), true, func() {
		a.settingsItemRow(c, "sh-url", iconGlobe, a.t("ext.addr"), "", func() {
			gui.TextInput(c.Key("pref-sync-url"), &a.syncServerURL).
				Placeholder("https://sync.your-domain.com").
				Width(240).
				OnChange(func() { a.syncConfigFromState() })
		})
		a.settingsItemRow(c, "sh-tok", iconLock, a.t("pref.advToken"), "", func() {
			gui.TextInput(c.Key("pref-sync-tok"), &a.syncToken).
				Placeholder(a.t("pref.advToken")).
				Width(240).
				OnChange(func() { a.syncConfigFromState() })
		})
		a.settingsItemRow(c, "sh-relay", iconLink, a.t("pref.advRelay"), a.t("pref.advRelayTip"), func() {
			gui.TextInput(c.Key("pref-signal-url"), &a.signalURL).
				Placeholder("https://relay.your-domain.com").
				Width(240).
				OnChange(func() {
					a.signalURL = strings.TrimSpace(a.signalURL)
					a.syncConfigFromState()
				})
		})
		gui.Row(c).FillWidth().Padding(8, 10, 4, 10).Gap(8).AlignItems(gui.Center).Children(func() {
			if a.syncStatusMsg != "" {
				gui.Text(c, a.syncStatusMsg).FontSize(11.5).TextColor(a.pal.inkMuted).Grow(1).SingleLine()
			} else {
				gui.Spacer(c)
			}
			btnLbl := a.t("sync.now")
			if a.syncBusy {
				btnLbl = a.t("sync.syncing")
			}
			a.arcButton(c, "pref-sync-now", btnLbl, true).
				Disabled(a.syncBusy || strings.TrimSpace(a.syncServerURL) == "").
				OnClick(func() { a.runCloudSyncNow() })
		})
	})
}

func (a *App) settingsSyncTab(c *gui.Context) {
	services := c.Services()

	if !a.hasSyncConnection() {
		gui.Column(c.Key("sync-locked-state")).FillWidth().Padding(24, 16).Gap(14).AlignItems(gui.Center).Children(func() {
			gui.Box(c).Size(42, 42).Radius(radiusPill).Background(a.pal.surfaceMuted).Center().Children(func() {
				gui.Icon(c, iconLock).Size(19, 19).TextColor(a.pal.inkSecondary)
			})
			gui.Text(c, a.t("sync.needAuth")).
				FontSize(12.5).
				TextColor(a.pal.inkSecondary).
				LineHeight(1.5).
				MaxWidth(420)
			a.arcButton(c, "sync-go-account", a.t("sync.goAccount"), true).OnClick(func() {
				a.setSettingsTab(settingsTabAccount)
			})
		})
		return
	}

	// 0. 智慧自動同步（預設開啟，編輯防抖＋背景定期靜默同步）
	a.settingsSwitchRowTip(c, "sync-auto-bg", iconRefresh, a.t("sync.autoTitle"), a.t("sync.autoTip"), &a.autoSync, func() {
		a.syncConfigFromState()
		if a.autoSync {
			a.scheduleAutoSync(1 * time.Second)
		}
	})
	if !a.lastSyncAt.IsZero() {
		gui.Text(c, fmt.Sprintf(a.t("sync.lastSynced"), a.lastSyncAt.Format("15:04:05"))).
			FontSize(11).
			TextColor(a.pal.inkMuted).
			Padding(0, 10, 4, 35)
	}
	a.settingsItemRow(c, "sync-e2ee-key", iconLock, a.t("sync.e2eeTitle"), a.t("sync.e2eeTip"), func() {
		gui.TextInput(c.Key("pref-sync-e2ee"), &a.syncE2EEPassphrase).
			Placeholder(a.t("sync.e2eePh")).
			Password().
			Width(240).
			OnChange(func() { a.syncConfigFromState() })
	})
	if strings.TrimSpace(a.syncE2EEPassphrase) != "" {
		gui.Row(c).FillWidth().Padding(0, 10, 4, 35).Gap(6).AlignItems(gui.Center).Children(func() {
			gui.Box(c).Size(7, 7).Radius(radiusPill).Background(gui.Hex("#10B981"))
			gui.Text(c, a.t("sync.e2eeActive")).FontSize(11).TextColor(gui.Hex("#10B981")).SingleLine()
		})
	}

	// 1. 選擇性資料夾同步（節省 20 GB 雲端配額）
	a.settingsSection(c, a.t("sync.foldersTitle"), a.t("sync.foldersSub"), true, func() {
		gui.Row(c).FillWidth().Padding(4, 10, 6, 10).Gap(6).AlignItems(gui.Center).Children(func() {
			a.toolButton(c, "sync-folders-all-on", iconCheck, a.t("sync.allOn"), false).OnClick(func() {
				a.syncExcludeFolders = map[string]bool{}
				a.syncIncludeUncat = true
				a.syncConfigFromState()
				a.scheduleAutoSync(3 * time.Second)
			})
			a.toolButton(c, "sync-folders-all-off", iconX, a.t("sync.allOff"), false).OnClick(func() {
				if a.syncExcludeFolders == nil {
					a.syncExcludeFolders = map[string]bool{}
				}
				for _, f := range a.folders {
					a.syncExcludeFolders[f.ID] = true
				}
				a.syncIncludeUncat = false
				a.syncConfigFromState()
			})
			gui.Spacer(c)
			btnLbl := a.t("sync.now")
			if a.syncBusy {
				btnLbl = a.t("sync.syncing")
			}
			a.arcButton(c, "sync-tab-run-now", btnLbl, true).
				Disabled(a.syncBusy).
				OnClick(func() { a.runCloudSyncNow() })
		})

		a.settingsIconSwitchRow(c, "sync-inc-uncat", iconInbox, a.t("sync.includeUncat"), &a.syncIncludeUncat, func() {
			a.syncConfigFromState()
			if a.syncIncludeUncat {
				a.scheduleAutoSync(3 * time.Second)
			}
		})

		if len(a.folders) == 0 {
			gui.Text(c, a.t("sync.noFolders")).
				FontSize(12).
				TextColor(a.pal.inkMuted).
				LineHeight(1.45).
				Padding(6, 10)
		} else {
			a.ensureFolderTree()
			var renderFolderSyncRows func(list []model.Folder, depth int, parentExcluded bool)
			renderFolderSyncRows = func(list []model.Folder, depth int, parentExcluded bool) {
				for _, f := range list {
					folder := f
					fid := folder.ID
					excluded := parentExcluded || a.syncExcludeFolders[fid]
					included := !excluded
					leftPad := float32(10 + depth*18)
					row := gui.Row(c.Key("sync-folder-row-"+fid)).
						FillWidth().
						Padding(6, 10, 6, leftPad).
						Gap(8).
						Radius(8).
						AlignItems(gui.Center)
					if row.Hovered() {
						row.Background(a.settingsRowHoverBg())
					}
					row.Children(func() {
						icCol := a.pal.inkSecondary
						if col, ok := parseHexColor(folder.Color); ok {
							icCol = col
						}
						gui.Icon(c, iconFolder).Size(15, 15).TextColor(icCol)
						gui.Text(c, folder.Name).FontSize(13).TextColor(a.pal.ink).SingleLine().Grow(1)
						if cnt := a.folderCounts[fid]; cnt > 0 {
							gui.Text(c, fmt.Sprintf("%d", cnt)).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
						}
						a.arcSwitch(c, "sync-folder-sw-"+fid, &included, func() {
							if a.syncExcludeFolders == nil {
								a.syncExcludeFolders = map[string]bool{}
							}
							if !included {
								// Turning OFF: exclude this folder and all its descendants.
								for descID := range a.folderDescendantSet(fid) {
									a.syncExcludeFolders[descID] = true
								}
							} else {
								// Turning ON: un-exclude this folder and its ancestors so it actually syncs.
								cur := fid
								for cur != "" {
									delete(a.syncExcludeFolders, cur)
									if parent, ok := a.folderByID[cur]; ok {
										cur = parent.ParentID
									} else {
										break
									}
								}
							}
							a.syncConfigFromState()
							if included {
								a.scheduleAutoSync(3 * time.Second)
							}
						})
					})
					if ch := a.folderChildren[fid]; len(ch) > 0 {
						renderFolderSyncRows(ch, depth+1, excluded)
					}
				}
			}
			renderFolderSyncRows(a.folderChildren[""], 0, false)
		}
	})

	// 2. 好友協作空間（共同維護參考資料夾與專案；白板採個人擁有制，不可修改他人白板）
	a.settingsSection(c, a.t("collab.title"), a.t("collab.sub"), true, func() {
		gui.Text(c, a.t("collab.sub")).
			FontSize(11.5).
			TextColor(a.pal.inkMuted).
			LineHeight(1.45).
			Padding(2, 10, 6, 10)

		_, tok := a.effectiveSyncEndpoint()
		if tok == "" {
			gui.Text(c, a.t("collab.needCloud")).
				FontSize(12).
				TextColor(a.pal.inkSecondary).
				Padding(6, 10)
			return
		}

		gui.Row(c).FillWidth().Padding(4, 10, 8, 10).Gap(6).AlignItems(gui.Center).Children(func() {
			gui.TextInput(c.Key("collab-join-input"), &a.collabJoinCode).
				Placeholder(a.t("collab.joinPh")).
				Grow(1).
				OnSubmit(func() {
					a.joinCollabByCode(a.collabJoinCode)
				})
			a.arcButton(c, "collab-join-btn", a.t("collab.joinBtn"), true).
				Disabled(a.collabBusy || strings.TrimSpace(a.collabJoinCode) == "").
				OnClick(func() {
					a.joinCollabByCode(a.collabJoinCode)
				})
			gui.PopoverMenuButton(c.Key("collab-create-menu"), a.t("collab.createBtn"), func(m *gui.Menu) {
				if len(a.projectList) == 0 && len(a.folders) == 0 {
					m.Item(a.t("sync.noFolders"))
					return
				}
				for _, p := range a.projectList {
					proj := p
					if a.menuItem(m, iconLayers, fmt.Sprintf(a.t("collab.createProj"), proj.Name), "", false).Chosen() {
						a.createCollabFromTarget("project", proj.ID)
					}
				}
				if len(a.projectList) > 0 && len(a.folders) > 0 {
					m.Separator()
				}
				for _, f := range a.folders {
					folder := f
					if a.menuItem(m, iconFolder, fmt.Sprintf(a.t("collab.createFolder"), folder.Name), "", false).Chosen() {
						a.createCollabFromTarget("folder", folder.ID)
					}
				}
			})
			a.iconButton(c, "collab-refresh-btn", iconRefresh, a.t("settings.recalc"), false).
				Disabled(a.collabBusy).
				OnClick(func() {
					a.refreshCollabSpaces()
				})
		})

		if len(a.collabList) == 0 {
			gui.Text(c, a.t("collab.empty")).
				FontSize(12).
				TextColor(a.pal.inkMuted).
				LineHeight(1.45).
				Padding(6, 10)
			return
		}

		for i, item := range a.collabList {
			cs := item
			if i > 0 {
				gui.Box(c).FillWidth().Height(1).Margin(2, 10).Background(a.chromeHairline())
			}
			isOwner := false
			for _, m := range cs.Members {
				if strings.EqualFold(m.Email, a.accountEmail) && m.Role == "owner" {
					isOwner = true
					break
				}
			}
			rowIcon := iconLayers
			if cs.Kind == "folder" {
				rowIcon = iconFolder
			}
			row := gui.Row(c.Key("collab-row-"+cs.ID)).
				FillWidth().
				Padding(8, 10).
				Gap(10).
				Radius(8).
				AlignItems(gui.Center)
			if row.Hovered() {
				row.Background(a.settingsRowHoverBg())
			}
			row.Children(func() {
				gui.Icon(c, rowIcon).Size(16, 16).TextColor(a.pal.inkSecondary)
				gui.Column(c).Grow(1).Gap(2).Children(func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						gui.Text(c, cs.Title).FontSize(13).Bold().TextColor(a.pal.ink).SingleLine()
						a.pillBadge(c, cs.Code, a.pal.inkSecondary, a.pal.surfaceMuted)
					})
					gui.Text(c, fmt.Sprintf(a.t("collab.meta"), len(cs.Members), cs.Revision, cs.AssetCount)).
						FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
				})
				a.iconButton(c, "collab-copy-"+cs.ID, iconCopy, a.t("collab.copyCode"), false).OnClick(func() {
					services.WriteClipboard(cs.Code)
					a.notify(a.t("share.codeCopied"))
				})
				a.toolButton(c, "collab-sync-"+cs.ID, iconRefresh, a.t("collab.syncBtn"), false).
					Disabled(a.collabBusy).
					OnClick(func() {
						a.syncCollabSpaceByID(cs.ID, cs.Kind)
					})
				leaveLbl := a.t("collab.leave")
				if isOwner {
					leaveLbl = a.t("collab.delete")
				}
				a.toolButton(c, "collab-leave-"+cs.ID, iconTrash, leaveLbl, false).
					Disabled(a.collabBusy).
					OnClick(func() {
						a.leaveOrDeleteCollabSpace(cs.ID)
					})
			})
		}
	})
}

func (a *App) settingsSharingTab(c *gui.Context) {
	services := c.Services()

	// 1. 本機與區網分享（已把接收專案與雲端同步獨立拆出，這裡只留純淨的本機設定）
	a.settingsSection(c, a.t("pref.grpShareLocal"), "", false, func() {
		lanText := a.t("settings.lanOff")
		if node := a.shareNode(); node != nil && node.DiscoveryOK() {
			lanText = a.t("settings.lanOK")
		}
		a.settingsKVRow(c, "lan-state", iconGlobe, a.t("pref.lanState"), lanText)
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
				tok := a.ensureServerToken()
				a.toolButton(c, "pref-srv-open", iconExternal, a.t("server.openWeb"), false).OnClick(func() {
					services.OpenURL(localURL + "/?token=" + tok)
				})
				a.toolButton(c, "pref-srv-mobile", iconExternal, a.t("sync.openMobile"), false).OnClick(func() {
					services.OpenURL(localURL + "/mobile?token=" + tok)
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
			a.settingsItemRow(c, "adv-port", iconSliders, a.t("pref.advPort"), "", func() {
				gui.TextInput(c.Key("pref-srv-port"), &a.serverPortStr).
					Placeholder("47595").
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
					a.pluginIconBadge(c, "inst-"+p.Manifest.ID, p.Manifest.Icon, p.Manifest.Accent, p.Dir)
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
					if enabled && (pluginHasHook(p.Manifest.Hooks, "importer") || pluginHasHook(p.Manifest.Hooks, "library_action")) {
						a.toolButton(c, "plug-run-"+p.Manifest.ID, iconPlay, a.t("plugins.run"), false).
							Padding(5, 10).
							OnClick(func() {
								a.runLibraryPlugin(p.Manifest.ID)
							})
					}
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
				a.pluginIconBadge(c, "mkt-"+entry.ID, entry.Icon, entry.Accent, "")
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
						if pluginHasHook(entry.Hooks, "importer") || pluginHasHook(entry.Hooks, "library_action") {
							a.toolButton(c, "mkt-run-"+entry.ID, iconPlay, a.t("plugins.run"), false).
								Padding(5, 10).
								OnClick(func() {
									a.runLibraryPlugin(entry.ID)
								})
						} else {
							a.pillBadge(c, "✓ "+a.t("plugins.installedBadge"), gui.Hex("#10B981"), gui.RGBA(16, 185, 129, 0.14))
						}
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

func (a *App) pluginIconBadge(c *gui.Context, key, iconSpec, accentHex, pluginDir string) {
	accent := a.pal.inkSecondary
	bg := a.pal.surfaceMuted
	if col, ok := parseHexColor(accentHex); ok {
		accent = col
		bg = gui.RGBA(col.R, col.G, col.B, 0.14)
	}

	// Support custom PNG/JPG icon files inside plugin directories.
	if pluginDir != "" && strings.Contains(iconSpec, ".") {
		iconPath := filepath.Join(pluginDir, filepath.Clean(iconSpec))
		if a.pluginIcons == nil {
			a.pluginIcons = map[string]*gui.Bitmap{}
		}
		bm, cached := a.pluginIcons[iconPath]
		if !cached {
			if raw, err := os.ReadFile(iconPath); err == nil {
				bm, _ = gui.DecodeBitmap(raw)
			}
			a.pluginIcons[iconPath] = bm
		}
		if bm != nil {
			gui.Box(c.Key("picon-"+key)).
				Size(34, 34).
				Radius(9).
				Clip().
				Background(bg).
				Border(1, a.chromeHairline()).
				Center().
				Children(func() {
					gui.Image(c, bm).Fit(gui.Cover).Fill()
				})
			return
		}
	}

	ic := iconSparkle
	switch strings.ToLower(strings.TrimSpace(iconSpec)) {
	case "import", "migrate":
		ic = iconImport
	case "duplicate", "copy":
		ic = iconCopy
	case "markdown", "doc", "text":
		ic = iconFileText
	case "palette", "color":
		ic = iconPalette
	case "crop", "aspect", "tag":
		ic = iconMaximize
	case "layers":
		ic = iconLayers
	case "folder":
		ic = iconFolder
	}

	gui.Box(c.Key("picon-" + key)).
		Size(34, 34).
		Radius(9).
		Background(bg).
		Center().
		Children(func() {
			gui.Icon(c, ic).Size(16, 16).TextColor(accent)
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
