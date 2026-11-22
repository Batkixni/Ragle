package ui

import (
	"fmt"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/model"
)

func (a *App) stackSelectedAssets(ids []string) {
	if a.lib == nil || len(ids) < 2 {
		return
	}
	coverID := ids[0]
	if cur, ok := a.selectedAsset(); ok {
		for _, id := range ids {
			if id == cur.ID {
				coverID = cur.ID
				break
			}
		}
	}
	st, err := a.lib.CreateStack("", ids, coverID)
	if err != nil {
		a.notify(err.Error())
		return
	}
	a.selectedIDs = map[string]bool{st.CoverID: true}
	a.multiSelectMode = false
	a.reload()
	a.notify(fmt.Sprintf(a.t("stack.created"), len(st.AssetIDs)))
}

func (a *App) autoStackVersions() {
	if a.lib == nil {
		return
	}
	n, err := a.lib.AutoStackByVersionName()
	if err != nil {
		a.notify(err.Error())
		return
	}
	a.reload()
	if n == 0 {
		a.notify(a.t("stack.autoNone"))
		return
	}
	a.notify(fmt.Sprintf(a.t("stack.autoGrouped"), n))
}

func (a *App) openStackView(stackID string) {
	if stackID == "" {
		return
	}
	if _, ok := a.stackByID[stackID]; !ok {
		return
	}
	a.openStackID = stackID
	a.selectedIDs = map[string]bool{}
	a.assetIndex = -1
	a.revealGen++
	a.applyFilters()
}

func (a *App) closeStackView() {
	if a.openStackID == "" {
		return
	}
	a.openStackID = ""
	a.selectedIDs = map[string]bool{}
	a.assetIndex = -1
	a.revealGen++
	a.applyFilters()
}

func (a *App) setStackCover(stackID, assetID string) {
	if a.lib == nil {
		return
	}
	if _, err := a.lib.SetStackCover(stackID, assetID); err != nil {
		a.notify(err.Error())
		return
	}
	a.reload()
	a.notify(a.t("stack.coverUpdated"))
}

func (a *App) removeFromStack(ids []string) {
	if a.lib == nil || len(ids) == 0 {
		return
	}
	if err := a.lib.RemoveFromStack(ids); err != nil {
		a.notify(err.Error())
		return
	}
	a.reload()
	if a.openStackID != "" {
		if _, ok := a.stackByID[a.openStackID]; !ok {
			a.openStackID = ""
			a.applyFilters()
		}
	}
}

func (a *App) ungroupStack(stackID string) {
	if a.lib == nil || stackID == "" {
		return
	}
	if err := a.lib.UngroupStack(stackID); err != nil {
		a.notify(err.Error())
		return
	}
	if a.openStackID == stackID {
		a.openStackID = ""
	}
	a.reload()
	a.notify(a.t("stack.ungrouped"))
}

func (a *App) stackMembers(stackID string) []model.Asset {
	st, ok := a.stackByID[stackID]
	if !ok {
		return nil
	}
	out := make([]model.Asset, 0, len(st.AssetIDs))
	for _, id := range st.AssetIDs {
		if asset, exists := a.assetByID[id]; exists {
			out = append(out, asset)
		}
	}
	return out
}

// stackBanner renders the header bar shown when inspecting the expanded versions
// inside an Asset Stack.
func (a *App) stackBanner(c *gui.Context) {
	st, ok := a.stackByID[a.openStackID]
	if !ok {
		return
	}
	gui.Row(c.Key("stack-open-banner")).
		Padding(8, 14).
		Gap(10).
		Radius(radiusPanel).
		Background(a.pal.surface).
		AlignItems(gui.Center).
		Transition(gui.ElementTransition{
			Animation: &springSmooth,
			Enter:     &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
			Exit:      &gui.Motion{Y: -6, Opacity: 0, Collapse: true},
		}).
		Children(func() {
			a.toolButton(c, "stack-back-btn", iconArrowLeft, a.t("stack.back"), false).OnClick(func() {
				a.closeStackView()
			})
			gui.Icon(c, iconLayers).Size(15, 15).TextColor(a.pal.ink)
			gui.Text(c, fmt.Sprintf(a.t("stack.banner"), st.Name, len(st.AssetIDs))).
				FontSize(13).
				Bold().
				TextColor(a.pal.ink)
			gui.Spacer(c)
			a.toolButton(c, "stack-ungroup-btn", iconUngroup, a.t("stack.ungroup"), false).OnClick(func() {
				a.ungroupStack(st.ID)
			})
		})
}

// previewStackStrip renders a sleek bottom version switcher inside the Inline
// Preview whenever the active asset belongs to a version stack.
func (a *App) previewStackStrip(c *gui.Context, active model.Asset) {
	if active.StackID == "" {
		return
	}
	members := a.stackMembers(active.StackID)
	if len(members) < 2 {
		return
	}
	st := a.stackByID[active.StackID]
	gui.Row(c.Key("preview-stack-strip")).
		Padding(8, 12).
		Gap(8).
		Radius(radiusPanel).
		Background(a.pal.surface).
		AlignItems(gui.Center).
		Children(func() {
			gui.Icon(c, iconLayers).Size(14, 14).TextColor(a.pal.inkSecondary)
			gui.Text(c, fmt.Sprintf("%s (%d)", a.t("stack.versions"), len(members))).
				FontSize(11).
				Bold().
				TextColor(a.pal.inkSecondary)
			for i, m := range members {
				item := m
				isCur := item.ID == active.ID
				lbl := fmt.Sprintf("v%d · %s", i+1, a.assetName(item))
				if len([]rune(lbl)) > 22 {
					lbl = string([]rune(lbl)[:21]) + "…"
				}
				bg := a.pal.surfaceMuted
				fg := a.pal.inkSecondary
				if isCur {
					bg = a.pal.controlOn
					fg = a.pal.controlGlyph
				}
				gui.ButtonBase(c.Key("pstack-"+item.ID)).
					Padding(5, 10).
					Radius(radiusPill).
					Background(bg).
					OnClick(func() {
						a.openPreview(item)
					}).
					Children(func() {
						gui.Text(c, lbl).FontSize(11).Bold().TextColor(fg).SingleLine()
					})
			}
			if active.ID != st.CoverID {
				gui.Spacer(c)
				a.toolButton(c, "pstack-set-cover", iconCheck, a.t("stack.setCover"), false).OnClick(func() {
					a.setStackCover(st.ID, active.ID)
				})
			}
		})
}
