package ui

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"

	gui "github.com/egoist/mygo/ui"
)

//go:embed logo.png
var logoPNG []byte

//go:embed app_icon.png
var appIconPNG []byte

var logoBitmap = func() *gui.Bitmap {
	bm, err := gui.DecodeBitmap(logoPNG)
	if err != nil {
		return nil
	}
	return bm
}()

var appIconBitmap = func() *gui.Bitmap {
	bm, err := gui.DecodeBitmap(appIconPNG)
	if err != nil {
		return nil
	}
	return bm
}()

// logoBitmapDark inverts dark-ink pixels to crisp pure white while preserving alpha,
// so the monochrome Ragle emblem stays razor-sharp in dark mode.
var logoBitmapDark = func() *gui.Bitmap {
	src, err := png.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		return logoBitmap
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			if c.A > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{
					R: 255 - c.R,
					G: 255 - c.G,
					B: 255 - c.B,
					A: c.A,
				})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return logoBitmap
	}
	bm, err := gui.DecodeBitmap(buf.Bytes())
	if err != nil {
		return logoBitmap
	}
	return bm
}()

// brandLogo returns the pure solid monochrome Ragle mark (white in dark mode,
// black in light mode) for top-left chrome and headers.
func (a *App) brandLogo() *gui.Bitmap {
	if a.dark && logoBitmapDark != nil {
		return logoBitmapDark
	}
	return logoBitmap
}

// appIcon returns the full dark-charcoal squircle app icon with #FF5C33 emblem.
func (a *App) appIcon() *gui.Bitmap {
	if appIconBitmap != nil {
		return appIconBitmap
	}
	return a.brandLogo()
}

func svgIcon(inner string) *gui.SVG {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#000000" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + inner + `</svg>`
	return gui.MustParseSVG([]byte(doc))
}

