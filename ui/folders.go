package ui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/library"
	"raglex/internal/model"
)

// folderReorderDrag is the value dragged when reordering folders in the sidebar.
type folderReorderDrag struct {
	ID string
}

// effectiveBrowseMode returns the active sidebar classification mode:
// "folders" or "tags". When not explicitly chosen by the user yet, libraries
// that already have Folders default to "folders" while tag-only libraries stay
// on "tags" for seamless backward compatibility.
func (a *App) effectiveBrowseMode() string {
	if a.sidebarBrowseMode == "folders" || a.sidebarBrowseMode == "tags" {
		return a.sidebarBrowseMode
	}
	if len(a.folders) > 0 {
		return "folders"
	}
	return "tags"
}

func (a *App) setSidebarBrowseMode(mode string) {
	if mode != "folders" && mode != "tags" {
		return
	}
	a.sidebarBrowseMode = mode
	a.syncConfigFromState()
}

func (a *App) rebuildFolderTree() {
	a.folderByID = make(map[string]model.Folder, len(a.folders))
	a.folderChildren = make(map[string][]model.Folder)
	for _, f := range a.folders {
		a.folderByID[f.ID] = f
	}
	for _, f := range a.folders {
		pid := f.ParentID
		if pid != "" {
			if _, ok := a.folderByID[pid]; !ok {
				pid = ""
			}
		}
		a.folderChildren[pid] = append(a.folderChildren[pid], f)
	}
	for pid := range a.folderChildren {
		list := a.folderChildren[pid]
		sort.Slice(list, func(i, j int) bool {
			return library.FolderLess(list[i], list[j])
		})
		a.folderChildren[pid] = list
	}
}

func (a *App) ensureFolderTree() {
	if len(a.folders) > 0 && len(a.folderByID) != len(a.folders) {
		a.rebuildFolderTree()
	}
}

// folderDescendantSet returns folderID plus all of its nested subfolder IDs.
func (a *App) folderDescendantSet(folderID string) map[string]bool {
	out := map[string]bool{folderID: true}
	var walk func(id string)
	walk = func(id string) {
		for _, ch := range a.folderChildren[id] {
			if !out[ch.ID] {
				out[ch.ID] = true
				walk(ch.ID)
			}
		}
	}
	walk(folderID)
	return out
}

func (a *App) hasFolder(assetID, folderID string) bool {
	for _, f := range a.assetFolders[assetID] {
		if f.ID == folderID {
			return true
		}
	}
	return false
}

func (a *App) hasAnyFolder(assetID string, set map[string]bool) bool {
	for _, f := range a.assetFolders[assetID] {
		if set[f.ID] {
			return true
		}
	}
	return false
}

func (a *App) folderDisplayCount(folderID string) int {
	if c, ok := a.folderRecursiveCounts[folderID]; ok {
		return c
	}
	return a.folderCounts[folderID]
}

// effectiveFolderColor returns a folder's display color (inheriting its nearest
// colored ancestor so nested subfolders stay visually cohesive).
func (a *App) effectiveFolderColor(folderID string) (gui.Color, bool) {
	if !a.showTagColors || folderID == "" {
		return gui.Color{}, false
	}
	walk := folderID
	for depth := 0; walk != "" && depth < 16; depth++ {
		f, ok := a.folderByID[walk]
		if !ok {
			break
		}
		if f.Color != "" {
			if col, ok := parseHexColor(f.Color); ok {
				return col, true
			}
		}
		walk = f.ParentID
	}
	return gui.Color{}, false
}

func (a *App) folderBranchMatches(folderID, q string) bool {
	f, ok := a.folderByID[folderID]
	if ok && strings.Contains(strings.ToLower(f.Name), q) {
		return true
	}
	for _, ch := range a.folderChildren[folderID] {
		if a.folderBranchMatches(ch.ID, q) {
			return true
		}
	}
	return false
}

