package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	gui "github.com/egoist/mygo/ui"

	"raglex/internal/library"
)

func (a *App) openRenameModal(ids []string) {
	if a.lib == nil || len(ids) == 0 {
		return
	}
	a.renameIDs = append([]string(nil), ids...)
	a.renameTemplate = ""
	a.renameFind = ""
	a.renameReplace = ""
	a.renameRegex = false
	a.renameIgnoreCase = true
	a.renamePrefix = ""
	a.renameSuffix = ""
	a.renameCaseMode = ""
	a.renameStartIdx = "1"
	a.renameDiskFile = true
	a.renameIncludeExt = false
	if len(ids) == 1 {
		if asset, err := a.lib.Get(ids[0]); err == nil {
			lbl := asset.Label()
			ext := filepath.Ext(lbl)
			a.renameTemplate = strings.TrimSuffix(lbl, ext)
		}
	}
	a.renameOpen = true
}

func (a *App) buildRenameSpec() library.RenameSpec {
	startIdx, err := strconv.Atoi(strings.TrimSpace(a.renameStartIdx))
	if err != nil || startIdx <= 0 {
		startIdx = 1
	}
	return library.RenameSpec{
		Template:        a.renameTemplate,
		Find:            a.renameFind,
		Replace:         a.renameReplace,
		UseRegex:        a.renameRegex,
		CaseInsensitive: a.renameIgnoreCase,
		Prefix:          a.renamePrefix,
		Suffix:          a.renameSuffix,
		CaseMode:        a.renameCaseMode,
		StartIndex:      startIdx,
		RenameDiskFile:  a.renameDiskFile,
		IncludeExternal: a.renameIncludeExt,
	}
}

func (a *App) applyBatchRename() {
	if a.lib == nil || len(a.renameIDs) == 0 {
		return
	}
	spec := a.buildRenameSpec()
	batch, err := a.lib.ApplyBatchRename(a.renameIDs, spec)
	if err != nil {
		a.notify(err.Error())
		return
	}
	a.renameOpen = false
	if len(batch.Entries) == 0 {
		a.notify(a.t("rename.noChange"))
		return
	}
	a.reload()
	a.notify(fmt.Sprintf(a.t("rename.done"), len(batch.Entries)))
}

func (a *App) undoLastBatchRename() {
	if a.lib == nil {
		return
	}
	n, err := a.lib.UndoLastBatchRename()
	if err != nil {
		a.notify(err.Error())
		return
	}
	a.reload()
	a.notify(fmt.Sprintf(a.t("rename.undone"), n))
}

