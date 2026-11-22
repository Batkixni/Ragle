package ui

import (
	"fmt"
	"time"

	gui "github.com/egoist/mygo/ui"
)

// Arc UI 4-step border radius scale (from design.md §3).
const (
	radiusControl float32 = 12
	radiusPanel   float32 = 16
	radiusSurface float32 = 20
	radiusPill    float32 = 9999
)

// Arc UI calm motion presets (critically damped springs, 150ms–260ms).
var (
	arcSpring = gui.Animation{
		Spring: gui.SpringOptions{Duration: 220 * time.Millisecond, Bounce: 0.06},
	}
	arcSpringSnappy = gui.Animation{
		Spring: gui.SpringOptions{Duration: 190 * time.Millisecond, Bounce: 0.10},
	}
	arcEnterAnim = gui.Animation{
		Spring: gui.SpringOptions{Duration: 240 * time.Millisecond, Bounce: 0.04},
	}
	arcColorAnim = gui.Animation{
		Duration: 160 * time.Millisecond,
		Ease:     gui.EaseOut,
	}
)

// pillMatchKey identifies a sliding pill indicator group for matched-geometry
// spring transitions (Arc UI layoutId equivalent).
type pillMatchKey struct {
	group string
}

// palette holds Arc UI frameless surface fill levels (zero hard 1px borders).
type palette struct {
	canvas        gui.Color // --background: pure white (light) / deep charcoal (dark)
	surface       gui.Color // --surface: frameless cards, sidebar, inspector, pill tracks
	surfaceRaised gui.Color // --surface-raised: floating navbar, modals, active sliding pill
	surfaceMuted  gui.Color // --surface-muted: hover fills, inputs, subtle badges
	inkSecondary  gui.Color // secondary readable text
	inkMuted      gui.Color // muted caption / timestamp text
	inkFaint      gui.Color // subtle hint / placeholder
	gold          gui.Color // star rating
	shadowSm      gui.Color // subtle grounded shadow for cards & pills
	shadowMd      gui.Color // floating navbar & card hover elevation
	shadowLg      gui.Color // modal & lightbox depth shadow
	rimLight      gui.Color // subtle specular top rim for floating surfaces
	ring          gui.Color // calm active focus/selection accent
	paper         gui.Color // moodboard canvas paper
	paperInk      gui.Color // moodboard text ink
	paperGrid     gui.Color // moodboard subtle dot grid
}