func (a *App) folderFullPath(folderID string) string {
	var parts []string
	walk := folderID
	for depth := 0; walk != "" && depth < 16; depth++ {
		f, ok := a.folderByID[walk]
		if !ok {
			break
		}
		parts = append(parts, f.Name)
		walk = f.ParentID
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, " / ")
}

func (a *App) toggleFolderExpand(id string) {
	if a.expandedFolders == nil {
		a.expandedFolders = map[string]bool{}
	}
	a.expandedFolders[id] = !a.expandedFolders[id]
}

func (a *App) expandAllFolders(expand bool) {
	if !expand {
		a.expandedFolders = map[string]bool{}
		return
	}
	if a.expandedFolders == nil {
		a.expandedFolders = map[string]bool{}
	}
	for _, f := range a.folders {
		if len(a.folderChildren[f.ID]) > 0 {
			a.expandedFolders[f.ID] = true
		}
	}
}

func (a *App) createFolder() {
	if a.lib == nil {
		return
	}
	name := trim(a.newFolderName)
	if name == "" {
		a.status = a.t("folder.needName")
		return
	}
	f, err := a.lib.CreateFolderWithParent(name, a.newFolderParentID)
	if err != nil {
		if errors.Is(err, library.ErrDuplicateName) {
			a.status = a.t("folder.exists")
			return
		}
		a.status = err.Error()
		return
	}
	if a.newFolderParentID != "" {
		if a.expandedFolders == nil {
			a.expandedFolders = map[string]bool{}
		}
		a.expandedFolders[a.newFolderParentID] = true
	}
	a.newFolderName = ""
	a.newFolderParentID = ""
	a.newFolderOpen = false
	a.folderID = f.ID
	a.tagID = ""
	a.navFilter = ""
	a.sidebarBrowseMode = "folders"
	a.reload()
}

func (a *App) openEditFolderModal(f model.Folder) {
	a.editFolderID = f.ID
	a.editFolderName = f.Name
	a.editFolderParentID = f.ParentID
	a.editFolderColor = f.Color
	a.editFolderOpen = true
}

func (a *App) saveEditFolder() {
	if a.lib == nil || a.editFolderID == "" {
		return
	}
	name := trim(a.editFolderName)
	if name == "" {
		a.status = a.t("folder.needName")
		return
	}
	if _, err := a.lib.UpdateFolder(a.editFolderID, name, a.editFolderParentID); err != nil {
		switch {
		case errors.Is(err, library.ErrDuplicateName):
			a.status = a.t("folder.exists")
		case errors.Is(err, library.ErrTagCycle):
			a.status = a.t("folder.cycleErr")
		default:
			a.status = err.Error()
		}
		return
	}
	_ = a.lib.SetFolderColor(a.editFolderID, a.editFolderColor)
	if a.editFolderParentID != "" {
		if a.expandedFolders == nil {
			a.expandedFolders = map[string]bool{}
		}
		a.expandedFolders[a.editFolderParentID] = true
	}
	a.editFolderOpen = false
	a.reload()
}

func (a *App) setFolderColor(id, hex string) {
	if a.lib == nil || id == "" {
		return
	}
	if err := a.lib.SetFolderColor(id, hex); err != nil {
		a.status = err.Error()
		return
	}
	a.reload()
}

func (a *App) moveFolderParent(id, parentID string) {
	if a.lib == nil || id == "" {
		return
	}
	f, ok := a.folderByID[id]
	if !ok {
		return
	}
	if _, err := a.lib.UpdateFolder(id, f.Name, parentID); err != nil {
		if errors.Is(err, library.ErrTagCycle) {
			a.status = a.t("folder.cycleErr")
		} else {
			a.status = err.Error()
		}
		return
	}
	if parentID != "" {
		if a.expandedFolders == nil {
			a.expandedFolders = map[string]bool{}
		}
		a.expandedFolders[parentID] = true
	}
	a.reload()
}