func (a *App) renameModal(c *gui.Context) {
	if !a.renameOpen || a.lib == nil {
		return
	}
	spec := a.buildRenameSpec()
	preview, previewErr := a.lib.PreviewBatchRename(a.renameIDs, spec)
	changedCount := 0
	conflictCount := 0
	for _, row := range preview {
		if row.Changed {
			changedCount++
		}
		if row.Conflict != "" {
			conflictCount++
		}
	}

	openProxy := a.renameOpen
	gui.DialogBase(c.Key("batch-rename-modal"), &openProxy, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.56))
		panel.
			Width(840).
			Height(590).
			Radius(radiusSurface).
			Clip().
			Background(a.popoverSurface()).
			Border(1, a.chromeHairline()).
			Shadow(0, 24, 60, 0, a.pal.shadowFloating)
		applyBlurReveal(panel, "rename-modal", 0)

		gui.Column(c).Fill().Children(func() {
			// Header.
			title := a.t("rename.title")
			if len(a.renameIDs) == 1 {
				title = a.t("rename.singleTitle")
			}
			gui.Row(c).
				Height(54).
				Padding(0, 20).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Icon(c, iconEdit).Size(16, 16).TextColor(a.pal.controlOn)
					gui.Text(c, title).FontSize(15).Bold()
					gui.Text(c, fmt.Sprintf(a.t("rename.summary"), len(a.renameIDs), changedCount)).
						FontSize(11).
						TextColor(a.pal.inkMuted)
					gui.Spacer(c)
					if a.lib.HasRenameUndo() {
						a.toolButton(c, "ren-undo-hdr", iconRefresh, a.t("rename.undo"), false).OnClick(func() {
							a.renameOpen = false
							a.undoLastBatchRename()
						})
					}
					a.iconButton(c, "ren-close", iconX, a.t("common.close"), false).OnClick(func() {
						a.renameOpen = false
					})
				})

			// 2-column body: left rules, right live preview.
			gui.Row(c).Grow(1).FillWidth().Padding(16).Gap(16).Children(func() {
				// Left column: Rule builder.
				gui.Scroll(c.Key("ren-rules-scroll")).Width(370).FillHeight().Children(func() {
					gui.Column(c).FillWidth().Gap(12).Children(func() {
						// Naming template + clickable token chips.
						a.settingsGroup(c, a.t("rename.template"), func() {
							gui.TextInput(c.Key("ren-tmpl"), &a.renameTemplate).
								Placeholder(a.t("rename.templatePlaceholder")).
								FillWidth().
								OnSubmit(func() {
									if conflictCount == 0 && previewErr == nil {
										a.applyBatchRename()
									}
								})
							gui.Text(c, a.t("rename.tokens")).FontSize(10).TextColor(a.pal.inkMuted)
							tokens := []string{"{name}", "{index:03}", "{date}", "{w}x{h}", "{tag}", "{rating}"}
							gui.Row(c).Gap(5).Wrap().Children(func() {
								for _, tok := range tokens {
									tVal := tok
									a.toolButton(c, "ren-tok-"+tVal, nil, tVal, false).
										Padding(4, 8).
										OnClick(func() {
											if a.renameTemplate == "" {
												a.renameTemplate = tVal
											} else {
												a.renameTemplate += "_" + tVal
											}
										})
								}
							})
						})

						// Find & replace.
						a.settingsGroup(c, a.t("rename.find"), func() {
							gui.Row(c).FillWidth().Gap(8).Children(func() {
								gui.TextInput(c.Key("ren-find"), &a.renameFind).Placeholder(a.t("rename.find")).Grow(1)
								gui.TextInput(c.Key("ren-repl"), &a.renameReplace).Placeholder(a.t("rename.replace")).Grow(1)
							})
							a.settingsSwitchRow(c, "ren-regex", a.t("rename.regex"), &a.renameRegex, nil)
							a.settingsSwitchRow(c, "ren-icase", a.t("rename.ignoreCase"), &a.renameIgnoreCase, nil)
						})

						// Prefix / Suffix / Case / Start index.
						a.settingsGroup(c, a.t("rename.case"), func() {
							gui.Row(c).FillWidth().Gap(8).Children(func() {
								gui.TextInput(c.Key("ren-pre"), &a.renamePrefix).Placeholder(a.t("rename.prefix")).Grow(1)
								gui.TextInput(c.Key("ren-suf"), &a.renameSuffix).Placeholder(a.t("rename.suffix")).Grow(1)
							})
							caseModes := []string{"", "lower", "upper", "title"}
							sel := 0
							for i, m := range caseModes {
								if a.renameCaseMode == m {
									sel = i
									break
								}
							}
							a.arcPillTabs(c, "ren-case", sel, []string{
								a.t("rename.caseKeep"),
								a.t("rename.caseLower"),
								a.t("rename.caseUpper"),
								a.t("rename.caseTitle"),
							}, func(idx int) {
								a.renameCaseMode = caseModes[idx]
							})
							gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
								gui.Text(c, a.t("rename.startIndex")).FontSize(12).TextColor(a.pal.inkSecondary)
								gui.Spacer(c)
								gui.TextInput(c.Key("ren-start"), &a.renameStartIdx).Width(72)
							})
						})

						// Disk safety toggles.
						a.settingsGroup(c, a.t("settings.dbBackup"), func() {
							a.settingsSwitchRow(c, "ren-disk", a.t("rename.diskSync"), &a.renameDiskFile, nil)
							if a.renameDiskFile {
								a.settingsSwitchRow(c, "ren-ext", a.t("rename.includeExt"), &a.renameIncludeExt, nil)
							}
						})
					})
				})

				// Right column: Live before -> after preview.
				gui.Column(c).
					Grow(1).
					FillHeight().
					Radius(radiusPanel).
					Background(a.pal.surface).
					Padding(14).
					Gap(10).
					Children(func() {
						gui.Row(c).FillWidth().AlignItems(gui.Center).Children(func() {
							a.sectionLabel(c, a.t("rename.preview"))
							gui.Spacer(c)
							if conflictCount > 0 {
								gui.Text(c, fmt.Sprintf(a.t("rename.conflicts"), conflictCount)).
									FontSize(11).
									Bold().
									TextColor(a.pal.danger)
							} else if previewErr != nil {
								gui.Text(c, previewErr.Error()).FontSize(11).TextColor(a.pal.danger)
							}
						})

						gui.Scroll(c.Key("ren-preview-scroll")).Grow(1).FillWidth().Children(func() {
							gui.Column(c).FillWidth().Gap(4).Children(func() {
								for i, row := range preview {
									rowBg := a.pal.surfaceRaised
									nameCol := a.pal.ink
									if row.Conflict != "" {
										rowBg = gui.RGBA(224, 122, 122, 0.14)
										nameCol = a.pal.danger
									} else if row.Changed {
										nameCol = a.pal.controlOn
									}
									gui.Column(c.Key(fmt.Sprintf("ren-row-%d", i))).
										FillWidth().
										Padding(7, 10).
										Gap(3).
										Radius(10).
										Background(rowBg).
										Children(func() {
											gui.Text(c, row.OldName).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
											gui.Row(c).FillWidth().Gap(6).AlignItems(gui.Center).Children(func() {
												gui.Text(c, "→").FontSize(11).TextColor(a.pal.inkFaint)
												gui.Text(c, row.NewName).FontSize(12).Bold().TextColor(nameCol).Grow(1).SingleLine()
												if row.Conflict != "" {
													gui.Text(c, row.Conflict).FontSize(10).TextColor(a.pal.danger).SingleLine()
												}
											})
										})
								}
							})
						})
					})
			})

			// Footer.
			gui.Row(c).
				Height(56).
				Padding(0, 20).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Spacer(c)
					a.arcButton(c, "ren-cancel", a.t("common.cancel"), false).OnClick(func() {
						a.renameOpen = false
					})
					canApply := previewErr == nil && conflictCount == 0 && changedCount > 0
					applyBtn := a.arcButton(c, "ren-apply", a.t("rename.apply"), true).Disabled(!canApply)
					applyBtn.OnClick(func() {
						if canApply {
							a.applyBatchRename()
						}
					})
				})
		})
	})
	if !openProxy {
		a.renameOpen = false
	}
}

