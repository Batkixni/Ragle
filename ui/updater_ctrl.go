package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	gui "github.com/egoist/mygo/ui"

	"raglex/internal/updater"
)

func (a *App) updaterClient() *updater.Client {
	return updater.NewClient(a.updateBetaChannel)
}

// startQuietUpdateCheck checks GitHub Releases shortly after startup when
// automatic updates are enabled, opening a clean confirmation popup if newer.
func (a *App) startQuietUpdateCheck() {
	if a.win == nil || !a.autoCheckUpdates {
		return
	}
	client := a.updaterClient()
	win := a.win
	go func() {
		time.Sleep(3 * time.Second)
		if a.isClosed() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rel, newer, err := client.Check(ctx)
		if err != nil || !newer || rel == nil || rel.DownloadURL == "" {
			return
		}
		win.Update(func() {
			if a.isClosed() {
				return
			}
			a.updateRelease = rel
			a.updateAvailable = true
			a.updateMsg = fmt.Sprintf(a.t("update.available"), rel.TagName)
			a.updatePromptOpen = true
		})
	}()
}

// checkForUpdatesNow manually queries GitHub Releases and opens the update popup
// immediately when a newer release is found.
func (a *App) checkForUpdatesNow() {
	if a.updateChecking || a.updateApplying {
		return
	}
	a.updateChecking = true
	a.updateErr = ""
	a.updateMsg = a.t("update.checking")
	client := a.updaterClient()
	win := a.win

	finish := func(rel *updater.Release, newer bool, err error) {
		a.updateChecking = false
		if err != nil {
			a.updateErr = err.Error()
			a.updateMsg = fmt.Sprintf(a.t("update.failed"), err.Error())
			return
		}
		a.updateRelease = rel
		a.updateAvailable = newer
		if newer && rel != nil {
			a.updateMsg = fmt.Sprintf(a.t("update.available"), rel.TagName)
			if rel.DownloadURL != "" {
				a.updatePromptOpen = true
			}
		} else {
			tag := updater.Version
			if rel != nil && rel.TagName != "" {
				tag = rel.TagName
			}
			a.updateMsg = fmt.Sprintf(a.t("update.upToDate"), tag)
		}
	}

	if win == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rel, newer, err := client.Check(ctx)
		finish(rel, newer, err)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rel, newer, err := client.Check(ctx)
		win.Update(func() {
			if a.isClosed() {
				return
			}
			finish(rel, newer, err)
		})
	}()
}

// applyUpdateNow downloads, verifies, and atomically installs a.updateRelease.
// When autoRestart is requested (from the update popup), Raglex relaunches
// automatically as soon as installation finishes.
func (a *App) applyUpdateNow() {
	if a.updateApplying || a.updateRelease == nil {
		return
	}
	rel := a.updateRelease
	if rel.DownloadURL == "" {
		a.updateMsg = a.t("update.noAsset")
		return
	}
	a.updateApplying = true
	a.updateErr = ""
	a.updatePct = 0
	a.updateMsg = fmt.Sprintf(a.t("update.downloading"), 0)

	client := a.updaterClient()
	win := a.win
	if win == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := client.DownloadAndApply(ctx, rel, func(p updater.Progress) {
			win.Update(func() {
				if a.isClosed() {
					return
				}
				a.updatePct = p.Percent
				switch p.Stage {
				case "downloading":
					a.updateMsg = fmt.Sprintf(a.t("update.downloading"), int(p.Percent))
				case "verifying":
					a.updateMsg = a.t("update.verifying")
				case "extracting":
					a.updateMsg = a.t("update.extracting")
				case "installing":
					a.updateMsg = a.t("update.installing")
				case "done":
					a.updateMsg = a.t("update.ready")
				}
			})
		})
		win.Update(func() {
			a.updateApplying = false
			if a.isClosed() {
				return
			}
			if err != nil {
				a.updateAutoRestart = false
				a.updateErr = err.Error()
				a.updateMsg = fmt.Sprintf(a.t("update.failed"), err.Error())
				a.notify(a.updateMsg)
				return
			}
			a.updateInstalled = true
			a.updatePct = 100
			a.updateMsg = a.t("update.ready")
			if a.updateAutoRestart {
				a.updateAutoRestart = false
				a.restartToApplyUpdate()
				return
			}
			a.notify(a.t("update.ready"))
		})
	}()
}

