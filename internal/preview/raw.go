package preview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"golang.org/x/image/tiff"

	"raglex/internal/ffmpeg"
)

const (
	rawMaxSingleJPEG  = 48 << 20 // 48 MiB cap for a single embedded preview JPEG
	rawFastScanBytes  = 8 << 20  // 8 MiB streaming fallback window (covers 99.9% of RAW previews)
	rawMaxScanBytes   = 32 << 20 // 32 MiB absolute streaming cap if nothing found in first 8 MiB
	rawScanChunkSize  = 256 << 10
	rawMinGoodPreview = 640 * 480
	rawMaxTIFFPixels  = 12_000_000 // guard uncompressed linear DNG fallback against huge sensor bitmaps
)

var (
	cr3PreviewUUID = [16]byte{0xea, 0xf4, 0x2b, 0x5e, 0x1c, 0x98, 0x4b, 0x88, 0xb9, 0xfb, 0xb7, 0xdc, 0x40, 0x6e, 0x4d, 0x16}
	cr3CanonUUID   = [16]byte{0x85, 0xc0, 0xb6, 0x87, 0x82, 0x0f, 0x11, 0xe0, 0x81, 0x11, 0xf4, 0xce, 0x46, 0x2b, 0x6a, 0x48}

	// Cap concurrent ffmpeg RAW fallback processes to 1 so exotic RAWs never peg CPU/RAM.
	rawFFmpegSem = make(chan struct{}, 1)
)

// rawJPEGEntry describes an embedded JPEG preview located inside a RAW file
// without reading its full compressed bytes into memory until selected.
type rawJPEGEntry struct {
	off    int64
	length int64
	width  int
	height int
	area   int
	orient int // EXIF orientation inside this JPEG (0 if unknown / 1 if normal)
}

func (e rawJPEGEntry) longEdge() int {
	if e.width > e.height {
		return e.width
	}
	return e.height
}

// CameraRAWImageSize returns the display dimensions (after EXIF orientation)
// of a Camera RAW file by probing only embedded preview JPEG headers without
// reading full JPEG payloads or decoding pixels.
func CameraRAWImageSize(path string) (int, int) {
	entries, containerOrient, err := indexEmbeddedJPEGs(path, 0)
	if err != nil || len(entries) == 0 {
		return 0, 0
	}
	best := pickCandidate(entries, 0)
	orient := best.orient
	if orient < 2 || orient > 8 {
		orient = containerOrient
	}
	w, h := best.width, best.height
	if orient >= 5 && orient <= 8 {
		w, h = h, w
	}
	return w, h
}

// DecodeCameraRAW extracts and auto-orients the highest-resolution preview from
// proprietary camera RAW files (Fujifilm RAF, Canon CR3/CR2/CRW, Sony ARW/SR2/SRF,
// Nikon NEF/NRW/NAF, Adobe DNG, Olympus ORF, Panasonic RW2, Pentax PEF, Samsung
// SRW, Hasselblad 3FR, Phase One IIQ, etc.).
func DecodeCameraRAW(path string) (image.Image, error) {
	img, _, _, err := DecodeCameraRAWForSize(path, 0)
	return img, err
}

// DecodeCameraRAWForSize decodes an embedded Camera RAW preview sized efficiently
// for maxEdge (e.g. 512 for library thumbnails, 2400 for Quick Look, 0 for full).
//
// For small maxEdge requests (thumbnails), it prefers a medium embedded preview
// (e.g. ~1024-1620px) over a huge 24-60MP full-res embedded JPEG, downscales
// with fast area-averaging BEFORE rotating, and still returns the original
// display dimensions (origW, origH) of the highest-res preview header.
func DecodeCameraRAWForSize(path string, maxEdge int) (image.Image, int, int, error) {
	if data, orient, origW, origH, err := ExtractEmbeddedJPEGForSize(path, maxEdge); err == nil && len(data) > 0 {
		if img, derr := jpeg.Decode(bytes.NewReader(data)); derr == nil && img != nil {
			// Release compressed JPEG bytes ASAP before downscaling/rotating.
			data = nil
			if maxEdge > 0 {
				img = FastFitImage(img, maxEdge)
			}
			img = applyEXIFOrientation(img, orient)
			return img, origW, origH, nil
		}
	}

	// Fallback 1: Uncompressed / linear DNG only (never attempt raw proprietary Bayer
	// ARW/NEF/CR2/ORF TIFF sensor bitmaps, and strictly cap at 12MP).
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".dng" || ext == ".tif" || ext == ".tiff" {
		if f, err := os.Open(path); err == nil {
			cfg, cerr := tiff.DecodeConfig(f)
			if cerr == nil && cfg.Width > 0 && cfg.Height > 0 && int64(cfg.Width)*int64(cfg.Height) <= rawMaxTIFFPixels {
				if _, serr := f.Seek(0, io.SeekStart); serr == nil {
					img, derr := tiff.Decode(f)
					_ = f.Close()
					if derr == nil && img != nil {
						ow, oh := img.Bounds().Dx(), img.Bounds().Dy()
						if maxEdge > 0 {
							img = FastFitImage(img, maxEdge)
						}
						return img, ow, oh, nil
					}
				} else {
					_ = f.Close()
				}
			} else {
				_ = f.Close()
			}
		}
	}

	// Fallback 2: Bundled/system ffmpeg frame decode — ONLY for explicit full-screen
	// Preview/Quick Look (maxEdge == 0 or > 1200), never during background thumbnailing
	// so importing a folder of exotic RAWs never spawns ffmpeg CPU/RAM spikes.
	if maxEdge == 0 || maxEdge > 1200 {
		if img, err := decodeRAWWithFFmpeg(path, maxEdge); err == nil && img != nil {
			return img, img.Bounds().Dx(), img.Bounds().Dy(), nil
		}
	}

	return nil, 0, 0, errors.New("raw: no readable preview found")
}

