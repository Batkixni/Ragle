// Package thumbnail builds previews and probes video with an external ffmpeg.
package thumbnail

import (
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"

	"github.com/disintegration/imaging"

	"ragle/internal/db"
	"ragle/internal/model"
)

const longEdge = 512

// Service fills ThumbnailPath in the background. At most two files are
// encoded at once. onDone runs after a path is stored; it may be nil.
type Service struct {
	db     *db.DB
	jobs   chan string
	onDone func(assetID string)
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Start launches the workers. Call Close before closing the database.
func Start(d *db.DB, onDone func(assetID string)) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		db:     d,
		jobs:   make(chan string, 64),
		onDone: onDone,
		ctx:    ctx,
		cancel: cancel,
	}
	for range 2 {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

// Enqueue asks for a preview of assetID. A full queue is drained from a
// goroutine so the caller, including the UI thread, does not wait.
func (s *Service) Enqueue(assetID string) {
	if s == nil || assetID == "" {
		return
	}
	select {
	case <-s.ctx.Done():
		return
	case s.jobs <- assetID:
		return
	default:
	}
	go func() {
		select {
		case <-s.ctx.Done():
		case s.jobs <- assetID:
		}
	}()
}

// Close stops the workers and waits for the current files to finish.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

func (s *Service) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case id := <-s.jobs:
			if s.ctx.Err() != nil {
				return
			}
			if err := s.build(id); err != nil {
				continue
			}
			if s.onDone != nil {
				s.onDone(id)
			}
		}
	}
}

func (s *Service) build(id string) error {
	var asset model.Asset
	err := s.db.View(func(tx *dbTx) error {
		var err error
		asset, err = db.GetAsset(tx, id)
		return err
	})
	if err != nil {
		return err
	}
	if asset.Missing {
		return nil
	}
	hasThumb := asset.ThumbnailPath != ""
	hasVisual := len(asset.Palette) > 0 && (asset.PHash != 0 || asset.DHash != 0)
	if hasThumb && hasVisual {
		return nil
	}

	dest := asset.ThumbnailPath
	if dest == "" {
		dest = filepath.Join(s.db.ThumbsDir(), asset.ID+".jpg")
	}
	var ph, dh uint64
	var pal []model.PaletteColor

	switch model.MediaKind(asset.Mime) {
	case model.MediaImage:
		var fitted image.Image
		if !hasThumb {
			var err error
			fitted, err = imageThumbAndExtract(asset.Path, dest)
			if err != nil {
				return err
			}
		} else {
			if img, err := imaging.Open(dest); err == nil {
				fitted = img
			} else if img, err := imaging.Open(asset.Path, imaging.AutoOrientation(true)); err == nil {
				fitted = img
			}
		}
		if fitted != nil {
			ph, dh, pal = ExtractVisualFromImage(fitted)
		}
	case model.MediaVideo:
		if !hasThumb {
			if err := ExtractFrame(asset.Path, dest); err != nil {
				return err
			}
		}
		if img, err := imaging.Open(dest); err == nil {
			ph, dh, pal = ExtractVisualFromImage(img)
		}
	default:
		return nil
	}
	return s.db.Update(func(tx *dbTx) error {
		cur, err := db.GetAsset(tx, id)
		if err != nil {
			return err
		}
		cur.ThumbnailPath = dest
		if len(pal) > 0 {
			cur.Palette = pal
		}
		if ph != 0 || dh != 0 {
			cur.PHash = ph
			cur.DHash = dh
		}
		return db.PutAsset(tx, cur)
	})
}

func imageThumbAndExtract(src, dest string) (image.Image, error) {
	img, err := imaging.Open(src, imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}
	fitted := imaging.Fit(img, longEdge, longEdge, imaging.Lanczos)
	if err := saveJPEG(fitted, dest); err != nil {
		return nil, err
	}
	return fitted, nil
}

func saveJPEG(img image.Image, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 82})
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(dest)
		return err
	}
	return closeErr
}

// dbTx is *bbolt.Tx as accepted by the db helpers. Declared in tx.go.
type dbTx = txAlias
