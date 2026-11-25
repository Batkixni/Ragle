// Package thumbnail builds previews and probes video with an external ffmpeg.
package thumbnail

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/disintegration/imaging"

	"raglex/internal/ai/embed"
	"raglex/internal/db"
	"raglex/internal/model"
	"raglex/internal/preview"
)

const longEdge = 512

// CanThumbnail reports whether the background thumbnail service can produce
// a raster thumbnail or visual index for an asset with the given mime/path.
// Plain text, .url bookmarks, archives, and SVGs (rendered directly as vectors)
// return false so the UI never queues no-op thumbnail jobs on every frame.
func CanThumbnail(mime, path string) bool {
	if model.IsSVG(mime, path) {
		return false
	}
	return model.IsIllustrator(mime, path) ||
		model.IsExtraCreativeImage(mime, path) ||
		model.MediaKind(mime) == model.MediaImage ||
		model.MediaKind(mime) == model.MediaVideo ||
		model.IsFont(mime, path) ||
		model.IsAudio(mime, path) ||
		model.IsPDF(mime, path)
}

// maxHeavyDecodes caps how many memory-hungry decodes (Camera RAW, PSD, EPS,
// Illustrator...) run at once. Each one can hold hundreds of MB while it runs,
// so letting all workers take them at once during a big RAW import pushed the
// process into gigabytes of commit and made Windows page.
const maxHeavyDecodes = 2

// Service fills ThumbnailPath in the background. At most four files are
// encoded at once, and at most maxHeavyDecodes of them may be heavy creative
// formats. onDone runs after a path is stored; it may be nil.
type Service struct {
	db     *db.DB
	jobs   chan string
	onDone func(assetID string)
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	heavy      chan struct{}
	heavyDirty atomic.Bool // a heavy decode ran since memory was last returned

	mu        sync.Mutex
	pending   []string
	queued    map[string]bool
	attempted map[string]bool
	wake      chan struct{}
}

// Start launches the workers. Call Close before closing the database.
func Start(d *db.DB, onDone func(assetID string)) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		db:        d,
		jobs:      make(chan string, 128),
		onDone:    onDone,
		ctx:       ctx,
		cancel:    cancel,
		queued:    make(map[string]bool),
		attempted: make(map[string]bool),
		wake:      make(chan struct{}, 1),
		heavy:     make(chan struct{}, maxHeavyDecodes),
	}
	s.wg.Add(1)
	go s.feeder()
	for range 4 {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

func (s *Service) feeder() {
	defer s.wg.Done()
	for {
		s.mu.Lock()
		if len(s.pending) == 0 {
			s.mu.Unlock()
			select {
			case <-s.ctx.Done():
				return
			case <-s.wake:
				continue
			}
		}
		id := s.pending[0]
		s.pending[0] = ""
		s.pending = s.pending[1:]
		if len(s.pending) == 0 {
			s.pending = nil
		}
		s.mu.Unlock()

		select {
		case <-s.ctx.Done():
			return
		case s.jobs <- id:
		}
	}
}

// Reset clears the attempted-asset memory so failed or rebuilt thumbnails can
// be queued again (e.g. after "Rebuild all thumbnails").
func (s *Service) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clear(s.attempted)
	s.mu.Unlock()
}

// Enqueue asks for a preview of assetID without ever blocking or spawning
// unbounded goroutines even on 250,000+ item libraries.
func (s *Service) Enqueue(assetID string) {
	if s == nil || assetID == "" {
		return
	}
	s.mu.Lock()
	if s.queued[assetID] || s.attempted[assetID] {
		s.mu.Unlock()
		return
	}
	s.queued[assetID] = true
	s.pending = append(s.pending, assetID)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Prioritize moves or inserts assetID at the front of the pending queue so
// currently visible viewport cards get their thumbnails generated first.
func (s *Service) Prioritize(assetID string) {
	if s == nil || assetID == "" {
		return
	}
	s.mu.Lock()
	if !s.queued[assetID] && !s.attempted[assetID] {
		s.queued[assetID] = true
		s.pending = append([]string{assetID}, s.pending...)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
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
			changed, err := s.build(id)
			s.mu.Lock()
			delete(s.queued, id)
			s.attempted[id] = true
			idle := len(s.pending) == 0 && len(s.jobs) == 0
			s.mu.Unlock()
			if idle && s.heavyDirty.CompareAndSwap(true, false) {
				// The batch is done: hand the decode buffers back to Windows
				// now instead of keeping the peak working set for minutes.
				debug.FreeOSMemory()
			}
			if err != nil || !changed {
				continue
			}
			if s.onDone != nil {
				s.onDone(id)
			}
		}
	}
}

