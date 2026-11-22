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
	if asset.ThumbnailPath != "" || asset.Missing {
		return nil
	}
	dest := filepath.Join(s.db.ThumbsDir(), asset.ID+".jpg")
	switch model.MediaKind(asset.Mime) {
	case model.MediaImage:
		if err := imageThumb(asset.Path, dest); err != nil {
			return err
		}
	case model.MediaVideo:
		if err := ExtractFrame(asset.Path, dest); err != nil {
			return err
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
		return db.PutAsset(tx, cur)
	})
}

func imageThumb(src, dest string) error {
	img, err := imaging.Open(src, imaging.AutoOrientation(true))
	if err != nil {
		return err
	}
	fitted := imaging.Fit(img, longEdge, longEdge, imaging.Lanczos)
	return saveJPEG(fitted, dest)
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
