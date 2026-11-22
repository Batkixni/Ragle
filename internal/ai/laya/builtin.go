package laya

import (
	"math"
	"sort"
	"strings"

	"raglex/internal/ai/embed"
	"raglex/internal/model"
)

// ClassifyBuiltIn performs 100% in-app offline classification when an external
// Ollaya daemon is not running yet. It combines:
//  1. Visual embedding cosine similarity to each tag's learned centroid
//  2. Lexical/metadata signal from filename, notes, and source URL/title
//
// Outputs calibrated softmax probabilities over candidateTags just like Laya.
func ClassifyBuiltIn(
	asset model.Asset,
	assetVec []float32,
	candidateTags []model.Tag,
	tagCentroids map[string][]float32,
	autoThreshold float64,
	suggestThreshold float64,
) ClassificationResult {
	if len(candidateTags) == 0 {
		return ClassificationResult{}
	}
	if autoThreshold <= 0 {
		autoThreshold = 0.72
	}
	if suggestThreshold <= 0 {
		suggestThreshold = 0.28
	}

	hay := strings.ToLower(asset.Label() + " " + asset.Notes + " " + asset.SourceTitle + " " + asset.SourceURL)
	logits := make([]float64, len(candidateTags))
	maxLogit := -1e9
	hasAnySignal := false

	for i, tg := range candidateTags {
		score := 0.0
		if len(assetVec) > 0 {
			if cen := tagCentroids[tg.ID]; len(cen) == len(assetVec) {
				sim := embed.CosineSimilarity(assetVec, cen)
				score += sim * 4.5
				hasAnySignal = true
			}
		}
		tLow := strings.ToLower(strings.TrimSpace(tg.Name))
		if tLow != "" && strings.Contains(hay, tLow) {
			score += 3.2
			hasAnySignal = true
		} else {
			for _, tok := range strings.FieldsFunc(tLow, func(r rune) bool {
				return r == ' ' || r == '_' || r == '-' || r == '/'
			}) {
				if len(tok) >= 2 && strings.Contains(hay, tok) {
					score += 1.6
					hasAnySignal = true
				}
			}
		}
		logits[i] = score
		if score > maxLogit {
			maxLogit = score
		}
	}
	if !hasAnySignal {
		return ClassificationResult{}
	}

	// Calibrated softmax probabilities.
	var sumExp float64
	probs := make([]float64, len(candidateTags))
	for i, lg := range logits {
		e := math.Exp(lg - maxLogit)
		probs[i] = e
		sumExp += e
	}
	var res ClassificationResult
	for i, tg := range candidateTags {
		p := probs[i] / sumExp
		item := TagSuggestion{Tag: tg, Probability: p}
		res.AllScores = append(res.AllScores, item)
		switch {
		case p >= autoThreshold && logits[i] >= 2.2:
			res.AutoApply = append(res.AutoApply, item)
		case p >= suggestThreshold && logits[i] > 0.5:
			res.Suggested = append(res.Suggested, item)
		}
	}
	sort.Slice(res.AutoApply, func(i, j int) bool { return res.AutoApply[i].Probability > res.AutoApply[j].Probability })
	sort.Slice(res.Suggested, func(i, j int) bool { return res.Suggested[i].Probability > res.Suggested[j].Probability })
	sort.Slice(res.AllScores, func(i, j int) bool { return res.AllScores[i].Probability > res.AllScores[j].Probability })
	return res
}
