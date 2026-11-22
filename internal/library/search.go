package library

import (
	"sort"

	"go.etcd.io/bbolt"

	"raglex/internal/ai/embed"
	"raglex/internal/db"
	"raglex/internal/model"
	"raglex/internal/thumbnail"
)

// VisualMatch is a ranked reverse-image or color search hit.
type VisualMatch struct {
	Asset      model.Asset
	Similarity float64 // 0..1 (higher is closer)
	Distance   float64 // perceptual distance (lower is closer)
}

// EnsureVisualFeatures computes and persists pHash, dHash, OKLab palette, and
// 128-dim local visual embedding for assetID if not already indexed.
func (l *Library) EnsureVisualFeatures(assetID string) (model.Asset, error) {
	a, err := l.Get(assetID)
	if err != nil {
		return model.Asset{}, err
	}
	var hasVec bool
	_ = l.DB.View(func(tx *bbolt.Tx) error {
		_, hasVec = db.GetAssetVector(tx, assetID)
		return nil
	})
	if len(a.Palette) > 0 && (a.PHash != 0 || a.DHash != 0) && hasVec {
		return a, nil
	}
	src := a.ThumbnailPath
	if src == "" {
		src = a.Path
	}
	if src == "" || a.Missing {
		return a, nil
	}
	ph, dh, pal, err := thumbnail.ExtractVisualFromPath(src)
	if err != nil {
		return a, err
	}
	vec, _ := embed.ExtractFromPath(src)
	err = l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetAsset(tx, assetID)
		if err != nil {
			return err
		}
		cur.PHash = ph
		cur.DHash = dh
		cur.Palette = pal
		a = cur
		if len(vec) > 0 {
			_ = db.PutAssetVector(tx, assetID, vec)
		}
		return db.PutAsset(tx, cur)
	})
	return a, err
}

// AssetVector returns the stored 128-dim visual embedding for assetID, if present.
func (l *Library) AssetVector(assetID string) ([]float32, bool) {
	var vec []float32
	var ok bool
	_ = l.DB.View(func(tx *bbolt.Tx) error {
		vec, ok = db.GetAssetVector(tx, assetID)
		return nil
	})
	return vec, ok
}

// VectorCount returns how many assets currently have visual embeddings indexed.
func (l *Library) VectorCount() int {
	var n int
	_ = l.DB.View(func(tx *bbolt.Tx) error {
		vecs, err := db.ListAssetVectors(tx)
		if err == nil {
			n = len(vecs)
		}
		return nil
	})
	return n
}

// ReindexAllVisuals extracts missing palettes, hashes, and 128-dim embeddings
// across all visual assets in the library.
func (l *Library) ReindexAllVisuals(progress func(done, total int)) (int, error) {
	assets, err := l.List(Filter{})
	if err != nil {
		return 0, err
	}
	total := len(assets)
	indexed := 0
	for i, a := range assets {
		if !a.Missing && (model.MediaKind(a.Mime) == model.MediaImage || a.ThumbnailPath != "") {
			if _, err := l.EnsureVisualFeatures(a.ID); err == nil {
				indexed++
			}
		}
		if progress != nil {
			progress(i+1, total)
		}
	}
	return indexed, nil
}

// TagCentroids builds the mean visual embedding centroid for each tag that has
// at least one indexed asset. Used by the built-in offline Laya classifier.
func (l *Library) TagCentroids() map[string][]float32 {
	out := map[string][]float32{}
	_ = l.DB.View(func(tx *bbolt.Tx) error {
		vecs, err := db.ListAssetVectors(tx)
		if err != nil || len(vecs) == 0 {
			return nil
		}
		tags, err := db.ListTags(tx)
		if err != nil {
			return nil
		}
		for _, tg := range tags {
			aids, err := db.AssetsOfTag(tx, tg.ID)
			if err != nil || len(aids) == 0 {
				continue
			}
			var samples [][]float32
			for _, aid := range aids {
				if v, ok := vecs[aid]; ok {
					samples = append(samples, v)
				}
			}
			if c := embed.MeanCentroid(samples); len(c) > 0 {
				out[tg.ID] = c
			}
		}
		return nil
	})
	return out
}

