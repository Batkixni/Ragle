package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"
)

// palette holds extra semantic colors beyond gui.Theme so cards, sidebars,
// hairlines, and chips share a cohesive Refto × Eagle visual hierarchy.
type palette struct {
	canvas    gui.Color // main gallery background
	sidebar   gui.Color // left sidebar & right inspector background
	card      gui.Color // elevated card & modal surface
	cardHover gui.Color // hovered card surface
	hairline  gui.Color // subtle 1px divider
	subtle    gui.Color // muted pill fill
	inkMuted  gui.Color // secondary caption text
	inkFaint  gui.Color // tertiary hint text
	gold      gui.Color // star rating badge
	shadow    gui.Color // soft card drop shadow
	ring      gui.Color // active selection ring
	paper     gui.Color // moodboard canvas paper
	paperInk  gui.Color // moodboard default text ink
	paperGrid gui.Color // moodboard subtle dot grid
}

func makeTheme(dark bool) (*gui.Theme, palette) {
	if dark {
		t := &gui.Theme{
			Dark:           true,
			Background:     gui.Hex("#111113"),
			Surface:        gui.Hex("#1C1C20"),
			SurfaceHover:   gui.Hex("#26262C"),
			SurfacePressed: gui.Hex("#303038"),
			Border:         gui.Hex("#2C2C33"),
			Text:           gui.Hex("#F4F4F6"),
			TextMuted:      gui.Hex("#8E8E98"),
			Accent:         gui.Hex("#3B82F6"),
			AccentHover:    gui.Hex("#60A5FA"),
			AccentPressed:  gui.Hex("#2563EB"),
			AccentText:     gui.Hex("#FFFFFF"),
			Danger:         gui.Hex("#F87171"),
			Warning:        gui.Hex("#FBBF24"),
			Success:        gui.Hex("#34D399"),
			Selection:      gui.RGBA(59, 130, 246, 0.35),
			Focus:          gui.RGBA(96, 165, 250, 0.65),
			Inverse:        gui.Hex("#F4F4F6"),
			InverseText:    gui.Hex("#111113"),
			Scrollbar:      gui.RGBA(255, 255, 255, 0.22),
			ScrollbarWidth: 6,
			Radius:         8,
			Spacing:        4,
			FontSize:       13,
		}
		p := palette{
			canvas:    gui.Hex("#111113"),
			sidebar:   gui.Hex("#16161A"),
			card:      gui.Hex("#1C1C20"),
			cardHover: gui.Hex("#24242A"),
			hairline:  gui.Hex("#25252B"),
			subtle:    gui.Hex("#232329"),
			inkMuted:  gui.Hex("#8E8E98"),
			inkFaint:  gui.Hex("#63636D"),
			gold:      gui.Hex("#F59E0B"),
			shadow:    gui.RGBA(0, 0, 0, 0.35),
			ring:      gui.Hex("#3B82F6"),
			paper:     gui.Hex("#161618"),
			paperInk:  gui.Hex("#F4F1EA"),
			paperGrid: gui.RGBA(255, 255, 255, 0.08),
		}
		return t, p
	}

	t := &gui.Theme{
		Dark:           false,
		Background:     gui.Hex("#FAFAFB"),
		Surface:        gui.Hex("#FFFFFF"),
		SurfaceHover:   gui.Hex("#F1F1F4"),
		SurfacePressed: gui.Hex("#E6E6EB"),
		Border:         gui.Hex("#E3E3E8"),
		Text:           gui.Hex("#141417"),
		TextMuted:      gui.Hex("#6C6C75"),
		Accent:         gui.Hex("#18181B"),
		AccentHover:    gui.Hex("#2E2E34"),
		AccentPressed:  gui.Hex("#09090B"),
		AccentText:     gui.Hex("#FFFFFF"),
		Danger:         gui.Hex("#DC2626"),
		Warning:        gui.Hex("#D97706"),
		Success:        gui.Hex("#16A34A"),
		Selection:      gui.RGBA(37, 99, 235, 0.20),
		Focus:          gui.RGBA(37, 99, 235, 0.55),
		Inverse:        gui.Hex("#18181B"),
		InverseText:    gui.Hex("#FFFFFF"),
		Scrollbar:      gui.RGBA(0, 0, 0, 0.22),
		ScrollbarWidth: 6,
		Radius:         8,
		Spacing:        4,
		FontSize:       13,
	}
	p := palette{
		canvas:    gui.Hex("#FAFAFB"),
		sidebar:   gui.Hex("#F3F3F6"),
		card:      gui.Hex("#FFFFFF"),
		cardHover: gui.Hex("#F7F7FA"),
		hairline:  gui.Hex("#E7E7EC"),
		subtle:    gui.Hex("#ECECF0"),
		inkMuted:  gui.Hex("#6C6C75"),
		inkFaint:  gui.Hex("#9A9AA3"),
		gold:      gui.Hex("#D97706"),
		shadow:    gui.RGBA(15, 23, 42, 0.07),
		ring:      gui.Hex("#2563EB"),
		paper:     gui.Hex("#F4F1EA"),
		paperInk:  gui.Hex("#1C1C1C"),
		paperGrid: gui.RGBA(0, 0, 0, 0.08),
	}
	return t, p
}

