package preview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"raglex/internal/model"
)

// DecodeCreativeImage decodes extended creative formats (PSD/PSB, PDF-backed
// AI, DOS EPS preview, ICO/CUR, TGA, Camera RAW embedded JPEG, plus standard
// TIFF/BMP/WebP/PNG/JPEG/GIF) into a Go image.Image.
func DecodeCreativeImage(path string, maxEdge int) (image.Image, error) {
	if maxEdge <= 0 {
		maxEdge = 1600
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ext == ".psd" || ext == ".psb":
		return DecodePSD(path)
	case ext == ".ai":
		if hasPDFHeader(path) {
			img, _, err := RenderPDFPage(path, 0, maxEdge)
			return img, err
		}
		return nil, errors.New("illustrator file has no PDF-compatible stream")
	case ext == ".eps":
		return DecodeEPSPreview(path)
	case ext == ".ico" || ext == ".cur":
		return DecodeICO(path)
	case ext == ".tga":
		return DecodeTGA(path)
	case model.IsCameraRAW(path):
		img, _, _, err := DecodeCameraRAWForSize(path, maxEdge)
		return img, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// CreativeImageSize returns dimensions quickly from file headers for PSD/PSB,
// ICO, and TGA without decoding full pixel buffers.
func CreativeImageSize(path string) (int, int) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".psd", ".psb":
		f, err := os.Open(path)
		if err != nil {
			return 0, 0
		}
		defer f.Close()
		var hdr [26]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return 0, 0
		}
		if string(hdr[0:4]) != "8BPS" {
			return 0, 0
		}
		h := int(binary.BigEndian.Uint32(hdr[14:18]))
		w := int(binary.BigEndian.Uint32(hdr[18:22]))
		return w, h
	case ".tga":
		f, err := os.Open(path)
		if err != nil {
			return 0, 0
		}
		defer f.Close()
		var hdr [18]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return 0, 0
		}
		w := int(binary.LittleEndian.Uint16(hdr[12:14]))
		h := int(binary.LittleEndian.Uint16(hdr[14:16]))
		return w, h
	}
	if model.IsCameraRAW(path) {
		return CameraRAWImageSize(path)
	}
	return 0, 0
}

func hasPDFHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var buf [64]byte
	n, _ := f.Read(buf[:])
	return bytes.Contains(buf[:n], []byte("%PDF-"))
}

