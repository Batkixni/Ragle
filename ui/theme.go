package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	gui "github.com/egoist/mygo/ui"
)

// Arc UI 4-step border radius scale (from foundation.css & design.md §3).
const (
	radiusControl float32 = 12
	radiusPanel   float32 = 16
	radiusSurface float32 = 20
	radiusPill    float32 = 9999
)

// cubicBezier matches CSS cubic-bezier(x1, y1, x2, y2) with Newton-Raphson solving.
func cubicBezier(x1, y1, x2, y2 float64) gui.Easing {
	return func(t float32) float32 {
		if t <= 0 {
			return 0
		}
		if t >= 1 {
			return 1
		}
		x := float64(t)
		u := x
		for range 6 {
			cx := 3*(1-u)*(1-u)*u*x1 + 3*(1-u)*u*u*x2 + u*u*u
			dx := 3*(1-u)*(1-u)*x1 + 6*(1-u)*u*(x2-x1) + 3*u*u*(1-x2)
			if math.Abs(dx) < 1e-6 {
				break
			}
			u -= (cx - x) / dx
			if u < 0 {
				u = 0
			} else if u > 1 {
				u = 1
			}
		}
		y := 3*(1-u)*(1-u)*u*y1 + 3*(1-u)*u*u*y2 + u*u*u
		return float32(y)
	}
}

// Exact Arc UI motion tokens from motion-tokens.ts.
var (
	easeArcEnter    = cubicBezier(0.16, 1, 0.3, 1)
	easeArcStandard = cubicBezier(0.22, 1, 0.36, 1)
	easeArcExit     = cubicBezier(0.7, 0, 0.84, 0)

	// Presses, active scale, immediate tactile feedback.
	springResponsive = gui.Animation{
		Spring: gui.SpringOptions{Stiffness: 520, Damping: 44},
	}
	// Card hover lifts & image hover zoom.
	springGentle = gui.Animation{
		Spring: gui.SpringOptions{Stiffness: 340, Damping: 34},
	}
	// Toggles, thumbs, small indicators — critically damped, zero bounce.
	springSnappy = gui.Animation{
		Spring: gui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0},
	}
	// Panels, height changes, collapse/expand shifts — critically damped, zero bounce.
	springSmooth = gui.Animation{
		Spring: gui.SpringOptions{Duration: 400 * time.Millisecond, Bounce: 0},
	}
	// Sidebar & inspector slide open/close — brisk, critically damped, zero bounce.
	springSidebar = gui.Animation{
		Spring: gui.SpringOptions{Duration: 280 * time.Millisecond, Bounce: 0},
	}
	// Shared-layout highlights & pill morphs — critically damped, zero bounce.
	springMorph = gui.Animation{
		Spring: gui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0},
	}

	animFast = gui.Animation{
		Duration: 160 * time.Millisecond,
		Ease:     easeArcStandard,
	}
	animCardFade = gui.Animation{
		Duration: 220 * time.Millisecond,
		Ease:     easeArcEnter,
	}
	animStandard = gui.Animation{
		Duration: 240 * time.Millisecond,
		Ease:     easeArcEnter,
	}
)

const (
	arcStaggerItem = 35 * time.Millisecond
	arcBlurSoft    = float32(4)
	arcBlurCard    = float32(5)
)

// palette is a pure achromatic monochrome system (zero blue, zero glare/rims),
// directly matching Arc UI foundation.css neutral surfaces.
type palette struct {
	canvas         gui.Color // --background: #FFFFFF / #111111
	surface        gui.Color // --surface: #F4F4F5 / #18181B
	surfaceRaised  gui.Color // --surface-raised: #FFFFFF / #1F1F23
	surfaceMuted   gui.Color // --surface-muted: #E4E4E7 / #27272A
	surfaceActive  gui.Color // subtle pressed / selected neutral surface
	ink            gui.Color // --foreground: #18181B / #FAFAFA
	inkSecondary   gui.Color // --text-secondary: #71717A / #A1A1AA
	inkMuted       gui.Color // --text-muted: #A1A1AA / #71717A
	inkFaint       gui.Color // quietest hint text
	controlOn      gui.Color // --control-on: #18181B / #EDEDED
	controlGlyph   gui.Color // --control-glyph: #FFFFFF / #18181B
	pillTrack      gui.Color // segmented control / tabs track
	pillActive     gui.Color // active gliding pill surface
	danger         gui.Color // muted rose for destructive actions / missing only
	shadowResting  gui.Color // --shadow-resting
	shadowRaised   gui.Color // --shadow-raised
	shadowFloating gui.Color // --shadow-floating
	paper          gui.Color // moodboard canvas paper
	paperInk       gui.Color // moodboard default ink
	paperGrid      gui.Color // moodboard subtle dot grid
}