func (a *App) openExportModal(ids []string) {
	if a.lib == nil || len(ids) == 0 {
		return
	}
	a.exportIDs = append([]string(nil), ids...)
	if a.exportFormat == "" {
		a.exportFormat = "original"
	}
	if a.exportQuality <= 0 {
		a.exportQuality = 90
	}
	a.exportOpen = true
}

func (a *App) runBatchExport() {
	if a.lib == nil || len(a.exportIDs) == 0 || a.win == nil {
		return
	}
	ids := append([]string(nil), a.exportIDs...)
	opts := library.ExportOptions{
		Format:  a.exportFormat,
		Quality: a.exportQuality,
		MaxEdge: a.exportMaxEdge,
	}
	a.exportOpen = false
	win := a.win
	lib := a.lib
	go func() {
		dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:     a.t("export.title"),
			Directory: true,
		})
		if err != nil || len(dirs) == 0 || dirs[0] == "" {
			return
		}
		opts.DestDir = dirs[0]
		res, err := lib.ExportBatch(ids, opts)
		win.Update(func() {
			if a.isClosed() {
				return
			}
			if err != nil {
				a.notify(err.Error())
				return
			}
			a.notify(fmt.Sprintf(a.t("export.done"), res.Exported, res.Failed))
		})
	}()
}

