package preview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"raglex/internal/model"
)

func makeTestJPEG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// injectEXIFWithNestedThumb inserts an APP1 segment containing an EXIF TIFF
// orientation tag AND a nested thumbnail JPEG (with its own FF D8 .. FF D9!)
// immediately after the parent JPEG's SOI marker. This is the exact layout
// used by real Canon, Sony, Nikon, and Fujifilm RAW embedded JPEGs.
func injectEXIFWithNestedThumb(t *testing.T, parentJPEG []byte, orient uint16, nestedThumb []byte) []byte {
	t.Helper()
	// Build minimal little-endian TIFF header with 1 IFD entry (Orientation 0x0112)
	// followed by the nested thumbnail bytes (containing FF D9!).
	var tiff bytes.Buffer
	tiff.WriteString("II")
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0x002A))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(1))      // 1 entry
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0x0112)) // Orientation
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(3))      // SHORT
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(1))      // count
	_ = binary.Write(&tiff, binary.LittleEndian, orient)
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(0)) // next IFD
	tiff.Write(nestedThumb)

	app1Payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	segLen := uint16(len(app1Payload) + 2)

	var out bytes.Buffer
	out.Write(parentJPEG[:2]) // FF D8
	out.Write([]byte{0xFF, 0xE1})
	_ = binary.Write(&out, binary.BigEndian, segLen)
	out.Write(app1Payload)
	out.Write(parentJPEG[2:])
	return out.Bytes()
}

func TestCameraRAWExtensionsRecognized(t *testing.T) {
	exts := []string{
		"photo.naf", "photo.NAF", "photo.nef", "photo.nrw",
		"photo.raf", "photo.RAF", "photo.cr3", "photo.CR3", "photo.cr2", "photo.crw",
		"photo.arw", "photo.ARW", "photo.sr2", "photo.srf",
		"photo.dng", "photo.orf", "photo.rw2", "photo.pef", "photo.srw",
		"photo.3fr", "photo.iiq", "photo.erf", "photo.kdc", "photo.mrw",
	}
	for _, name := range exts {
		if !model.IsCameraRAW(name) {
			t.Errorf("expected IsCameraRAW(%q) == true", name)
		}
		if !model.IsExtraCreativeImage("application/octet-stream", name) {
			t.Errorf("expected IsExtraCreativeImage(%q) == true", name)
		}
	}
}

func TestDecodeRAFExactOffsetAndOrientation(t *testing.T) {
	thumb := makeTestJPEG(t, 40, 30, color.RGBA{R: 120, G: 120, B: 120, A: 255})
	mainJPEG := injectEXIFWithNestedThumb(t, makeTestJPEG(t, 320, 200, color.RGBA{R: 220, G: 80, B: 40, A: 255}), 6, thumb)

	// Build a synthetic Fujifilm .RAF header (magic + offset at 84 + length at 88).
	var raf bytes.Buffer
	raf.WriteString("FUJIFILMCCD-RAW ")
	raf.Write(make([]byte, 84-16))
	jpegOffset := uint32(256)
	_ = binary.Write(&raf, binary.BigEndian, jpegOffset)
	_ = binary.Write(&raf, binary.BigEndian, uint32(len(mainJPEG)))
	raf.Write(make([]byte, int(jpegOffset)-raf.Len()))
	raf.Write(mainJPEG)
	raf.Write(make([]byte, 4096)) // simulated raw sensor data tail

	p := filepath.Join(t.TempDir(), "fuji_test.RAF")
	if err := os.WriteFile(p, raf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// Orientation 6 rotates 320x200 landscape to 200x320 portrait!
	w, h := CameraRAWImageSize(p)
	if w != 200 || h != 320 {
		t.Fatalf("CameraRAWImageSize(RAF) = %dx%d, want 200x320", w, h)
	}
	img, err := DecodeCameraRAW(p)
	if err != nil {
		t.Fatalf("DecodeCameraRAW(RAF) failed: %v", err)
	}
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 320 {
		t.Fatalf("decoded RAF bounds = %v, want 200x320", img.Bounds())
	}
}

func TestDecodeCR3PreviewBoxAndCMT1Orientation(t *testing.T) {
	thumb := makeTestJPEG(t, 48, 32, color.RGBA{R: 80, G: 80, B: 80, A: 255})
	prvwJPEG := injectEXIFWithNestedThumb(t, makeTestJPEG(t, 400, 260, color.RGBA{R: 40, G: 140, B: 230, A: 255}), 1, thumb)

	makeBox := func(typ string, payload []byte) []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.BigEndian, uint32(8+len(payload)))
		b.WriteString(typ)
		b.Write(payload)
		return b.Bytes()
	}

	// ftyp crx box
	ftyp := makeBox("ftyp", []byte("crx \x00\x00\x00\x01crx isom"))

	// CMT1 TIFF IFD0 with Orientation = 8 (rotate 90 CCW -> 260x400)
	var cmt1 bytes.Buffer
	cmt1.WriteString("II")
	_ = binary.Write(&cmt1, binary.LittleEndian, uint16(0x002A))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint32(8))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint16(1))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint16(0x0112))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint16(3))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint32(1))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint16(8))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint16(0))
	_ = binary.Write(&cmt1, binary.LittleEndian, uint32(0))
	canonUUIDPayload := append(cr3CanonUUID[:], makeBox("CMT1", cmt1.Bytes())...)
	moov := makeBox("moov", makeBox("uuid", canonUUIDPayload))

	// PRVW inside preview UUID box
	prvwBody := append(make([]byte, 16), prvwJPEG...)
	prvwUUIDPayload := append(cr3PreviewUUID[:], make([]byte, 8)...)
	prvwUUIDPayload = append(prvwUUIDPayload, makeBox("PRVW", prvwBody)...)
	prvwUUID := makeBox("uuid", prvwUUIDPayload)

	var cr3 bytes.Buffer
	cr3.Write(ftyp)
	cr3.Write(moov)
	cr3.Write(prvwUUID)

	p := filepath.Join(t.TempDir(), "canon_test.CR3")
	if err := os.WriteFile(p, cr3.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	w, h := CameraRAWImageSize(p)
	if w != 260 || h != 400 {
		t.Fatalf("CameraRAWImageSize(CR3) = %dx%d, want 260x400", w, h)
	}
	img, err := DecodeCameraRAW(p)
	if err != nil {
		t.Fatalf("DecodeCameraRAW(CR3) failed: %v", err)
	}
	if img.Bounds().Dx() != 260 || img.Bounds().Dy() != 400 {
		t.Fatalf("decoded CR3 bounds = %v, want 260x400", img.Bounds())
	}
}

