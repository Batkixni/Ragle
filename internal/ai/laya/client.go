// Package laya provides an open-source, local-first client for Laya decision
// models served via Ollaya (https://github.com/ollaya-dev/ollaya) or any
// /v1/decisions & /v1/systemone compatible local endpoint.
package laya

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ragle/internal/model"
)

const (
	DefaultBaseURL = "http://127.0.0.1:11435"
	DefaultModel   = "laya:multilingual"
)

// Client calls a local Ollaya/Laya decision server.
type Client struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// NewClient constructs a Laya client with sensible local defaults.
func NewClient(baseURL, modelName string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = DefaultModel
	}
	return &Client{
		BaseURL: baseURL,
		Model:   modelName,
		HTTPClient: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

// ChoiceScore is one candidate option with its calibrated probability (0..1).
type ChoiceScore struct {
	Choice      string  `json:"choice"`
	Probability float64 `json:"probability"`
}

// TagSuggestion is a scored tag from the user's existing tag tree.
type TagSuggestion struct {
	Tag         model.Tag
	Probability float64
}

// ClassificationResult partitions Laya scores into auto-apply vs. review pills.
type ClassificationResult struct {
	AutoApply []TagSuggestion
	Suggested []TagSuggestion
	AllScores []TagSuggestion
}

// Health verifies that the local Ollaya/Laya server is reachable.
func (c *Client) Health(ctx context.Context) error {
	for _, ep := range []string{"/", "/api/version", "/health"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+ep, nil)
		if err != nil {
			return err
		}
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return fmt.Errorf("ollaya unreachable at %s: %w", c.BaseURL, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 500 {
			return nil
		}
	}
	return fmt.Errorf("ollaya server returned error at %s", c.BaseURL)
}

type systemOneQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

type systemOneRequest struct {
	Model     string                       `json:"model"`
	State     string                       `json:"state"`
	Questions map[string]systemOneQuestion `json:"questions"`
}

// DecideChoices sends a single-pass closed-set decision query to Ollaya using
// the normative /v1/systemone & /api/decide wire contract.
func (c *Client) DecideChoices(ctx context.Context, state, question string, choices []string) ([]ChoiceScore, error) {
	if len(choices) == 0 {
		return nil, errors.New("laya: no choices provided")
	}
	q := systemOneQuestion{
		Type:         "choice",
		Instructions: question,
		Criteria:     choices,
	}
	if len(choices) == 1 {
		q = systemOneQuestion{
			Type:         "noul",
			Instructions: fmt.Sprintf("%s Does it belong to tag %q?", question, choices[0]),
		}
	}
	payload := systemOneRequest{
		Model: c.Model,
		State: state,
		Questions: map[string]systemOneQuestion{
			"tag": q,
		},
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, ep := range []string{"/v1/systemone", "/v1/decisions", "/api/decide"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+ep, bytes.NewReader(rawBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollaya request failed: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("endpoint %s returned 404", ep)
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("ollaya %s status %d: %s", ep, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		scores, err := parseDecisionResponse(body, choices)
		if err != nil {
			lastErr = err
			continue
		}
		return scores, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("laya: no compatible decision endpoint responded")
}

// ClassifyAssetTags scores all existing tags in candidateTags against an asset's
// metadata, dimensions, dominant palette, and notes using a local Laya model.
func (c *Client) ClassifyAssetTags(
	ctx context.Context,
	asset model.Asset,
	candidateTags []model.Tag,
	autoThreshold float64,
	suggestThreshold float64,
) (ClassificationResult, error) {
	if len(candidateTags) == 0 {
		return ClassificationResult{}, nil
	}
	if autoThreshold <= 0 {
		autoThreshold = 0.78
	}
	if suggestThreshold <= 0 {
		suggestThreshold = 0.35
	}

	choices := make([]string, len(candidateTags))
	byName := make(map[string]model.Tag, len(candidateTags))
	for i, tg := range candidateTags {
		choices[i] = tg.Name
		byName[strings.ToLower(tg.Name)] = tg
	}

	state := BuildAssetContext(asset)
	question := "Which existing library tag best categorizes this creative reference asset?"
	scores, err := c.DecideChoices(ctx, state, question, choices)
	if err != nil {
		return ClassificationResult{}, err
	}

	var res ClassificationResult
	for _, sc := range scores {
		tg, ok := byName[strings.ToLower(sc.Choice)]
		if !ok {
			continue
		}
		item := TagSuggestion{Tag: tg, Probability: sc.Probability}
		res.AllScores = append(res.AllScores, item)
		switch {
		case sc.Probability >= autoThreshold:
			res.AutoApply = append(res.AutoApply, item)
		case sc.Probability >= suggestThreshold:
			res.Suggested = append(res.Suggested, item)
		}
	}
	sort.Slice(res.AutoApply, func(i, j int) bool { return res.AutoApply[i].Probability > res.AutoApply[j].Probability })
	sort.Slice(res.Suggested, func(i, j int) bool { return res.Suggested[i].Probability > res.Suggested[j].Probability })
	sort.Slice(res.AllScores, func(i, j int) bool { return res.AllScores[i].Probability > res.AllScores[j].Probability })
	return res, nil
}

// BuildAssetContext formats an asset's rich structured context for Laya scoring.
func BuildAssetContext(a model.Asset) string {
	var parts []string
	parts = append(parts, "filename: "+a.Label())
	if ext := strings.TrimPrefix(filepath.Ext(a.Path), "."); ext != "" {
		parts = append(parts, "ext: "+strings.ToLower(ext))
	}
	if a.Mime != "" {
		parts = append(parts, "mime: "+a.Mime)
	}
	if a.Width > 0 && a.Height > 0 {
		orient := "square"
		if a.Width > a.Height {
			orient = "landscape"
		} else if a.Height > a.Width {
			orient = "portrait"
		}
		parts = append(parts, fmt.Sprintf("dimensions: %dx%d (%s)", a.Width, a.Height, orient))
	}
	if len(a.Palette) > 0 {
		hexes := make([]string, 0, len(a.Palette))
		for _, p := range a.Palette {
			hexes = append(hexes, p.Hex)
		}
		parts = append(parts, "colors: "+strings.Join(hexes, ", "))
	}
	if a.SourceTitle != "" {
		parts = append(parts, "source_title: "+a.SourceTitle)
	}
	if a.SourceURL != "" {
		parts = append(parts, "source_url: "+a.SourceURL)
	}
	if a.Notes != "" {
		parts = append(parts, "notes: "+a.Notes)
	}
	return strings.Join(parts, "\n")
}

// parseDecisionResponse accepts normative Ollaya /v1/systemone responses:
//   - {"answers": {"tag": {"choice": "Poster", "confidence": 0.8, "probabilities": {"Poster": 0.88, ...}}}}
//   - {"answers": {"tag": {"noul": 0.82}}}
// as well as flat {"probabilities": {...}} / {"scores": {...}} / {"choices": [...]}.
func parseDecisionResponse(body []byte, choices []string) ([]ChoiceScore, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	// Normative Ollaya /v1/systemone & /api/decide answers envelope.
	if answers, ok := raw["answers"].(map[string]any); ok {
		for _, v := range answers {
			ans, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if m := extractProbMap(ans, "probabilities"); len(m) > 0 {
				return mapToSortedScores(m), nil
			}
			if noul, ok := ans["noul"].(float64); ok && len(choices) > 0 {
				return []ChoiceScore{{Choice: choices[0], Probability: noul}}, nil
			}
			if ch, ok := ans["choice"].(string); ok && ch != "" {
				conf := readFloat(ans, "confidence", "probability")
				if conf <= 0 {
					conf = 0.85
				}
				return []ChoiceScore{{Choice: ch, Probability: conf}}, nil
			}
		}
	}

	if m := extractProbMap(raw, "probabilities"); len(m) > 0 {
		return mapToSortedScores(m), nil
	}
	if m := extractProbMap(raw, "scores"); len(m) > 0 {
		return mapToSortedScores(m), nil
	}
	if list, ok := raw["choices"].([]any); ok && len(list) > 0 {
		var out []ChoiceScore
		for _, el := range list {
			obj, ok := el.(map[string]any)
			if !ok {
				continue
			}
			label, _ := obj["choice"].(string)
			if label == "" {
				label, _ = obj["label"].(string)
			}
			prob := readFloat(obj, "probability", "score", "confidence")
			if label != "" {
				out = append(out, ChoiceScore{Choice: label, Probability: prob})
			}
		}
		if len(out) > 0 {
			sort.Slice(out, func(i, j int) bool { return out[i].Probability > out[j].Probability })
			return out, nil
		}
	}
	if dec, ok := raw["decision"].(string); ok && dec != "" {
		conf := readFloat(raw, "confidence", "probability", "score")
		if conf <= 0 {
			conf = 0.85
		}
		return []ChoiceScore{{Choice: dec, Probability: conf}}, nil
	}
	if dec, ok := raw["choice"].(string); ok && dec != "" {
		conf := readFloat(raw, "confidence", "probability", "score")
		if conf <= 0 {
			conf = 0.85
		}
		return []ChoiceScore{{Choice: dec, Probability: conf}}, nil
	}
	return nil, fmt.Errorf("laya: unrecognized response format: %s", strings.TrimSpace(string(body)))
}

func extractProbMap(raw map[string]any, key string) map[string]float64 {
	sub, ok := raw[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]float64, len(sub))
	for k, v := range sub {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out
}

func readFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if f, ok := m[k].(float64); ok {
			return f
		}
	}
	return 0
}

func mapToSortedScores(m map[string]float64) []ChoiceScore {
	out := make([]ChoiceScore, 0, len(m))
	for k, v := range m {
		out = append(out, ChoiceScore{Choice: k, Probability: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Probability > out[j].Probability })
	return out
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