// DecodePSD decodes Adobe Photoshop PSD (v1) and PSB (v2) merged composite
// image data (Raw or PackBits RLE, 8/16-bit, Grayscale/RGB/RGBA/CMYK), falling
// back to the embedded JPEG thumbnail resource (ID 1036/1033) when needed.
func DecodePSD(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hdr [26]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, err
	}
	if string(hdr[0:4]) != "8BPS" {
		return nil, errors.New("psd: invalid signature")
	}
	version := binary.BigEndian.Uint16(hdr[4:6])
	if version != 1 && version != 2 {
		return nil, fmt.Errorf("psd: unsupported version %d", version)
	}
	channels := int(binary.BigEndian.Uint16(hdr[12:14]))
	height := int(binary.BigEndian.Uint32(hdr[14:18]))
	width := int(binary.BigEndian.Uint32(hdr[18:22]))
	depth := int(binary.BigEndian.Uint16(hdr[22:24]))
	colorMode := int(binary.BigEndian.Uint16(hdr[24:26]))
	if width <= 0 || height <= 0 || width > 32768 || height > 32768 {
		return nil, fmt.Errorf("psd: unsupported dimensions %dx%d", width, height)
	}

	// Section 2: Color Mode Data.
	var secLen32 uint32
	if err := binary.Read(f, binary.BigEndian, &secLen32); err != nil {
		return nil, err
	}
	if secLen32 > 0 {
		if _, err := f.Seek(int64(secLen32), io.SeekCurrent); err != nil {
			return nil, err
		}
	}

	// Section 3: Image Resources (scan for embedded JPEG thumbnail ID 1036 / 1033).
	if err := binary.Read(f, binary.BigEndian, &secLen32); err != nil {
		return nil, err
	}
	var thumbImg image.Image
	if secLen32 > 0 && secLen32 < 64<<20 {
		resBuf := make([]byte, secLen32)
		if _, err := io.ReadFull(f, resBuf); err != nil {
			return nil, err
		}
		thumbImg = extractPSDResourceThumb(resBuf)
	} else if secLen32 > 0 {
		if _, err := f.Seek(int64(secLen32), io.SeekCurrent); err != nil {
			return nil, err
		}
	}

	// Section 4: Layer & Mask Information (4-byte len in PSD, 8-byte len in PSB).
	if version == 2 {
		var secLen64 uint64
		if err := binary.Read(f, binary.BigEndian, &secLen64); err != nil {
			if thumbImg != nil {
				return thumbImg, nil
			}
			return nil, err
		}
		if secLen64 > 0 {
			if _, err := f.Seek(int64(secLen64), io.SeekCurrent); err != nil {
				if thumbImg != nil {
					return thumbImg, nil
				}
				return nil, err
			}
		}
	} else {
		if err := binary.Read(f, binary.BigEndian, &secLen32); err != nil {
			if thumbImg != nil {
				return thumbImg, nil
			}
			return nil, err
		}
		if secLen32 > 0 {
			if _, err := f.Seek(int64(secLen32), io.SeekCurrent); err != nil {
				if thumbImg != nil {
					return thumbImg, nil
				}
				return nil, err
			}
		}
	}

	// Section 5: Merged Image Data.
	// Guard memory for huge PSB files: if full composite exceeds 64M pixels and we
	// already have an embedded thumbnail, return the thumbnail immediately.
	if int64(width)*int64(height) > 64_000_000 && thumbImg != nil {
		return thumbImg, nil
	}
	img, err := decodePSDComposite(f, version, width, height, channels, depth, colorMode)
	if err != nil {
		if thumbImg != nil {
			return thumbImg, nil
		}
		return nil, err
	}
	return img, nil
}

func extractPSDResourceThumb(buf []byte) image.Image {
	pos := 0
	for pos+12 <= len(buf) {
		if string(buf[pos:pos+4]) != "8BIM" {
			break
		}
		pos += 4
		resID := binary.BigEndian.Uint16(buf[pos : pos+2])
		pos += 2
		// Pascal string name, padded to even length.
		nameLen := int(buf[pos])
		pos++
		nameBytes := nameLen
		if (nameLen+1)%2 != 0 {
			nameBytes++
		}
		pos += nameBytes
		if pos+4 > len(buf) {
			break
		}
		dataSize := int(binary.BigEndian.Uint32(buf[pos : pos+4]))
		pos += 4
		if pos+dataSize > len(buf) {
			break
		}
		data := buf[pos : pos+dataSize]
		pos += dataSize
		if dataSize%2 != 0 {
			pos++
		}
		// Resource 1036 (PS 5.0+) or 1033 (PS 4.0): 28-byte header + JPEG bytes.
		if (resID == 1036 || resID == 1033) && len(data) > 28 {
			if img, err := jpeg.Decode(bytes.NewReader(data[28:])); err == nil {
				return img
			}
		}
	}
	return nil
}

