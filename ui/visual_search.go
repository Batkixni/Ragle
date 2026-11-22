package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"ragle/internal/ai/laya"
	"ragle/internal/model"
)

var colorFilterSwatches = []string{
	"#EF4444", "#FF5C33", "#F59E0B", "#10B981", "#06B6D4",
	"#3B82F6", "#8B5CF6", "#EC4899", "#F4F4F5", "#18181B",
}

func (a *App) setColorFilter(hex string) {
	if strings.EqualFold(a.colorFilter, hex) {
		a.colorFilter = ""
	} else {
		a.colorFilter = strings.ToUpper(strings.TrimSpace(hex))
	}
	a.revealGen++
	a.applyFilters()
}

func (a *App) clearVisualSearch() {
	a.colorFilter = ""
	a.similarQueryLabel = ""
	a.similarScores = nil
	a.revealGen++
	a.applyFilters()
}

// searchSimilarByAsset indexes missing visual hashes if needed and ranks
// library assets by perceptual reverse-image similarity to asset.
func (a *App) searchSimilarByAsset(asset model.Asset) {
	if a.lib == nil {
		return
	}
	run := func() {
		// Ensure every image asset has pHash/dHash/palette indexed so search works on first try.
		for _, item := range a.allAssets {
			if model.MediaKind(item.Mime) == model.MediaImage && item.PHash == 0 && item.DHash == 0 && !item.Missing {
				_, _ = a.lib.EnsureVisualFeatures(item.ID)
			}
		}
		matches, err := a.lib.FindSimilarByAsset(asset.ID, 0.55, 60)
		apply := func() {
			if err != nil {
				a.notify(err.Error())
				return
			}
			if len(matches) == 0 {
				a.notify(a.t("visual.noSimilar"))
				return
			}
			scores := make(map[string]float64, len(matches))
			for _, m := range matches {
				scores[m.Asset.ID] = m.Similarity
			}
			a.similarScores = scores
			a.similarQueryLabel = a.assetName(asset)
			a.screen = screenLibrary
			a.reload()
		}
		if a.win == nil {
			apply()
			return
		}
		a.win.Update(func() {
			if !a.isClosed() {
				apply()
			}
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

// searchSimilarByFileDialog lets the user choose any external image on disk
// and runs perceptual reverse-image search against the library.
func (a *App) searchSimilarByFileDialog() {
	if a.lib == nil || a.win == nil {
		return
	}
	win := a.win
	lib := a.lib
	all := append([]model.Asset(nil), a.allAssets...)
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: a.t("visual.searchByImage"),
		})
		if err != nil || len(paths) == 0 || paths[0] == "" {
			return
		}
		qPath := paths[0]
		for _, item := range all {
			if model.MediaKind(item.Mime) == model.MediaImage && item.PHash == 0 && item.DHash == 0 && !item.Missing {
				_, _ = lib.EnsureVisualFeatures(item.ID)
			}
		}
		matches, err := lib.FindSimilarByImage(qPath, 0.55, 60)
		win.Update(func() {
			if a.isClosed() {
				return
			}
			if err != nil {
				a.notify(err.Error())
				return
			}
			if len(matches) == 0 {
				a.notify(a.t("visual.noSimilar"))
				return
			}
			scores := make(map[string]float64, len(matches))
			for _, m := range matches {
				scores[m.Asset.ID] = m.Similarity
			}
			a.similarScores = scores
			a.similarQueryLabel = filepath.Base(qPath)
			a.screen = screenLibrary
			a.reload()
		})
	}()
}

// classifyAssetsWithLaya calls the local open-source Ollaya/Laya decision
// model to auto-tag high-confidence matches and surface medium-confidence
// tag suggestions in the Inspector.
func (a *App) classifyAssetsWithLaya(ids []string) {
	if a.lib == nil || len(ids) == 0 {
		return
	}
	if len(a.tags) == 0 {
		a.notify(a.t("laya.needTags"))
		return
	}
	client := laya.NewClient(a.layaURL, a.layaModel)
	tags := append([]model.Tag(nil), a.tags...)
	lib := a.lib
	win := a.win

	run := func() {
		ctx := context.Background()
		autoCount := 0
		sugCount := 0
		sugs := map[string][]laya.TagSuggestion{}
		var firstErr error
		for _, id := range ids {
			asset, err := lib.Get(id)
			if err != nil {
				continue
			}
			res, err := client.ClassifyAssetTags(ctx, asset, tags, 0.78, 0.35)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				break
			}
			for _, hit := range res.AutoApply {
				if err := lib.SetAssetTag(id, hit.Tag.ID, true); err == nil {
					autoCount++
				}
			}
			if len(res.Suggested) > 0 {
				sugs[id] = res.Suggested
				sugCount += len(res.Suggested)
			}
		}
		finish := func() {
			if firstErr != nil {
				a.notify(firstErr.Error())
				return
			}
			if a.layaSuggestions == nil {
				a.layaSuggestions = map[string][]laya.TagSuggestion{}
			}
			for id, list := range sugs {
				a.layaSuggestions[id] = list
			}
			a.reload()
			a.notify(fmt.Sprintf(a.t("laya.done"), autoCount, sugCount))
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) applyLayaSuggestion(assetID, tagID string) {
	if a.lib == nil {
		return
	}
	if err := a.lib.SetAssetTag(assetID, tagID, true); err != nil {
		a.notify(err.Error())
		return
	}
	a.dismissLayaSuggestion(assetID, tagID)
	a.reload()
}

func (a *App) dismissLayaSuggestion(assetID, tagID string) {
	list := a.layaSuggestions[assetID]
	out := make([]laya.TagSuggestion, 0, len(list))
	for _, s := range list {
		if s.Tag.ID != tagID {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		delete(a.layaSuggestions, assetID)
	} else {
		a.layaSuggestions[assetID] = out
	}
}

func (a *App) testLayaConnection() {
	client := laya.NewClient(a.layaURL, a.layaModel)
	win := a.win
	run := func() {
		err := client.Health(context.Background())
		finish := func() {
			if err != nil {
				a.notify(err.Error())
				return
			}
			a.notify(fmt.Sprintf(a.t("laya.connected"), client.BaseURL))
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}