// FastFitImage shrinks img so its long edge does not exceed maxEdge using a
// low-memory, low-CPU two-stage downscaler (fast integer box reduction for
// large scale factors, then bilinear/CatmullRom polish), avoiding the massive
// RAM/CPU spike of single-pass Lanczos on 24-60MP images.
func FastFitImage(img image.Image, maxEdge int) image.Image {
	if img == nil || maxEdge <= 0 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxEdge && h <= maxEdge {
		return img
	}
	longSide := w
	if h > longSide {
		longSide = h
	}
	// Stage 1: If shrinking by >= 2.5x, fast-path integer stride/box downscale directly
	// from YCbCr (standard JPEG output) or NRGBA/RGBA into a small NRGBA (~2x target)
	// without ever allocating a full-resolution NRGBA copy of a 24-60MP photo!
	if longSide >= maxEdge*5/2 {
		targetMid := maxEdge * 2
		step := longSide / targetMid
		if step >= 2 {
			img = fastBoxShrink(img, step)
		}
	}
	return imaging.Fit(img, maxEdge, maxEdge, imaging.Linear)
}

// fastBoxShrink performs an integer factor box-average shrink directly from
// YCbCr / RGBA / NRGBA into a much smaller NRGBA buffer.
func fastBoxShrink(src image.Image, step int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dw := max(1, sw/step)
	dh := max(1, sh/step)
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))

	switch s := src.(type) {
	case *image.YCbCr:
		// Fast YCbCr direct sampling (2x2 tap average inside each step×step cell)
		// avoids allocating an intermediate full-res NRGBA buffer.
		dx := step / 2
		if dx < 1 {
			dx = 1
		}
		for y := 0; y < dh; y++ {
			sy0 := b.Min.Y + y*step
			sy1 := min(b.Max.Y-1, sy0+dx)
			rowOff := y * dst.Stride
			for x := 0; x < dw; x++ {
				sx0 := b.Min.X + x*step
				sx1 := min(b.Max.X-1, sx0+dx)
				r0, g0, b0 := ycbcrRGBAt(s, sx0, sy0)
				r1, g1, b1 := ycbcrRGBAt(s, sx1, sy0)
				r2, g2, b2 := ycbcrRGBAt(s, sx0, sy1)
				r3, g3, b3 := ycbcrRGBAt(s, sx1, sy1)
				o := rowOff + x*4
				dst.Pix[o] = uint8((r0 + r1 + r2 + r3 + 2) >> 2)
				dst.Pix[o+1] = uint8((g0 + g1 + g2 + g3 + 2) >> 2)
				dst.Pix[o+2] = uint8((b0 + b1 + b2 + b3 + 2) >> 2)
				dst.Pix[o+3] = 255
			}
		}
		return dst
	case *image.NRGBA:
		dx := max(1, step/2)
		for y := 0; y < dh; y++ {
			sy0 := y * step
			sy1 := min(sh-1, sy0+dx)
			rowOff := y * dst.Stride
			for x := 0; x < dw; x++ {
				sx0 := x * step
				sx1 := min(sw-1, sx0+dx)
				p0 := sy0*s.Stride + sx0*4
				p1 := sy0*s.Stride + sx1*4
				p2 := sy1*s.Stride + sx0*4
				p3 := sy1*s.Stride + sx1*4
				o := rowOff + x*4
				dst.Pix[o] = uint8((uint32(s.Pix[p0]) + uint32(s.Pix[p1]) + uint32(s.Pix[p2]) + uint32(s.Pix[p3]) + 2) >> 2)
				dst.Pix[o+1] = uint8((uint32(s.Pix[p0+1]) + uint32(s.Pix[p1+1]) + uint32(s.Pix[p2+1]) + uint32(s.Pix[p3+1]) + 2) >> 2)
				dst.Pix[o+2] = uint8((uint32(s.Pix[p0+2]) + uint32(s.Pix[p1+2]) + uint32(s.Pix[p2+2]) + uint32(s.Pix[p3+2]) + 2) >> 2)
				dst.Pix[o+3] = uint8((uint32(s.Pix[p0+3]) + uint32(s.Pix[p1+3]) + uint32(s.Pix[p2+3]) + uint32(s.Pix[p3+3]) + 2) >> 2)
			}
		}
		return dst
	default:
		dx := max(1, step/2)
		for y := 0; y < dh; y++ {
			sy0 := b.Min.Y + y*step
			sy1 := min(b.Max.Y-1, sy0+dx)
			rowOff := y * dst.Stride
			for x := 0; x < dw; x++ {
				sx0 := b.Min.X + x*step
				sx1 := min(b.Max.X-1, sx0+dx)
				r0, g0, b0, a0 := src.At(sx0, sy0).RGBA()
				r1, g1, b1, a1 := src.At(sx1, sy0).RGBA()
				r2, g2, b2, a2 := src.At(sx0, sy1).RGBA()
				r3, g3, b3, a3 := src.At(sx1, sy1).RGBA()
				o := rowOff + x*4
				dst.Pix[o] = uint8((r0 + r1 + r2 + r3) >> 10)
				dst.Pix[o+1] = uint8((g0 + g1 + g2 + g3) >> 10)
				dst.Pix[o+2] = uint8((b0 + b1 + b2 + b3) >> 10)
				dst.Pix[o+3] = uint8((a0 + a1 + a2 + a3) >> 10)
			}
		}
		return dst
	}
}

