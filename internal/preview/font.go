// Package preview provides native designer format previews (TTF/OTF fonts,
// audio waveform & playback, and multi-page PDF rendering) with CGO_ENABLED=0.
package preview

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// FontInfo holds metadata extracted from a TTF or OTF font file.
type FontInfo struct {
	Family    string
	Subfamily string
	FullName  string
	Format    string
	NumGlyphs int
}

// InspectFont parses a .ttf or .otf file and returns its name table metadata.
func InspectFont(path string) (FontInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return FontInfo{}, err
	}
	f, err := opentype.Parse(raw)
	if err != nil {
		return FontInfo{}, fmt.Errorf("parse font: %w", err)
	}
	var buf sfnt.Buffer
	info := FontInfo{
		Format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")),
		NumGlyphs: f.NumGlyphs(),
	}
	if s, err := f.Name(&buf, sfnt.NameIDFamily); err == nil {
		info.Family = strings.TrimSpace(s)
	}
	if s, err := f.Name(&buf, sfnt.NameIDSubfamily); err == nil {
		info.Subfamily = strings.TrimSpace(s)
	}
	if s, err := f.Name(&buf, sfnt.NameIDFull); err == nil {
		info.FullName = strings.TrimSpace(s)
	}
	if info.Family == "" {
		info.Family = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return info, nil
}

// RenderFontSpecimen renders custom preview text plus a multi-size typography
// waterfall using the font at path into an anti-aliased RGBA image.
func RenderFontSpecimen(path, sampleText string, sizePt float64, width, height int, fg, bg color.RGBA) (*image.RGBA, FontInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, FontInfo{}, err
	}
	f, err := opentype.Parse(raw)
	if err != nil {
		return nil, FontInfo{}, fmt.Errorf("parse font: %w", err)
	}
	var buf sfnt.Buffer
	info := FontInfo{
		Format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")),
		NumGlyphs: f.NumGlyphs(),
	}
	if s, err := f.Name(&buf, sfnt.NameIDFamily); err == nil {
		info.Family = strings.TrimSpace(s)
	}
	if s, err := f.Name(&buf, sfnt.NameIDSubfamily); err == nil {
		info.Subfamily = strings.TrimSpace(s)
	}
	if s, err := f.Name(&buf, sfnt.NameIDFull); err == nil {
		info.FullName = strings.TrimSpace(s)
	}
	if info.Family == "" {
		info.Family = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	if width < 320 {
		width = 880
	}
	if height < 240 {
		height = 560
	}
	if sizePt < 14 {
		sizePt = 44
	} else if sizePt > 120 {
		sizePt = 120
	}
	if strings.TrimSpace(sampleText) == "" {
		sampleText = "The quick brown fox jumps over the lazy dog 0123456789"
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	mutedFg := color.RGBA{
		R: uint8((int(fg.R)*60 + int(bg.R)*40) / 100),
		G: uint8((int(fg.G)*60 + int(bg.G)*40) / 100),
		B: uint8((int(fg.B)*60 + int(bg.B)*40) / 100),
		A: 255,
	}
	divCol := color.RGBA{
		R: uint8((int(fg.R)*14 + int(bg.R)*86) / 100),
		G: uint8((int(fg.G)*14 + int(bg.G)*86) / 100),
		B: uint8((int(fg.B)*14 + int(bg.B)*86) / 100),
		A: 255,
	}

	padX := 36
	curY := 32

	drawWrapped := func(text string, pt float64, col color.RGBA, maxLines int) {
		face, ferr := opentype.NewFace(f, &opentype.FaceOptions{
			Size:    pt,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if ferr != nil {
			return
		}
		defer face.Close()

		metrics := face.Metrics()
		ascent := metrics.Ascent.Ceil()
		lineH := (metrics.Height.Ceil() * 122) / 100
		if lineH < int(pt*1.15) {
			lineH = int(pt * 1.15)
		}
		maxW := fixed.I(width - padX*2)

		d := &font.Drawer{
			Dst:  img,
			Src:  &image.Uniform{C: col},
			Face: face,
		}

		// Wrap by rune/word to fit maxW cleanly.
		lines := wrapTextForFace(face, text, maxW)
		if maxLines > 0 && len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		for _, line := range lines {
			if curY+ascent > height-18 {
				return
			}
			d.Dot = fixed.P(padX, curY+ascent)
			d.DrawString(line)
			curY += lineH
		}
	}

	// Primary custom preview text at user-chosen point size.
	drawWrapped(sampleText, sizePt, fg, 4)

	// Subtle divider line.
	curY += 14
	if curY < height-60 {
		for x := padX; x < width-padX; x++ {
			img.SetRGBA(x, curY, divCol)
		}
		curY += 20
	}

	// Waterfall character set & numerals.
	waterfall := []struct {
		pt   float64
		text string
		col  color.RGBA
	}{
		{28, "ABCDEFGHIJKLMNOPQRSTUVWXYZ", fg},
		{28, "abcdefghijklmnopqrstuvwxyz", fg},
		{24, "0123456789  !@#$%^&*()_+-=[]{}|;:',.<>?/", mutedFg},
		{18, sampleText, mutedFg},
	}
	for _, row := range waterfall {
		if curY > height-32 {
			break
		}
		drawWrapped(row.text, row.pt, row.col, 1)
		curY += 6
	}

	return img, info, nil
}

// RenderFontThumbnail draws a crisp "Aa" specimen card thumbnail for library grids.
func RenderFontThumbnail(path string, fg, bg color.RGBA) (*image.RGBA, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := opentype.Parse(raw)
	if err != nil {
		return nil, err
	}
	const w, h = 440, 340
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	bigFace, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 148, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer bigFace.Close()

	d := &font.Drawer{Dst: img, Src: &image.Uniform{C: fg}, Face: bigFace}
	specimen := "Aa"
	adv := d.MeasureString(specimen).Ceil()
	x := (w - adv) / 2
	if x < 16 {
		x = 16
	}
	y := (h+bigFace.Metrics().Ascent.Ceil())/2 - 22
	d.Dot = fixed.P(x, y)
	d.DrawString(specimen)

	// Subtle secondary alphabet line beneath "Aa".
	smallFace, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 22, DPI: 72, Hinting: font.HintingFull})
	if err == nil {
		defer smallFace.Close()
		subCol := color.RGBA{
			R: uint8((int(fg.R)*55 + int(bg.R)*45) / 100),
			G: uint8((int(fg.G)*55 + int(bg.G)*45) / 100),
			B: uint8((int(fg.B)*55 + int(bg.B)*45) / 100),
			A: 255,
		}
		sd := &font.Drawer{Dst: img, Src: &image.Uniform{C: subCol}, Face: smallFace}
		sub := "ABC abc 123"
		sadv := sd.MeasureString(sub).Ceil()
		sx := (w - sadv) / 2
		if sx < 16 {
			sx = 16
		}
		sd.Dot = fixed.P(sx, h-46)
		sd.DrawString(sub)
	}
	return img, nil
}

func wrapTextForFace(face font.Face, text string, maxW fixed.Int26_6) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		var cur strings.Builder
		var curW fixed.Int26_6
		for _, r := range para {
			adv, ok := face.GlyphAdvance(r)
			if !ok {
				adv = fixed.I(8)
			}
			if cur.Len() > 0 && curW+adv > maxW {
				out = append(out, cur.String())
				cur.Reset()
				curW = 0
			}
			cur.WriteRune(r)
			curW += adv
		}
		if cur.Len() > 0 {
			out = append(out, cur.String())
		}
	}
	return out
}
