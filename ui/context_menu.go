package ui

import (
	gui "github.com/egoist/mygo/ui"
)

// popoverMenuOpts returns our curated Raycast/Linear-inspired menu styling:
// crisp floating panel, soft shadow, 1px hairline border, tight elegant row
// spacing, and subtle neutral hover fill instead of an overpowering solid pill.
func (a *App) popoverMenuOpts() gui.PopoverMenuOptions {
	return gui.PopoverMenuOptions{
		Panel: func(panel gui.Element) {
			panel.
				MinWidth(224).
				Padding(5).
				Radius(12).
				Background(a.popoverSurface()).
				Border(1, a.chromeHairline()).
				Shadow(0, 14, 38, 0, a.pal.shadowFloating)
		},
		Item: func(row gui.Element) {
			row.Padding(6, 10).Gap(8).Radius(7)
			if row.Highlighted() {
				hoverBg := gui.RGBA(255, 255, 255, 0.085)
				if !a.dark {
					hoverBg = gui.RGBA(0, 0, 0, 0.055)
				}
				row.Background(hoverBg).TextColor(a.pal.ink)
			}
		},
	}
}

func (a *App) popoverMenuBtnOpts() gui.PopoverMenuButtonOptions {
	opts := a.popoverMenuOpts()
	return gui.PopoverMenuButtonOptions{
		Panel: opts.Panel,
		Item:  opts.Item,
	}
}

// menuItem creates a sleek Screenshot-6 style menu row with a left outline SVG
// icon, clean label, optional right-aligned muted keyboard shortcut hint, and
// optional danger tinting for destructive actions.
func (a *App) menuItem(m *gui.Menu, icon *gui.SVG, label, shortcut string, danger bool) *gui.MenuItem {
	it := m.Item(label)
	if danger {
		it.Danger()
	}
	it.Content(func(x gui.MenuItemContent) {
		gui.Row(x.Context).FillWidth().Gap(9).AlignItems(gui.Center).Children(func() {
			if icon != nil {
				icCol := a.pal.inkSecondary
				if danger {
					icCol = a.pal.danger
				} else if x.Highlighted {
					icCol = a.pal.ink
				}
				gui.Icon(x.Context, icon).Size(14, 14).TextColor(icCol)
			}
			lblCol := a.pal.ink
			if danger {
				lblCol = a.pal.danger
			}
			if x.Disabled {
				lblCol = a.pal.inkMuted
			}
			gui.Text(x.Context, label).FontSize(12.5).TextColor(lblCol).SingleLine().Grow(1)
			if shortcut != "" {
				gui.Text(x.Context, shortcut).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
			}
		})
	})
	return it
}

// menuSub creates a sleek submenu trigger row with a left outline SVG icon and
// crisp label while retaining MyGo's native submenu hover aim-grace & chevron.
func (a *App) menuSub(m *gui.Menu, icon *gui.SVG, label string, build func(sm *gui.Menu)) *gui.MenuItem {
	it := m.Submenu(label, build)
	it.Content(func(x gui.MenuItemContent) {
		gui.Row(x.Context).FillWidth().Gap(9).AlignItems(gui.Center).Children(func() {
			if icon != nil {
				icCol := a.pal.inkSecondary
				if x.Highlighted {
					icCol = a.pal.ink
				}
				gui.Icon(x.Context, icon).Size(14, 14).TextColor(icCol)
			}
			gui.Text(x.Context, label).FontSize(12.5).TextColor(a.pal.ink).SingleLine().Grow(1)
		})
	})
	return it
}

// menuRadio creates a clean radio-selectable menu row with an active check dot
// and muted-on-hover styling that keeps text legible against our subtle highlight.
func (a *App) menuRadio(m *gui.Menu, label string, checked bool) *gui.MenuItem {
	it := m.Item(label).Radio(checked)
	it.Content(func(x gui.MenuItemContent) {
		gui.Row(x.Context).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
			fg := a.pal.ink
			if !checked && !x.Highlighted {
				fg = a.pal.inkSecondary
			}
			t := gui.Text(x.Context, label).FontSize(12.5).TextColor(fg).SingleLine().Grow(1)
			if checked {
				t.Bold()
			}
		})
	})
	return it
}