func decodePSDComposite(r io.Reader, version uint16, w, h, channels, depth, colorMode int) (image.Image, error) {
	if depth != 8 && depth != 16 {
		return nil, fmt.Errorf("psd: unsupported bit depth %d", depth)
	}
	if colorMode != 1 && colorMode != 3 && colorMode != 4 {
		return nil, fmt.Errorf("psd: unsupported color mode %d", colorMode)
	}
	var comp uint16
	if err := binary.Read(r, binary.BigEndian, &comp); err != nil {
		return nil, err
	}
	if comp != 0 && comp != 1 {
		return nil, fmt.Errorf("psd: unsupported compression %d", comp)
	}

	bpc := depth / 8
	rowBytes := w * bpc
	wantCh := channels
	if wantCh > 4 {
		wantCh = 4
	}
	planes := make([][]byte, wantCh)
	for c := 0; c < wantCh; c++ {
		planes[c] = make([]byte, w*h)
	}

	if comp == 0 {
		// Raw planar channels.
		row := make([]byte, rowBytes)
		for ch := 0; ch < channels; ch++ {
			for y := 0; y < h; y++ {
				if _, err := io.ReadFull(r, row); err != nil {
					return nil, err
				}
				if ch < wantCh {
					dst := planes[ch][y*w : (y+1)*w]
					if bpc == 1 {
						copy(dst, row)
					} else {
						for x := 0; x < w; x++ {
							dst[x] = row[x*2]
						}
					}
				}
			}
		}
	} else {
		// PackBits RLE: scanline byte counts for all channels * height rows first.
		totalRows := channels * h
		counts := make([]int, totalRows)
		if version == 2 {
			for i := 0; i < totalRows; i++ {
				var n uint32
				if err := binary.Read(r, binary.BigEndian, &n); err != nil {
					return nil, err
				}
				counts[i] = int(n)
			}
		} else {
			for i := 0; i < totalRows; i++ {
				var n uint16
				if err := binary.Read(r, binary.BigEndian, &n); err != nil {
					return nil, err
				}
				counts[i] = int(n)
			}
		}
		row := make([]byte, rowBytes)
		var packed []byte
		idx := 0
		for ch := 0; ch < channels; ch++ {
			for y := 0; y < h; y++ {
				n := counts[idx]
				idx++
				if cap(packed) < n {
					packed = make([]byte, n)
				}
				packed = packed[:n]
				if _, err := io.ReadFull(r, packed); err != nil {
					return nil, err
				}
				if ch >= wantCh {
					continue
				}
				unpackPackBits(packed, row)
				dst := planes[ch][y*w : (y+1)*w]
				if bpc == 1 {
					copy(dst, row)
				} else {
					for x := 0; x < w; x++ {
						dst[x] = row[x*2]
					}
				}
			}
		}
	}

	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	pix := out.Pix
	nPix := w * h
	switch colorMode {
	case 1: // Grayscale (+ optional alpha)
		g := planes[0]
		var a []byte
		if wantCh >= 2 {
			a = planes[1]
		}
		for i := 0; i < nPix; i++ {
			v := g[i]
			av := byte(255)
			if a != nil {
				av = a[i]
			}
			o := i * 4
			pix[o] = v
			pix[o+1] = v
			pix[o+2] = v
			pix[o+3] = av
		}
	case 3: // RGB (+ optional alpha)
		if wantCh < 3 {
			return nil, errors.New("psd: RGB missing channels")
		}
		rp, gp, bp := planes[0], planes[1], planes[2]
		var ap []byte
		if wantCh >= 4 {
			ap = planes[3]
		}
		for i := 0; i < nPix; i++ {
			o := i * 4
			pix[o] = rp[i]
			pix[o+1] = gp[i]
			pix[o+2] = bp[i]
			if ap != nil {
				pix[o+3] = ap[i]
			} else {
				pix[o+3] = 255
			}
		}
	case 4: // CMYK (Photoshop stores inverted 255 - C/M/Y/K)
		if wantCh < 4 {
			return nil, errors.New("psd: CMYK missing channels")
		}
		cp, mp, yp, kp := planes[0], planes[1], planes[2], planes[3]
		for i := 0; i < nPix; i++ {
			cVal := uint32(cp[i])
			mVal := uint32(mp[i])
			yVal := uint32(yp[i])
			kVal := uint32(kp[i])
			o := i * 4
			pix[o] = byte((cVal * kVal) / 255)
			pix[o+1] = byte((mVal * kVal) / 255)
			pix[o+2] = byte((yVal * kVal) / 255)
			pix[o+3] = 255
		}
	}
	return out, nil
}