// iconButton creates a clean, flat Refto-style icon button with hover/active feedback.
func (a *App) iconButton(c *gui.Context, key string, icon *gui.SVG, tip string, active bool) gui.Element {
	t := c.Theme()
	btn := gui.ButtonBase(c.Key(key)).Size(30, 30).Center().Radius(7).Label(tip).Tooltip(tip)
	bg := gui.Transparent
	fg := t.TextMuted
	if active {
		bg = a.pal.ring.Alpha(0.14)
		fg = a.pal.ring
	}
	if btn.Pressed() {
		bg = t.SurfacePressed
		fg = t.Text
	} else if btn.Hovered() {
		if !active {
			bg = t.SurfaceHover
		}
		fg = t.Text
	}
	btn.Background(bg).TextColor(fg).Children(func() {
		gui.Icon(c, icon).Size(15, 15)
	})
	return btn
}

// toolButton creates a compact toolbar button with icon + optional label.
func (a *App) toolButton(c *gui.Context, key string, icon *gui.SVG, label string, active bool) gui.Element {
	t := c.Theme()
	btn := gui.ButtonBase(c.Key(key)).Padding(5, 10).Gap(6).Radius(7).AlignItems(gui.Center)
	bg := gui.Transparent
	fg := t.Text
	if active {
		bg = a.pal.ring
		fg = gui.Hex("#FFFFFF")
	}
	if btn.Pressed() {
		bg = t.SurfacePressed
		if active {
			bg = a.pal.ring.Alpha(0.85)
		}
	} else if btn.Hovered() && !active {
		bg = t.SurfaceHover
	}
	btn.Background(bg).TextColor(fg).Children(func() {
		if icon != nil {
			gui.Icon(c, icon).Size(14, 14)
		}
		if label != "" {
			gui.Text(c, label).FontSize(12).SingleLine()
		}
	})
	return btn
}

// tagChip renders a compact, tactile pill for a tag (removable or toggleable).
func (a *App) tagChip(c *gui.Context, key, label string, active bool) gui.Element {
	t := c.Theme()
	btn := gui.ButtonBase(c.Key(key)).Padding(4, 10).Gap(5).Radius(999).AlignItems(gui.Center)
	bg := a.pal.subtle
	fg := t.TextMuted
	border := gui.Transparent
	if active {
		bg = a.pal.ring.Alpha(0.14)
		fg = a.pal.ring
		border = a.pal.ring.Alpha(0.35)
	}
	if btn.Hovered() {
		if active {
			bg = a.pal.ring.Alpha(0.22)
		} else {
			bg = t.SurfaceHover
			fg = t.Text
		}
	}
	btn.Background(bg).TextColor(fg)
	if border.A > 0 {
		btn.Border(1, border)
	}
	btn.Children(func() {
		if active {
			gui.Icon(c, iconCheck).Size(11, 11)
		}
		gui.Text(c, label).FontSize(12).SingleLine()
	})
	return btn
}

// pillBadge renders a non-interactive small metadata pill.
func (a *App) pillBadge(c *gui.Context, text string, fg, bg gui.Color) {
	gui.Box(c).Padding(2, 7).Radius(5).Background(bg).Children(func() {
		gui.Text(c, text).FontSize(10).Bold().TextColor(fg).SingleLine()
	})
}

func (a *App) sectionLabel(c *gui.Context, text string) {
	gui.Text(c, text).FontSize(11).Bold().TextColor(a.pal.inkMuted).LetterSpacing(0.3)
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