func makeTheme(dark bool) (*gui.Theme, palette) {
	if dark {
		t := &gui.Theme{
			Dark:           true,
			Background:     gui.Hex("#111111"),
			Surface:        gui.Hex("#18181B"),
			SurfaceHover:   gui.Hex("#27272A"),
			SurfacePressed: gui.Hex("#323238"),
			Border:         gui.Transparent, // Frameless & stroke-free
			Text:           gui.Hex("#FAFAFA"),
			TextMuted:      gui.Hex("#A1A1AA"),
			Accent:         gui.Hex("#FAFAFA"),
			AccentHover:    gui.Hex("#E4E4E7"),
			AccentPressed:  gui.Hex("#D4D4D8"),
			AccentText:     gui.Hex("#111111"),
			Danger:         gui.Hex("#F87171"),
			Warning:        gui.Hex("#FBBF24"),
			Success:        gui.Hex("#34D399"),
			Selection:      gui.RGBA(59, 130, 246, 0.35),
			Focus:          gui.RGBA(96, 165, 250, 0.55),
			Inverse:        gui.Hex("#FAFAFA"),
			InverseText:    gui.Hex("#111111"),
			Scrollbar:      gui.RGBA(255, 255, 255, 0.20),
			ScrollbarWidth: 6,
			Radius:         radiusControl,
			Spacing:        4,
			FontSize:       13,
		}
		p := palette{
			canvas:        gui.Hex("#111111"),
			surface:       gui.Hex("#18181B"),
			surfaceRaised: gui.Hex("#1F1F23"),
			surfaceMuted:  gui.Hex("#27272A"),
			inkSecondary:  gui.Hex("#A1A1AA"),
			inkMuted:      gui.Hex("#71717A"),
			inkFaint:      gui.Hex("#52525B"),
			gold:          gui.Hex("#F59E0B"),
			shadowSm:      gui.RGBA(0, 0, 0, 0.28),
			shadowMd:      gui.RGBA(0, 0, 0, 0.42),
			shadowLg:      gui.RGBA(0, 0, 0, 0.58),
			rimLight:      gui.RGBA(255, 255, 255, 0.08),
			ring:          gui.Hex("#3B82F6"),
			paper:         gui.Hex("#141416"),
			paperInk:      gui.Hex("#F4F1EA"),
			paperGrid:     gui.RGBA(255, 255, 255, 0.07),
		}
		return t, p
	}

	t := &gui.Theme{
		Dark:           false,
		Background:     gui.Hex("#FFFFFF"),
		Surface:        gui.Hex("#F4F4F5"),
		SurfaceHover:   gui.Hex("#E4E4E7"),
		SurfacePressed: gui.Hex("#D4D4D8"),
		Border:         gui.Transparent, // Frameless & stroke-free
		Text:           gui.Hex("#18181B"),
		TextMuted:      gui.Hex("#71717A"),
		Accent:         gui.Hex("#18181B"),
		AccentHover:    gui.Hex("#27272A"),
		AccentPressed:  gui.Hex("#09090B"),
		AccentText:     gui.Hex("#FFFFFF"),
		Danger:         gui.Hex("#DC2626"),
		Warning:        gui.Hex("#D97706"),
		Success:        gui.Hex("#16A34A"),
		Selection:      gui.RGBA(37, 99, 235, 0.20),
		Focus:          gui.RGBA(37, 99, 235, 0.50),
		Inverse:        gui.Hex("#18181B"),
		InverseText:    gui.Hex("#FFFFFF"),
		Scrollbar:      gui.RGBA(0, 0, 0, 0.18),
		ScrollbarWidth: 6,
		Radius:         radiusControl,
		Spacing:        4,
		FontSize:       13,
	}
	p := palette{
		canvas:        gui.Hex("#FFFFFF"),
		surface:       gui.Hex("#F4F4F5"),
		surfaceRaised: gui.Hex("#FFFFFF"),
		surfaceMuted:  gui.Hex("#E4E4E7"),
		inkSecondary:  gui.Hex("#71717A"),
		inkMuted:      gui.Hex("#A1A1AA"),
		inkFaint:      gui.Hex("#B4B4BC"),
		gold:          gui.Hex("#D97706"),
		shadowSm:      gui.RGBA(15, 23, 42, 0.06),
		shadowMd:      gui.RGBA(15, 23, 42, 0.10),
		shadowLg:      gui.RGBA(15, 23, 42, 0.18),
		rimLight:      gui.RGBA(255, 255, 255, 0.80),
		ring:          gui.Hex("#2563EB"),
		paper:         gui.Hex("#F4F1EA"),
		paperInk:      gui.Hex("#18181B"),
		paperGrid:     gui.RGBA(0, 0, 0, 0.07),
	}
	return t, p
}

// iconButton creates an Arc-style frameless circular icon button with smooth
// color transition and tactile spring press scale.
func (a *App) iconButton(c *gui.Context, key string, icon *gui.SVG, tip string, active bool) gui.Element {
	t := c.Theme()
	btn := gui.ButtonBase(c.Key(key)).Size(32, 32).Center().Radius(radiusPill).Label(tip).Tooltip(tip)
	targetBg := gui.Transparent
	targetFg := a.pal.inkSecondary
	if active {
		targetBg = a.pal.ring.Alpha(0.14)
		targetFg = a.pal.ring
	}
	scale := float32(1)
	if btn.Pressed() {
		targetBg = a.pal.surfaceMuted
		targetFg = t.Text
		scale = 0.93
	} else if btn.Hovered() {
		if !active {
			targetBg = a.pal.surfaceMuted
		}
		targetFg = t.Text
	}
	bg := btn.AnimateColor("bg", targetBg, arcColorAnim)
	fg := btn.AnimateColor("fg", targetFg, arcColorAnim)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, arcSpringSnappy).Children(func() {
		gui.Icon(c, icon).Size(15, 15)
	})
	return btn
}

// toolButton creates an Arc-style frameless tactile button with smooth color &
// spring press feedback.
func (a *App) toolButton(c *gui.Context, key string, icon *gui.SVG, label string, active bool) gui.Element {
	t := c.Theme()
	btn := gui.ButtonBase(c.Key(key)).Padding(6, 12).Gap(6).Radius(radiusControl).AlignItems(gui.Center)
	targetBg := gui.Transparent
	targetFg := t.Text
	if active {
		targetBg = t.Accent
		targetFg = t.AccentText
	}
	scale := float32(1)
	if btn.Pressed() {
		scale = 0.96
		if !active {
			targetBg = a.pal.surfaceMuted
		}
	} else if btn.Hovered() && !active {
		targetBg = a.pal.surfaceMuted
	}
	bg := btn.AnimateColor("bg", targetBg, arcColorAnim)
	fg := btn.AnimateColor("fg", targetFg, arcColorAnim)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, arcSpringSnappy).Children(func() {
		if icon != nil {
			gui.Icon(c, icon).Size(14, 14)
		}
		if label != "" {
			gui.Text(c, label).FontSize(12).Bold().SingleLine()
		}
	})
	return btn
}

