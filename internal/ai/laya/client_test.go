package laya

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragle/internal/model"
)

func TestClassifyAssetTagsWithMockOllaya(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			return
		case "/v1/decisions":
			var req decisionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode req: %v", err)
			}
			if len(req.Choices) != 3 {
				t.Fatalf("expected 3 choices, got %d", len(req.Choices))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"probabilities": map[string]float64{
					"Poster":     0.88,
					"Typography": 0.52,
					"3D":         0.08,
				},
			})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "laya")
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health failed: %v", err)
	}

	asset := model.Asset{
		ID:     "a1",
		Path:   "/lib/swiss_poster_2026.png",
		Mime:   "image/png",
		Width:  1200,
		Height: 1800,
		Palette: []model.PaletteColor{
			{Hex: "#FF5C33", Ratio: 0.45},
		},
	}
	tags := []model.Tag{
		{ID: "t1", Name: "Poster"},
		{ID: "t2", Name: "Typography"},
		{ID: "t3", Name: "3D"},
	}
	res, err := client.ClassifyAssetTags(context.Background(), asset, tags, 0.78, 0.35)
	if err != nil {
		t.Fatalf("ClassifyAssetTags failed: %v", err)
	}
	if len(res.AutoApply) != 1 || res.AutoApply[0].Tag.ID != "t1" {
		t.Fatalf("expected Poster in AutoApply, got %+v", res.AutoApply)
	}
	if len(res.Suggested) != 1 || res.Suggested[0].Tag.ID != "t2" {
		t.Fatalf("expected Typography in Suggested, got %+v", res.Suggested)
	}
}
