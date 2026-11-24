package thumbnail

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

func TestServiceWritesJPEG(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "still.png")
	writePNG(t, src)

	d, err := db.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	asset := model.Asset{
		ID:          model.NewID(),
		Path:        src,
		StorageMode: model.StorageIndex,
		Hash:        "still",
		Mime:        "image/png",
		Width:       8,
		Height:      4,
		CreatedAt:   time.Now().UTC(),
	}
	if err := d.Update(func(tx *bbolt.Tx) error { return db.PutAsset(tx, asset) }); err != nil {
		t.Fatal(err)
	}

	done := make(chan string, 1)
	svc := Start(d, func(id string) { done <- id })
	defer svc.Close()
	svc.Enqueue(asset.ID)

	select {
	case id := <-done:
		if id != asset.ID {
			t.Fatalf("id %s", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("thumbnail timed out")
	}

	got, err := func() (model.Asset, error) {
		var out model.Asset
		err := d.View(func(tx *bbolt.Tx) error {
			var err error
			out, err = db.GetAsset(tx, asset.ID)
			return err
		})
		return out, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	if got.ThumbnailPath == "" {
		t.Fatal("empty thumbnail path")
	}
	info, err := os.Stat(got.ThumbnailPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("thumb file: %v", err)
	}
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	img.SetNRGBA(0, 0, color.NRGBA{R: 20, G: 40, B: 60, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("png %v %v", err, closeErr)
	}
}

func TestUnsupportedURLAssetDoesNotSpinOnDone(t *testing.T) {
	if CanThumbnail("text/plain; charset=utf-8", "video.url") {
		t.Fatal("expected .url text/plain asset not to be thumbnailable")
	}
	if !CanThumbnail("image/png", "still.png") {
		t.Fatal("expected image/png to be thumbnailable")
	}

	root := t.TempDir()
	urlPath := filepath.Join(root, "bookmark.url")
	if err := os.WriteFile(urlPath, []byte("[InternetShortcut]\r\nURL=https://example.com\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	asset := model.Asset{
		ID:          model.NewID(),
		Path:        urlPath,
		StorageMode: model.StorageCopy,
		Hash:        "urlhash",
		Mime:        "text/plain; charset=utf-8",
		CreatedAt:   time.Now().UTC(),
	}
	if err := d.Update(func(tx *bbolt.Tx) error { return db.PutAsset(tx, asset) }); err != nil {
		t.Fatal(err)
	}

	callbacks := make(chan string, 8)
	svc := Start(d, func(id string) { callbacks <- id })
	defer svc.Close()

	// Even if Prioritize is called repeatedly on a .url asset, onDone must NEVER fire.
	for i := 0; i < 5; i++ {
		svc.Prioritize(asset.ID)
	}
	select {
	case id := <-callbacks:
		t.Fatalf("unexpected onDone callback for non-thumbnailable .url asset %s (would cause UI spin loop)", id)
	case <-time.After(150 * time.Millisecond):
	}
}