// arcPillTabs renders an Arc UI sliding pill navigator: a frameless muted track
// with a raised indicator pill that glides smoothly between options using
// matched-geometry spring physics (ElementTransition.Match).
func (a *App) arcPillTabs(c *gui.Context, group string, selected int, labels []string, onSelect func(int)) {
	t := c.Theme()
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
					scale = 0.96
				}
				btn.AnimatePose(gui.Pose{Scale: scale}, arcSpringSnappy)
				targetFg := a.pal.inkSecondary
				if active || btn.Hovered() {
					targetFg = t.Text
				}
				fg := btn.AnimateColor("fg", targetFg, arcColorAnim)
				btn.Children(func() {
					if active {
						gui.Row(c.Key(fmt.Sprintf("%s-pill-%d", group, idx))).
							Padding(5, 12).
							Radius(radiusPill).
							AlignItems(gui.Center).
							Background(a.pal.surfaceRaised).
							Shadow(0, 2, 8, 0, a.pal.shadowSm).
							InsetShadow(0, 1, 1, 0, a.pal.rimLight).
							Transition(gui.ElementTransition{
								Animation: &arcSpringSnappy,
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

// arcPillTabsWithIcons is arcPillTabs with leading SVG icons (e.g. Board tool switcher).
func (a *App) arcPillTabsWithIcons(c *gui.Context, group string, selected int, items []struct {
	icon  *gui.SVG
	label string
}, onSelect func(int)) {
	t := c.Theme()
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
					scale = 0.96
				}
				btn.AnimatePose(gui.Pose{Scale: scale}, arcSpringSnappy)
				targetFg := a.pal.inkSecondary
				if active || btn.Hovered() {
					targetFg = t.Text
				}
				fg := btn.AnimateColor("fg", targetFg, arcColorAnim)
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
							Shadow(0, 2, 8, 0, a.pal.shadowSm).
							InsetShadow(0, 1, 1, 0, a.pal.rimLight).
							Transition(gui.ElementTransition{
								Animation: &arcSpringSnappy,
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

// tagChip renders a frameless Arc UI pill chip with smooth color & spring press.
func (a *App) tagChip(c *gui.Context, key, label string, active bool) gui.Element {
	t := c.Theme()
	btn := gui.ButtonBase(c.Key(key)).Padding(5, 11).Gap(5).Radius(radiusPill).AlignItems(gui.Center)
	targetBg := a.pal.surfaceMuted
	targetFg := a.pal.inkSecondary
	if active {
		targetBg = t.Accent
		targetFg = t.AccentText
	}
	scale := float32(1)
	if btn.Pressed() {
		scale = 0.95
	} else if btn.Hovered() && !active {
		targetFg = t.Text
		targetBg = t.SurfacePressed
	}
	bg := btn.AnimateColor("bg", targetBg, arcColorAnim)
	fg := btn.AnimateColor("fg", targetFg, arcColorAnim)
	btn.Background(bg).TextColor(fg).AnimatePose(gui.Pose{Scale: scale}, arcSpringSnappy).Children(func() {
		if active {
			gui.Icon(c, iconCheck).Size(11, 11)
		}
		gui.Text(c, label).FontSize(12).SingleLine()
	})
	return btn
}

// pillBadge renders a frameless small metadata capsule.
func (a *App) pillBadge(c *gui.Context, text string, fg, bg gui.Color) {
	gui.Box(c).Padding(3, 8).Radius(radiusPill).Background(bg).Children(func() {
		gui.Text(c, text).FontSize(10).Bold().TextColor(fg).SingleLine()
	})
}

func (a *App) sectionLabel(c *gui.Context, text string) {
	gui.Text(c, text).FontSize(11).Bold().TextColor(a.pal.inkMuted).LetterSpacing(0.4)
}

func (a *App) metaRow(c *gui.Context, label, value string) {
	t := c.Theme()
	gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
		gui.Text(c, label).Width(74).FontSize(12).TextColor(a.pal.inkMuted).SingleLine()
		gui.Text(c, value).Grow(1).FontSize(12).TextColor(t.Text).SingleLine()
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