func unpackPackBits(src, dst []byte) {
	si, di := 0, 0
	for si < len(src) && di < len(dst) {
		n := int(int8(src[si]))
		si++
		if n >= 0 {
			cnt := n + 1
			if si+cnt > len(src) {
				cnt = len(src) - si
			}
			if di+cnt > len(dst) {
				cnt = len(dst) - di
			}
			copy(dst[di:di+cnt], src[si:si+cnt])
			si += cnt
			di += cnt
		} else if n != -128 {
			cnt := 1 - n
			if si >= len(src) {
				break
			}
			v := src[si]
			si++
			if di+cnt > len(dst) {
				cnt = len(dst) - di
			}
			for j := 0; j < cnt; j++ {
				dst[di+j] = v
			}
			di += cnt
		}
	}
}

// DecodeEPSPreview extracts the embedded TIFF preview from a DOS EPS binary file.
func DecodeEPSPreview(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hdr [30]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, err
	}
	// DOS EPS binary magic: C5 D0 D3 C6.
	if hdr[0] != 0xC5 || hdr[1] != 0xD0 || hdr[2] != 0xD3 || hdr[3] != 0xC6 {
		return nil, errors.New("eps: no embedded binary preview")
	}
	tiffOff := int64(binary.LittleEndian.Uint32(hdr[20:24]))
	tiffLen := int64(binary.LittleEndian.Uint32(hdr[24:28]))
	if tiffOff <= 0 || tiffLen <= 0 || tiffLen > 64<<20 {
		return nil, errors.New("eps: no embedded TIFF preview")
	}
	if _, err := f.Seek(tiffOff, io.SeekStart); err != nil {
		return nil, err
	}
	return tiff.Decode(io.LimitReader(f, tiffLen))
}

