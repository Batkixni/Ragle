package preview

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExtractWAVWaveformAndThumbnail(t *testing.T) {
	// Synthesize a clean 1-second 440Hz 16-bit mono WAV file.
	const sampleRate = 8000
	const numSamples = sampleRate
	var buf bytes.Buffer
	dataSize := uint32(numSamples * 2)
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)
	for i := 0; i < numSamples; i++ {
		env := float64(i) / float64(numSamples)
		s := int16(env * 28000 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		_ = binary.Write(&buf, binary.LittleEndian, s)
	}

	wavPath := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(wavPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	wf, err := ExtractWaveform(wavPath, 64)
	if err != nil {
		t.Fatalf("ExtractWaveform failed: %v", err)
	}
	if len(wf.Peaks) != 64 {
		t.Fatalf("expected 64 peaks, got %d", len(wf.Peaks))
	}
	if math.Abs(wf.Duration-1.0) > 0.05 {
		t.Fatalf("expected ~1.0s duration, got %.3f", wf.Duration)
	}
	if wf.Peaks[60] <= wf.Peaks[4] {
		t.Fatalf("expected rising ramp peaks, got start=%.2f end=%.2f", wf.Peaks[4], wf.Peaks[60])
	}
	thumb := RenderWaveformThumbnail(wf, color.RGBA{237, 237, 237, 255}, color.RGBA{24, 24, 27, 255})
	if thumb == nil || thumb.Bounds().Dx() == 0 {
		t.Fatal("expected non-empty waveform thumbnail")
	}
}

func TestFontSpecimenRendering(t *testing.T) {
	// Try a standard Windows or system TrueType font when available.
	candidates := []string{
		`C:\Windows\Fonts\segoeui.ttf`,
		`C:\Windows\Fonts\arial.ttf`,
		`/System/Library/Fonts/Supplemental/Arial.ttf`,
		`/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf`,
	}
	var fontPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			fontPath = p
			break
		}
	}
	if fontPath == "" {
		t.Skip("no system TTF font found for rendering test")
	}
	img, info, err := RenderFontSpecimen(fontPath, "Ragle 0123456789", 40, 720, 420, color.RGBA{250, 250, 250, 255}, color.RGBA{18, 18, 20, 255})
	if err != nil {
		t.Fatalf("RenderFontSpecimen failed: %v", err)
	}
	if img.Bounds().Dx() != 720 || info.NumGlyphs == 0 {
		t.Fatalf("unexpected font render output: %+v", info)
	}
	thumb, err := RenderFontThumbnail(fontPath, color.RGBA{250, 250, 250, 255}, color.RGBA{24, 24, 27, 255})
	if err != nil || thumb.Bounds().Dx() == 0 {
		t.Fatalf("RenderFontThumbnail failed: %v", err)
	}
}

func TestMinimalPDFRenderWindows(t *testing.T) {
	// Minimal valid 2-page PDF document.
	pdf := `%PDF-1.4
1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj
2 0 obj << /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >> endobj
3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >> endobj
4 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >> endobj
xref
0 5
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000123 00000 n 
0000000192 00000 n 
trailer << /Size 5 /Root 1 0 R >>
startxref
261
%%EOF
`
	pdfPath := filepath.Join(t.TempDir(), "sample.pdf")
	if err := os.WriteFile(pdfPath, []byte(pdf), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := CountPDFPagesFallback(pdfPath); n != 2 {
		t.Fatalf("CountPDFPagesFallback = %d, want 2", n)
	}
	if runtime.GOOS == "windows" {
		img, total, err := RenderPDFPage(pdfPath, 1, 400)
		if err != nil {
			t.Fatalf("Windows native RenderPDFPage failed: %v", err)
		}
		if total != 2 || img == nil || img.Bounds().Dx() == 0 {
			t.Fatalf("unexpected PDF render: total=%d img=%v", total, img)
		}
	}
}