func (s *Service) build(id string) (bool, error) {
	var asset model.Asset
	err := s.db.View(func(tx *dbTx) error {
		var err error
		asset, err = db.GetAsset(tx, id)
		return err
	})
	if err != nil {
		return false, err
	}
	if asset.Missing {
		return false, nil
	}
	hasThumb := asset.ThumbnailPath != ""
	hasVisual := len(asset.Palette) > 0 && (asset.PHash != 0 || asset.DHash != 0)
	if hasThumb && hasVisual {
		return false, nil
	}
	if !hasThumb && isHeavyDecode(asset) {
		select {
		case s.heavy <- struct{}{}:
		case <-s.ctx.Done():
			return false, s.ctx.Err()
		}
		defer func() { <-s.heavy }()
		s.heavyDirty.Store(true)
	}

	dest := asset.ThumbnailPath
	if dest == "" {
		dest = filepath.Join(s.db.ThumbsDir(), asset.ID+".jpg")
	}
	var ph, dh uint64
	var pal []model.PaletteColor
	var vec []float32
	var decodedW, decodedH int

	switch {
	case model.IsSVG(asset.Mime, asset.Path):
		// SVG is rendered losslessly by the UI vector engine on demand.
		return false, nil
	case model.IsIllustrator(asset.Mime, asset.Path):
		if !hasThumb {
			img, err := preview.DecodeCreativeImage(asset.Path, longEdge)
			if err != nil {
				return false, err
			}
			fitted := imaging.Fit(img, longEdge, longEdge, imaging.Lanczos)
			savedPath, err := saveSmartThumb(fitted, s.db.ThumbsDir(), asset.ID)
			if err != nil {
				return false, err
			}
			dest = savedPath
			ph, dh, pal = ExtractVisualFromImage(fitted)
			vec = embed.ExtractFromImage(fitted)
		}
	case model.IsExtraCreativeImage(asset.Mime, asset.Path):
		var fitted image.Image
		if !hasThumb {
			img, err := preview.DecodeCreativeImage(asset.Path, longEdge)
			if err != nil {
				return false, err
			}
			if model.IsCameraRAW(asset.Path) {
				// RAW decodes are now shrunk to the thumbnail size, so read
				// the real dimensions from the preview header instead.
				decodedW, decodedH = preview.CameraRAWImageSize(asset.Path)
			} else {
				decodedW = img.Bounds().Dx()
				decodedH = img.Bounds().Dy()
			}
			fitted = imaging.Fit(img, longEdge, longEdge, imaging.Lanczos)
			savedPath, err := saveSmartThumb(fitted, s.db.ThumbsDir(), asset.ID)
			if err != nil {
				return false, err
			}
			dest = savedPath
		} else if img, err := imaging.Open(dest); err == nil {
			fitted = img
		}
		if fitted != nil {
			ph, dh, pal = ExtractVisualFromImage(fitted)
			vec = embed.ExtractFromImage(fitted)
		}
	case model.MediaKind(asset.Mime) == model.MediaImage:
		var fitted image.Image
		if !hasThumb {
			var err error
			var savedPath string
			fitted, savedPath, err = imageThumbAndExtract(asset.Path, s.db.ThumbsDir(), asset.ID)
			if err != nil {
				return false, err
			}
			dest = savedPath
		} else {
			if img, err := imaging.Open(dest); err == nil {
				fitted = img
			} else if img, err := imaging.Open(asset.Path, imaging.AutoOrientation(true)); err == nil {
				fitted = img
			}
		}
		if fitted != nil {
			ph, dh, pal = ExtractVisualFromImage(fitted)
			vec = embed.ExtractFromImage(fitted)
		}
	case model.MediaKind(asset.Mime) == model.MediaVideo:
		if !hasThumb {
			if err := ExtractFrame(asset.Path, dest); err != nil {
				return false, err
			}
		}
		if img, err := imaging.Open(dest); err == nil {
			ph, dh, pal = ExtractVisualFromImage(img)
			vec = embed.ExtractFromImage(img)
		}
	case model.IsFont(asset.Mime, asset.Path):
		if !hasThumb {
			img, err := preview.RenderFontThumbnail(asset.Path, color.RGBA{244, 244, 245, 255}, color.RGBA{26, 26, 30, 255})
			if err != nil {
				return false, err
			}
			if err := saveJPEG(img, dest); err != nil {
				return false, err
			}
		}
	case model.IsAudio(asset.Mime, asset.Path):
		if !hasThumb {
			wf, err := preview.ExtractWaveform(asset.Path, 64)
			if err != nil {
				return false, err
			}
			img := preview.RenderWaveformThumbnail(wf, color.RGBA{237, 237, 237, 255}, color.RGBA{24, 24, 27, 255})
			if err := saveJPEG(img, dest); err != nil {
				return false, err
			}
		}
	case model.IsPDF(asset.Mime, asset.Path):
		if !hasThumb {
			img, _, err := preview.RenderPDFPage(asset.Path, 0, longEdge)
			if err != nil {
				return false, err
			}
			fitted := imaging.Fit(img, longEdge, longEdge, imaging.Lanczos)
			if err := saveJPEG(fitted, dest); err != nil {
				return false, err
			}
		}
	default:
		return false, nil
	}
	err = s.db.Update(func(tx *dbTx) error {
		cur, err := db.GetAsset(tx, id)
		if err != nil {
			return err
		}
		cur.ThumbnailPath = dest
		if (cur.Width == 0 || cur.Height == 0) && decodedW > 0 && decodedH > 0 {
			cur.Width = decodedW
			cur.Height = decodedH
		}
		if model.IsCameraRAW(cur.Path) && model.MediaKind(cur.Mime) != model.MediaImage {
			cur.Mime = "image/x-raw"
		}
		if len(pal) > 0 {
			cur.Palette = pal
		}
		if ph != 0 || dh != 0 {
			cur.PHash = ph
			cur.DHash = dh
		}
		if len(vec) > 0 {
			_ = db.PutAssetVector(tx, id, vec)
		}
		return db.PutAsset(tx, cur)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// isHeavyDecode reports whether building this asset's thumbnail decodes a
// large creative file in-process (as opposed to a normal image, or a job
// handed to ffmpeg/pdftoppm).
func isHeavyDecode(asset model.Asset) bool {
	return model.IsIllustrator(asset.Mime, asset.Path) || model.IsExtraCreativeImage(asset.Mime, asset.Path)
}

func imageThumbAndExtract(src, thumbsDir, id string) (image.Image, string, error) {
	img, err := imaging.Open(src, imaging.AutoOrientation(true))
	if err != nil {
		// Fall back to extended creative decoders (ICO, TGA, TIFF, BMP, RAW, PSD).
		img, err = preview.DecodeCreativeImage(src, longEdge)
		if err != nil {
			return nil, "", err
		}
	}
	fitted := imaging.Fit(img, longEdge, longEdge, imaging.Lanczos)
	dest, err := saveSmartThumb(fitted, thumbsDir, id)
	if err != nil {
		return nil, "", err
	}
	return fitted, dest, nil
}

// saveSmartThumb saves alpha-transparent thumbnails as PNG (preserving cutouts
// for Thumbnail Background modes) and opaque thumbnails as fast JPEG.
func saveSmartThumb(img image.Image, thumbsDir, id string) (string, error) {
	if hasTransparentPixels(img) {
		dest := filepath.Join(thumbsDir, id+".png")
		if err := savePNG(img, dest); err != nil {
			return "", err
		}
		return dest, nil
	}
	dest := filepath.Join(thumbsDir, id+".jpg")
	if err := saveJPEG(img, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func hasTransparentPixels(img image.Image) bool {
	if op, ok := img.(interface{ Opaque() bool }); ok {
		return !op.Opaque()
	}
	b := img.Bounds()
	stepX := max(1, b.Dx()/64)
	stepY := max(1, b.Dy()/64)
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xFF00 {
				return true
			}
		}
	}
	return false
}

func savePNG(img image.Image, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	err = enc.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(dest)
		return err
	}
	return closeErr
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