// DecodeICO decodes Windows .ico / .cur files, picking the largest high-res frame.
func DecodeICO(path string) (image.Image, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 6 {
		return nil, errors.New("ico: too short")
	}
	count := int(binary.LittleEndian.Uint16(raw[4:6]))
	if count == 0 || len(raw) < 6+count*16 {
		return nil, errors.New("ico: invalid directory")
	}
	bestIdx := -1
	bestScore := -1
	for i := 0; i < count; i++ {
		off := 6 + i*16
		w := int(raw[off])
		h := int(raw[off+1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		bpp := int(binary.LittleEndian.Uint16(raw[off+6 : off+8]))
		score := w*h*100 + bpp
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil, errors.New("ico: no frames")
	}
	off := 6 + bestIdx*16
	size := int(binary.LittleEndian.Uint32(raw[off+8 : off+12]))
	imgOff := int(binary.LittleEndian.Uint32(raw[off+12 : off+16]))
	if imgOff < 0 || size <= 0 || imgOff+size > len(raw) {
		return nil, errors.New("ico: invalid frame bounds")
	}
	frame := raw[imgOff : imgOff+size]
	// PNG-compressed ICO frame (Vista+).
	if len(frame) >= 8 && string(frame[:4]) == "\x89PNG" {
		return png.Decode(bytes.NewReader(frame))
	}
	// Classic 32-bit / 24-bit DIB BMP frame inside ICO.
	if len(frame) < 40 {
		return nil, errors.New("ico: invalid dib header")
	}
	dibHdrSize := int(binary.LittleEndian.Uint32(frame[0:4]))
	w := int(int32(binary.LittleEndian.Uint32(frame[4:8])))
	hTotal := int(int32(binary.LittleEndian.Uint32(frame[8:12])))
	bpp := int(binary.LittleEndian.Uint16(frame[14:16]))
	h := hTotal / 2
	if w <= 0 || h <= 0 || (bpp != 32 && bpp != 24) {
		return nil, fmt.Errorf("ico: unsupported dib %dx%d %dbpp", w, h, bpp)
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	stride := ((w*bpp + 31) / 32) * 4
	pixOff := dibHdrSize
	for y := 0; y < h; y++ {
		srcY := h - 1 - y
		rowStart := pixOff + srcY*stride
		if rowStart+w*(bpp/8) > len(frame) {
			break
		}
		for x := 0; x < w; x++ {
			p := rowStart + x*(bpp/8)
			b, g, r := frame[p], frame[p+1], frame[p+2]
			a := byte(255)
			if bpp == 32 {
				a = frame[p+3]
			}
			out.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return out, nil
}

// DecodeTGA decodes uncompressed and RLE-compressed Truevision TGA images.
func DecodeTGA(path string) (image.Image, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 18 {
		return nil, errors.New("tga: too short")
	}
	idLen := int(raw[0])
	colorMapType := raw[1]
	imgType := raw[2]
	cmLen := int(binary.LittleEndian.Uint16(raw[5:7]))
	cmBits := int(raw[7])
	w := int(binary.LittleEndian.Uint16(raw[12:14]))
	h := int(binary.LittleEndian.Uint16(raw[14:16]))
	bpp := int(raw[16])
	desc := raw[17]
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 {
		return nil, errors.New("tga: invalid dimensions")
	}
	if imgType != 2 && imgType != 3 && imgType != 10 && imgType != 11 {
		return nil, fmt.Errorf("tga: unsupported image type %d", imgType)
	}
	bytesPP := bpp / 8
	if bytesPP != 1 && bytesPP != 3 && bytesPP != 4 {
		return nil, fmt.Errorf("tga: unsupported bpp %d", bpp)
	}
	pos := 18 + idLen
	if colorMapType != 0 {
		pos += cmLen * ((cmBits + 7) / 8)
	}
	if pos >= len(raw) {
		return nil, errors.New("tga: truncated header")
	}
	data := raw[pos:]
	topOrigin := (desc & 0x20) != 0
	out := image.NewNRGBA(image.Rect(0, 0, w, h))

	writePix := func(idx int, px []byte) {
		x := idx % w
		y := idx / w
		if !topOrigin {
			y = h - 1 - y
		}
		switch bytesPP {
		case 1:
			out.SetNRGBA(x, y, color.NRGBA{R: px[0], G: px[0], B: px[0], A: 255})
		case 3:
			out.SetNRGBA(x, y, color.NRGBA{R: px[2], G: px[1], B: px[0], A: 255})
		case 4:
			out.SetNRGBA(x, y, color.NRGBA{R: px[2], G: px[1], B: px[0], A: px[3]})
		}
	}

	total := w * h
	if imgType == 2 || imgType == 3 {
		for i := 0; i < total && (i+1)*bytesPP <= len(data); i++ {
			writePix(i, data[i*bytesPP:(i+1)*bytesPP])
		}
		return out, nil
	}
	// RLE (types 10 and 11).
	di := 0
	for pxIdx := 0; pxIdx < total && di < len(data); {
		hdr := data[di]
		di++
		cnt := int(hdr&0x7F) + 1
		if (hdr & 0x80) != 0 {
			if di+bytesPP > len(data) {
				break
			}
			px := data[di : di+bytesPP]
			di += bytesPP
			for j := 0; j < cnt && pxIdx < total; j++ {
				writePix(pxIdx, px)
				pxIdx++
			}
		} else {
			for j := 0; j < cnt && pxIdx < total && di+bytesPP <= len(data); j++ {
				writePix(pxIdx, data[di:di+bytesPP])
				di += bytesPP
				pxIdx++
			}
		}
	}
	return out, nil
}

// ExtractEmbeddedJPEG extracts and auto-orients the highest-resolution preview
// image from a Camera RAW file (CR3, CR2, CRW, NEF, NRW, NAF, ARW, SR2, SRF,
// RAF, DNG, ORF, RW2, PEF, SRW, 3FR, IIQ, etc.).
func ExtractEmbeddedJPEG(path string) (image.Image, error) {
	return DecodeCameraRAW(path)
}