func (a *App) deleteFolderPrompt(id string) {
	a.ensureFolderTree()
	if _, ok := a.folderByID[id]; !ok {
		return
	}
	if len(a.folderChildren[id]) > 0 {
		a.delFolderID = id
		a.delFolderIncludeSubs = true
		a.delFolderOpen = true
		return
	}
	a.ask(a.t("folder.delete"), a.t("folder.deleteNote"), a.t("common.delete"), func() {
		if err := a.lib.DeleteFolder(id); err != nil {
			a.notify(err.Error())
			return
		}
		if a.folderID == id {
			a.folderID = ""
			a.selectedNav = "tag-all"
			a.lastNav = "tag-all"
		}
		a.reload()
	})
}

func (a *App) executeDeleteSingleFolder() {
	id := a.delFolderID
	includeSubs := a.delFolderIncludeSubs
	a.delFolderOpen = false
	a.delFolderID = ""
	if a.lib == nil || id == "" {
		return
	}
	a.ensureFolderTree()
	desc := a.folderDescendantSet(id)
	if includeSubs {
		n, err := a.lib.DeleteFoldersBatch([]string{id}, true)
		if err != nil {
			a.notify(err.Error())
			return
		}
		if desc[a.folderID] {
			a.folderID = ""
			a.selectedNav = "tag-all"
			a.lastNav = "tag-all"
		}
		a.reload()
		a.notify(fmt.Sprintf(a.t("folder.batchDeleted"), n))
		return
	}
	if err := a.lib.DeleteFolder(id); err != nil {
		a.notify(err.Error())
		return
	}
	if a.folderID == id {
		a.folderID = ""
		a.selectedNav = "tag-all"
		a.lastNav = "tag-all"
	}
	a.reload()
}

func (a *App) deleteFolderModal(c *gui.Context) {
	if !a.delFolderOpen || a.delFolderID == "" {
		return
	}
	a.ensureFolderTree()
	f, ok := a.folderByID[a.delFolderID]
	if !ok {
		a.delFolderOpen = false
		return
	}
	subCount := len(a.folderDescendantSet(a.delFolderID)) - 1
	a.styledDialog(c, "del-folder-modal", &a.delFolderOpen, 430, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("folder.delete")+" · "+f.Name).FontSize(15).Bold().Grow(1).SingleLine()
				a.iconButton(c, "df-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.delFolderOpen = false
				})
			})
			note := a.t("folder.deleteNote")
			if a.delFolderIncludeSubs && subCount > 0 {
				note = fmt.Sprintf(a.t("folder.deleteWithSubsNote"), f.Name, subCount)
			}
			gui.Text(c, note).FontSize(12.5).TextColor(a.pal.inkSecondary).LineHeight(1.5)
			if subCount > 0 {
				gui.Checkbox(c.Key("df-inc-subs"), &a.delFolderIncludeSubs, a.t("folder.batchIncludeSubs"))
			}
			gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
			gui.Row(c).FillWidth().Gap(8).Justify(gui.End).Children(func() {
				a.arcButton(c, "df-cancel", a.t("common.cancel"), false).OnClick(func() {
					a.delFolderOpen = false
				})
				a.arcButton(c, "df-submit", a.t("common.delete"), true).OnClick(func() {
					a.executeDeleteSingleFolder()
				})
			})
		})
	})
}

func (a *App) openBatchDeleteFoldersModal() {
	a.ensureFolderTree()
	a.batchDelFolderIDs = map[string]bool{}
	a.batchDelIncludeSubs = true
	a.batchDelFoldersOpen = true
}