// themeOverrides lets users pick a built-in studio preset or customize
// canvas, surface, accent, and text colors in Preferences.
type themeOverrides struct {
	preset  string
	canvas  string
	surface string
	accent  string
	text    string
}

type themePresetSpec struct {
	id      string
	dark    bool
	canvas  string
	surface string
	accent  string
	text    string
}

var themePresets = []themePresetSpec{
	{id: "", dark: true, canvas: "#111111", surface: "#18181B", accent: "#EDEDED", text: "#FAFAFA"},
	{id: "graphite", dark: true, canvas: "#18191C", surface: "#222428", accent: "#E4E4E7", text: "#F4F5F7"},
	{id: "warm", dark: false, canvas: "#F6F3EE", surface: "#EDE8DF", accent: "#1F1B17", text: "#1F1B17"},
	{id: "midnight", dark: true, canvas: "#090A0D", surface: "#12141A", accent: "#FFFFFF", text: "#FAFAFA"},
}

func parseHexColor(s string) (gui.Color, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return gui.Color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return gui.Color{}, false
	}
	return gui.RGBA(
		uint8((v>>16)&0xFF),
		uint8((v>>8)&0xFF),
		uint8(v&0xFF),
		1,
	), true
}

func mixColor(a, b gui.Color, t float32) gui.Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	lerp := func(x, y uint8) uint8 {
		v := float32(x)*(1-t) + float32(y)*t
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v + 0.5)
	}
	return gui.RGBA(lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B), 1)
}

func colorLuminance(c gui.Color) float32 {
	return (0.299*float32(c.R) + 0.587*float32(c.G) + 0.114*float32(c.B)) / 255
}

func makeThemeWithOverrides(dark bool, ov themeOverrides) (*gui.Theme, palette, bool) {
	// Apply preset defaults when a named preset is chosen (unless custom).
	if ov.preset != "" && ov.preset != "custom" {
		for _, pr := range themePresets {
			if pr.id == ov.preset {
				dark = pr.dark
				if ov.canvas == "" {
					ov.canvas = pr.canvas
				}
				if ov.surface == "" {
					ov.surface = pr.surface
				}
				if ov.accent == "" {
					ov.accent = pr.accent
				}
				if ov.text == "" {
					ov.text = pr.text
				}
				break
			}
		}
	}
	if c, ok := parseHexColor(ov.canvas); ok {
		dark = colorLuminance(c) < 0.45
	}
	t, p := makeTheme(dark)
	hasCustom := false

	if c, ok := parseHexColor(ov.canvas); ok {
		hasCustom = true
		p.canvas = c
		t.Background = c
		if dark {
			p.paper = mixColor(c, gui.Hex("#FFFFFF"), 0.03)
		} else {
			p.paper = mixColor(c, gui.Hex("#000000"), 0.03)
		}
	}
	if c, ok := parseHexColor(ov.surface); ok {
		hasCustom = true
		p.surface = c
		t.Surface = c
		if dark {
			p.surfaceRaised = mixColor(c, gui.Hex("#FFFFFF"), 0.05)
			p.surfaceMuted = mixColor(c, gui.Hex("#FFFFFF"), 0.10)
			p.surfaceActive = mixColor(c, gui.Hex("#FFFFFF"), 0.16)
			p.pillTrack = mixColor(p.canvas, c, 0.35)
			p.pillActive = mixColor(c, gui.Hex("#FFFFFF"), 0.14)
		} else {
			p.surfaceRaised = mixColor(c, gui.Hex("#FFFFFF"), 0.72)
			p.surfaceMuted = mixColor(c, gui.Hex("#000000"), 0.07)
			p.surfaceActive = mixColor(c, gui.Hex("#000000"), 0.13)
			p.pillTrack = mixColor(c, gui.Hex("#000000"), 0.08)
			p.pillActive = gui.Hex("#FFFFFF")
		}
		t.SurfaceHover = p.surfaceMuted
		t.SurfacePressed = p.surfaceActive
	}
	if c, ok := parseHexColor(ov.accent); ok {
		hasCustom = true
		p.controlOn = c
		if colorLuminance(c) > 0.56 {
			p.controlGlyph = gui.Hex("#141416")
		} else {
			p.controlGlyph = gui.Hex("#FFFFFF")
		}
		t.Accent = c
		t.AccentText = p.controlGlyph
		t.Inverse = c
		t.InverseText = p.controlGlyph
		if dark {
			t.AccentHover = mixColor(c, gui.Hex("#FFFFFF"), 0.14)
			t.AccentPressed = mixColor(c, gui.Hex("#000000"), 0.14)
		} else {
			t.AccentHover = mixColor(c, gui.Hex("#000000"), 0.12)
			t.AccentPressed = mixColor(c, gui.Hex("#000000"), 0.22)
		}
	}
	if c, ok := parseHexColor(ov.text); ok {
		hasCustom = true
		p.ink = c
		p.paperInk = c
		t.Text = c
		p.inkSecondary = mixColor(c, p.surface, 0.36)
		p.inkMuted = mixColor(c, p.surface, 0.56)
		p.inkFaint = mixColor(c, p.surface, 0.70)
		t.TextMuted = p.inkSecondary
	}
	_ = hasCustom
	return t, p, dark
}