func ycbcrRGBAt(img *image.YCbCr, x, y int) (uint32, uint32, uint32) {
	yi := img.YOffset(x, y)
	ci := img.COffset(x, y)
	yy := int32(img.Y[yi]) * 0x10101
	cb := int32(img.Cb[ci]) - 128
	cr := int32(img.Cr[ci]) - 128

	r := (yy + 91881*cr) >> 16
	g := (yy - 22554*cb - 46802*cr) >> 16
	bl := (yy + 116130*cb) >> 16
	if r < 0 {
		r = 0
	} else if r > 255 {
		r = 255
	}
	if g < 0 {
		g = 0
	} else if g > 255 {
		g = 255
	}
	if bl < 0 {
		bl = 0
	} else if bl > 255 {
		bl = 255
	}
	return uint32(r), uint32(g), uint32(bl)
}

// ExtractEmbeddedJPEGBytes locates the largest valid embedded JPEG preview inside
// a Camera RAW file along with its resolved EXIF orientation (1..8).
func ExtractEmbeddedJPEGBytes(path string) ([]byte, int, error) {
	data, orient, _, _, err := ExtractEmbeddedJPEGForSize(path, 0)
	return data, orient, err
}

// ExtractEmbeddedJPEGForSize extracts the best-fit embedded JPEG preview for
// maxEdge along with resolved orientation and the highest-resolution display
// dimensions found in the RAW container.
func ExtractEmbeddedJPEGForSize(path string, maxEdge int) ([]byte, int, int, int, error) {
	entries, containerOrient, err := indexEmbeddedJPEGs(path, maxEdge)
	if err != nil || len(entries) == 0 {
		if err == nil {
			err = errors.New("raw: no embedded JPEG preview found")
		}
		return nil, 1, 0, 0, err
	}

	chosen := pickCandidate(entries, maxEdge)
	largest := pickCandidate(entries, 0)

	f, err := os.Open(path)
	if err != nil {
		return nil, 1, 0, 0, err
	}
	defer f.Close()

	buf := make([]byte, chosen.length)
	if _, err := f.ReadAt(buf, chosen.off); err != nil {
		return nil, 1, 0, 0, err
	}

	orient := chosen.orient
	if orient < 2 || orient > 8 {
		orient = resolveRAWOrientation(buf, containerOrient)
	}
	origW, origH := largest.width, largest.height
	if orient >= 5 && orient <= 8 {
		origW, origH = origH, origW
	}
	return buf, orient, origW, origH, nil
}

// pickCandidate selects the optimal embedded JPEG entry:
// - For thumbnails (0 < maxEdge <= 1200): picks the SMALLEST preview whose
//   long edge is >= maxEdge (e.g. 1616x1080 instead of 8192x5464!), avoiding
//   decoding a 45MP JPEG just to produce a 512px thumbnail.
// - For full-res previews (maxEdge == 0 or > 1200): picks the LARGEST preview.
func pickCandidate(entries []rawJPEGEntry, maxEdge int) rawJPEGEntry {
	if len(entries) == 1 {
		return entries[0]
	}
	if maxEdge > 0 && maxEdge <= 1200 {
		var bestFit rawJPEGEntry
		for _, e := range entries {
			if e.longEdge() >= maxEdge {
				if bestFit.area == 0 || e.area < bestFit.area {
					bestFit = e
				}
			}
		}
		if bestFit.area > 0 {
			return bestFit
		}
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if e.area > best.area {
			best = e
		}
	}
	return best
}

// indexEmbeddedJPEGs locates embedded preview JPEGs inside a Camera RAW file
// using header-only probes (<= 64 KiB per candidate) so memory stays tiny.
func indexEmbeddedJPEGs(path string, targetEdge int) ([]rawJPEGEntry, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 1, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, 1, err
	}
	fileSize := fi.Size()
	if fileSize < 16 {
		return nil, 1, errors.New("raw: file too small")
	}

	var hdr [128]byte
	n, _ := f.ReadAt(hdr[:], 0)
	head := hdr[:n]

	var entries []rawJPEGEntry
	seenOff := map[int64]bool{}
	bestArea := 0
	containerOrient := 1

	registerRegion := func(off, length int64) {
		if off < 0 || length < 32 || length > rawMaxSingleJPEG || off+length > fileSize || seenOff[off] {
			return
		}
		w, h, orient, ok := probeJPEGConfigAt(f, off, length)
		if !ok || w <= 0 || h <= 0 {
			return
		}
		seenOff[off] = true
		area := w * h
		if area > bestArea {
			bestArea = area
		}
		entries = append(entries, rawJPEGEntry{
			off:    off,
			length: length,
			width:  w,
			height: h,
			area:   area,
			orient: orient,
		})
	}

	minEnoughArea := rawMinGoodPreview
	if targetEdge > 0 && targetEdge <= 1200 {
		// For a 512px thumbnail, any preview with long edge >= targetEdge (e.g. 640x480 or 1616x1080) is ideal.
		minEnoughArea = max(160*120, targetEdge*targetEdge/2)
	}

	// Path 1: Fujifilm .RAF — exact JPEG offset at byte 84 and length at byte 88.
	if len(head) >= 92 && bytes.HasPrefix(head, []byte("FUJIFILMCCD-RAW ")) {
		off := int64(binary.BigEndian.Uint32(head[84:88]))
		length := int64(binary.BigEndian.Uint32(head[88:92]))
		registerRegion(off, length)
		if bestArea >= minEnoughArea {
			return entries, containerOrient, nil
		}
	}

	// Path 2: Canon .CR3 (ISO BMFF / MP4 container: ftyp crx / PRVW / THMB / Track 1 JPEG).
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		cr3Orient := extractCR3PreviewRegions(f, fileSize, registerRegion)
		if cr3Orient >= 1 && cr3Orient <= 8 {
			containerOrient = cr3Orient
		}
		if bestArea >= minEnoughArea {
			return entries, containerOrient, nil
		}
	}

	// Path 3: TIFF-based RAW containers (ARW, SR2, SRF, NEF, NRW, NAF, CR2, DNG, ORF, RW2, PEF, SRW, 3FR, IIQ, etc.).
	if isTIFFHeader(head) {
		tiffOrient := extractTIFFRAWPreviewRegions(f, fileSize, registerRegion)
		if tiffOrient >= 1 && tiffOrient <= 8 {
			containerOrient = tiffOrient
		}
		if bestArea >= minEnoughArea {
			return entries, containerOrient, nil
		}
	}

	// Path 4: Streaming bounded JPEG marker-segment walker (NEVER reads 96 MiB into RAM!).
	// Scans the first 8 MiB in 256 KiB chunks, expanding up to 32 MiB only if nothing usable was found.
	scanLimit := min(fileSize, int64(rawFastScanBytes))
	stopArea := 1_500_000 // ~1.5MP (covers Sony ARW 1616x1080 immediately without scanning 60MB of Bayer tail!)
	if targetEdge > 0 && targetEdge <= 1200 {
		stopArea = max(rawMinGoodPreview, targetEdge*targetEdge)
	}
	scanJPEGStreamsFromFile(f, fileSize, scanLimit, stopArea, registerRegion)
	if len(entries) == 0 && fileSize > scanLimit {
		extLimit := min(fileSize, int64(rawMaxScanBytes))
		scanJPEGStreamsFromFile(f, fileSize, extLimit, stopArea, registerRegion)
	}

	if len(entries) == 0 {
		return nil, 1, errors.New("raw: no embedded JPEG preview found")
	}
	return entries, containerOrient, nil
}