func (a *App) executeBatchDeleteFolders() {
	if a.lib == nil || len(a.batchDelFolderIDs) == 0 {
		return
	}
	ids := make([]string, 0, len(a.batchDelFolderIDs))
	for id, on := range a.batchDelFolderIDs {
		if on {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	n, err := a.lib.DeleteFoldersBatch(ids, a.batchDelIncludeSubs)
	if err != nil {
		a.notify(err.Error())
		return
	}
	a.batchDelFoldersOpen = false
	a.batchDelFolderIDs = map[string]bool{}
	a.reload()
	if a.folderID != "" {
		if _, ok := a.folderByID[a.folderID]; !ok {
			a.folderID = ""
			a.selectedNav = "tag-all"
			a.lastNav = "tag-all"
			a.applyFilters()
		}
	}
	a.notify(fmt.Sprintf(a.t("folder.batchDeleted"), n))
}

func (a *App) reorderFolderLive(dragID, targetID string) {
	if dragID == "" || targetID == "" || dragID == targetID {
		return
	}
	pair := "f:" + dragID + ">" + targetID
	if a.lastReorderPair == pair && time.Since(a.lastReorderAt) < 180*time.Millisecond {
		return
	}
	a.reorderFolderDrop(dragID, targetID)
	a.lastReorderPair = pair
	a.lastReorderAt = time.Now()
}

func (a *App) reorderFolderDrop(dragID, targetID string) {
	if a.lib == nil || dragID == "" || targetID == "" || dragID == targetID {
		return
	}
	a.ensureFolderTree()
	if _, ok := a.folderByID[dragID]; !ok {
		return
	}
	targetFolder, ok := a.folderByID[targetID]
	if !ok {
		return
	}
	if a.folderDescendantSet(dragID)[targetID] {
		return
	}
	parentID := targetFolder.ParentID
	siblings := a.folderChildren[parentID]
	dragIdx, targetIdx := -1, -1
	for i, s := range siblings {
		if s.ID == dragID {
			dragIdx = i
		}
		if s.ID == targetID {
			targetIdx = i
		}
	}
	if targetIdx < 0 {
		return
	}
	insertAfter := dragIdx >= 0 && dragIdx < targetIdx
	ids := make([]string, 0, len(siblings)+1)
	for _, s := range siblings {
		if s.ID == dragID {
			continue
		}
		if !insertAfter && s.ID == targetID {
			ids = append(ids, dragID)
		}
		ids = append(ids, s.ID)
		if insertAfter && s.ID == targetID {
			ids = append(ids, dragID)
		}
	}
	if err := a.lib.ReorderFolders(parentID, ids); err != nil {
		a.notify(err.Error())
		return
	}
	if folders, err := a.lib.Folders(); err == nil {
		a.folders = folders
		a.rebuildFolderTree()
	}
}

func (a *App) moveFolderOrder(folderID string, delta int) {
	if a.lib == nil || folderID == "" || delta == 0 {
		return
	}
	a.ensureFolderTree()
	f, ok := a.folderByID[folderID]
	if !ok {
		return
	}
	siblings := a.folderChildren[f.ParentID]
	idx := -1
	ids := make([]string, len(siblings))
	for i, s := range siblings {
		ids[i] = s.ID
		if s.ID == folderID {
			idx = i
		}
	}
	next := idx + delta
	if idx < 0 || next < 0 || next >= len(ids) {
		return
	}
	ids[idx], ids[next] = ids[next], ids[idx]
	if err := a.lib.ReorderFolders(f.ParentID, ids); err != nil {
		a.notify(err.Error())
		return
	}
	if folders, err := a.lib.Folders(); err == nil {
		a.folders = folders
		a.rebuildFolderTree()
	}
}

// setAssetFolderFast links or unlinks a folder on an asset without reloading the entire catalog.
func (a *App) setAssetFolderFast(assetID, folderID string, on bool) {
	if a.lib == nil || assetID == "" || folderID == "" {
		return
	}
	if err := a.lib.SetAssetFolder(assetID, folderID, on); err != nil {
		a.status = err.Error()
		return
	}
	f, ok := a.folderByID[folderID]
	if !ok {
		a.reload()
		return
	}
	cur := a.assetFolders[assetID]
	had := false
	for _, ex := range cur {
		if ex.ID == folderID {
			had = true
			break
		}
	}
	if on && !had {
		wasUncat := len(cur) == 0
		cur = append(cur, f)
		a.assetFolders[assetID] = cur
		if wasUncat && a.uncategorizedCount > 0 {
			a.uncategorizedCount--
		}
	} else if !on && had {
		next := make([]model.Folder, 0, len(cur)-1)
		for _, ex := range cur {
			if ex.ID != folderID {
				next = append(next, ex)
			}
		}
		a.assetFolders[assetID] = next
		if len(next) == 0 {
			a.uncategorizedCount++
		}
	}
	a.recomputeFolderCounts()
	if asset, ok := a.assetByID[assetID]; ok {
		a.assetSearchKey[assetID] = a.buildAssetSearchKey(asset, a.assetTags[assetID], a.assetFolders[assetID])
	}
	if a.folderID != "" || a.navFilter == "uncategorized" || a.searchQuery != "" {
		a.applyFilters()
	}
	a.scheduleAutoSync(4 * time.Second)
}

func (a *App) recomputeFolderCounts() {
	a.folderCounts = make(map[string]int, len(a.folders))
	a.folderRecursiveCounts = make(map[string]int, len(a.folders))
	for _, afolders := range a.assetFolders {
		if len(afolders) == 0 {
			continue
		}
		countedAncestors := map[string]bool{}
		for _, f := range afolders {
			a.folderCounts[f.ID]++
			walk := f.ID
			for walk != "" && !countedAncestors[walk] {
				countedAncestors[walk] = true
				a.folderRecursiveCounts[walk]++
				walk = a.folderByID[walk].ParentID
			}
		}
	}
}

func (a *App) batchApplyFolder(ids []string, folderID string) {
	if len(ids) == 0 || folderID == "" || a.lib == nil {
		return
	}
	applied := 0
	for _, id := range ids {
		if err := a.lib.SetAssetFolder(id, folderID, true); err == nil {
			applied++
		}
	}
	fName := a.folderByID[folderID].Name
	a.notify(fmt.Sprintf(a.t("folder.applied"), applied, fName))
	a.reload()
}

// buildFolderContextMenu populates the right-click menu for sidebar folders.
func (a *App) buildFolderContextMenu(m *gui.Menu, f model.Folder, services gui.Services) {
	folderID := f.ID
	children := a.folderChildren[folderID]

	if a.menuItem(m, iconShare, a.t("folder.share"), "", false).Chosen() {
		a.openShareFolder(folderID)
	}
	m.Separator()

	if a.menuItem(m, iconPlus, a.t("folder.newSub"), "", false).Chosen() {
		a.newFolderParentID = folderID
		a.newFolderName = ""
		a.newFolderOpen = true
		if a.expandedFolders == nil {
			a.expandedFolders = map[string]bool{}
		}
		a.expandedFolders[folderID] = true
	}
	if a.menuItem(m, iconEdit, a.t("folder.edit"), "", false).Chosen() {
		a.openEditFolderModal(f)
	}

	siblings := a.folderChildren[f.ParentID]
	if len(siblings) > 1 {
		idx := -1
		for i, s := range siblings {
			if s.ID == folderID {
				idx = i
				break
			}
		}
		if idx > 0 {
			if a.menuItem(m, iconChevronUp, a.t("common.moveUp"), "", false).Chosen() {
				a.moveFolderOrder(folderID, -1)
			}
		}
		if idx >= 0 && idx < len(siblings)-1 {
			if a.menuItem(m, iconChevronDown, a.t("common.moveDown"), "", false).Chosen() {
				a.moveFolderOrder(folderID, 1)
			}
		}
	}

	// Move to parent submenu (cycle-safe).
	forbidden := a.folderDescendantSet(folderID)
	a.menuSub(m, iconFolder, a.t("folder.moveTo"), func(sm *gui.Menu) {
		if a.menuRadio(sm, a.t("folder.noParent"), f.ParentID == "").Chosen() {
			a.moveFolderParent(folderID, "")
		}
		sm.Separator()
		for _, cand := range a.folders {
			cnd := cand
			if forbidden[cnd.ID] {
				continue
			}
			if a.menuRadio(sm, a.folderFullPath(cnd.ID), f.ParentID == cnd.ID).Chosen() {
				a.moveFolderParent(folderID, cnd.ID)
			}
		}
	})

	// Folder color submenu.
	a.menuSub(m, iconPalette, a.t("folder.color"), func(sm *gui.Menu) {
		if a.menuRadio(sm, a.t("tags.colorDefault"), f.Color == "").Chosen() {
			a.setFolderColor(folderID, "")
		}
		sm.Separator()
		for _, opt := range tagColorPalette {
			ch := opt
			if a.menuRadio(sm, a.t(ch.key), strings.EqualFold(f.Color, ch.hex)).Chosen() {
				a.setFolderColor(folderID, ch.hex)
			}
		}
	})

	m.Separator()

	if len(children) > 0 {
		lbl := a.t("tags.expand")
		if a.expandedFolders[folderID] {
			lbl = a.t("tags.collapse")
		}
		if a.menuItem(m, iconChevronDown, lbl, "", false).Chosen() {
			a.toggleFolderExpand(folderID)
		}
	}
	if a.menuItem(m, iconChevronDown, a.t("tags.expandAll"), "", false).Chosen() {
		a.expandAllFolders(true)
	}
	if a.menuItem(m, iconChevronRgt, a.t("tags.collapseAll"), "", false).Chosen() {
		a.expandAllFolders(false)
	}

	m.Separator()

	if a.menuItem(m, iconCopy, a.t("tags.copyName"), "", false).Chosen() {
		services.WriteClipboard(f.Name)
		a.status = a.t("common.copied")
	}

	m.Separator()

	if a.menuItem(m, iconTrash, a.t("folder.delete"), "Del", true).Chosen() {
		a.deleteFolderPrompt(folderID)
	}
	if len(a.folders) > 1 {
		if a.menuItem(m, iconCheckSquare, a.t("folder.batchDelete")+"…", "", false).Chosen() {
			a.openBatchDeleteFoldersModal()
		}
	}
}

// tintedFolderChip renders a compact folder pill in the Inspector with an unlink "×" button.
func (a *App) tintedFolderChip(c *gui.Context, key string, f model.Folder, onRemove func()) {
	bg := a.pal.surfaceMuted
	fg := a.pal.ink
	border := a.chromeHairline()
	if col, ok := a.effectiveFolderColor(f.ID); ok {
		alphaBg := float32(0.17)
		if !a.dark {
			alphaBg = 0.12
		}
		bg = gui.RGBA(col.R, col.G, col.B, alphaBg)
		border = gui.RGBA(col.R, col.G, col.B, 0.45)
	}
	gui.Row(c.Key(key)).
		Padding(3, 6, 3, 8).
		Gap(5).
		Radius(radiusPill).
		Background(bg).
		Border(1, border).
		AlignItems(gui.Center).
		Children(func() {
			iconCol := fg
			if col, ok := a.effectiveFolderColor(f.ID); ok {
				iconCol = col
			}
			gui.Icon(c, iconFolder).Size(12, 12).TextColor(iconCol)
			gui.Text(c, a.folderFullPath(f.ID)).FontSize(11).Bold().TextColor(fg).SingleLine()
			if onRemove != nil {
				rm := gui.ButtonBase(c.Key(key + "-rm")).
					Size(16, 16).
					Center().
					Radius(8).
					OnClick(onRemove)
				rbg := gui.Transparent
				if rm.Hovered() {
					rbg = a.pal.surfaceActive
				}
				rm.Background(rbg).TextColor(fg).Children(func() {
					gui.Icon(c, iconX).Size(10, 10)
				})
			}
		})
}

// editFolderModal edits a folder's name, parent folder, and color.
func (a *App) editFolderModal(c *gui.Context) {
	if !a.editFolderOpen || a.editFolderID == "" {
		return
	}
	a.styledDialog(c, "edit-folder-modal", &a.editFolderOpen, 440, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).AlignItems(gui.Center).Children(func() {
				gui.Text(c, a.t("folder.edit")).FontSize(16).Bold().Grow(1)
				a.iconButton(c, "ef-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.editFolderOpen = false
				})
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("folder.new"))
				gui.TextInput(c.Key("ef-name"), &a.editFolderName).
					AutoFocus().
					FillWidth().
					OnSubmit(func() { a.saveEditFolder() })
			})

			forbidden := a.folderDescendantSet(a.editFolderID)
			parentLabel := a.t("folder.noParent")
			if a.editFolderParentID != "" {
				if _, ok := a.folderByID[a.editFolderParentID]; ok {
					parentLabel = a.folderFullPath(a.editFolderParentID)
				}
			}
			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("folder.parent"))
				gui.PopoverMenuButton(c.Key("ef-parent"), parentLabel, func(m *gui.Menu) {
					if a.menuRadio(m, a.t("folder.noParent"), a.editFolderParentID == "").Chosen() {
						a.editFolderParentID = ""
					}
					m.Separator()
					for _, f := range a.folders {
						cand := f
						if forbidden[cand.ID] {
							continue
						}
						if a.menuRadio(m, a.folderFullPath(cand.ID), a.editFolderParentID == cand.ID).Chosen() {
							a.editFolderParentID = cand.ID
						}
					}
				}, a.popoverMenuBtnOpts()).FillWidth()
			})

			gui.Column(c).Gap(8).Children(func() {
				a.sectionLabel(c, a.t("folder.color"))
				a.tagColorDots(c, "ef-col", a.editFolderColor, func(hex string) {
					a.editFolderColor = hex
				})
			})

			gui.Divider(c)
			gui.Row(c).Gap(8).Justify(gui.End).Children(func() {
				a.arcButton(c, "ef-cancel", a.t("common.cancel"), false).OnClick(func() {
					a.editFolderOpen = false
				})
				a.arcButton(c, "ef-submit", a.t("common.save"), true).OnClick(func() {
					a.saveEditFolder()
				})
			})
		})
	})
}

