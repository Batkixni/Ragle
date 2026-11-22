package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

func (a *App) libraryView(c *gui.Context) {
	if a.libraryPercent <= 0 {
		a.libraryPercent = 72
	}
	gui.SplitPercent(c, &a.libraryPercent, func() {
		a.libraryMain(c)
	}, func() {
		a.inspector(c)
	}).Fill()
}

func (a *App) libraryMain(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Fill().Padding(12).Gap(10).Children(func() {
		a.importBar(c)
		if a.referenceMode && a.project.Name != "" {
			gui.Row(c).Padding(8, 12).Radius(6).Background(t.Accent.Alpha(0.15)).AlignItems(gui.Center).Children(func() {
				gui.Text(c, fmt.Sprintf(a.t("library.referencing"), a.project.Name)).TextColor(t.Accent).Bold()
				gui.Spacer(c)
				gui.Button(c, a.t("library.reference")).OnClick(func() { a.referenceSelected() })
			})
		}
		if len(a.assets) == 0 {
			gui.Column(c).Grow(1).Center().Children(func() {
				gui.Text(c, a.t("library.empty")).TextColor(t.TextMuted).FontSize(15)
			})
		} else {
			a.assetGrid(c)
		}
		if !a.ffmpeg {
			gui.Text(c, a.t("library.noFfmpeg")).TextColor(t.TextMuted).FontSize(11)
		}
	})
}

func (a *App) importBar(c *gui.Context) {
	t := c.Theme()
	modes := []string{
		a.t("library.mode.index"),
		a.t("library.mode.symlink"),
		a.t("library.mode.move"),
		a.t("library.mode.copy"),
	}
	kinds := []string{
		a.t("library.kind.all"),
		a.t("library.kind.image"),
		a.t("library.kind.video"),
		a.t("library.kind.other"),
	}
	ratings := []string{"0", "1+", "2+", "3+", "4+", "5"}
	gui.Column(c).Padding(8, 12).Radius(8).Background(t.Surface).Gap(8).Children(func() {
		gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("library.mode")).FontSize(12).TextColor(t.TextMuted)
			gui.Segmented(c.Key("storage"), &a.storageMode, modes...)
			gui.Spacer(c)
			gui.Button(c, a.t("library.files")).OnClick(func() { a.startImport(false) })
			gui.Button(c, a.t("library.folder")).OnClick(func() { a.startImport(true) })
		})
		gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.Text(c, a.t("library.kind")).FontSize(12).TextColor(t.TextMuted)
			gui.Segmented(c.Key("kind"), &a.kindFilter, kinds...).OnChange(func() {
				a.assetIndex = -1
				a.reload()
			})
			gui.Spacer(c)
			gui.Text(c, a.t("library.rating")).FontSize(12).TextColor(t.TextMuted)
			gui.Segmented(c.Key("rating"), &a.minRating, ratings...).OnChange(func() {
				a.assetIndex = -1
				a.reload()
			})
		})
	})
}

func (a *App) assetGrid(c *gui.Context) {
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	a.grid.Selected = &a.assetIndex
	a.grid.Key = func(i int) any {
		if i < 0 || i >= len(a.assets) {
			return i
		}
		return a.assets[i].ID
	}
	a.grid.Label = func(i int) string {
		if i < 0 || i >= len(a.assets) {
			return ""
		}
		return a.assetName(a.assets[i])
	}
	gui.GridView(c.Key("asset-grid"), &a.grid, len(a.assets), 150, 160, func(i int) {
		asset := a.assets[i]
		a.assetCard(c, asset)
	}).Grow(1)
}

func (a *App) assetCard(c *gui.Context, asset model.Asset) {
	t := c.Theme()
	name := a.assetName(asset)
	card := gui.Column(c).Fill().Padding(4).Gap(4).Drag(assetDrag{ID: asset.ID})
	card.Children(func() {
		gui.Box(c).Grow(1).Radius(6).Background(t.Background).Center().Children(func() {
			if bm := a.bitmaps[asset.ID]; bm != nil {
				gui.Image(c, bm).Fit(gui.Contain).Grow(1).Radius(6)
			} else {
				ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(asset.Path), "."))
				if ext == "" {
					ext = "FILE"
				}
				gui.Text(c, ext).TextColor(t.TextMuted).Bold()
			}
		})
		gui.Text(c, name).SingleLine().FontSize(12)
		if asset.Missing {
			gui.Text(c, a.t("library.missing")).TextColor(t.Danger).FontSize(10)
		}
	})
}