// --- Canon CR3 (ISO BMFF) header-only parser ---

type cr3TrackInfo struct {
	offsets []int64
	sizes   []int64
}

func extractCR3PreviewRegions(f *os.File, fileSize int64, register func(off, length int64)) int {
	orient := 1
	var tracks []cr3TrackInfo
	walkISOBMFF(f, 0, fileSize, 0, &orient, &tracks, register)

	// Canon CR3 stores a full-resolution JPEG in Track 1 inside mdat.
	for _, tr := range tracks {
		if len(tr.offsets) == 0 || len(tr.sizes) == 0 {
			continue
		}
		off := tr.offsets[0]
		sz := tr.sizes[0]
		if off <= 0 || sz < 1024 || sz > rawMaxSingleJPEG || off+sz > fileSize {
			continue
		}
		register(off, sz)
	}
	return orient
}

func walkISOBMFF(f *os.File, start, end int64, depth int, orient *int, tracks *[]cr3TrackInfo, register func(off, length int64)) {
	if depth > 8 || start >= end {
		return
	}
	pos := start
	var hdr [16]byte
	for pos+8 <= end {
		n, err := f.ReadAt(hdr[:8], pos)
		if err != nil || n < 8 {
			return
		}
		boxSize := int64(binary.BigEndian.Uint32(hdr[0:4]))
		boxType := string(hdr[4:8])
		hdrLen := int64(8)
		if boxSize == 1 {
			if pos+16 > end {
				return
			}
			if _, err := f.ReadAt(hdr[8:16], pos+8); err != nil {
				return
			}
			boxSize = int64(binary.BigEndian.Uint64(hdr[8:16]))
			hdrLen = 16
		} else if boxSize == 0 {
			boxSize = end - pos
		}
		if boxSize < hdrLen || pos+boxSize > end {
			return
		}
		bodyStart := pos + hdrLen
		bodyEnd := pos + boxSize
		bodySize := bodyEnd - bodyStart

		switch boxType {
		case "moov", "mdia", "minf", "stbl":
			walkISOBMFF(f, bodyStart, bodyEnd, depth+1, orient, tracks, register)
		case "trak":
			var tr cr3TrackInfo
			walkCR3Track(f, bodyStart, bodyEnd, 0, &tr)
			if len(tr.offsets) > 0 && len(tr.sizes) > 0 {
				*tracks = append(*tracks, tr)
			}
		case "uuid":
			if bodySize >= 16 {
				var uid [16]byte
				if _, err := f.ReadAt(uid[:], bodyStart); err == nil {
					if uid == cr3PreviewUUID {
						walkISOBMFF(f, bodyStart+16, bodyEnd, depth+1, orient, tracks, register)
						if bodyStart+24 < bodyEnd {
							walkISOBMFF(f, bodyStart+24, bodyEnd, depth+1, orient, tracks, register)
						}
					} else if uid == cr3CanonUUID {
						walkISOBMFF(f, bodyStart+16, bodyEnd, depth+1, orient, tracks, register)
					}
				}
			}
		case "PRVW", "THMB":
			if bodySize > 32 && bodySize <= rawMaxSingleJPEG {
				var head64 [64]byte
				hn := min(int64(len(head64)), bodySize)
				if _, err := f.ReadAt(head64[:hn], bodyStart); err == nil {
					if idx := bytes.Index(head64[:hn], []byte{0xFF, 0xD8, 0xFF}); idx >= 0 {
						register(bodyStart+int64(idx), bodySize-int64(idx))
					}
				}
			}
		case "CMT1":
			if bodySize >= 16 && bodySize <= 2<<20 {
				buf := make([]byte, min(bodySize, 64<<10))
				if _, err := f.ReadAt(buf, bodyStart); err == nil {
					if o := parseTIFFOrientationBytes(buf); o >= 1 && o <= 8 {
						*orient = o
					}
				}
			}
		}
		pos = bodyEnd
	}
}

