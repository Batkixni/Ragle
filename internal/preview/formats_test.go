package preview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodePSD_RGBA(t *testing.T) {
	// Synthesize a valid minimal 4x4 8-bit RGBA PSD (v1, Raw compression=0).
	var buf bytes.Buffer
	buf.WriteString("8BPS")
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // version
	buf.Write(make([]byte, 6))                          // reserved
	_ = binary.Write(&buf, binary.BigEndian, uint16(4)) // channels (R,G,B,A)
	_ = binary.Write(&buf, binary.BigEndian, uint32(4)) // height
	_ = binary.Write(&buf, binary.BigEndian, uint32(4)) // width
	_ = binary.Write(&buf, binary.BigEndian, uint16(8)) // depth
	_ = binary.Write(&buf, binary.BigEndian, uint16(3)) // RGB mode
	_ = binary.Write(&buf, binary.BigEndian, uint32(0)) // color mode data len
	_ = binary.Write(&buf, binary.BigEndian, uint32(0)) // image resources len
	_ = binary.Write(&buf, binary.BigEndian, uint32(0)) // layer/mask len
	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // compression: Raw
	// Planar R, G, B, A (16 bytes each)
	rPlane := bytes.Repeat([]byte{255}, 16)
	gPlane := bytes.Repeat([]byte{92}, 16)
	bPlane := bytes.Repeat([]byte{51}, 16)
	aPlane := bytes.Repeat([]byte{128}, 16)
	buf.Write(rPlane)
	buf.Write(gPlane)
	buf.Write(bPlane)
	buf.Write(aPlane)

	p := filepath.Join(t.TempDir(), "sample.psd")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	w, h := CreativeImageSize(p)
	if w != 4 || h != 4 {
		t.Fatalf("CreativeImageSize=%dx%d, want 4x4", w, h)
	}
	img, err := DecodePSD(p)
	if err != nil {
		t.Fatalf("DecodePSD: %v", err)
	}
	nrgba := img.(*image.NRGBA)
	got := nrgba.NRGBAAt(1, 1)
	want := color.NRGBA{R: 255, G: 92, B: 51, A: 128}
	if got != want {
		t.Fatalf("pixel=%+v, want %+v", got, want)
	}
}

func TestDecodeICO_PNGFrame(t *testing.T) {
	// Build a valid 1-frame ICO containing a 16x16 PNG.
	src := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	src.SetNRGBA(0, 0, color.NRGBA{R: 20, G: 180, B: 220, A: 255})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, src); err != nil {
		t.Fatal(err)
	}
	var ico bytes.Buffer
	_ = binary.Write(&ico, binary.LittleEndian, uint16(0))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	ico.Write([]byte{16, 16, 0, 0})
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(32))
	_ = binary.Write(&ico, binary.LittleEndian, uint32(pngBuf.Len()))
	_ = binary.Write(&ico, binary.LittleEndian, uint32(22))
	ico.Write(pngBuf.Bytes())

	p := filepath.Join(t.TempDir(), "icon.ico")
	if err := os.WriteFile(p, ico.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := DecodeICO(p)
	if err != nil {
		t.Fatalf("DecodeICO: %v", err)
	}
	if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 16 {
		t.Fatalf("ico bounds=%v", img.Bounds())
	}
}

func TestDecodeTGA_UncompressedBGRA(t *testing.T) {
	var tga bytes.Buffer
	hdr := make([]byte, 18)
	hdr[2] = 2 // uncompressed truecolor
	binary.LittleEndian.PutUint16(hdr[12:14], 2)
	binary.LittleEndian.PutUint16(hdr[14:16], 2)
	hdr[16] = 32
	hdr[17] = 0x20 // top-left origin
	tga.Write(hdr)
	// 4 pixels BGRA: (B=10, G=20, R=30, A=200)
	for range 4 {
		tga.Write([]byte{10, 20, 30, 200})
	}
	p := filepath.Join(t.TempDir(), "tex.tga")
	if err := os.WriteFile(p, tga.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := DecodeTGA(p)
	if err != nil {
		t.Fatalf("DecodeTGA: %v", err)
	}
	got := img.(*image.NRGBA).NRGBAAt(0, 0)
	want := color.NRGBA{R: 30, G: 20, B: 10, A: 200}
	if got != want {
		t.Fatalf("tga pixel=%+v, want %+v", got, want)
	}
}
