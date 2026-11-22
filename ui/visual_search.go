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

// classifyAssetsWithLaya uses local Ollaya/Laya when reachable, or falls back
// seamlessly to Ragle's built-in offline embedding centroid + lexical decision
// classifier so smart classification works immediately inside the app.
func (a *App) classifyAssetsWithLaya(ids []string) {
	if a.lib == nil || len(ids) == 0 {
		return
	}
	if !a.layaEnabled {
		a.notify(a.t("ai.layaDisabled"))
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
		useRemote := client.Health(ctx) == nil
		var centroids map[string][]float32
		if !useRemote {
			centroids = lib.TagCentroids()
		}
		autoCount := 0
		sugCount := 0
		sugs := map[string][]laya.TagSuggestion{}
		for _, id := range ids {
			asset, err := lib.Get(id)
			if err != nil {
				continue
			}
			var res laya.ClassificationResult
			if useRemote {
				r, err := client.ClassifyAssetTags(ctx, asset, tags, 0.78, 0.35)
				if err == nil {
					res = r
				} else {
					useRemote = false
					if centroids == nil {
						centroids = lib.TagCentroids()
					}
				}
			}
			if !useRemote {
				if _, err := lib.EnsureVisualFeatures(id); err == nil {
					asset, _ = lib.Get(id)
				}
				vec, _ := lib.AssetVector(id)
				res = laya.ClassifyBuiltIn(asset, vec, tags, centroids, 0.72, 0.28)
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

// ensureInspectorSimilar populates the Inspector "Similar" tab grid for asset.
func (a *App) ensureInspectorSimilar(asset model.Asset) {
	if a.lib == nil || asset.ID == "" {
		return
	}
	if a.inspectorSimilarID == asset.ID && (len(a.inspectorSimilar) > 0 || a.inspectorSimilarBusy) {
		return
	}
	a.inspectorSimilarID = asset.ID
	a.inspectorSimilar = nil
	if !a.embedEnabled {
		return
	}
	a.inspectorSimilarBusy = true
	lib := a.lib
	win := a.win
	aid := asset.ID
	all := append([]model.Asset(nil), a.allAssets...)

	run := func() {
		for _, item := range all {
			if model.MediaKind(item.Mime) == model.MediaImage && !item.Missing {
				_, _ = lib.EnsureVisualFeatures(item.ID)
			}
		}
		matches, _ := lib.FindSimilarByAsset(aid, 0.45, 24)
		finish := func() {
			if a.inspectorSimilarID != aid {
				return
			}
			a.inspectorSimilarBusy = false
			a.inspectorSimilar = matches
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

// reindexVisualEmbeddings rebuilds 128-dim visual embeddings and palettes across
// the entire library in a background worker.
func (a *App) reindexVisualEmbeddings() {
	if a.lib == nil || a.aiIndexBusy {
		return
	}
	a.aiIndexBusy = true
	lib := a.lib
	win := a.win
	run := func() {
		n, _ := lib.ReindexAllVisuals(nil)
		finish := func() {
			a.aiIndexBusy = false
			a.aiIndexedCount = lib.VectorCount()
			a.reload()
			a.notify(fmt.Sprintf(a.t("ai.reindexDone"), n))
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