func walkCR3Track(f *os.File, start, end int64, depth int, tr *cr3TrackInfo) {
	if depth > 6 || start >= end {
		return
	}
	pos := start
	var hdr [16]byte
	for pos+8 <= end {
		if _, err := f.ReadAt(hdr[:8], pos); err != nil {
			return
		}
		boxSize := int64(binary.BigEndian.Uint32(hdr[0:4]))
		boxType := string(hdr[4:8])
		hdrLen := int64(8)
		if boxSize == 1 {
			if pos+16 > end {
				return
			}
			if _, err := f.ReadAt(hdr[8:16], pos+8); err != nil {
				return
			}
			boxSize = int64(binary.BigEndian.Uint64(hdr[8:16]))
			hdrLen = 16
		} else if boxSize == 0 {
			boxSize = end - pos
		}
		if boxSize < hdrLen || pos+boxSize > end {
			return
		}
		bodyStart := pos + hdrLen
		bodyEnd := pos + boxSize
		bodySize := bodyEnd - bodyStart

		switch boxType {
		case "mdia", "minf", "stbl":
			walkCR3Track(f, bodyStart, bodyEnd, depth+1, tr)
		case "stsz":
			if bodySize >= 12 {
				var b [16]byte
				n := min(int64(len(b)), bodySize)
				if _, err := f.ReadAt(b[:n], bodyStart); err == nil {
					sampleSize := binary.BigEndian.Uint32(b[4:8])
					count := binary.BigEndian.Uint32(b[8:12])
					if sampleSize > 0 {
						tr.sizes = append(tr.sizes, int64(sampleSize))
					} else if count > 0 && bodySize >= 16 {
						tr.sizes = append(tr.sizes, int64(binary.BigEndian.Uint32(b[12:16])))
					}
				}
			}
		case "stco":
			if bodySize >= 12 {
				var b [12]byte
				if _, err := f.ReadAt(b[:], bodyStart); err == nil {
					count := binary.BigEndian.Uint32(b[4:8])
					if count > 0 {
						tr.offsets = append(tr.offsets, int64(binary.BigEndian.Uint32(b[8:12])))
					}
				}
			}
		case "co64":
			if bodySize >= 16 {
				var b [16]byte
				if _, err := f.ReadAt(b[:], bodyStart); err == nil {
					count := binary.BigEndian.Uint32(b[4:8])
					if count > 0 {
						tr.offsets = append(tr.offsets, int64(binary.BigEndian.Uint64(b[8:16])))
					}
				}
			}
		}
		pos = bodyEnd
	}
}

// --- TIFF / EXIF IFD fast-path (ARW, NEF, NAF, CR2, DNG, ORF, RW2, PEF, SRW, 3FR, IIQ) ---

func isTIFFHeader(head []byte) bool {
	if len(head) < 8 {
		return false
	}
	if head[0] == 'I' && head[1] == 'I' {
		magic := binary.LittleEndian.Uint16(head[2:4])
		return magic == 0x002A || magic == 0x0055 || magic == 0x4F52 || magic == 0x5352 // TIFF, RW2, ORF
	}
	if head[0] == 'M' && head[1] == 'M' {
		magic := binary.BigEndian.Uint16(head[2:4])
		return magic == 0x002A
	}
	return false
}