func (a *App) inspector(c *gui.Context) {
	t := c.Theme()
	asset, ok := a.selectedAsset()
	gui.Scroll(c).Fill().Background(t.Surface).Children(func() {
		gui.Column(c).Padding(16).Gap(12).Children(func() {
			if !ok {
				gui.Column(c).Padding(16).Center().Children(func() {
					gui.Text(c, a.t("inspector.none")).TextColor(t.TextMuted)
				})
				return
			}
			gui.Text(c, a.assetName(asset)).FontSize(15).Bold()

			if bm := a.bitmaps[asset.ID]; bm != nil {
				gui.Box(c).Height(160).Radius(6).Background(t.Background).Center().Children(func() {
					gui.Image(c, bm).Fit(gui.Contain).Grow(1)
				})
			}

			gui.Column(c).Gap(2).Children(func() {
				gui.Text(c, a.t("library.path")).TextColor(t.TextMuted).FontSize(11)
				gui.Text(c, asset.Path).FontSize(12)
			})

			if asset.Missing {
				gui.Text(c, a.t("library.missing")).TextColor(t.Danger).Bold()
			}

			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("library.storage")+":").TextColor(t.TextMuted).FontSize(11)
				gui.Text(c, a.modeLabel(asset.StorageMode)).FontSize(12)
			})

			if asset.Mime != "" {
				gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Text(c, a.t("library.mime")+":").TextColor(t.TextMuted).FontSize(11)
					gui.Text(c, asset.Mime).FontSize(12)
				})
			}

			gui.Column(c).Gap(4).Children(func() {
				gui.Text(c, a.t("library.rating")).TextColor(t.TextMuted).FontSize(11)
				rating := asset.Rating
				if gui.Rating(c.Key("rating-"+asset.ID), &rating, 5).Changed() {
					if err := a.lib.SetRating(asset.ID, rating); err == nil {
						a.reload()
					}
				}
			})

			gui.Column(c).Gap(6).Children(func() {
				gui.Text(c, a.t("library.tags")).TextColor(t.TextMuted).FontSize(11)
				owned := a.tagSet(asset.ID)
				for _, tag := range a.tags {
					id := tag.ID
					label := tag.Name
					if owned[id] {
						label = "✓ " + label
					}
					gui.Button(c.Key("asset-tag-"+asset.ID+"-"+id), label).OnClick(func() {
						if err := a.lib.SetAssetTag(asset.ID, id, !owned[id]); err != nil {
							a.status = err.Error()
							return
						}
						a.reload()
					})
				}
			})

			if a.projectID != "" {
				gui.Button(c, a.t("library.reference")).OnClick(func() { a.referenceSelected() })
			}

			gui.Divider(c)

			gui.Button(c, a.t("library.delete")).OnClick(func() {
				id := asset.ID
				a.ask(a.t("library.delete"), a.deleteAssetCopy(asset), a.t("common.delete"), func() {
					if a.session != nil {
						var drop []string
						for _, item := range a.session.Items {
							if item.Kind == model.KindImage && item.AssetID == id {
								drop = append(drop, item.ID)
							}
						}
						if len(drop) > 0 {
							a.session.Delete(drop)
							a.flushBoard(true)
						}
					}
					if err := a.lib.Delete(id); err != nil {
						a.status = err.Error()
						return
					}
					delete(a.bitmaps, id)
					a.assetIndex = -1
					a.reload()
				})
			})
		})
	})
}

func (a *App) tagSet(assetID string) map[string]bool {
	out := map[string]bool{}
	if a.lib == nil {
		return out
	}
	tags, err := a.lib.AssetTags(assetID)
	if err != nil {
		return out
	}
	for _, tag := range tags {
		out[tag.ID] = true
	}
	return out
}