func makeTheme(dark bool) (*gui.Theme, palette) {
	if dark {
		t := &gui.Theme{
			Dark:           true,
			Background:     gui.Hex("#111111"),
			Surface:        gui.Hex("#18181B"),
			SurfaceHover:   gui.Hex("#27272A"),
			SurfacePressed: gui.Hex("#323236"),
			Border:         gui.Transparent,
			Text:           gui.Hex("#FAFAFA"),
			TextMuted:      gui.Hex("#A1A1AA"),
			Accent:         gui.Hex("#EDEDED"),
			AccentHover:    gui.Hex("#FFFFFF"),
			AccentPressed:  gui.Hex("#D4D4D8"),
			AccentText:     gui.Hex("#141416"),
			Danger:         gui.Hex("#E07A7A"),
			Warning:        gui.Hex("#D4D4D8"),
			Success:        gui.Hex("#D4D4D8"),
			Selection:      gui.RGBA(255, 255, 255, 0.14),
			Focus:          gui.Transparent,
			Inverse:        gui.Hex("#EDEDED"),
			InverseText:    gui.Hex("#141416"),
			Scrollbar:      gui.RGBA(255, 255, 255, 0.16),
			ScrollbarWidth: 5,
			Radius:         radiusControl,
			Spacing:        4,
			FontSize:       13,
		}
		p := palette{
			canvas:         gui.Hex("#111111"),
			surface:        gui.Hex("#18181B"),
			surfaceRaised:  gui.Hex("#1F1F23"),
			surfaceMuted:   gui.Hex("#27272A"),
			surfaceActive:  gui.Hex("#323236"),
			ink:            gui.Hex("#FAFAFA"),
			inkSecondary:   gui.Hex("#A1A1AA"),
			inkMuted:       gui.Hex("#71717A"),
			inkFaint:       gui.Hex("#52525B"),
			controlOn:      gui.Hex("#EDEDED"),
			controlGlyph:   gui.Hex("#141416"),
			pillTrack:      gui.Hex("#141417"),
			pillActive:     gui.Hex("#2C2C31"),
			danger:         gui.Hex("#E07A7A"),
			shadowResting:  gui.RGBA(0, 0, 0, 0.18),
			shadowRaised:   gui.RGBA(0, 0, 0, 0.24),
			shadowFloating: gui.RGBA(0, 0, 0, 0.38),
			paper:          gui.Hex("#141416"),
			paperInk:       gui.Hex("#EDEDED"),
			paperGrid:      gui.RGBA(255, 255, 255, 0.06),
		}
		return t, p
	}

	t := &gui.Theme{
		Dark:           false,
		Background:     gui.Hex("#FFFFFF"),
		Surface:        gui.Hex("#F4F4F5"),
		SurfaceHover:   gui.Hex("#E4E4E7"),
		SurfacePressed: gui.Hex("#D4D4D8"),
		Border:         gui.Transparent,
		Text:           gui.Hex("#18181B"),
		TextMuted:      gui.Hex("#71717A"),
		Accent:         gui.Hex("#18181B"),
		AccentHover:    gui.Hex("#27272A"),
		AccentPressed:  gui.Hex("#09090B"),
		AccentText:     gui.Hex("#FFFFFF"),
		Danger:         gui.Hex("#B91C1C"),
		Warning:        gui.Hex("#71717A"),
		Success:        gui.Hex("#18181B"),
		Selection:      gui.RGBA(24, 24, 27, 0.12),
		Focus:          gui.Transparent,
		Inverse:        gui.Hex("#18181B"),
		InverseText:    gui.Hex("#FFFFFF"),
		Scrollbar:      gui.RGBA(0, 0, 0, 0.15),
		ScrollbarWidth: 5,
		Radius:         radiusControl,
		Spacing:        4,
		FontSize:       13,
	}
	p := palette{
		canvas:         gui.Hex("#FFFFFF"),
		surface:        gui.Hex("#F4F4F5"),
		surfaceRaised:  gui.Hex("#FFFFFF"),
		surfaceMuted:   gui.Hex("#E4E4E7"),
		surfaceActive:  gui.Hex("#D4D4D8"),
		ink:            gui.Hex("#18181B"),
		inkSecondary:   gui.Hex("#71717A"),
		inkMuted:       gui.Hex("#A1A1AA"),
		inkFaint:       gui.Hex("#C4C4CC"),
		controlOn:      gui.Hex("#18181B"),
		controlGlyph:   gui.Hex("#FFFFFF"),
		pillTrack:      gui.Hex("#E4E4E7"),
		pillActive:     gui.Hex("#FFFFFF"),
		danger:         gui.Hex("#B91C1C"),
		shadowResting:  gui.RGBA(0, 0, 0, 0.035),
		shadowRaised:   gui.RGBA(0, 0, 0, 0.065),
		shadowFloating: gui.RGBA(0, 0, 0, 0.11),
		paper:          gui.Hex("#F5F5F4"),
		paperInk:       gui.Hex("#18181B"),
		paperGrid:      gui.RGBA(0, 0, 0, 0.06),
	}
	return t, p
}