func extractTIFFRAWPreviewRegions(f *os.File, fileSize int64, register func(off, length int64)) int {
	var hdr [16]byte
	if _, err := f.ReadAt(hdr[:12], 0); err != nil {
		return 1
	}
	var bo binary.ByteOrder
	var magic uint16
	if hdr[0] == 'I' && hdr[1] == 'I' {
		bo = binary.LittleEndian
		magic = bo.Uint16(hdr[2:4])
	} else if hdr[0] == 'M' && hdr[1] == 'M' {
		bo = binary.BigEndian
		magic = bo.Uint16(hdr[2:4])
	} else {
		return 1
	}

	firstIFD := int64(bo.Uint32(hdr[4:8]))
	if magic == 0x0055 && firstIFD == 0 {
		firstIFD = 16
	}

	visited := map[int64]bool{}
	orient := 1
	var walkIFD func(off int64, baseOff int64, order binary.ByteOrder, depth int)
	walkIFD = func(off int64, baseOff int64, order binary.ByteOrder, depth int) {
		if depth > 6 || off <= 0 || off+2 >= fileSize || visited[off] {
			return
		}
		visited[off] = true

		var cntBuf [2]byte
		if _, err := f.ReadAt(cntBuf[:], off); err != nil {
			return
		}
		count := int(order.Uint16(cntBuf[:]))
		if count <= 0 || count > 512 {
			return
		}
		tableLen := int64(count) * 12
		if off+2+tableLen > fileSize {
			return
		}
		table := make([]byte, tableLen)
		if _, err := f.ReadAt(table, off+2); err != nil {
			return
		}

		var jpegOff, jpegLen int64
		var stripOff, stripLen int64
		var compression uint32
		var subIFDs []int64
		var makerOff, makerLen int64

		for i := 0; i < count; i++ {
			e := table[i*12 : (i+1)*12]
			tag := order.Uint16(e[0:2])
			typ := order.Uint16(e[2:4])
			valCnt := order.Uint32(e[4:8])
			valRaw := e[8:12]

			readU32 := func() uint32 {
				if typ == 3 && valCnt == 1 {
					return uint32(order.Uint16(valRaw[0:2]))
				}
				return order.Uint32(valRaw)
			}

			switch tag {
			case 0x0112: // Orientation
				if depth == 0 && orient == 1 {
					o := int(readU32())
					if o >= 1 && o <= 8 {
						orient = o
					}
				}
			case 0x0103: // Compression
				compression = readU32()
			case 0x0201: // JPEGInterchangeFormat
				jpegOff = baseOff + int64(readU32())
			case 0x0202: // JPEGInterchangeFormatLength
				jpegLen = int64(readU32())
			case 0x0111: // StripOffsets (single-strip preview JPEG)
				if valCnt == 1 {
					stripOff = baseOff + int64(readU32())
				} else if valCnt > 1 && valCnt < 16 {
					arrOff := baseOff + int64(order.Uint32(valRaw))
					var first [4]byte
					if arrOff > 0 && arrOff+4 <= fileSize {
						if _, err := f.ReadAt(first[:], arrOff); err == nil {
							stripOff = baseOff + int64(order.Uint32(first[:]))
						}
					}
				}
			case 0x0117: // StripByteCounts
				if valCnt == 1 {
					stripLen = int64(readU32())
				}
			case 0x002E: // Panasonic RW2 JpgFromRaw
				if valCnt > 32 && int64(valCnt) <= rawMaxSingleJPEG {
					register(baseOff+int64(order.Uint32(valRaw)), int64(valCnt))
				}
			case 0x2001: // Sony MakerNote PreviewImage (inline UNDEFINED JPEG blob!)
				if valCnt > 64 && int64(valCnt) <= rawMaxSingleJPEG {
					register(baseOff+int64(order.Uint32(valRaw)), int64(valCnt))
				}
			case 0x0011: // Nikon MakerNote PreviewIFD offset
				if depth >= 2 && valCnt == 1 {
					subIFDs = append(subIFDs, baseOff+int64(readU32()))
				}
			case 0x014A: // SubIFDs
				if valCnt == 1 {
					subIFDs = append(subIFDs, baseOff+int64(readU32()))
				} else if valCnt > 1 && valCnt <= 16 {
					arrOff := baseOff + int64(order.Uint32(valRaw))
					if arrOff > 0 && arrOff+int64(valCnt)*4 <= fileSize {
						subBuf := make([]byte, valCnt*4)
						if _, err := f.ReadAt(subBuf, arrOff); err == nil {
							for j := uint32(0); j < valCnt; j++ {
								subIFDs = append(subIFDs, baseOff+int64(order.Uint32(subBuf[j*4:(j+1)*4])))
							}
						}
					}
				}
			case 0x8769: // ExifIFD
				subIFDs = append(subIFDs, baseOff+int64(readU32()))
			case 0x927C: // MakerNote (Sony ARW PreviewImage 0x2001 / Nikon PreviewIFD 0x0011)
				if valCnt >= 16 && int64(valCnt) <= rawMaxSingleJPEG {
					makerOff = baseOff + int64(order.Uint32(valRaw))
					makerLen = int64(valCnt)
				}
			}
		}

		if jpegOff > 0 && jpegLen > 0 {
			register(jpegOff, jpegLen)
		}
		if stripOff > 0 && stripLen >= 64 {
			register(stripOff, stripLen)
		}
		_ = compression

		// Parse Sony / Nikon / Olympus MakerNote IFDs for embedded preview pointers without reading whole file.
		if makerOff > 0 && makerLen >= 16 && makerOff+16 <= fileSize && depth <= 3 {
			var mnHead [24]byte
			mnN, _ := f.ReadAt(mnHead[:], makerOff)
			mn := mnHead[:mnN]
			switch {
			case mnN >= 18 && bytes.HasPrefix(mn, []byte("Nikon\x00")):
				// Nikon Type-3 MakerNote: 10-byte header + embedded TIFF header whose offsets are relative to makerOff+10.
				subBase := makerOff + 10
				if isTIFFHeader(mn[10:18]) {
					var subBO binary.ByteOrder = binary.LittleEndian
					if mn[10] == 'M' {
						subBO = binary.BigEndian
					}
					subFirst := int64(subBO.Uint32(mn[14:18]))
					if subFirst > 0 {
						walkIFD(subBase+subFirst, subBase, subBO, depth+1)
					}
				}
			case mnN >= 12 && bytes.HasPrefix(mn, []byte("OLYMPUS\x00II")):
				walkIFD(makerOff+12, makerOff, binary.LittleEndian, depth+1)
			case mnN >= 12 && bytes.HasPrefix(mn, []byte("OLYMP\x00")):
				walkIFD(makerOff+8, baseOff, order, depth+1)
			case mnN >= 12 && bytes.HasPrefix(mn, []byte("SONY DSC \x00\x00\x00")):
				walkIFD(makerOff+12, baseOff, order, depth+1)
			case mnN >= 12 && bytes.HasPrefix(mn, []byte("SONY CAM \x00\x00\x00")):
				walkIFD(makerOff+12, baseOff, order, depth+1)
			default:
				// Classic Sony Alpha MakerNote starts directly with a TIFF IFD using main TIFF baseOff.
				walkIFD(makerOff, baseOff, order, depth+1)
			}
		}

		for _, sub := range subIFDs {
			walkIFD(sub, baseOff, order, depth+1)
		}

		// Follow next IFD in chain.
		if off+2+tableLen+4 <= fileSize {
			var nextBuf [4]byte
			if _, err := f.ReadAt(nextBuf[:], off+2+tableLen); err == nil {
				nextRel := int64(order.Uint32(nextBuf[:]))
				if nextRel > 0 {
					nextIFD := baseOff + nextRel
					if nextIFD > 0 && nextIFD < fileSize {
						walkIFD(nextIFD, baseOff, order, depth)
					}
				}
			}
		}
	}

	walkIFD(firstIFD, 0, bo, 0)
	return orient
}

// --- Streaming Bounded JPEG Segment-Walking Scanner (Zero 96 MiB Allocations!) ---