// updatePromptModal shows a clean popup asking whether to update now.
// Clicking "Update & Restart" runs the download/verify/install progress bar
// right inside the popup and relaunches automatically once done.
func (a *App) updatePromptModal(c *gui.Context) {
	if !a.updatePromptOpen || a.updateRelease == nil {
		return
	}
	rel := a.updateRelease
	services := c.Services()
	openProxy := a.updatePromptOpen

	a.styledDialog(c, "update-prompt-modal", &openProxy, 440, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
				if bm := a.brandLogo(); bm != nil {
					gui.Image(c, bm).Size(24, 24).Fit(gui.Contain)
				}
				gui.Column(c).Grow(1).Gap(2).Children(func() {
					gui.Text(c, fmt.Sprintf(a.t("update.popupTitle"), rel.TagName)).FontSize(16).Bold()
					gui.Text(c, fmt.Sprintf("v%s  →  %s", updater.Version, rel.TagName)).FontSize(11.5).TextColor(a.pal.inkMuted)
				})
				if !a.updateApplying {
					a.iconButton(c, "upd-pop-x", iconX, a.t("common.close"), false).OnClick(func() {
						a.updatePromptOpen = false
					})
				}
			})

			if snippet := summarizeReleaseNotes(rel.Body, 5); snippet != "" {
				gui.Box(c).FillWidth().Padding(10, 12).Radius(radiusControl).Background(a.pal.surfaceMuted).Children(func() {
					gui.Text(c, snippet).FontSize(12).TextColor(a.pal.inkSecondary).LineHeight(1.5)
				})
			}

			if a.updateApplying || a.updateErr != "" || a.updateInstalled {
				msgCol := a.pal.ink
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

			gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
				if rel.HTMLURL != "" {
					relURL := rel.HTMLURL
					a.toolButton(c, "upd-pop-notes", iconExternal, a.t("update.openRelease"), false).OnClick(func() {
						services.OpenURL(relURL)
					})
				}
				gui.Spacer(c)
				if !a.updateApplying {
					a.arcButton(c, "upd-pop-later", a.t("update.later"), false).OnClick(func() {
						a.updatePromptOpen = false
					})
					if a.updateInstalled {
						a.arcButton(c, "upd-pop-restart", a.t("update.restartBtn"), true).OnClick(func() {
							a.restartToApplyUpdate()
						})
					} else {
						a.arcButton(c, "upd-pop-go", a.t("update.updateAndRestart"), true).OnClick(func() {
							a.updateAutoRestart = true
							a.applyUpdateNow()
						})
					}
				}
			})
		})
	})

	if !openProxy && !a.updateApplying {
		a.updatePromptOpen = false
	}
}

func summarizeReleaseNotes(body string, maxLines int) string {
	var kept []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "**Full Changelog**") {
			continue
		}
		line = strings.TrimLeft(line, "# ")
		line = strings.ReplaceAll(line, "**", "")
		kept = append(kept, line)
		if len(kept) >= maxLines {
			break
		}
	}
	return strings.Join(kept, "\n")
}

// restartToApplyUpdate relaunches the freshly installed Raglex binary and exits.
// Using mygo.App.Relaunch ensures windows and the bbolt database lock are closed
// cleanly on the main thread before the new process starts.
func (a *App) restartToApplyUpdate() {
	if a.win != nil {
		mygo.App.Relaunch()
		return
	}
	a.shutdown()
	if err := updater.Relaunch(); err != nil {
		a.notify(fmt.Sprintf(a.t("update.failed"), err.Error()))
		return
	}
	os.Exit(0)
}