// batchDeleteFoldersModal lets users check multiple folders at once and delete
// them in a single action (optionally including all nested subfolders).
func (a *App) batchDeleteFoldersModal(c *gui.Context) {
	if !a.batchDelFoldersOpen {
		return
	}
	a.ensureFolderTree()
	if a.batchDelFolderIDs == nil {
		a.batchDelFolderIDs = map[string]bool{}
	}
	// Count effective selected folders (including descendants when batchDelIncludeSubs is on).
	selectedCount := 0
	effectiveSet := map[string]bool{}
	for id, on := range a.batchDelFolderIDs {
		if !on {
			continue
		}
		if a.batchDelIncludeSubs {
			for did := range a.folderDescendantSet(id) {
				effectiveSet[did] = true
			}
		} else {
			effectiveSet[id] = true
		}
	}
	selectedCount = len(effectiveSet)

	a.styledDialog(c, "batch-del-folders-modal", &a.batchDelFoldersOpen, 480, 520, func(_ gui.Element) {
		gui.Column(c).Fill().Padding(20, 22).Gap(12).Children(func() {
			gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Icon(c, iconTrash).Size(17, 17).TextColor(a.pal.danger)
				gui.Text(c, a.t("folder.batchDelete")).FontSize(15).Bold().Grow(1).SingleLine()
				a.iconButton(c, "bdf-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.batchDelFoldersOpen = false
				})
			})
			gui.Text(c, a.t("folder.batchDeleteSub")).FontSize(12).TextColor(a.pal.inkSecondary)

			gui.Row(c).FillWidth().Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Checkbox(c.Key("bdf-inc-subs"), &a.batchDelIncludeSubs, a.t("folder.batchIncludeSubs"))
				gui.Spacer(c)
			})
			gui.Row(c).FillWidth().Gap(6).AlignItems(gui.Center).Children(func() {
				a.toolButton(c, "bdf-all", nil, a.t("folder.batchSelectAll"), false).Padding(3, 8).OnClick(func() {
					for _, f := range a.folders {
						a.batchDelFolderIDs[f.ID] = true
					}
				})
				a.toolButton(c, "bdf-none", nil, a.t("folder.batchClear"), false).Padding(3, 8).OnClick(func() {
					a.batchDelFolderIDs = map[string]bool{}
				})
			})

			gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())

			gui.Scroll(c.Key("bdf-scroll")).Grow(1).FillWidth().Children(func() {
				gui.Column(c).FillWidth().Gap(2).Padding(4, 0).Children(func() {
					var renderRows func(list []model.Folder, depth int)
					renderRows = func(list []model.Folder, depth int) {
						for _, folder := range list {
							f := folder
							fid := f.ID
							checked := a.batchDelFolderIDs[fid]
							implied := !checked && effectiveSet[fid]
							indent := float32(depth * 20)
							row := gui.ButtonBase(c.Key("bdf-row-"+fid)).
								FillWidth().
								Padding(6, 10, 6, 8+indent).
								Gap(8).
								Radius(8).
								AlignItems(gui.Center).
								OnClick(func() {
									if a.batchDelFolderIDs[fid] {
										delete(a.batchDelFolderIDs, fid)
									} else {
										a.batchDelFolderIDs[fid] = true
									}
								})
							rbg := gui.Transparent
							if row.Hovered() {
								rbg = a.pal.surfaceMuted
							}
							row.Background(rbg).Children(func() {
								chkBg := a.pal.surfaceMuted
								if checked {
									chkBg = a.pal.danger
								} else if implied {
									chkBg = gui.RGBA(224, 122, 122, 0.45)
								}
								gui.Box(c).Size(16, 16).Radius(4).Background(chkBg).Center().Children(func() {
									if checked || implied {
										gui.Icon(c, iconCheck).Size(10, 10).TextColor(gui.Hex("#FFFFFF"))
									}
								})
								icCol := a.pal.inkSecondary
								if col, ok := a.effectiveFolderColor(fid); ok {
									icCol = col
								}
								gui.Icon(c, iconFolder).Size(14, 14).TextColor(icCol)
								gui.Text(c, f.Name).FontSize(13).TextColor(a.pal.ink).Grow(1).SingleLine()
								if cnt := a.folderDisplayCount(fid); cnt > 0 {
									gui.Text(c, fmt.Sprintf("%d", cnt)).FontSize(11).TextColor(a.pal.inkMuted).SingleLine()
								}
							})
							if ch := a.folderChildren[fid]; len(ch) > 0 {
								renderRows(ch, depth+1)
							}
						}
					}
					renderRows(a.folderChildren[""], 0)
				})
			})

			gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
			gui.Row(c).FillWidth().Gap(8).Justify(gui.End).Children(func() {
				a.arcButton(c, "bdf-cancel", a.t("common.cancel"), false).OnClick(func() {
					a.batchDelFoldersOpen = false
				})
				a.arcButton(c, "bdf-submit", fmt.Sprintf(a.t("folder.batchDeleteBtn"), selectedCount), true).
					Disabled(selectedCount == 0).
					OnClick(func() {
						a.executeBatchDeleteFolders()
					})
			})
		})
	})
}