func scanJPEGStreamsFromFile(f *os.File, fileSize, scanLimit int64, stopArea int, register func(off, length int64)) {
	if scanLimit > fileSize {
		scanLimit = fileSize
	}
	if scanLimit < 32 {
		return
	}

	soi := []byte{0xFF, 0xD8, 0xFF}
	chunk := make([]byte, rawScanChunkSize+4)
	bestAreaSeen := 0

	var pos int64
	for pos < scanLimit-4 {
		toRead := min(int64(len(chunk)), scanLimit-pos)
		n, err := f.ReadAt(chunk[:toRead], pos)
		if n < 4 || (err != nil && err != io.EOF) {
			break
		}
		buf := chunk[:n]

		local := 0
		jumped := false
		for local < n-4 {
			rel := bytes.Index(buf[local:], soi)
			if rel < 0 {
				break
			}
			idx := local + rel
			m := buf[idx+3]
			if !isLikelyFirstJPEGMarker(m) {
				local = idx + 3
				continue
			}
			absStart := pos + int64(idx)
			absEnd, ok := findTrueJPEGEndAt(f, absStart, fileSize)
			if ok && absEnd > absStart+32 {
				length := absEnd - absStart
				// Probe dimensions via O(1) marker skipping (immune to >64KiB EXIF/XMP/ICC headers!).
				if w, h, _, pok := probeJPEGConfigAt(f, absStart, length); pok && w > 0 && h > 0 {
					if area := w * h; area > bestAreaSeen {
						bestAreaSeen = area
					}
				}
				register(absStart, length)
				if bestAreaSeen >= stopArea {
					return
				}
				pos = absEnd
				jumped = true
				break
			}
			local = idx + 3
		}
		if !jumped {
			if n <= 4 {
				break
			}
			pos += int64(n - 4)
		}
	}
}

func isLikelyFirstJPEGMarker(m byte) bool {
	switch {
	case m >= 0xE0 && m <= 0xEF: // APP0..APP15 (JFIF, EXIF, ICC, etc.)
		return true
	case m == 0xDB, m == 0xC0, m == 0xC1, m == 0xC2, m == 0xC4, m == 0xDD, m == 0xFE:
		return true
	default:
		return false
	}
}

func isSOFMarker(m byte) bool {
	switch m {
	case 0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF:
		return true
	default:
		return false
	}
}

// probeJPEGConfigAt reads a JPEG's (width, height, EXIF orientation) directly
// from *os.File by walking marker segments and jumping over arbitrarily large
// APPn/EXIF/XMP/ICC blocks in O(1) without reading them into RAM.
// This avoids the 64 KiB truncation bug that rejected real Sony ARW / Nikon NEF /
// Canon CR3 embedded JPEGs whose EXIF+MakerNote+XMP headers exceed 64 KiB.
func probeJPEGConfigAt(f *os.File, off, length int64) (int, int, int, bool) {
	if off < 0 || length < 32 {
		return 0, 0, 1, false
	}
	end := off + length
	var head [3]byte
	if _, err := f.ReadAt(head[:], off); err != nil {
		return 0, 0, 1, false
	}
	if head[0] != 0xFF || head[1] != 0xD8 || head[2] != 0xFF {
		return 0, 0, 1, false
	}

	orient := 1
	pos := off + 2
	var hdr [7]byte
	var exifBuf [4096]byte

	for steps := 0; steps < 256 && pos+2 < end; steps++ {
		if _, err := f.ReadAt(hdr[:2], pos); err != nil {
			return 0, 0, orient, false
		}
		if hdr[0] != 0xFF {
			return 0, 0, orient, false
		}
		marker := hdr[1]
		pos += 2
		for marker == 0xFF {
			if pos >= end {
				return 0, 0, orient, false
			}
			if _, err := f.ReadAt(hdr[:1], pos); err != nil {
				return 0, 0, orient, false
			}
			marker = hdr[0]
			pos++
		}

		// Reached EOI or Start-Of-Scan without seeing SOF -> invalid JPEG header.
		if marker == 0xD9 || marker == 0xDA {
			return 0, 0, orient, false
		}
		// Standalone markers without length field.
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8) {
			continue
		}
		if pos+2 > end {
			return 0, 0, orient, false
		}
		if _, err := f.ReadAt(hdr[:2], pos); err != nil {
			return 0, 0, orient, false
		}
		segLen := int64(binary.BigEndian.Uint16(hdr[:2]))
		if segLen < 2 || pos+segLen > end {
			return 0, 0, orient, false
		}

		if isSOFMarker(marker) {
			if segLen < 7 || pos+7 > end {
				return 0, 0, orient, false
			}
			if _, err := f.ReadAt(hdr[:7], pos); err != nil {
				return 0, 0, orient, false
			}
			h := int(binary.BigEndian.Uint16(hdr[3:5]))
			w := int(binary.BigEndian.Uint16(hdr[5:7]))
			if w > 0 && h > 0 {
				return w, h, orient, true
			}
			return 0, 0, orient, false
		}

		if marker == 0xE1 && orient == 1 && segLen > 14 {
			readLen := min(segLen-2, int64(len(exifBuf)))
			if n, err := f.ReadAt(exifBuf[:readLen], pos+2); (err == nil || err == io.EOF) && n > 12 {
				seg := exifBuf[:n]
				if bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
					if o := parseTIFFOrientationBytes(seg[6:]); o >= 1 && o <= 8 {
						orient = o
					}
				}
			}
		}

		pos += segLen
	}
	return 0, 0, orient, false
}