func (a *App) exportModal(c *gui.Context) {
	if !a.exportOpen || a.lib == nil {
		return
	}
	openProxy := a.exportOpen
	gui.DialogBase(c.Key("batch-export-modal"), &openProxy, func(backdrop, panel gui.Element) {
		backdrop.Background(gui.RGBA(0, 0, 0, 0.56))
		panel.
			Width(480).
			Radius(radiusSurface).
			Clip().
			Background(a.popoverSurface()).
			Border(1, a.chromeHairline()).
			Shadow(0, 24, 60, 0, a.pal.shadowFloating)
		applyBlurReveal(panel, "export-modal", 0)

		gui.Column(c).FillWidth().Children(func() {
			gui.Row(c).
				Height(54).
				Padding(0, 20).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Icon(c, iconExport).Size(16, 16).TextColor(a.pal.controlOn)
					gui.Text(c, fmt.Sprintf("%s (%d)", a.t("export.title"), len(a.exportIDs))).FontSize(15).Bold()
					gui.Spacer(c)
					a.iconButton(c, "exp-close", iconX, a.t("common.close"), false).OnClick(func() {
						a.exportOpen = false
					})
				})

			gui.Column(c).FillWidth().Padding(18).Gap(14).Children(func() {
				// Output format.
				a.settingsGroup(c, a.t("export.format"), func() {
					formats := []string{"original", "png", "jpg"}
					sel := 0
					for i, f := range formats {
						if a.exportFormat == f {
							sel = i
							break
						}
					}
					a.arcPillTabs(c, "exp-fmt", sel, []string{
						a.t("export.fmtOriginal"),
						"PNG",
						"JPEG",
					}, func(idx int) {
						a.exportFormat = formats[idx]
					})
				})

				// Max edge.
				a.settingsGroup(c, a.t("export.maxEdge"), func() {
					edges := []int{0, 2048, 1280, 800}
					sel := 0
					for i, e := range edges {
						if a.exportMaxEdge == e {
							sel = i
							break
						}
					}
					a.arcPillTabs(c, "exp-edge", sel, []string{
						a.t("export.edgeOriginal"),
						"2048px",
						"1280px",
						"800px",
					}, func(idx int) {
						a.exportMaxEdge = edges[idx]
					})
				})

				if a.exportFormat == "jpg" {
					a.settingsGroup(c, a.t("export.quality"), func() {
						quals := []int{75, 85, 92, 98}
						sel := 1
						for i, q := range quals {
							if a.exportQuality == q {
								sel = i
								break
							}
						}
						a.arcPillTabs(c, "exp-qual", sel, []string{"75%", "85%", "92%", "98%"}, func(idx int) {
							a.exportQuality = quals[idx]
						})
					})
				}
			})

			gui.Row(c).
				Height(56).
				Padding(0, 20).
				Gap(10).
				AlignItems(gui.Center).
				Background(a.pal.surface).
				Children(func() {
					gui.Spacer(c)
					a.arcButton(c, "exp-cancel", a.t("common.cancel"), false).OnClick(func() {
						a.exportOpen = false
					})
					a.arcButton(c, "exp-run", a.t("export.chooseAndRun"), true).OnClick(func() {
						a.runBatchExport()
					})
				})
		})
	})
	if !openProxy {
		a.exportOpen = false
	}
}

// copySelectedFilesToClipboard writes real OS file paths for the selected
// assets so users can paste them directly into Photoshop, Figma, Blender, etc.
func (a *App) copySelectedFilesToClipboard(ids []string) {
	if a.lib == nil || len(ids) == 0 {
		return
	}
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		if asset, err := a.lib.Get(id); err == nil && !asset.Missing && asset.Path != "" {
			paths = append(paths, asset.Path)
		}
	}
	if len(paths) == 0 {
		return
	}
	if a.win == nil {
		a.notify(fmt.Sprintf(a.t("clipboard.copiedFiles"), len(paths)))
		return
	}
	go func() {
		err := mygo.Clipboard.WriteFiles(paths...)
		a.win.Update(func() {
			if a.isClosed() {
				return
			}
			if err != nil {
				a.notify(err.Error())
				return
			}
			a.notify(fmt.Sprintf(a.t("clipboard.copiedFiles"), len(paths)))
		})
	}()
}

// pasteFromClipboard imports local files or a PNG image from the OS clipboard
// directly into the library (auto-tagging the active tag if enabled).
func (a *App) pasteFromClipboard() {
	if a.lib == nil || a.win == nil {
		return
	}
	var tags []string
	if a.autoTagOnImport && a.tagID != "" {
		tags = []string{a.tagID}
	}
	mode := storageModes[0]
	if a.storageMode >= 0 && a.storageMode < len(storageModes) {
		mode = storageModes[a.storageMode]
	}
	win := a.win
	lib := a.lib

	go func() {
		if files, err := mygo.Clipboard.ReadFiles(); err == nil && len(files) > 0 {
			res, err := lib.Import(context.Background(), files, mode, tags, nil)
			win.Update(func() {
				if a.isClosed() {
					return
				}
				if err != nil {
					a.notify(err.Error())
					return
				}
				a.reload()
				a.notify(fmt.Sprintf(a.t("clipboard.pastedFiles"), res.Added+res.Existing))
			})
			return
		}
		if pngData, err := mygo.Clipboard.ReadFormat(transfer.PNG); err == nil && len(pngData) > 0 {
			fname := "clip_" + time.Now().Format("20060102_150405") + ".png"
			asset, _, err := lib.ImportBytes(fname, pngData, tags, "", "")
			win.Update(func() {
				if a.isClosed() {
					return
				}
				if err != nil {
					a.notify(err.Error())
					return
				}
				a.reload()
				a.notify(fmt.Sprintf(a.t("clipboard.pastedImg"), asset.Label()))
			})
			return
		}
		win.Update(func() {
			if a.isClosed() {
				return
			}
			a.notify(a.t("clipboard.empty"))
		})
	}()
}