// FindSimilarByAsset performs reverse image search using an asset already in
// the library, ranking all other assets by combined 128-dim embedding + pHash/dHash + palette similarity.
func (l *Library) FindSimilarByAsset(assetID string, minSimilarity float64, limit int) ([]VisualMatch, error) {
	query, err := l.EnsureVisualFeatures(assetID)
	if err != nil {
		return nil, err
	}
	qVec, _ := l.AssetVector(assetID)
	return l.findSimilarByFeatures(query.ID, query.PHash, query.DHash, query.Palette, qVec, minSimilarity, limit)
}

// FindSimilarByImage performs reverse image search from any external image file on disk.
func (l *Library) FindSimilarByImage(imagePath string, minSimilarity float64, limit int) ([]VisualMatch, error) {
	ph, dh, pal, err := thumbnail.ExtractVisualFromPath(imagePath)
	if err != nil {
		return nil, err
	}
	qVec, _ := embed.ExtractFromPath(imagePath)
	return l.findSimilarByFeatures("", ph, dh, pal, qVec, minSimilarity, limit)
}

func (l *Library) findSimilarByFeatures(excludeID string, qPH, qDH uint64, qPal []model.PaletteColor, qVec []float32, minSimilarity float64, limit int) ([]VisualMatch, error) {
	assets, err := l.List(Filter{})
	if err != nil {
		return nil, err
	}
	var allVecs map[string][]float32
	_ = l.DB.View(func(tx *bbolt.Tx) error {
		allVecs, _ = db.ListAssetVectors(tx)
		return nil
	})
	if minSimilarity <= 0 {
		minSimilarity = 0.50
	}
	var out []VisualMatch
	for _, a := range assets {
		if excludeID != "" && a.ID == excludeID {
			continue
		}
		vecB := allVecs[a.ID]
		hasHash := a.PHash != 0 || a.DHash != 0
		hasVec := len(qVec) > 0 && len(vecB) == len(qVec)
		if !hasHash && !hasVec {
			continue
		}
		hashSim := 0.0
		if hasHash {
			hashSim = thumbnail.VisualSimilarity(qPH, qDH, a.PHash, a.DHash)
		}
		embSim := hashSim
		if hasVec {
			embSim = embed.CosineSimilarity(qVec, vecB)
		}
		colorSim := embSim
		if len(qPal) > 0 && len(a.Palette) > 0 {
			if d, ok := thumbnail.ColorDistanceToPalette(qPal[0].Hex, a.Palette); ok {
				colorSim = 1.0 - d*2.0
				if colorSim < 0 {
					colorSim = 0
				}
			}
		}
		var score float64
		if hasVec && hasHash {
			score = 0.52*embSim + 0.36*hashSim + 0.12*colorSim
		} else if hasVec {
			score = 0.85*embSim + 0.15*colorSim
		} else {
			score = 0.80*hashSim + 0.20*colorSim
		}
		if score >= minSimilarity {
			out = append(out, VisualMatch{
				Asset:      a,
				Similarity: score,
				Distance:   1.0 - score,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Similarity > out[j].Similarity
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// SearchByColor finds assets whose extracted 5-color OKLab palette is close to
// targetHex (maxDistance defaults to 0.22 in OKLab space when <= 0).
func (l *Library) SearchByColor(targetHex string, maxDistance float64) ([]VisualMatch, error) {
	if maxDistance <= 0 {
		maxDistance = 0.22
	}
	assets, err := l.List(Filter{})
	if err != nil {
		return nil, err
	}
	var out []VisualMatch
	for _, a := range assets {
		d, ok := thumbnail.ColorDistanceToPalette(targetHex, a.Palette)
		if !ok || d > maxDistance {
			continue
		}
		sim := 1.0 - d/maxDistance
		if sim < 0 {
			sim = 0
		}
		out = append(out, VisualMatch{
			Asset:      a,
			Similarity: sim,
			Distance:   d,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Distance < out[j].Distance
	})
	return out, nil
}