// findTrueJPEGEndAt parses JPEG marker segments directly from *os.File starting
// at absStart using small buffered reads (never loading the entire RAW file).
func findTrueJPEGEndAt(f *os.File, absStart, fileSize int64) (int64, bool) {
	limit := min(fileSize, absStart+rawMaxSingleJPEG)
	if absStart+4 > limit {
		return 0, false
	}
	var sig [2]byte
	if _, err := f.ReadAt(sig[:], absStart); err != nil || sig[0] != 0xFF || sig[1] != 0xD8 {
		return 0, false
	}

	pos := absStart + 2
	sawSOS := false
	var small [4]byte
	scanBuf := make([]byte, 64<<10)

	for pos+1 < limit {
		if _, err := f.ReadAt(small[:2], pos); err != nil {
			return 0, false
		}
		if small[0] != 0xFF {
			return 0, false
		}
		// Skip optional padding 0xFF bytes.
		marker := small[1]
		pos += 2
		for marker == 0xFF {
			if pos >= limit {
				return 0, false
			}
			if _, err := f.ReadAt(small[:1], pos); err != nil {
				return 0, false
			}
			marker = small[0]
			pos++
		}

		// End of Image!
		if marker == 0xD9 {
			if !sawSOS {
				return 0, false
			}
			return pos, true
		}
		// Standalone markers without length field.
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8) {
			continue
		}
		// Marker with 2-byte big-endian segment length (includes the 2 length bytes).
		if pos+2 > limit {
			return 0, false
		}
		if _, err := f.ReadAt(small[:2], pos); err != nil {
			return 0, false
		}
		segLen := int64(binary.BigEndian.Uint16(small[:2]))
		if segLen < 2 || pos+segLen > limit {
			return 0, false
		}
		pos += segLen

		if marker == 0xDA {
			// Start of Scan: fast-forward through entropy-coded data in 64 KiB chunks.
			sawSOS = true
			foundNextMarker := false
			for pos+1 < limit {
				rn := min(int64(len(scanBuf)), limit-pos)
				n, err := f.ReadAt(scanBuf[:rn], pos)
				if n < 2 || (err != nil && err != io.EOF) {
					return 0, false
				}
				slice := scanBuf[:n]
				i := 0
				for i < n-1 {
					rel := bytes.IndexByte(slice[i:n-1], 0xFF)
					if rel < 0 {
						break
					}
					i += rel
					next := slice[i+1]
					if next == 0x00 || (next >= 0xD0 && next <= 0xD7) {
						i += 2
						continue
					}
					if next == 0xFF {
						i++
						continue
					}
					// Reached a real marker (EOI FF D9, or another SOS/DHT in progressive JPEG).
					pos += int64(i)
					foundNextMarker = true
					break
				}
				if foundNextMarker {
					break
				}
				// Advance by n-1 so a trailing 0xFF at slice[n-1] is inspected with the next byte.
				pos += int64(n - 1)
			}
			if !foundNextMarker {
				return 0, false
			}
		}
	}
	return 0, false
}

// --- EXIF Orientation helpers ---

func resolveRAWOrientation(jpegData []byte, containerOrient int) int {
	if o := parseJPEGEXIFOrientation(jpegData); o >= 2 && o <= 8 {
		return o
	}
	if containerOrient >= 1 && containerOrient <= 8 {
		return containerOrient
	}
	return 1
}

func parseJPEGEXIFOrientation(data []byte) int {
	n := len(data)
	if n < 16 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	pos := 2
	for pos+4 <= n {
		if data[pos] != 0xFF {
			return 1
		}
		for pos < n && data[pos] == 0xFF {
			pos++
		}
		if pos >= n {
			return 1
		}
		marker := data[pos]
		pos++
		if marker == 0xD9 || marker == 0xDA {
			return 1
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8) {
			continue
		}
		if pos+2 > n {
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if segLen < 2 || pos+segLen > n {
			return 1
		}
		if marker == 0xE1 && segLen > 14 {
			seg := data[pos+2 : pos+segLen]
			if bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
				if o := parseTIFFOrientationBytes(seg[6:]); o >= 1 && o <= 8 {
					return o
				}
			}
		}
		pos += segLen
	}
	return 1
}

func parseTIFFOrientationBytes(tiffData []byte) int {
	if len(tiffData) < 12 {
		return 1
	}
	var bo binary.ByteOrder
	switch {
	case tiffData[0] == 'I' && tiffData[1] == 'I':
		bo = binary.LittleEndian
	case tiffData[0] == 'M' && tiffData[1] == 'M':
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(tiffData[2:4]) != 0x002A {
		return 1
	}
	ifdOff := int(bo.Uint32(tiffData[4:8]))
	if ifdOff < 8 || ifdOff+2 > len(tiffData) {
		return 1
	}
	count := int(bo.Uint16(tiffData[ifdOff : ifdOff+2]))
	base := ifdOff + 2
	for i := 0; i < count; i++ {
		off := base + i*12
		if off+12 > len(tiffData) {
			break
		}
		tag := bo.Uint16(tiffData[off : off+2])
		if tag == 0x0112 {
			typ := bo.Uint16(tiffData[off+2 : off+4])
			var v int
			if typ == 3 {
				v = int(bo.Uint16(tiffData[off+8 : off+10]))
			} else {
				v = int(bo.Uint32(tiffData[off+8 : off+12]))
			}
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

func applyEXIFOrientation(img image.Image, orient int) image.Image {
	switch orient {
	case 2:
		return imaging.FlipH(img)
	case 3:
		return imaging.Rotate180(img)
	case 4:
		return imaging.FlipV(img)
	case 5:
		return imaging.Transpose(img)
	case 6:
		return imaging.Rotate270(img)
	case 7:
		return imaging.Transverse(img)
	case 8:
		return imaging.Rotate90(img)
	default:
		return img
	}
}

func decodeRAWWithFFmpeg(path string, maxEdge int) (image.Image, error) {
	ff := ffmpeg.Find()
	if ff == "" {
		return nil, exec.ErrNotFound
	}
	rawFFmpegSem <- struct{}{}
	defer func() { <-rawFFmpegSem }()

	args := []string{"-v", "error", "-threads", "2", "-i", path, "-frames:v", "1"}
	if maxEdge > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease", maxEdge, maxEdge))
	}
	args = append(args, "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "3", "pipe:1")

	cmd := exec.Command(ff, args...)
	hideCmd(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		if err == nil {
			err = errors.New("ffmpeg produced empty output")
		}
		return nil, err
	}
	return jpeg.Decode(bytes.NewReader(out.Bytes()))
}
