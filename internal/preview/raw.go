package preview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"

	"github.com/disintegration/imaging"
	"golang.org/x/image/tiff"

	"raglex/internal/ffmpeg"
)

const (
	rawMaxSingleJPEG  = 48 << 20 // 48 MiB cap for a single embedded preview JPEG
	rawMaxScanBytes   = 96 << 20 // 96 MiB cap for fallback stream scanning
	rawMinGoodPreview = 640 * 480
	rawFullResStop    = 4_000_000 // stop scanning once a >=4MP preview is found
)

var (
	cr3PreviewUUID = [16]byte{0xea, 0xf4, 0x2b, 0x5e, 0x1c, 0x98, 0x4b, 0x88, 0xb9, 0xfb, 0xb7, 0xdc, 0x40, 0x6e, 0x4d, 0x16}
	cr3CanonUUID   = [16]byte{0x85, 0xc0, 0xb6, 0x87, 0x82, 0x0f, 0x11, 0xe0, 0x81, 0x11, 0xf4, 0xce, 0x46, 0x2b, 0x6a, 0x48}
)

type rawCandidate struct {
	data   []byte
	width  int
	height int
	area   int
}

// CameraRAWImageSize returns the display dimensions (after EXIF orientation)
// of a Camera RAW file by reading its embedded preview JPEG header without
// decoding pixels.
func CameraRAWImageSize(path string) (int, int) {
	data, orient, err := ExtractEmbeddedJPEGBytes(path)
	if err != nil || len(data) == 0 {
		return 0, 0
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	w, h := cfg.Width, cfg.Height
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
	if data, orient, err := ExtractEmbeddedJPEGBytes(path); err == nil && len(data) > 0 {
		if img, derr := jpeg.Decode(bytes.NewReader(data)); derr == nil && img != nil {
			return applyEXIFOrientation(img, orient), nil
		}
	}

	// Fallback 1: Uncompressed / linear DNG or TIFF-decodable RAW.
	if f, err := os.Open(path); err == nil {
		img, derr := tiff.Decode(f)
		_ = f.Close()
		if derr == nil && img != nil {
			return img, nil
		}
	}

	// Fallback 2: Bundled/system ffmpeg frame decode for exotic RAW containers.
	if img, err := decodeRAWWithFFmpeg(path); err == nil && img != nil {
		return img, nil
	}

	return nil, errors.New("raw: no readable preview found")
}

// ExtractEmbeddedJPEGBytes locates the largest valid embedded JPEG preview inside
// a Camera RAW file along with its resolved EXIF orientation (1..8).
func ExtractEmbeddedJPEGBytes(path string) ([]byte, int, error) {
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

	var best rawCandidate
	containerOrient := 1

	consider := func(data []byte) {
		if len(data) < 32 {
			return
		}
		if data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
			return
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
			return
		}
		area := cfg.Width * cfg.Height
		if area > best.area {
			best = rawCandidate{
				data:   data,
				width:  cfg.Width,
				height: cfg.Height,
				area:   area,
			}
		}
	}

	// Path 1: Fujifilm .RAF — exact JPEG offset at byte 84 and length at byte 88.
	if len(head) >= 92 && bytes.HasPrefix(head, []byte("FUJIFILMCCD-RAW ")) {
		off := int64(binary.BigEndian.Uint32(head[84:88]))
		length := int64(binary.BigEndian.Uint32(head[88:92]))
		if off > 0 && length > 32 && length <= rawMaxSingleJPEG && off+length <= fileSize {
			buf := make([]byte, length)
			if _, err := f.ReadAt(buf, off); err == nil {
				consider(buf)
			}
		}
		if best.area >= rawMinGoodPreview {
			orient := resolveRAWOrientation(best.data, containerOrient)
			return best.data, orient, nil
		}
	}

	// Path 2: Canon .CR3 (ISO BMFF / MP4 container: ftyp crx / PRVW / THMB / Track 1 JPEG).
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		cr3Orient := extractCR3Previews(f, fileSize, consider)
		if cr3Orient >= 1 && cr3Orient <= 8 {
			containerOrient = cr3Orient
		}
		if best.area >= rawMinGoodPreview {
			orient := resolveRAWOrientation(best.data, containerOrient)
			return best.data, orient, nil
		}
	}

	// Path 3: TIFF-based RAW containers (ARW, SR2, SRF, NEF, NRW, NAF, CR2, DNG, ORF, RW2, PEF, SRW, 3FR, IIQ, etc.).
	if isTIFFHeader(head) {
		tiffOrient := extractTIFFRAWPreviews(f, fileSize, consider)
		if tiffOrient >= 1 && tiffOrient <= 8 {
			containerOrient = tiffOrient
		}
		if best.area >= rawMinGoodPreview {
			orient := resolveRAWOrientation(best.data, containerOrient)
			return best.data, orient, nil
		}
	}

	// Path 4: Universal JPEG marker-segment walker (properly skips nested EXIF APP1
	// thumbnails & their inner FF D9 so MakerNote previews and NAF/CRW/proprietary
	// RAW files decode cleanly without truncation).
	scanLimit := fileSize
	if scanLimit > rawMaxScanBytes {
		scanLimit = rawMaxScanBytes
	}
	if _, err := f.Seek(0, io.SeekStart); err == nil {
		if buf, err := io.ReadAll(io.LimitReader(f, scanLimit)); err == nil {
			scanJPEGStreams(buf, consider)
		}
	}

	if len(best.data) == 0 {
		return nil, 1, errors.New("raw: no embedded JPEG preview found")
	}
	orient := resolveRAWOrientation(best.data, containerOrient)
	return best.data, orient, nil
}