// applyBlurReveal runs Arc UI's signature soft blur-in reveal for initial
// page/modal/inspector entrance (opacity 0->1, blur 4px->0px, y 6px->0px).
// Once settled, Blur and Opacity are omitted so resting GPU cost is zero.
func applyBlurReveal(e gui.Element, key any, delay time.Duration) {
	started := gui.Local(e, key, func() bool { return false })
	if !*started {
		*started = true
		e.Jump("arc-reveal", 0)
	}
	anim := animStandard
	anim.Delay = delay
	p := e.AnimateTo("arc-reveal", 1, anim)
	if p < 0.999 {
		e.Opacity(p)
		if sigma := (1 - p) * arcBlurSoft; sigma > 0.08 {
			e.Blur(sigma)
		}
		e.Translate(0, (1-p)*6)
	}
}

// applyCardBlurFade matches MatchesPage.tsx on esports.sorai.tw/matches:
// pure in-place staggered blur-fade (opacity 0->1, blur 5px->0px, 220ms
// easeArcEnter, stagger min(i, 6)*35ms) with zero vertical jump or scale
// jitter, and zero conflict with card hover AnimatePose.
func applyCardBlurFade(e gui.Element, key any, delay time.Duration) {
	started := gui.Local(e, key, func() bool { return false })
	if !*started {
		*started = true
		e.Jump("arc-card-fade", 0)
	}
	anim := animCardFade
	anim.Delay = delay
	p := e.AnimateTo("arc-card-fade", 1, anim)
	if p < 0.999 {
		e.Opacity(p)
		if sigma := (1 - p) * arcBlurCard; sigma > 0.08 {
			e.Blur(sigma)
		}
	}
}

