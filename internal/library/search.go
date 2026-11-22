package library

import (
	"sort"

	"go.etcd.io/bbolt"

	"ragle/internal/db"
	"ragle/internal/model"
	"ragle/internal/thumbnail"
)

// VisualMatch is a ranked reverse-image or color search hit.
type VisualMatch struct {
	Asset      model.Asset
	Similarity float64 // 0..1 (higher is closer)
	Distance   float64 // perceptual distance (lower is closer)
}

// EnsureVisualFeatures computes and persists pHash, dHash, and OKLab palette
// for assetID if not already indexed.
func (l *Library) EnsureVisualFeatures(assetID string) (model.Asset, error) {
	a, err := l.Get(assetID)
	if err != nil {
		return model.Asset{}, err
	}
	if len(a.Palette) > 0 && (a.PHash != 0 || a.DHash != 0) {
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
	err = l.DB.Update(func(tx *bbolt.Tx) error {
		cur, err := db.GetAsset(tx, assetID)
		if err != nil {
			return err
		}
		cur.PHash = ph
		cur.DHash = dh
		cur.Palette = pal
		a = cur
		return db.PutAsset(tx, cur)
	})
	return a, err
}

// FindSimilarByAsset performs reverse image search using an asset already in
// the library, ranking all other assets by combined pHash+dHash+palette similarity.
func (l *Library) FindSimilarByAsset(assetID string, minSimilarity float64, limit int) ([]VisualMatch, error) {
	query, err := l.EnsureVisualFeatures(assetID)
	if err != nil {
		return nil, err
	}
	return l.findSimilarByFeatures(query.ID, query.PHash, query.DHash, query.Palette, minSimilarity, limit)
}

// FindSimilarByImage performs reverse image search from any external image file on disk.
func (l *Library) FindSimilarByImage(imagePath string, minSimilarity float64, limit int) ([]VisualMatch, error) {
	ph, dh, pal, err := thumbnail.ExtractVisualFromPath(imagePath)
	if err != nil {
		return nil, err
	}
	return l.findSimilarByFeatures("", ph, dh, pal, minSimilarity, limit)
}

func (l *Library) findSimilarByFeatures(excludeID string, qPH, qDH uint64, qPal []model.PaletteColor, minSimilarity float64, limit int) ([]VisualMatch, error) {
	assets, err := l.List(Filter{})
	if err != nil {
		return nil, err
	}
	if minSimilarity <= 0 {
		minSimilarity = 0.55
	}
	var out []VisualMatch
	for _, a := range assets {
		if excludeID != "" && a.ID == excludeID {
			continue
		}
		if a.PHash == 0 && a.DHash == 0 {
			continue
		}
		hashSim := thumbnail.VisualSimilarity(qPH, qDH, a.PHash, a.DHash)
		// Blend slight dominant-color affinity so images with both matching
		// composition and matching color palette bubble to the top.
		colorSim := hashSim
		if len(qPal) > 0 && len(a.Palette) > 0 {
			if d, ok := thumbnail.ColorDistanceToPalette(qPal[0].Hex, a.Palette); ok {
				colorSim = 1.0 - d*2.0
				if colorSim < 0 {
					colorSim = 0
				}
			}
		}
		score := 0.80*hashSim + 0.20*colorSim
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