// --- Canon CR3 (ISO BMFF) parser ---

type cr3TrackInfo struct {
	offsets []int64
	sizes   []int64
	isJPEG  bool
}

func extractCR3Previews(f *os.File, fileSize int64, consider func([]byte)) int {
	orient := 1
	var tracks []cr3TrackInfo
	walkISOBMFF(f, 0, fileSize, 0, &orient, &tracks, consider)

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
		var sig [3]byte
		if _, err := f.ReadAt(sig[:], off); err != nil {
			continue
		}
		if sig[0] == 0xFF && sig[1] == 0xD8 && sig[2] == 0xFF {
			buf := make([]byte, sz)
			if _, err := f.ReadAt(buf, off); err == nil {
				consider(buf)
			}
		}
	}
	return orient
}

func walkISOBMFF(f *os.File, start, end int64, depth int, orient *int, tracks *[]cr3TrackInfo, consider func([]byte)) {
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
			walkISOBMFF(f, bodyStart, bodyEnd, depth+1, orient, tracks, consider)
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
						// Canon Preview UUID box contains a 8-byte header before PRVW or direct child boxes.
						walkISOBMFF(f, bodyStart+16, bodyEnd, depth+1, orient, tracks, consider)
						if bodyStart+24 < bodyEnd {
							walkISOBMFF(f, bodyStart+24, bodyEnd, depth+1, orient, tracks, consider)
						}
					} else if uid == cr3CanonUUID {
						walkISOBMFF(f, bodyStart+16, bodyEnd, depth+1, orient, tracks, consider)
					}
				}
			}
		case "PRVW", "THMB":
			if bodySize > 32 && bodySize <= rawMaxSingleJPEG {
				buf := make([]byte, bodySize)
				if _, err := f.ReadAt(buf, bodyStart); err == nil {
					if idx := bytes.Index(buf[:min(len(buf), 64)], []byte{0xFF, 0xD8, 0xFF}); idx >= 0 {
						consider(buf[idx:])
					}
				}
			}
		case "CMT1":
			// Canon IFD0 EXIF TIFF block inside moov/uuid.
			if bodySize >= 16 && bodySize <= 2<<20 {
				buf := make([]byte, bodySize)
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

func extractTIFFRAWPreviews(f *os.File, fileSize int64, consider func([]byte)) int {
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
	// Panasonic RW2 (magic 0x0055) places the first IFD offset at bytes 4..8 (usually 16).
	if magic == 0x0055 && firstIFD == 0 {
		firstIFD = 16
	}

	visited := map[int64]bool{}
	orient := 1
	var walkIFD func(off int64, depth int)
	walkIFD = func(off int64, depth int) {
		if depth > 6 || off <= 0 || off+2 >= fileSize || visited[off] {
			return
		}
		visited[off] = true

		var cntBuf [2]byte
		if _, err := f.ReadAt(cntBuf[:], off); err != nil {
			return
		}
		count := int(bo.Uint16(cntBuf[:]))
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

		for i := 0; i < count; i++ {
			e := table[i*12 : (i+1)*12]
			tag := bo.Uint16(e[0:2])
			typ := bo.Uint16(e[2:4])
			valCnt := bo.Uint32(e[4:8])
			valRaw := e[8:12]

			readU32 := func() uint32 {
				if typ == 3 && valCnt == 1 {
					return uint32(bo.Uint16(valRaw[0:2]))
				}
				return bo.Uint32(valRaw)
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
				jpegOff = int64(readU32())
			case 0x0202: // JPEGInterchangeFormatLength
				jpegLen = int64(readU32())
			case 0x0111: // StripOffsets (single-strip preview JPEG)
				if valCnt == 1 {
					stripOff = int64(readU32())
				} else if valCnt > 1 && valCnt < 16 {
					arrOff := int64(bo.Uint32(valRaw))
					var first [4]byte
					if arrOff > 0 && arrOff+4 <= fileSize {
						if _, err := f.ReadAt(first[:], arrOff); err == nil {
							stripOff = int64(bo.Uint32(first[:]))
						}
					}
				}
			case 0x0117: // StripByteCounts
				if valCnt == 1 {
					stripLen = int64(readU32())
				}
			case 0x002E: // Panasonic RW2 JpgFromRaw
				if valCnt > 32 && int64(valCnt) <= rawMaxSingleJPEG {
					dataOff := int64(bo.Uint32(valRaw))
					if dataOff > 0 && dataOff+int64(valCnt) <= fileSize {
						buf := make([]byte, valCnt)
						if _, err := f.ReadAt(buf, dataOff); err == nil {
							consider(buf)
						}
					}
				}
			case 0x014A: // SubIFDs
				if valCnt == 1 {
					subIFDs = append(subIFDs, int64(readU32()))
				} else if valCnt > 1 && valCnt <= 16 {
					arrOff := int64(bo.Uint32(valRaw))
					if arrOff > 0 && arrOff+int64(valCnt)*4 <= fileSize {
						subBuf := make([]byte, valCnt*4)
						if _, err := f.ReadAt(subBuf, arrOff); err == nil {
							for j := uint32(0); j < valCnt; j++ {
								subIFDs = append(subIFDs, int64(bo.Uint32(subBuf[j*4:(j+1)*4])))
							}
						}
					}
				}
			case 0x8769: // ExifIFD
				subIFDs = append(subIFDs, int64(readU32()))
			}
		}

		tryRegion := func(regionOff, regionLen int64) {
			if regionOff <= 0 || regionLen < 64 || regionLen > rawMaxSingleJPEG || regionOff+regionLen > fileSize {
				return
			}
			var sig [3]byte
			if _, err := f.ReadAt(sig[:], regionOff); err != nil {
				return
			}
			if sig[0] != 0xFF || sig[1] != 0xD8 || sig[2] != 0xFF {
				return
			}
			buf := make([]byte, regionLen)
			if _, err := f.ReadAt(buf, regionOff); err == nil {
				consider(buf)
			}
		}

		if jpegOff > 0 && jpegLen > 0 {
			tryRegion(jpegOff, jpegLen)
		}
		if stripOff > 0 && stripLen > 0 && (compression == 6 || compression == 7 || compression == 0) {
			tryRegion(stripOff, stripLen)
		}

		for _, sub := range subIFDs {
			walkIFD(sub, depth+1)
		}

		// Follow next IFD in chain.
		if off+2+tableLen+4 <= fileSize {
			var nextBuf [4]byte
			if _, err := f.ReadAt(nextBuf[:], off+2+tableLen); err == nil {
				nextIFD := int64(bo.Uint32(nextBuf[:]))
				if nextIFD > 0 && nextIFD < fileSize {
					walkIFD(nextIFD, depth)
				}
			}
		}
	}

	walkIFD(firstIFD, 0)
	return orient
}

// --- True JPEG Segment-Walking Scanner (safe against nested EXIF APP1 FF D9!) ---

func scanJPEGStreams(buf []byte, consider func([]byte)) {
	soi := []byte{0xFF, 0xD8, 0xFF}
	bestAreaSeen := 0
	wrapped := func(data []byte) {
		if cfg, err := jpeg.DecodeConfig(bytes.NewReader(data)); err == nil {
			area := cfg.Width * cfg.Height
			if area > bestAreaSeen {
				bestAreaSeen = area
			}
		}
		consider(data)
	}

	for i := 0; i < len(buf)-4; {
		rel := bytes.Index(buf[i:], soi)
		if rel < 0 {
			break
		}
		start := i + rel
		m := buf[start+3]
		if !isLikelyFirstJPEGMarker(m) {
			i = start + 3
			continue
		}
		end, ok := findTrueJPEGEnd(buf, start)
		if ok && end > start+32 {
			wrapped(buf[start:end])
			if bestAreaSeen >= rawFullResStop {
				return
			}
			i = end
			continue
		}
		i = start + 3
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

// findTrueJPEGEnd parses JPEG marker segments starting at buf[start] == FF D8
// so embedded EXIF APP1 thumbnails (which contain their own FF D9) are skipped
// cleanly instead of truncating the parent JPEG.
func findTrueJPEGEnd(buf []byte, start int) (int, bool) {
	n := len(buf)
	if start+4 > n || buf[start] != 0xFF || buf[start+1] != 0xD8 {
		return 0, false
	}
	limit := start + rawMaxSingleJPEG
	if limit > n {
		limit = n
	}

	pos := start + 2
	sawSOS := false

	for pos+1 < limit {
		// Skip optional padding 0xFF bytes before marker.
		if buf[pos] != 0xFF {
			return 0, false
		}
		for pos < limit && buf[pos] == 0xFF {
			pos++
		}
		if pos >= limit {
			return 0, false
		}
		marker := buf[pos]
		pos++

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
		segLen := int(binary.BigEndian.Uint16(buf[pos : pos+2]))
		if segLen < 2 || pos+segLen > limit {
			return 0, false
		}
		pos += segLen

		if marker == 0xDA {
			// Start of Scan: skip entropy-coded data until the next real marker.
			sawSOS = true
			for pos+1 < limit {
				// Fast-forward to next 0xFF byte.
				rel := bytes.IndexByte(buf[pos:limit], 0xFF)
				if rel < 0 {
					return 0, false
				}
				pos += rel
				if pos+1 >= limit {
					return 0, false
				}
				next := buf[pos+1]
				if next == 0x00 {
					// Byte-stuffed literal 0xFF inside entropy stream.
					pos += 2
					continue
				}
				if next == 0xFF {
					// Fill byte.
					pos++
					continue
				}
				if next >= 0xD0 && next <= 0xD7 {
					// Restart marker RST0..RST7 inside entropy stream.
					pos += 2
					continue
				}
				// Reached a real marker (EOI FF D9, or another SOS/DHT in progressive JPEG).
				break
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

func decodeRAWWithFFmpeg(path string) (image.Image, error) {
	ff := ffmpeg.Find()
	if ff == "" {
		return nil, exec.ErrNotFound
	}
	cmd := exec.Command(ff,
		"-v", "error",
		"-i", path,
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)
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