var (
	iconLibrary       = svgIcon(`<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>`)
	iconInbox         = svgIcon(`<polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>`)
	iconTag           = svgIcon(`<path d="M12 2H2v10l9.29 9.29a1 1 0 0 0 1.41 0l6.59-6.59a1 1 0 0 0 0-1.41Z"/><circle cx="7.5" cy="7.5" r="1.2"/>`)
	iconFolder        = svgIcon(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
	iconBoard         = svgIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/><path d="M9 21V9"/>`)
	iconPlus          = svgIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	iconImport        = svgIcon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>`)
	iconExport        = svgIcon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/>`)
	iconFilter        = svgIcon(`<polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/>`)
	iconSort          = svgIcon(`<path d="m3 16 4 4 4-4"/><path d="M7 20V4"/><path d="m21 8-4-4-4 4"/><path d="M17 4v16"/>`)
	iconPanelLeft     = svgIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/>`)
	iconPanelRight    = svgIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M15 3v18"/>`)
	iconPanelBottom   = svgIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 15h18"/>`)
	iconShare         = svgIcon(`<circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>`)
	iconLink          = svgIcon(`<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>`)
	iconTrash         = svgIcon(`<polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	iconCopy          = svgIcon(`<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	iconEye           = svgIcon(`<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>`)
	iconAlert         = svgIcon(`<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>`)
	iconRefresh       = svgIcon(`<path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/><path d="M16 16h5v5"/>`)
	iconExternal      = svgIcon(`<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/>`)
	iconArrowLeft     = svgIcon(`<line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/>`)
	iconChevronLeft   = svgIcon(`<polyline points="15 18 9 12 15 6"/>`)
	iconChevronRgt    = svgIcon(`<polyline points="9 18 15 12 9 6"/>`)
	iconX             = svgIcon(`<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>`)
	iconCheck         = svgIcon(`<polyline points="20 6 9 17 4 12"/>`)
	iconCursor        = svgIcon(`<path d="m4 4 7.07 17 2.51-7.39L21 11.07z"/>`)
	iconHand          = svgIcon(`<path d="M18 11V6a2 2 0 0 0-2-2 2 2 0 0 0-2 2"/><path d="M14 10V4a2 2 0 0 0-2-2 2 2 0 0 0-2 2v2"/><path d="M10 10.5V6a2 2 0 0 0-2-2 2 2 0 0 0-2 2v8"/><path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/>`)
	iconType          = svgIcon(`<polyline points="4 7 4 4 20 4 20 7"/><line x1="9" y1="20" x2="15" y2="20"/><line x1="12" y1="4" x2="12" y2="20"/>`)
	iconGroup         = svgIcon(`<rect x="3" y="3" width="8" height="8" rx="1"/><rect x="13" y="13" width="8" height="8" rx="1"/><path d="M13 5h6v6"/><path d="M5 13v6h6"/>`)
	iconUngroup       = svgIcon(`<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/><line x1="14" y1="5" x2="20" y2="5"/><line x1="5" y1="14" x2="5" y2="20"/>`)
	iconZoomIn        = svgIcon(`<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/>`)
	iconZoomOut       = svgIcon(`<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="8" y1="11" x2="14" y2="11"/>`)
	iconImage         = svgIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"/>`)
	iconVideo         = svgIcon(`<polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2"/>`)
	iconFile          = svgIcon(`<path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/>`)
	iconSettings      = svgIcon(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>`)
	iconSparkle       = svgIcon(`<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>`)
	iconDatabase      = svgIcon(`<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14a9 3 0 0 0 18 0V5"/><path d="M3 12a9 3 0 0 0 18 0"/>`)
	iconSliders       = svgIcon(`<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>`)
	iconEdit          = svgIcon(`<path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 1 1 3 3L7 19l-4 1 1-4 12.5-12.5z"/>`)
	iconMinimize      = svgIcon(`<line x1="5" y1="12" x2="19" y2="12"/>`)
	iconMaximize      = svgIcon(`<rect x="5" y="5" width="14" height="14" rx="1.8"/>`)
	iconRestore       = svgIcon(`<rect x="8" y="4" width="12" height="12" rx="1.5"/><path d="M16 16v3a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 4 19v-9A1.5 1.5 0 0 1 5.5 8H8"/>`)
	iconLayoutBento   = svgIcon(`<rect x="3" y="3" width="8" height="18" rx="1.5"/><rect x="14" y="3" width="7" height="7.5" rx="1.5"/><rect x="14" y="13.5" width="7" height="7.5" rx="1.5"/>`)
	iconLayoutMasonry = svgIcon(`<rect x="3" y="3" width="7" height="10" rx="1.5"/><rect x="14" y="3" width="7" height="6" rx="1.5"/><rect x="3" y="16" width="7" height="5" rx="1.5"/><rect x="14" y="12" width="7" height="9" rx="1.5"/>`)
	iconPlay          = svgIcon(`<polygon points="6 3 20 12 6 21 6 3"/>`)
	iconPause         = svgIcon(`<rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/>`)
	iconRepeat        = svgIcon(`<polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>`)
	iconCheckSquare   = svgIcon(`<polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/>`)
	iconChevronDown   = svgIcon(`<polyline points="6 9 12 15 18 9"/>`)
	iconPalette       = svgIcon(`<circle cx="13.5" cy="6.5" r=".5"/><circle cx="17.5" cy="10.5" r=".5"/><circle cx="8.5" cy="7.5" r=".5"/><circle cx="6.5" cy="12.5" r=".5"/><path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c.926 0 1.648-.746 1.648-1.688 0-.437-.18-.835-.437-1.125-.29-.289-.438-.652-.438-1.125a1.64 1.64 0 0 1 1.668-1.668h1.996c3.051 0 5.555-2.503 5.555-5.554C21.965 6.012 17.461 2 12 2z"/>`)
	iconHelp          = svgIcon(`<circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/>`)
	iconClock         = svgIcon(`<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>`)
	iconShuffle       = svgIcon(`<polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/><polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/><line x1="4" y1="4" x2="9" y2="9"/>`)
	iconCommand       = svgIcon(`<path d="M18 3a3 3 0 0 0-3 3v12a3 3 0 0 0 3 3 3 3 0 0 0 3-3 3 3 0 0 0-3-3H6a3 3 0 0 0-3 3 3 3 0 0 0 3 3 3 3 0 0 0 3-3V6a3 3 0 0 0-3-3 3 3 0 0 0-3 3 3 3 0 0 0 3 3h12a3 3 0 0 0 3-3 3 3 0 0 0-3-3z"/>`)
	iconLayers        = svgIcon(`<polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/>`)
	iconPin           = svgIcon(`<line x1="12" y1="17" x2="12" y2="22"/><path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>`)
	iconMusic         = svgIcon(`<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`)
	iconVolume        = svgIcon(`<polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>`)
	iconFileText      = svgIcon(`<path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/>`)
	iconChecker       = svgIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><rect x="3" y="3" width="6" height="6" fill="currentColor" stroke="none"/><rect x="15" y="3" width="6" height="6" fill="currentColor" stroke="none"/><rect x="9" y="9" width="6" height="6" fill="currentColor" stroke="none"/><rect x="3" y="15" width="6" height="6" fill="currentColor" stroke="none"/><rect x="15" y="15" width="6" height="6" fill="currentColor" stroke="none"/>`)
)
