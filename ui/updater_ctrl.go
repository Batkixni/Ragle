package ui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo"

	"raglex/internal/updater"
)

func (a *App) updaterClient() *updater.Client {
	return updater.NewClient(a.updateBetaChannel)
}

// startQuietUpdateCheck checks GitHub Releases shortly after startup when
// automatic updates are enabled. Only notifies if a newer version exists.
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
			a.notify(fmt.Sprintf(a.t("update.toastAvailable"), rel.TagName))
		})
	}()
}

// checkForUpdatesNow manually queries GitHub Releases and updates UI status.
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
			a.notify(fmt.Sprintf(a.t("update.toastAvailable"), rel.TagName))
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
				a.updateErr = err.Error()
				a.updateMsg = fmt.Sprintf(a.t("update.failed"), err.Error())
				a.notify(a.updateMsg)
				return
			}
			a.updateInstalled = true
			a.updatePct = 100
			a.updateMsg = a.t("update.ready")
			a.notify(a.t("update.ready"))
		})
	}()
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