// applyPanelSwitch matches Arc UI TabsContent directional panel transition:
// smooth opacity fade (240ms easeArcEnter) + critically-damped directional
// glide (dx, dy -> 0 with springSmooth) + brief subtle blur.
func applyPanelSwitch(e gui.Element, key any, dx, dy float32) {
	started := gui.Local(e, key, func() bool { return false })
	if !*started {
		*started = true
		e.Jump("arc-panel-op", 0)
		e.Jump("arc-panel-mv", 0)
	}
	op := e.AnimateTo("arc-panel-op", 1, animStandard)
	mv := e.AnimateTo("arc-panel-mv", 1, springSmooth)
	if op < 0.999 {
		e.Opacity(op)
		if sigma := (1 - op) * 3; sigma > 0.08 {
			e.Blur(sigma)
		}
	}
	if mv < 0.999 && (dx != 0 || dy != 0) {
		e.Translate((1-mv)*dx, (1-mv)*dy)
	}
}

// attachGlidingPill paints an Arc UI shared-layout selection pill underneath
// all stationary child labels using track.Draw + LayoutBounds + springMorph
// (420ms, zero bounce). Labels never move, stretch, or flicker during switches.
func attachGlidingPill(track, activeBtn gui.Element, hasActive bool, radius float32, fill, shadow gui.Color) {
	if !hasActive {
		return
	}
	tb := track.LayoutBounds()
	ab := activeBtn.LayoutBounds()
	if tb.W <= 0 || ab.W <= 0 || ab.H <= 0 {
		// Initial layout frame before LayoutBounds are populated.
		activeBtn.Background(fill).Shadow(0, 1, 3, 0, shadow)
		return
	}
	rx := ab.X - tb.X
	ry := ab.Y - tb.Y
	rw := ab.W
	rh := ab.H

	inited := gui.Local(track, "arc-pill-init", func() bool { return false })
	if !*inited {
		*inited = true
		track.Jump("pill-x", rx)
		track.Jump("pill-y", ry)
		track.Jump("pill-w", rw)
		track.Jump("pill-h", rh)
	}
	px := track.AnimateTo("pill-x", rx, springMorph)
	py := track.AnimateTo("pill-y", ry, springMorph)
	pw := track.AnimateTo("pill-w", rw, springMorph)
	ph := track.AnimateTo("pill-h", rh, springMorph)

	track.Draw(func(p *gui.Painter, r gui.Rect) {
		pill := gui.Rect{X: r.X + px, Y: r.Y + py, W: pw, H: ph}
		rad := radius
		if maxR := pill.H / 2; rad > maxR {
			rad = maxR
		}
		p.Shadow(pill, rad, 0, 1, 3, 0, shadow)
		p.Fill(pill, fill, rad)
	})
}

