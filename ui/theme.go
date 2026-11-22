package ui

import (
	"fmt"
	"math"
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
	// Shared-layout highlights & pill morphs — critically damped, zero bounce.
	springMorph = gui.Animation{
		Spring: gui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0},
	}

	animFast = gui.Animation{
		Duration: 160 * time.Millisecond,
		Ease:     easeArcStandard,
	}
	animStandard = gui.Animation{
		Duration: 240 * time.Millisecond,
		Ease:     easeArcEnter,
	}
)

const (
	arcStaggerItem = 35 * time.Millisecond
	arcBlurSoft    = float32(4)
)

// pillMatchKey identifies a shared-layout morph group (Arc UI layoutId).
type pillMatchKey struct {
	group string
}

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
	danger         gui.Color // muted rose for destructive actions / missing only
	shadowResting  gui.Color // --shadow-resting
	shadowRaised   gui.Color // --shadow-raised
	shadowFloating gui.Color // --shadow-floating
	paper          gui.Color // moodboard canvas paper
	paperInk       gui.Color // moodboard default ink
	paperGrid      gui.Color // moodboard subtle dot grid
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
			AccentText:     gui.Hex("#18181B"),
			Danger:         gui.Hex("#E07A7A"),
			Warning:        gui.Hex("#A1A1AA"),
			Success:        gui.Hex("#D4D4D8"),
			Selection:      gui.RGBA(255, 255, 255, 0.14),
			Focus:          gui.Transparent,
			Inverse:        gui.Hex("#EDEDED"),
			InverseText:    gui.Hex("#18181B"),
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
			controlGlyph:   gui.Hex("#18181B"),
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
		Selection:      gui.RGBA(24, 24, 27, 0.10),
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

// applyBlurReveal runs Arc UI's signature soft blur-in reveal for panels,
// inspector switches, modals, and headings (opacity 0->1, blur 4px->0px, y 6px->0px).
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

// applyStaggerReveal runs Arc UI's GPU-cheap transform+opacity staggered reveal
// for dense card grids (opacity 0->1, y 8px->0px, scale 0.98->1, 240ms easeArcEnter).
func applyStaggerReveal(e gui.Element, key any, delay time.Duration) {
	started := gui.Local(e, key, func() bool { return false })
	if !*started {
		*started = true
		e.Jump("arc-stagger", 0)
	}
	anim := animStandard
	anim.Delay = delay
	p := e.AnimateTo("arc-stagger", 1, anim)
	if p < 0.999 {
		e.Opacity(p)
		e.Translate(0, (1-p)*8)
		e.Scale(0.98+0.02*p, 0.98+0.02*p)
	}
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

// arcPillTabs renders an Arc UI sliding pill navigator with zero-bounce morph
// spring (springMorph, 420ms, bounce: 0) via ElementTransition.Match.
func (a *App) arcPillTabs(c *gui.Context, group string, selected int, labels []string, onSelect func(int)) {
	gui.Row(c.Key("track-" + group)).
		Padding(3).
		Gap(2).
		Radius(radiusPill).
		Background(a.pal.surface).
		AlignItems(gui.Center).
		Children(func() {
			for i, lbl := range labels {
				idx := i
				active := selected == idx
				btn := gui.ButtonBase(c.Key(fmt.Sprintf("%s-%d", group, idx))).
					Radius(radiusPill).
					AlignItems(gui.Center).
					OnClick(func() { onSelect(idx) })
				scale := float32(1)
				if btn.Pressed() {
					scale = 0.97
				}
				btn.AnimatePose(gui.Pose{Scale: scale}, springResponsive)
				targetFg := a.pal.inkSecondary
				if active || btn.Hovered() {
					targetFg = a.pal.ink
				}
				fg := btn.AnimateColor("fg", targetFg, animFast)
				btn.Children(func() {
					if active {
						gui.Row(c.Key(fmt.Sprintf("%s-pill-%d", group, idx))).
							Padding(5, 12).
							Radius(radiusPill).
							AlignItems(gui.Center).
							Background(a.pal.surfaceRaised).
							Shadow(0, 1, 3, 0, a.pal.shadowResting).
							Transition(gui.ElementTransition{
								Animation: &springMorph,
								Match:     pillMatchKey{group: group},
							}).
							Children(func() {
								gui.Text(c, lbl).FontSize(12).Bold().TextColor(fg).SingleLine()
							})
					} else {
						gui.Row(c).Padding(5, 12).Radius(radiusPill).AlignItems(gui.Center).Children(func() {
							gui.Text(c, lbl).FontSize(12).Bold().TextColor(fg).SingleLine()
						})
					}
				})
			}
		})
}

// arcPillTabsWithIcons is arcPillTabs with leading SVG icons.
func (a *App) arcPillTabsWithIcons(c *gui.Context, group string, selected int, items []struct {
	icon  *gui.SVG
	label string
}, onSelect func(int)) {
	gui.Row(c.Key("track-" + group)).
		Padding(3).
		Gap(2).
		Radius(radiusPill).
		Background(a.pal.surface).
		AlignItems(gui.Center).
		Children(func() {
			for i, it := range items {
				idx := i
				item := it
				active := selected == idx
				btn := gui.ButtonBase(c.Key(fmt.Sprintf("%s-%d", group, idx))).
					Radius(radiusPill).
					AlignItems(gui.Center).
					OnClick(func() { onSelect(idx) })
				scale := float32(1)
				if btn.Pressed() {
					scale = 0.97
				}
				btn.AnimatePose(gui.Pose{Scale: scale}, springResponsive)
				targetFg := a.pal.inkSecondary
				if active || btn.Hovered() {
					targetFg = a.pal.ink
				}
				fg := btn.AnimateColor("fg", targetFg, animFast)
				btn.Children(func() {
					content := func() {
						if item.icon != nil {
							gui.Icon(c, item.icon).Size(14, 14).TextColor(fg)
						}
						gui.Text(c, item.label).FontSize(12).Bold().TextColor(fg).SingleLine()
					}
					if active {
						gui.Row(c.Key(fmt.Sprintf("%s-pill-%d", group, idx))).
							Padding(5, 12).
							Gap(6).
							Radius(radiusPill).
							AlignItems(gui.Center).
							Background(a.pal.surfaceRaised).
							Shadow(0, 1, 3, 0, a.pal.shadowResting).
							Transition(gui.ElementTransition{
								Animation: &springMorph,
								Match:     pillMatchKey{group: group},
							}).
							Children(content)
					} else {
						gui.Row(c).Padding(5, 12).Gap(6).Radius(radiusPill).AlignItems(gui.Center).Children(content)
					}
				})
			}
		})
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
