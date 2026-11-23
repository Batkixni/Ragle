package library

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"raglex/internal/model"
)

func TestExtractWebMeta_OGImage(t *testing.T) {
	htmlDoc := []byte(`<!doctype html><html><head>
		<title>Fallback Title</title>
		<meta property="og:title" content="Swiss Editorial Poster &amp; Grid">
		<meta property="og:description" content="A crisp visual study.">
		<meta property="og:image" content="/assets/hero.png">
	</head><body></body></html>`)

	meta := ExtractWebMeta("https://example.com/posts/123", htmlDoc)
	if meta.ImageURL != "https://example.com/assets/hero.png" {
		t.Fatalf("ImageURL=%q, want resolved absolute URL", meta.ImageURL)
	}
	if meta.Title != "Swiss Editorial Poster & Grid" {
		t.Fatalf("Title=%q", meta.Title)
	}
	if meta.Description != "A crisp visual study." {
		t.Fatalf("Description=%q", meta.Description)
	}
}

func TestImportFromURL_OGImageEndToEnd(t *testing.T) {
	var pngBuf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 92, B: 51, A: 255})
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="Neon Reference"><meta property="og:image" content="/img/neon.png"><meta property="og:description" content="Mood study"></head></html>`))
		case "/img/neon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	asset, err := lib.ImportFromURL(context.Background(), srv.URL+"/article", model.StorageCopy)
	if err != nil {
		t.Fatalf("ImportFromURL: %v", err)
	}
	if asset.Width != 32 || asset.Height != 24 {
		t.Fatalf("got %dx%d, want 32x24", asset.Width, asset.Height)
	}
	if asset.SourceURL != srv.URL+"/article" {
		t.Fatalf("SourceURL=%q", asset.SourceURL)
	}
	if !asset.IsBookmark {
		t.Fatalf("expected HTML page import to be marked IsBookmark=true")
	}
	if asset.DisplayName != "Neon Reference" {
		t.Fatalf("DisplayName=%q, want Neon Reference", asset.DisplayName)
	}
	if asset.SourceTitle != "Neon Reference" || asset.Notes != "Mood study" {
		t.Fatalf("SourceTitle=%q Notes=%q", asset.SourceTitle, asset.Notes)
	}
}