// windowControls renders integrated monochrome frameless window buttons
// (minimize, maximize/restore with Windows 11 Snap Layouts support, close).
func (a *App) windowControls(c *gui.Context) {
	maximized := a.win != nil && a.win.IsMaximized()

	gui.Row(c.Key("win-controls")).Gap(2).AlignItems(gui.Center).Children(func() {
		// Minimize.
		minBtn := gui.ButtonBase(c.Key("win-min")).
			Size(30, 28).
			Center().
			Radius(8).
			Label(a.t("win.minimize")).
			Tooltip(a.t("win.minimize")).
			OnClick(func() {
				if a.win != nil {
					a.win.Minimize()
				}
			})
		minBg := gui.Transparent
		minFg := a.pal.inkSecondary
		if minBtn.Pressed() {
			minBg = a.pal.surfaceActive
			minFg = a.pal.ink
		} else if minBtn.Hovered() {
			minBg = a.pal.surfaceMuted
			minFg = a.pal.ink
		}
		minBtn.Background(minBtn.AnimateColor("bg", minBg, animFast)).
			TextColor(minBtn.AnimateColor("fg", minFg, animFast)).
			Children(func() {
				gui.Icon(c, iconMinimize).Size(13, 13)
			})

		// Maximize / Restore (with Windows 11 Snap Layouts via MaximizeButton).
		maxTip := a.t("win.maximize")
		maxIcon := iconMaximize
		if maximized {
			maxTip = a.t("win.restore")
			maxIcon = iconRestore
		}
		maxBtn := gui.ButtonBase(c.Key("win-max")).
			Size(30, 28).
			Center().
			Radius(8).
			Label(maxTip).
			Tooltip(maxTip).
			MaximizeButton().
			OnClick(func() {
				if a.win == nil {
					return
				}
				if a.win.IsMaximized() {
					a.win.Unmaximize()
				} else {
					a.win.Maximize()
				}
			})
		maxBg := gui.Transparent
		maxFg := a.pal.inkSecondary
		if maxBtn.Pressed() {
			maxBg = a.pal.surfaceActive
			maxFg = a.pal.ink
		} else if maxBtn.Hovered() {
			maxBg = a.pal.surfaceMuted
			maxFg = a.pal.ink
		}
		maxBtn.Background(maxBtn.AnimateColor("bg", maxBg, animFast)).
			TextColor(maxBtn.AnimateColor("fg", maxFg, animFast)).
			Children(func() {
				gui.Icon(c, maxIcon).Size(12, 12)
			})

		// Close.
		closeBtn := gui.ButtonBase(c.Key("win-close")).
			Size(30, 28).
			Center().
			Radius(8).
			Label(a.t("win.close")).
			Tooltip(a.t("win.close")).
			OnClick(func() {
				if a.win != nil {
					a.win.Close()
				}
			})
		closeBg := gui.Transparent
		closeFg := a.pal.inkSecondary
		if closeBtn.Pressed() || closeBtn.Hovered() {
			if a.dark {
				closeBg = gui.RGBA(224, 122, 122, 0.22)
			} else {
				closeBg = gui.RGBA(185, 28, 28, 0.14)
			}
			closeFg = a.pal.danger
		}
		closeBtn.Background(closeBtn.AnimateColor("bg", closeBg, animFast)).
			TextColor(closeBtn.AnimateColor("fg", closeFg, animFast)).
			Children(func() {
				gui.Icon(c, iconX).Size(13, 13)
			})
	})
}

// arcButton is a frameless monochrome Arc button with critically damped press scale.
func (a *App) arcButton(c *gui.Context, key, label string, primary bool) gui.Element {
	btn := gui.ButtonBase(c.Key(key)).
		Padding(8, 14).
		Radius(radiusControl).
		AlignItems(gui.Center).
		Justify(gui.Center)
	targetBg := a.pal.surfaceMuted
	targetFg := a.pal.ink
	if primary {
		targetBg = a.pal.controlOn
		targetFg = a.pal.controlGlyph
	}
	scale := float32(1)
	if btn.Pressed() {
		scale = 0.97
		if !primary {
			targetBg = a.pal.surfaceActive
		}
	} else if btn.Hovered() {
		if primary {
			targetBg = c.Theme().AccentHover
		} else {
			targetBg = a.pal.surfaceActive
		}
	}
	bg := btn.AnimateColor("bg", targetBg, animFast)
	fg := btn.AnimateColor("fg", targetFg, animFast)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, springResponsive).Children(func() {
		gui.Text(c, label).FontSize(12).Bold().SingleLine()
	})
	return btn
}

// iconButton is a frameless circular monochrome icon button.
func (a *App) iconButton(c *gui.Context, key string, icon *gui.SVG, tip string, active bool) gui.Element {
	btn := gui.ButtonBase(c.Key(key)).Size(32, 32).Center().Radius(radiusPill).Label(tip).Tooltip(tip)
	targetBg := gui.Transparent
	targetFg := a.pal.inkSecondary
	if active {
		targetBg = a.pal.surfaceMuted
		targetFg = a.pal.ink
	}
	scale := float32(1)
	if btn.Pressed() {
		targetBg = a.pal.surfaceActive
		targetFg = a.pal.ink
		scale = 0.94
	} else if btn.Hovered() {
		targetBg = a.pal.surfaceMuted
		targetFg = a.pal.ink
	}
	bg := btn.AnimateColor("bg", targetBg, animFast)
	fg := btn.AnimateColor("fg", targetFg, animFast)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, springResponsive).Children(func() {
		gui.Icon(c, icon).Size(15, 15)
	})
	return btn
}

