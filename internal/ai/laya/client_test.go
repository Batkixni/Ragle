package laya

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"raglex/internal/model"
)

func TestClassifyAssetTagsWithNormativeOllaya(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/api/version":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ollaya is running"))
			return
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{{"name": "laya:multilingual"}},
			})
			return
		case "/api/pull":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte("{\"status\":\"downloading\",\"total\":100,\"completed\":50}\n{\"status\":\"success\",\"total\":100,\"completed\":100}\n"))
			return
		case "/v1/systemone":
			var req systemOneRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode req: %v", err)
			}
			if _, ok := req.Questions["tag"]; !ok {
				t.Fatalf("expected 'tag' question, got %+v", req.Questions)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": req.Model,
				"answers": map[string]any{
					"tag": map[string]any{
						"choice":     "Poster",
						"confidence": 0.82,
						"probabilities": map[string]float64{
							"Poster":     0.88,
							"Typography": 0.52,
							"3D":         0.08,
						},
					},
				},
			})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, DefaultModelMultilingual)
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	models, err := client.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0] != "laya:multilingual" {
		t.Fatalf("ListModels got %+v err=%v", models, err)
	}
	var lastPct float64
	if err := client.PullModel(context.Background(), "laya:multilingual", func(_ string, pct float64) { lastPct = pct }); err != nil {
		t.Fatalf("PullModel failed: %v", err)
	}
	if lastPct != 1.0 {
		t.Fatalf("expected pull pct 1.0, got %v", lastPct)
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