func TestDecodeARWAndNAFNestedEXIFEOISafety(t *testing.T) {
	// Nested tiny EXIF thumbnail (contains an inner FF D9!) inside a larger main JPEG.
	// The old naive bytes.Index(..., FF D9) failed with unexpected EOF on this exact pattern!
	nestedThumb := makeTestJPEG(t, 64, 48, color.RGBA{R: 50, G: 50, B: 50, A: 255})
	mainJPEG := injectEXIFWithNestedThumb(t, makeTestJPEG(t, 480, 320, color.RGBA{R: 30, G: 200, B: 110, A: 255}), 1, nestedThumb)

	// 1. Test TIFF-based Sony .ARW with SubIFD JPEGInterchangeFormat pointing to mainJPEG.
	var arw bytes.Buffer
	arw.WriteString("II")
	_ = binary.Write(&arw, binary.LittleEndian, uint16(0x002A))
	_ = binary.Write(&arw, binary.LittleEndian, uint32(8)) // IFD0 at 8
	// IFD0: 2 entries (JPEGInterchangeFormat + Length)
	jpegOff := uint32(8 + 2 + 2*12 + 4 + 32)
	_ = binary.Write(&arw, binary.LittleEndian, uint16(2))
	_ = binary.Write(&arw, binary.LittleEndian, uint16(0x0201))
	_ = binary.Write(&arw, binary.LittleEndian, uint16(4))
	_ = binary.Write(&arw, binary.LittleEndian, uint32(1))
	_ = binary.Write(&arw, binary.LittleEndian, jpegOff)
	_ = binary.Write(&arw, binary.LittleEndian, uint16(0x0202))
	_ = binary.Write(&arw, binary.LittleEndian, uint16(4))
	_ = binary.Write(&arw, binary.LittleEndian, uint32(1))
	_ = binary.Write(&arw, binary.LittleEndian, uint32(len(mainJPEG)))
	_ = binary.Write(&arw, binary.LittleEndian, uint32(0)) // next IFD
	arw.Write(make([]byte, 32))
	arw.Write(mainJPEG)

	arwPath := filepath.Join(t.TempDir(), "sony_test.ARW")
	if err := os.WriteFile(arwPath, arw.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := DecodeCameraRAW(arwPath)
	if err != nil {
		t.Fatalf("DecodeCameraRAW(ARW) failed: %v", err)
	}
	if img.Bounds().Dx() != 480 || img.Bounds().Dy() != 320 {
		t.Fatalf("decoded ARW bounds = %v, want 480x320", img.Bounds())
	}

	// 2. Test proprietary container (.NAF) via universal segment-walking scanner
	// (no TIFF/RAF/CR3 header, nested EXIF FF D9 inside mainJPEG!).
	var naf bytes.Buffer
	naf.WriteString("PROPRIETARY-NAF-HEADER-V1\x00\x00\x00")
	naf.Write(make([]byte, 128))
	naf.Write(mainJPEG)
	naf.Write([]byte("TRAILING-SENSOR-DATA"))

	nafPath := filepath.Join(t.TempDir(), "camera_test.NAF")
	if err := os.WriteFile(nafPath, naf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	img2, err := DecodeCameraRAW(nafPath)
	if err != nil {
		t.Fatalf("DecodeCameraRAW(NAF) failed: %v", err)
	}
	if img2.Bounds().Dx() != 480 || img2.Bounds().Dy() != 320 {
		t.Fatalf("decoded NAF bounds = %v, want 480x320", img2.Bounds())
	}
}