// toolButton is a frameless monochrome toolbar action with tactile spring press.
func (a *App) toolButton(c *gui.Context, key string, icon *gui.SVG, label string, active bool) gui.Element {
	btn := gui.ButtonBase(c.Key(key)).Padding(6, 12).Gap(6).Radius(radiusControl).AlignItems(gui.Center)
	targetBg := gui.Transparent
	targetFg := a.pal.inkSecondary
	if active {
		targetBg = a.pal.controlOn
		targetFg = a.pal.controlGlyph
	}
	scale := float32(1)
	if btn.Pressed() {
		scale = 0.97
		if !active {
			targetBg = a.pal.surfaceActive
			targetFg = a.pal.ink
		}
	} else if btn.Hovered() && !active {
		targetBg = a.pal.surfaceMuted
		targetFg = a.pal.ink
	}
	bg := btn.AnimateColor("bg", targetBg, animFast)
	fg := btn.AnimateColor("fg", targetFg, animFast)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, springResponsive).Children(func() {
		if icon != nil {
			gui.Icon(c, icon).Size(14, 14)
		}
		if label != "" {
			gui.Text(c, label).FontSize(12).Bold().SingleLine()
		}
	})
	return btn
}

// arcPillTabs renders an Arc UI segmented tab track whose highlight pill
// glides smoothly underneath stationary labels (springMorph, 420ms, bounce: 0).
func (a *App) arcPillTabs(c *gui.Context, group string, selected int, labels []string, onSelect func(int)) {
	track := gui.Row(c.Key("track-" + group)).
		Padding(3).
		Gap(2).
		Radius(radiusControl).
		Background(a.pal.pillTrack).
		AlignItems(gui.Center)
	var activeBtn gui.Element
	var hasActive bool
	track.Children(func() {
		for i, lbl := range labels {
			idx := i
			active := selected == idx
			btn := gui.ButtonBase(c.Key(fmt.Sprintf("%s-%d", group, idx))).
				Padding(5, 12).
				Radius(radiusControl - 3).
				AlignItems(gui.Center).
				OnClick(func() { onSelect(idx) })
			targetFg := a.pal.inkSecondary
			if active || btn.Hovered() {
				targetFg = a.pal.ink
			}
			fg := btn.AnimateColor("fg", targetFg, animFast)
			btn.Children(func() {
				gui.Text(c, lbl).FontSize(12).Bold().TextColor(fg).SingleLine()
			})
			if active {
				activeBtn = btn
				hasActive = true
			}
		}
	})
	attachGlidingPill(track, activeBtn, hasActive, radiusControl-3, a.pal.pillActive, a.pal.shadowResting)
}

// arcPillTabsWithIcons is arcPillTabs with leading SVG icons.
func (a *App) arcPillTabsWithIcons(c *gui.Context, group string, selected int, items []struct {
	icon  *gui.SVG
	label string
}, onSelect func(int)) {
	track := gui.Row(c.Key("track-" + group)).
		Padding(3).
		Gap(2).
		Radius(radiusControl).
		Background(a.pal.pillTrack).
		AlignItems(gui.Center)
	var activeBtn gui.Element
	var hasActive bool
	track.Children(func() {
		for i, it := range items {
			idx := i
			item := it
			active := selected == idx
			btn := gui.ButtonBase(c.Key(fmt.Sprintf("%s-%d", group, idx))).
				Padding(5, 12).
				Gap(6).
				Radius(radiusControl - 3).
				AlignItems(gui.Center).
				OnClick(func() { onSelect(idx) })
			targetFg := a.pal.inkSecondary
			if active || btn.Hovered() {
				targetFg = a.pal.ink
			}
			fg := btn.AnimateColor("fg", targetFg, animFast)
			btn.Children(func() {
				if item.icon != nil {
					gui.Icon(c, item.icon).Size(14, 14).TextColor(fg)
				}
				gui.Text(c, item.label).FontSize(12).Bold().TextColor(fg).SingleLine()
			})
			if active {
				activeBtn = btn
				hasActive = true
			}
		}
	})
	attachGlidingPill(track, activeBtn, hasActive, radiusControl-3, a.pal.pillActive, a.pal.shadowResting)
}

