package library

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"raglex/internal/model"
)

func writePNG(t *testing.T, path string, mark byte) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	img.SetNRGBA(0, 0, color.NRGBA{R: mark, G: 255 - mark, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportCopyMoveIndexAndDedup(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	original := filepath.Join(srcDir, "shot.png")
	writePNG(t, original, 1)
	before, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := lib.Import(context.Background(), []string{original}, model.StorageCopy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Added != 1 {
		t.Fatalf("copy result %+v", copied)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatal("copy removed the source")
	}
	got, err := lib.Get(copied.Items[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 8 || got.Height != 4 {
		t.Fatalf("size %d %d", got.Width, got.Height)
	}
	stored, err := os.ReadFile(got.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, before) {
		t.Fatal("copied bytes differ")
	}

	again, err := lib.Import(context.Background(), []string{original}, model.StorageCopy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.Existing != 1 || again.Added != 0 {
		t.Fatalf("dedup %+v", again)
	}

	other := filepath.Join(srcDir, "move-me.png")
	writePNG(t, other, 2)
	moved, err := lib.Import(context.Background(), []string{other}, model.StorageMove, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Added != 1 {
		t.Fatalf("move %+v", moved)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("moved source still statable: %v", err)
	}

	indexed := filepath.Join(srcDir, "index.png")
	writePNG(t, indexed, 3)
	indexedResult, err := lib.Import(context.Background(), []string{indexed}, model.StorageIndex, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := lib.Get(indexedResult.Items[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.Path != indexed {
		t.Fatalf("index path %s", asset.Path)
	}
	if _, err := os.Stat(indexed); err != nil {
		t.Fatal(err)
	}
}

func TestSymlinkFailureLeavesSource(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	src := filepath.Join(t.TempDir(), "link.png")
	writePNG(t, src, 4)
	// A destination whose parent is a file cannot receive a symlink.
	blocked := filepath.Join(root, "files")
	if err := os.RemoveAll(blocked); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := lib.Import(context.Background(), []string{src}, model.StorageSymlink, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("source disappeared")
	}
	if result.Failed != 1 || !errors.Is(result.Items[0].Err, ErrSymlink) {
		t.Fatalf("result %+v err %v", result, result.Items[0].Err)
	}
	if _, err := os.Stat(filepath.Join(root, "files", "link.png")); err == nil {
		t.Fatalf("symlink failure left a file")
	}
}

func TestTagFilterAndDeleteKeepsAsset(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	tag, err := lib.CreateTag(" 參考 ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.CreateTag("參考"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("duplicate err %v", err)
	}
	src := filepath.Join(t.TempDir(), "a.png")
	writePNG(t, src, 4)
	imported, err := lib.Import(context.Background(), []string{src}, model.StorageIndex, []string{tag.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := lib.List(Filter{TagID: tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("tagged %d", len(listed))
	}
	if err := lib.SetRating(imported.Items[0].AssetID, 9); err != nil {
		t.Fatal(err)
	}
	high, err := lib.List(Filter{MinRating: 5})
	if err != nil || len(high) != 1 || high[0].Rating != 5 {
		t.Fatalf("rating %+v %v", high, err)
	}
	if err := lib.DeleteTag(tag.ID); err != nil {
		t.Fatal(err)
	}
	all, err := lib.List(Filter{})
	if err != nil || len(all) != 1 {
		t.Fatalf("asset gone with the tag: %+v %v", all, err)
	}
}

func TestDeleteCopyRemovesOwnedFileAndIndexKeepsIt(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	src := filepath.Join(t.TempDir(), "own.png")
	writePNG(t, src, 4)
	copied, err := lib.Import(context.Background(), []string{src}, model.StorageCopy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := lib.Get(copied.Items[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.Delete(asset.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(asset.Path); !os.IsNotExist(err) {
		t.Fatalf("owned copy remains: %v", err)
	}

	indexedPath := filepath.Join(t.TempDir(), "keep.png")
	writePNG(t, indexedPath, 7)
	indexed, err := lib.Import(context.Background(), []string{indexedPath}, model.StorageIndex, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.Delete(indexed.Items[0].AssetID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(indexedPath); err != nil {
		t.Fatal("index delete removed the user's file")
	}
}

func TestRelinkByHash(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.png")
	writePNG(t, src, 4)
	imported, err := lib.Import(context.Background(), []string{src}, model.StorageIndex, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if changed, err := lib.MarkMissing(src); err != nil || !changed {
		t.Fatalf("mark missing %v %v", changed, err)
	}
	moved := filepath.Join(dir, "clip.png")
	writePNG(t, moved, 4)
	ok, err := lib.TryRelink(moved)
	if err != nil || !ok {
		t.Fatalf("relink %v %v", ok, err)
	}
	asset, err := lib.Get(imported.Items[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.Missing || asset.Path != moved {
		t.Fatalf("asset %+v", asset)
	}
	ok, err = lib.TryRelink(moved)
	if err != nil || ok {
		t.Fatalf("second relink hashed with nothing missing: %v %v", ok, err)
	}
}

func TestImportSkipsLibraryInternals(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	writePNG(t, filepath.Join(root, "files", "already.png"), 8)
	writePNG(t, filepath.Join(root, "outside.png"), 9)
	// outside.png is inside the library root but not in files/.
	result, err := lib.Import(context.Background(), []string{root}, model.StorageIndex, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 {
		t.Fatalf("added %d items %+v", result.Added, result.Items)
	}
}

func TestDeleteBatchPreservesOriginals(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	src := t.TempDir()
	idxSrc := filepath.Join(src, "indexed.png")
	copySrc := filepath.Join(src, "copied.png")
	writePNG(t, idxSrc, 12)
	writePNG(t, copySrc, 14)

	r1, err := lib.Import(context.Background(), []string{idxSrc}, model.StorageIndex, nil, nil)
	if err != nil || r1.Added != 1 {
		t.Fatalf("index import failed: %+v %v", r1, err)
	}
	r2, err := lib.Import(context.Background(), []string{copySrc}, model.StorageCopy, nil, nil)
	if err != nil || r2.Added != 1 {
		t.Fatalf("copy import failed: %+v %v", r2, err)
	}

	copiedAsset, err := lib.Get(r2.Items[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}

	n, err := lib.DeleteBatch([]string{r1.Items[0].AssetID, r2.Items[0].AssetID})
	if err != nil || n != 2 {
		t.Fatalf("DeleteBatch got n=%d err=%v", n, err)
	}

	// Original source files MUST stay untouched!
	if _, err := os.Stat(idxSrc); err != nil {
		t.Fatalf("indexed original file was unexpectedly removed: %v", err)
	}
	if _, err := os.Stat(copySrc); err != nil {
		t.Fatalf("copy source file was unexpectedly removed: %v", err)
	}
	// Library-owned copy inside files/ MUST be removed.
	if _, err := os.Stat(copiedAsset.Path); !os.IsNotExist(err) {
		t.Fatalf("expected library-owned copy to be removed, stat err=%v", err)
	}
	remaining, err := lib.List(Filter{})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("expected 0 remaining assets, got %d err=%v", len(remaining), err)
	}
}

func TestReorderTagsPersistsCustomOrder(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	a, err := lib.CreateTag("Alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, err := lib.CreateTag("Beta")
	if err != nil {
		t.Fatal(err)
	}
	c, err := lib.CreateTag("Gamma")
	if err != nil {
		t.Fatal(err)
	}

	// Reorder root tags to [Gamma, Alpha, Beta].
	if err := lib.ReorderTags("", []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	tags, err := lib.Tags()
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 || tags[0].ID != c.ID || tags[1].ID != a.ID || tags[2].ID != b.ID {
		t.Fatalf("unexpected tag order: %+v", tags)
	}

	// Reorder also supports moving a tag under a new parent cleanly.
	if err := lib.ReorderTags(c.ID, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	tags, err = lib.Tags()
	if err != nil {
		t.Fatal(err)
	}
	var gotB model.Tag
	for _, tg := range tags {
		if tg.ID == b.ID {
			gotB = tg
		}
	}
	if gotB.ParentID != c.ID || gotB.Order <= 0 {
		t.Fatalf("expected Beta reparented under Gamma with positive Order, got %+v", gotB)
	}
}
