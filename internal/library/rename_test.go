package library

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"raglex/internal/model"
)

func makeTestPNGBytes(t *testing.T, w, h int, c color.RGBA) []byte {
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

func TestBatchRenamePreviewApplyAndUndo(t *testing.T) {
	dir := t.TempDir()
	lib, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	tg, err := lib.CreateTag("Poster")
	if err != nil {
		t.Fatal(err)
	}
	a1, added1, err := lib.ImportBytes("alpha.png", makeTestPNGBytes(t, 120, 80, color.RGBA{255, 92, 51, 255}), []string{tg.ID}, "", "")
	if err != nil || !added1 {
		t.Fatalf("import a1: %v added=%v", err, added1)
	}
	a2, added2, err := lib.ImportBytes("beta.png", makeTestPNGBytes(t, 64, 64, color.RGBA{40, 120, 220, 255}), []string{tg.ID}, "", "")
	if err != nil || !added2 {
		t.Fatalf("import a2: %v added=%v", err, added2)
	}

	spec := RenameSpec{
		Template:       "{tag}_{index:03}_{w}x{h}",
		CaseMode:       "lower",
		StartIndex:     1,
		RenameDiskFile: true,
	}
	preview, err := lib.PreviewBatchRename([]string{a1.ID, a2.ID}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 2 {
		t.Fatalf("expected 2 previews, got %d", len(preview))
	}
	if preview[0].NewName != "poster_001_120x80.png" || preview[1].NewName != "poster_002_64x64.png" {
		t.Fatalf("unexpected preview names: %q, %q", preview[0].NewName, preview[1].NewName)
	}

	batch, err := lib.ApplyBatchRename([]string{a1.ID, a2.ID}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Entries) != 2 {
		t.Fatalf("expected 2 applied entries, got %d", len(batch.Entries))
	}
	got1, _ := lib.Get(a1.ID)
	if got1.Label() != "poster_001_120x80.png" {
		t.Fatalf("expected renamed label, got %q", got1.Label())
	}
	if _, err := os.Stat(got1.Path); err != nil {
		t.Fatalf("expected renamed file on disk at %s: %v", got1.Path, err)
	}

	// Test overlapping circular rename on disk (swap 001 -> 002, 002 -> 001).
	swapSpec := RenameSpec{
		Template:       "swap_{index:02}",
		StartIndex:     1,
		RenameDiskFile: true,
	}
	if _, err := lib.ApplyBatchRename([]string{a2.ID, a1.ID}, swapSpec); err != nil {
		t.Fatalf("swap batch rename failed: %v", err)
	}
	if _, err := lib.ApplyBatchRename([]string{a1.ID, a2.ID}, swapSpec); err != nil {
		t.Fatalf("overlapping circular swap rename failed: %v", err)
	}

	// Undo restores previous names.
	reverted, err := lib.UndoLastBatchRename()
	if err != nil || reverted != 2 {
		t.Fatalf("undo failed: reverted=%d err=%v", reverted, err)
	}
	got1After, _ := lib.Get(a1.ID)
	if got1After.Label() != "swap_02.png" {
		t.Fatalf("expected swap_02.png after undo, got %q", got1After.Label())
	}
}

func TestBatchRenameConflictAndExternalSafety(t *testing.T) {
	dir := t.TempDir()
	extDir := t.TempDir()
	lib, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	extFile := filepath.Join(extDir, "external.png")
	if err := os.WriteFile(extFile, makeTestPNGBytes(t, 40, 40, color.RGBA{10, 200, 100, 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := lib.Import(context.Background(), []string{extFile}, model.StorageIndex, nil, nil)
	if err != nil || res.Added != 1 {
		t.Fatalf("import external failed: %v", err)
	}
	id := res.Items[0].AssetID

	// With RenameDiskFile=true and IncludeExternal=false, external file on disk stays untouched,
	// while DisplayName updates safely.
	if _, err := lib.ApplyBatchRename([]string{id}, RenameSpec{
		Prefix:          "curated_",
		RenameDiskFile:  true,
		IncludeExternal: false,
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.Get(id)
	if got.Label() != "curated_external.png" {
		t.Fatalf("expected DisplayName curated_external.png, got %q", got.Label())
	}
	if got.Path != extFile {
		t.Fatalf("external disk path should remain untouched, got %q", got.Path)
	}
}

func TestBatchExportConvertAndResize(t *testing.T) {
	dir := t.TempDir()
	outDir := t.TempDir()
	lib, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	a, _, err := lib.ImportBytes("hero.png", makeTestPNGBytes(t, 400, 200, color.RGBA{255, 92, 51, 255}), nil, "https://example.com", "Hero Page")
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceURL != "https://example.com" || a.SourceTitle != "Hero Page" {
		t.Fatalf("source metadata not saved: %+v", a)
	}

	res, err := lib.ExportBatch([]string{a.ID}, ExportOptions{
		DestDir: outDir,
		Format:  "jpg",
		Quality: 85,
		MaxEdge: 100,
	})
	if err != nil || res.Exported != 1 || len(res.Paths) != 1 {
		t.Fatalf("export failed: %+v err=%v", res, err)
	}
	if filepath.Ext(res.Paths[0]) != ".jpg" {
		t.Fatalf("expected .jpg output, got %s", res.Paths[0])
	}
	f, err := os.Open(res.Paths[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 100 || cfg.Height != 50 {
		t.Fatalf("expected 100x50 resized export, got %dx%d", cfg.Width, cfg.Height)
	}
}
