package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"raglex/internal/library"
	"raglex/internal/project"
)

func makePNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUnifiedServerAPIWebAndSignaling(t *testing.T) {
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	projs := project.New(lib.DB)

	changed := 0
	srv := New(Options{
		Library:  lib,
		Projects: projs,
		Token:    "secret123",
		OnChange: func() { changed++ },
	})
	addr, err := srv.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()
	base := "http://" + addr

	// 1. Embedded signaling healthz.
	resp, err := http.Get(base + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("signaling /healthz failed: %v", err)
	}
	_ = resp.Body.Close()

	// 2. Mutating route without token is rejected.
	payload, _ := json.Marshal(map[string]any{
		"filename":    "clip.png",
		"data_base64": base64.StdEncoding.EncodeToString(makePNG(t, 64, 64, color.RGBA{255, 92, 51, 255})),
		"source_url":  "https://example.com/art",
		"source_title": "Orange Study",
	})
	resp, err = http.Post(base+"/api/v1/import/base64", "application/json", bytes.NewReader(payload))
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %v %v", resp.StatusCode, err)
	}
	_ = resp.Body.Close()

	// 3. Mutating route WITH token imports asset and triggers OnChange.
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/import/base64", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Raglex-Token", "secret123")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 on import, got %v %v", resp.StatusCode, err)
	}
	var impResp struct {
		Added bool     `json:"added"`
		Asset apiAsset `json:"asset"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&impResp)
	_ = resp.Body.Close()
	if !impResp.Added || impResp.Asset.ID == "" || changed == 0 {
		t.Fatalf("unexpected import response: %+v changed=%d", impResp, changed)
	}

	// 4. Create standalone board via API with the imported asset.
	bBody, _ := json.Marshal(map[string]any{
		"name":      "Web Mood",
		"asset_ids": []string{impResp.Asset.ID},
	})
	req, _ = http.NewRequest(http.MethodPost, base+"/api/v1/boards", bytes.NewReader(bBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret123")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create board failed: %v %v", resp.StatusCode, err)
	}
	var boardResp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&boardResp)
	_ = resp.Body.Close()

	// 5. Web Gallery & Interactive Board HTML render cleanly.
	resp, err = http.Get(base + "/")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("web index failed: %v", err)
	}
	var htmlBuf bytes.Buffer
	_, _ = htmlBuf.ReadFrom(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(htmlBuf.String(), "clip.png") || !strings.Contains(htmlBuf.String(), "Web Mood") {
		t.Fatalf("web index missing expected content")
	}

	resp, err = http.Get(base + "/board/" + boardResp.ID)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("web board failed: %v", err)
	}
	_ = resp.Body.Close()

	// 6. Multi-source companion clip (/api/v1/import/source) + auto-stack + board attachment.
	srcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/post":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><script type="application/ld+json">{
				"@type":"CreativeWork","name":"Clip Gallery","image":["/p1.png","/p2.png"]
			}</script></head><body></body></html>`))
		case "/p1.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(makePNG(t, 16, 16, color.RGBA{255, 92, 51, 255}))
		case "/p2.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(makePNG(t, 16, 16, color.RGBA{56, 189, 248, 255}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srcSrv.Close()

	srcBody, _ := json.Marshal(map[string]any{
		"url":        srcSrv.URL + "/post",
		"board_id":   boardResp.ID,
		"auto_stack": true,
	})
	req, _ = http.NewRequest(http.MethodPost, base+"/api/v1/import/source", bytes.NewReader(srcBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Raglex-Token", "secret123")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("import source failed: err=%v status=%v", err, resp.StatusCode)
	}
	var srcResp struct {
		Added  int `json:"added"`
		Stacks int `json:"stacks"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&srcResp)
	_ = resp.Body.Close()
	if srcResp.Added != 2 || srcResp.Stacks != 1 {
		t.Fatalf("expected 2 assets + 1 auto stack from /api/v1/import/source, got %+v", srcResp)
	}
}