// arcSwitch renders an Arc UI monochrome toggle switch following foundation.css:
// in dark mode a near-white ON track carries a dark thumb so ON and OFF never
// blend together; thumb glides with springSnappy (260ms, zero bounce).
func (a *App) arcSwitch(c *gui.Context, key string, val *bool, onChange func()) gui.Element {
	on := *val
	btn := gui.ButtonBase(c.Key(key)).
		Size(40, 22).
		Padding(3).
		Radius(radiusPill).
		Justify(gui.Start).
		AlignItems(gui.Center).
		OnClick(func() {
			*val = !*val
			if onChange != nil {
				onChange()
			}
		})
	trackTarget := a.pal.surfaceActive
	thumbTarget := a.pal.inkSecondary
	thumbX := float32(0)
	if !a.dark {
		trackTarget = a.pal.surfaceActive
		thumbTarget = gui.Hex("#FFFFFF")
	}
	if on {
		trackTarget = a.pal.controlOn
		thumbTarget = a.pal.controlGlyph
		thumbX = 18
	}
	trackCol := btn.AnimateColor("sw-track", trackTarget, animFast)
	thumbCol := btn.AnimateColor("sw-thumb", thumbTarget, animFast)
	tx := btn.AnimateTo("sw-x", thumbX, springSnappy)
	btn.Background(trackCol).Children(func() {
		gui.Box(c).
			Size(16, 16).
			Radius(radiusPill).
			Background(thumbCol).
			Shadow(0, 1, 2, 0, a.pal.shadowResting).
			Translate(tx, 0)
	})
	return btn
}

// tagChip renders a pure monochrome Arc pill chip.
func (a *App) tagChip(c *gui.Context, key, label string, active bool) gui.Element {
	btn := gui.ButtonBase(c.Key(key)).Padding(5, 11).Gap(5).Radius(radiusPill).AlignItems(gui.Center)
	targetBg := a.pal.surfaceMuted
	targetFg := a.pal.inkSecondary
	if active {
		targetBg = a.pal.controlOn
		targetFg = a.pal.controlGlyph
	}
	scale := float32(1)
	if btn.Pressed() {
		scale = 0.96
	} else if btn.Hovered() && !active {
		targetFg = a.pal.ink
		targetBg = a.pal.surfaceActive
	}
	bg := btn.AnimateColor("bg", targetBg, animFast)
	fg := btn.AnimateColor("fg", targetFg, animFast)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, springResponsive).Children(func() {
		if active {
			gui.Icon(c, iconCheck).Size(11, 11)
		}
		gui.Text(c, label).FontSize(12).SingleLine()
	})
	return btn
}

// pillBadge renders a quiet frameless monochrome capsule.
func (a *App) pillBadge(c *gui.Context, text string, fg, bg gui.Color) {
	gui.Box(c).Padding(3, 8).Radius(radiusPill).Background(bg).Children(func() {
		gui.Text(c, text).FontSize(10).Bold().TextColor(fg).SingleLine()
	})
}

func (a *App) sectionLabel(c *gui.Context, text string) {
	gui.Text(c, text).FontSize(11).Bold().TextColor(a.pal.inkMuted).LetterSpacing(0.4)
}

func (a *App) metaRow(c *gui.Context, label, value string) {
	gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
		gui.Text(c, label).Width(72).FontSize(12).TextColor(a.pal.inkMuted).SingleLine()
		gui.Text(c, value).Grow(1).FontSize(12).TextColor(a.pal.ink).SingleLine()
	})
}

func formatBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024))
	}
}

func aspectRatioLabel(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	a, b := w, h
	for b != 0 {
		a, b = b, a%b
	}
	rw, rh := w/a, h/a
	if rw > 32 || rh > 32 {
		return fmt.Sprintf("%.2f:1", float64(w)/float64(h))
	}
	return fmt.Sprintf("%d:%d", rw, rh)
}
