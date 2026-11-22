package library

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func makePatternPNGBytes(t *testing.T, w, h int, fn func(x, y int) color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, fn(x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReverseImageSearchAndColorSearch(t *testing.T) {
	dir := t.TempDir()
	lib, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	// Asset 1 & 2: very similar orange diagonal gradient (different sizes -> different SHA-256).
	a1, _, err := lib.ImportBytes("orange_hi.png", makePatternPNGBytes(t, 160, 120, func(x, y int) color.RGBA {
		if x < 80 {
			return color.RGBA{255, 92, 51, 255}
		}
		return color.RGBA{24, 24, 27, 255}
	}), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	a2, _, err := lib.ImportBytes("orange_lo.png", makePatternPNGBytes(t, 80, 60, func(x, y int) color.RGBA {
		if x < 40 {
			return color.RGBA{250, 96, 54, 255}
		}
		return color.RGBA{26, 26, 30, 255}
	}), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Asset 3: completely different bright teal checker pattern.
	a3, _, err := lib.ImportBytes("teal_grid.png", makePatternPNGBytes(t, 120, 120, func(x, y int) color.RGBA {
		if ((x / 15) + (y / 15)) % 2 == 0 {
			return color.RGBA{16, 185, 129, 255}
		}
		return color.RGBA{240, 245, 250, 255}
	}), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Ensure visual features extracted for all 3.
	for _, id := range []string{a1.ID, a2.ID, a3.ID} {
		got, err := lib.EnsureVisualFeatures(id)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Palette) == 0 {
			t.Fatalf("expected palette extracted for %s", id)
		}
	}

	// Reverse image search from a1 should rank a2 first with high similarity!
	matches, err := lib.FindSimilarByAsset(a1.ID, 0.5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 || matches[0].Asset.ID != a2.ID {
		t.Fatalf("expected a2 (%s) as top similar match, got %+v", a2.ID, matches)
	}

	// Color search for #FF5C33 should find orange assets and rank them before teal.
	colorHits, err := lib.SearchByColor("#FF5C33", 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if len(colorHits) < 2 {
		t.Fatalf("expected >= 2 orange color hits, got %d", len(colorHits))
	}
	for _, h := range colorHits {
		if h.Asset.ID == a3.ID {
			t.Fatalf("teal asset should not match #FF5C33 within 0.25 OKLab distance")
		}
	}
}
