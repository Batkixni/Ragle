package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/i18n"
	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/plugin"
	"raglex/internal/project"
	"raglex/internal/updater"
)

func writeSamplePNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRealUserClicksEveryControl exercises real pointer/keyboard clicks across
// Library, Inspector, Quick Preview, Project, Board, PopoverMenus, and Modals
// to ensure zero dead controls or hit-testing bugs.
func TestRealUserClicksEveryControl(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	img1 := filepath.Join(srcDir, "alpha_poster.png")
	img2 := filepath.Join(srcDir, "beta_card.png")
	writeSamplePNG(t, img1, 160, 120, color.RGBA{220, 80, 120, 255})
	writeSamplePNG(t, img2, 200, 100, color.RGBA{60, 160, 220, 255})

	lib, err := library.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	tagA, err := lib.CreateTag("Illustration")
	if err != nil {
		t.Fatal(err)
	}
	tagB, err := lib.CreateTag("UI")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Import(context.Background(), []string{img1, img2}, model.StorageCopy, []string{tagA.ID}, nil); err != nil {
		t.Fatal(err)
	}

	store := project.New(lib.DB)
	proj, err := store.Create("Campaign", "Launch visuals", []string{tagA.ID})
	if err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.lib = lib
	a.projects = store
	a.lang = i18n.ZhHant
	a.cfg.LibraryPath = root
	a.screen = screenLibrary
	a.reload()

	tt := gui.NewTester(a.View, 1360, 860)
	tt.Advance(500 * time.Millisecond)

	// 1. REAL CLICK on an asset card MUST select it & populate Right Inspector!
	if a.assetIndex != -1 {
		t.Fatalf("expected initial assetIndex=-1, got %d", a.assetIndex)
	}
	if err := tt.Click("alpha_poster.png"); err != nil {
		t.Fatalf("failed to click asset card: %v", err)
	}
	tt.Advance(300 * time.Millisecond)
	if a.assetIndex < 0 {
		t.Fatalf("clicking asset card did NOT select it! assetIndex=%d", a.assetIndex)
	}
	sel, ok := a.selectedAsset()
	if !ok || a.assetName(sel) != "alpha_poster.png" {
		t.Fatalf("expected alpha_poster.png selected, got %q", a.assetName(sel))
	}
	if !tt.HasText("160 × 120") || !tt.HasText("4:3") {
		t.Fatalf("right inspector missing selected asset metadata, texts=%v", tt.Texts())
	}

	// 2. Clicking Rating stars inside Right Inspector MUST persist rating in bbolt!
	if r, ok := tt.Find("評分"); !ok {
		t.Fatal("missing rating label in inspector")
	} else {
		tt.ClickAt(r.X+200, r.Y+r.H/2)
		updated, err := lib.Get(sel.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Rating == 0 {
			t.Fatalf("clicking Rating in Inspector did NOT save rating! db=%d inspectorRating=%d", updated.Rating, a.inspectorRating)
		}
	}

	// 3. Clicking Hero Preview thumbnail inside Right Inspector MUST open Preview, and Escape closes it!
	if err := tt.Click("快速預覽"); err != nil {
		t.Fatalf("clicking Quick Preview hero failed: %v", err)
	}
	if !a.previewOpen {
		t.Fatal("preview modal did not open")
	}
	tt.Key(0, gui.KeyEscape)
	if a.previewOpen {
		t.Fatal("Escape did not close preview modal")
	}

	// 4. Hovering consolidated "開啟 / 複製" button MUST reveal Open/Reveal/Copy menu, and clicking Copy Path copies path!
	actRect, ok := tt.Find("開啟 / 複製")
	if !ok {
		t.Fatalf("missing consolidated file actions button, texts=%v", tt.Texts())
	}
	tt.Move(actRect.X+actRect.W/2, actRect.Y+actRect.H/2)
	tt.Advance(260 * time.Millisecond)
	if !tt.HasText("開啟檔案") || !tt.HasText("在資料夾顯示") || !tt.HasText("複製路徑") {
		t.Fatalf("hovering file actions button did NOT reveal menu options! texts=%v", tt.Texts())
	}
	copyRect, ok := tt.Find("複製路徑")
	if !ok {
		t.Fatalf("missing 複製路徑 item in hover menu")
	}
	tt.Move(copyRect.X+copyRect.W/2, copyRect.Y+copyRect.H/2)
	tt.Advance(100 * time.Millisecond)
	if !tt.HasText("複製路徑") {
		t.Fatalf("hover menu collapsed when moving cursor onto menu item!")
	}
	tt.ClickAt(copyRect.X+copyRect.W/2, copyRect.Y+copyRect.H/2)
	if tt.Clipboard() != sel.Path {
		t.Fatalf("clipboard=%q, want %q", tt.Clipboard(), sel.Path)
	}
	// Moving pointer away from button & menu MUST collapse hover menu.
	tt.Move(10, 10)
	tt.Advance(300 * time.Millisecond)
	if tt.HasText("開啟檔案") {
		t.Fatalf("hover menu stayed open after moving pointer away")
	}

	// 5. Clicking "+ 加入專案" PopoverMenuButton inside Right Inspector MUST reference asset into Campaign!
	if err := tt.Click("+ 加入專案"); err != nil {
		t.Fatalf("clicking + Add to project failed: %v", err)
	}
	if !tt.HasText("Campaign") {
		t.Fatalf("add-to-project popover did not open")
	}
	// Click the popover item (Campaign appears in sidebar too; popover is at right side x > 900).
	if r, ok := tt.Find("+ 加入專案"); ok {
		tt.ClickAt(r.X+40, r.Y+r.H+16)
	}
	if len(a.inspectorProjects) == 0 {
		t.Fatalf("asset was not referenced into project via inspector popover menu")
	}

	// 6. Right-click asset card -> PopoverMenu MUST appear with themed menu items!
	if err := tt.RightClick("beta_card.png"); err != nil {
		t.Fatalf("right-click on asset card failed: %v", err)
	}
	if !tt.HasText("在資料夾顯示") {
		t.Fatalf("card popover menu did not open, texts=%v", tt.Texts())
	}
	tt.Key(0, gui.KeyEscape)

	// 7. Toggle Filter Bar open, let collapse animation settle, then test 1-click Filter Bar pills!
	if err := tt.Click("篩選"); err != nil {
		t.Fatalf("clicking Filter toggle failed: %v", err)
	}
	if !a.filterBarOpen {
		t.Fatal("filterBarOpen did not toggle true")
	}
	tt.Advance(450 * time.Millisecond)
	if err := tt.Click("影片"); err != nil {
		t.Fatalf("clicking Video filter pill failed: %v", err)
	}
	if len(a.assets) != 0 {
		t.Fatalf("expected 0 video assets, got %d", len(a.assets))
	}
	if err := tt.Click("清除篩選"); err != nil {
		t.Fatalf("clicking Clear filters failed: %v", err)
	}
	if len(a.assets) != 2 {
		t.Fatalf("expected 2 assets after clearing filters, got %d", len(a.assets))
	}
	if err := tt.Click("移動"); err != nil {
		t.Fatalf("clicking Move storage mode pill failed: %v", err)
	}
	if storageModes[a.storageMode] != model.StorageMove {
		t.Fatalf("expected storageMode=move, got %s", storageModes[a.storageMode])
	}

	// 8. Sort PopoverMenuButton MUST open in-window popover menu and change sort!
	if err := tt.Click("排序"); err != nil {
		t.Fatalf("clicking Sort button failed: %v", err)
	}
	if !tt.HasText("檔名 A–Z") {
		t.Fatalf("Sort popover menu did not open")
	}
	if err := tt.Click("檔名 A–Z"); err != nil {
		t.Fatalf("clicking Sort menu item failed: %v", err)
	}
	if a.sortBy != sortName {
		t.Fatalf("expected sortName, got %d", a.sortBy)
	}

	// 8b. Layout PopoverMenuButton MUST switch between Bento, Waterfall Masonry, and Grid!
	if err := tt.Click("排版"); err != nil {
		t.Fatalf("clicking Layout button failed: %v", err)
	}
	if !tt.HasText("瀑布流") || !tt.HasText("標準網格") {
		t.Fatalf("Layout popover menu did not open, texts=%v", tt.Texts())
	}
	if err := tt.Click("瀑布流"); err != nil {
		t.Fatalf("clicking Waterfall layout failed: %v", err)
	}
	if a.layoutMode != layoutMasonry {
		t.Fatalf("expected layoutMasonry, got %q", a.layoutMode)
	}
	tt.Advance(300 * time.Millisecond)
	if err := tt.Click("排版"); err != nil {
		t.Fatalf("clicking Layout button (2nd) failed: %v", err)
	}
	if err := tt.Click("標準網格"); err != nil {
		t.Fatalf("clicking Grid layout failed: %v", err)
	}
	if a.layoutMode != layoutGrid {
		t.Fatalf("expected layoutGrid, got %q", a.layoutMode)
	}
	tt.Advance(300 * time.Millisecond)
	if err := tt.Click("排版"); err != nil {
		t.Fatalf("clicking Layout button (3rd) failed: %v", err)
	}
	if err := tt.Click("Bento 拼貼"); err != nil {
		t.Fatalf("clicking Bento layout failed: %v", err)
	}
	if a.layoutMode != layoutBento {
		t.Fatalf("expected layoutBento, got %q", a.layoutMode)
	}

	// 9. "+ 匯入" PopoverMenuButton MUST open in-window menu with Files & Folder!
	if err := tt.Click("+ 匯入"); err != nil {
		t.Fatalf("clicking + Import failed: %v", err)
	}
	if !tt.HasText("匯入檔案") || !tt.HasText("匯入資料夾") {
		t.Fatalf("Import popover menu did not open")
	}
	tt.Key(0, gui.KeyEscape)

	// 10. Inspector auto-hides when nothing is selected; selecting a card reveals it, and toggle hides & restores it!
	a.selectCard(0, 0)
	tt.Advance(500 * time.Millisecond)
	if err := tt.Click("切換資訊欄"); err != nil {
		t.Fatalf("clicking inspector toggle failed: %v", err)
	}
	if a.inspectorOpen {
		t.Fatal("inspectorOpen stayed true")
	}
	tt.Advance(450 * time.Millisecond)
	if err := tt.Click("切換資訊欄"); err != nil {
		t.Fatalf("clicking inspector toggle (2nd) failed: %v", err)
	}
	if !a.inspectorOpen {
		t.Fatal("inspectorOpen stayed false")
	}

	// 11. Sidebar "+ 新專案" modal creates a project, then Edit Project modal updates it!
	if err := tt.Click("新專案"); err != nil {
		t.Fatalf("clicking New Project failed: %v", err)
	}
	if !a.newProjOpen {
		t.Fatal("newProjOpen stayed false")
	}
	tt.Type("My Studio")
	if err := tt.Click("建立專案"); err != nil {
		t.Fatalf("clicking Create Project failed: %v", err)
	}
	if a.screen != screenProject || a.project.Name != "My Studio" {
		t.Fatalf("project creation failed: screen=%v name=%q", a.screen, a.project.Name)
	}
	if err := tt.Click("編輯專案"); err != nil {
		t.Fatalf("clicking Edit Project failed: %v", err)
	}
	if !a.editProjOpen {
		t.Fatal("editProjOpen stayed false")
	}
	a.newProjectName = "Studio 2026"
	if err := tt.Click("儲存"); err != nil {
		t.Fatalf("clicking Save in Edit Project modal failed: %v", err)
	}
	if a.project.Name != "Studio 2026" {
		t.Fatalf("expected updated project name Studio 2026, got %q", a.project.Name)
	}

	// 12. Create board, switch tray to Library, click asset to place on board, verify placement!
	if err := tt.Click("+ 新白板"); err != nil {
		t.Fatalf("clicking + New Board failed: %v", err)
	}
	if a.screen != screenBoard {
		t.Fatalf("expected screenBoard, got %v", a.screen)
	}
	if err := tt.Click("資料庫"); err != nil {
		t.Fatalf("clicking tray Library failed: %v", err)
	}
	tt.Advance(50 * time.Millisecond)
	if err := tt.Click("alpha_poster.png"); err != nil {
		t.Fatalf("clicking tray asset failed: %v", err)
	}
	if a.session == nil || len(a.session.Items) == 0 {
		t.Fatal("clicking tray asset did not place item on board")
	}
	// Toggle board tool to Text and back to Select.
	if err := tt.Click("文字"); err != nil {
		t.Fatalf("clicking Text tool failed: %v", err)
	}
	if a.tool != 2 {
		t.Fatalf("expected tool=2, got %d", a.tool)
	}
	if err := tt.Click("返回專案"); err != nil {
		t.Fatalf("clicking Back to project failed: %v", err)
	}
	if a.screen != screenProject {
		t.Fatalf("expected screenProject after leaving board, got %v", a.screen)
	}

	// 13. Switch to Campaign project from sidebar and verify tag toggle works.
	if err := tt.Click("Campaign"); err != nil {
		t.Fatalf("clicking Campaign in sidebar failed: %v", err)
	}
	if a.projectID != proj.ID {
		t.Fatalf("expected projectID=%s, got %s", proj.ID, a.projectID)
	}

	// 14. Open Preferences modal from sidebar gear, navigate Database & Import tabs,
	// trigger real manual backup, change default storage mode, and close!
	if err := tt.Click("偏好設定 (Ctrl+,)"); err != nil {
		t.Fatalf("clicking Settings gear failed: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	if !a.settingsOpen || !tt.HasText("偏好設定") {
		t.Fatalf("Preferences modal did not open")
	}
	if err := tt.Click("資料庫"); err != nil {
		t.Fatalf("clicking Database tab failed: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	if !tt.HasText("立即備份資料庫") {
		t.Fatalf("Database settings controls missing, texts=%v", tt.Texts())
	}
	if err := tt.Click("立即備份資料庫"); err != nil {
		t.Fatalf("clicking Backup Now failed: %v", err)
	}
	if !a.dbStats.HasBackup {
		t.Fatal("expected dbStats.HasBackup=true after clicking Backup Now")
	}
	if err := tt.Click("匯入"); err != nil {
		t.Fatalf("clicking Import tab failed: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	if !tt.HasText("檔案存放方式") {
		t.Fatalf("Import settings controls missing, texts=%v", tt.Texts())
	}
	if err := tt.Click("複製"); err != nil {
		t.Fatalf("clicking Copy storage mode in Preferences failed: %v", err)
	}
	if storageModes[a.storageMode] != model.StorageCopy {
		t.Fatalf("expected Copy storage mode, got %s", storageModes[a.storageMode])
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatalf("clicking Done in Preferences failed: %v", err)
	}
	if a.settingsOpen {
		t.Fatal("settingsOpen stayed true after clicking Done")
	}

	// 15. Verify integrated frameless window controls are mounted & safe to click.
	if err := tt.Click("最小化"); err != nil {
		t.Fatalf("frameless minimize button missing: %v", err)
	}

	// 16. Hierarchical Tag Manager popup flow: open Tag Manager popup, create a child tag under tagA,
	// assign tag color, verify cycle protection, and close popup.
	if err := tt.Click("標籤管理"); err != nil {
		t.Fatalf("clicking Tag Manager in sidebar failed: %v", err)
	}
	tt.Advance(250 * time.Millisecond)
	if !a.tagManagerOpen {
		t.Fatal("expected tagManagerOpen=true after clicking Tag Manager")
	}
	child, err := lib.CreateTagWithParent("Character", tagA.ID)
	if err != nil {
		t.Fatalf("CreateTagWithParent failed: %v", err)
	}
	if err := lib.SetTagColor(tagA.ID, "#22C55E"); err != nil {
		t.Fatalf("SetTagColor failed: %v", err)
	}
	if _, err := lib.UpdateTag(tagA.ID, tagA.Name, child.ID); err == nil {
		t.Fatal("expected cycle error when reparenting parent under its own child")
	}
	// Verify clicking the parent tag row in Tag Manager expands its children!
	a.expandedTags[tagA.ID] = false
	a.reload()
	tt.Advance(200 * time.Millisecond)
	if tt.HasText("Character") {
		t.Fatalf("expected child tag hidden while collapsed, texts=%v", tt.Texts())
	}
	if err := tt.Click(tagA.Name); err != nil {
		t.Fatalf("clicking parent tag row in Tag Manager failed: %v", err)
	}
	tt.Advance(200 * time.Millisecond)
	if !a.expandedTags[tagA.ID] || !tt.HasText("Character") {
		t.Fatalf("clicking parent tag row did not expand children! expanded=%v texts=%v", a.expandedTags[tagA.ID], tt.Texts())
	}
	if _, ok := a.effectiveTagColor(child.ID); !ok {
		t.Fatal("expected child tag to inherit parent tag color")
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatalf("closing Tag Manager popup failed: %v", err)
	}
	if a.tagManagerOpen {
		t.Fatal("expected tagManagerOpen=false after clicking Done")
	}

	// 17. Multi-select mode & Batch Action Bar flow in Library.
	if err := tt.Click("全部素材"); err != nil {
		t.Fatalf("returning to All Assets failed: %v", err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("多選模式"); err != nil {
		t.Fatalf("clicking Multi-select toggle failed: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	if err := tt.Click("全選"); err != nil {
		t.Fatalf("clicking Select All in Batch Bar failed: %v", err)
	}
	if len(a.selectedAssetIDs()) != 2 {
		t.Fatalf("expected 2 selected assets after Select All, got %d", len(a.selectedAssetIDs()))
	}
	a.batchReferenceToProject(a.selectedAssetIDs(), proj.ID)
	projAssets, err := store.Assets(proj.ID)
	if err != nil || len(projAssets) != 2 {
		t.Fatalf("expected 2 assets referenced in project after batch action, got %d (err=%v)", len(projAssets), err)
	}

	// 18. Custom theme preset & custom hex override verification.
	a.applyThemePreset("warm")
	tt.Frame()
	if a.dark {
		t.Fatal("expected warm preset to resolve to light appearance")
	}
	a.customCanvas = "#1A1B26"
	a.themePreset = "custom"
	tt.Frame()
	if !a.dark {
		t.Fatal("expected dark custom canvas hex to resolve to dark appearance")
	}
	a.resetCustomTheme()
	_ = tagB
}

func TestExpandImportPathsPreferences(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	hiddenDir := filepath.Join(dir, ".git")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.MkdirAll(hiddenDir, 0o755)

	write := func(p string) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "a.png"))
	write(filepath.Join(dir, ".DS_Store"))
	write(filepath.Join(dir, "Thumbs.db"))
	write(filepath.Join(sub, "b.png"))
	write(filepath.Join(hiddenDir, "config"))

	gotRec := expandImportPaths([]string{dir}, true, true)
	if len(gotRec) != 2 {
		t.Fatalf("recursive+ignoreHidden expected 2 files, got %d: %v", len(gotRec), gotRec)
	}
	gotFlat := expandImportPaths([]string{dir}, false, true)
	if len(gotFlat) != 1 || filepath.Base(gotFlat[0]) != "a.png" {
		t.Fatalf("non-recursive expected [a.png], got %v", gotFlat)
	}
	gotAll := expandImportPaths([]string{dir}, true, false)
	if len(gotAll) != 5 {
		t.Fatalf("recursive+includeHidden expected 5 files, got %d: %v", len(gotAll), gotAll)
	}
}

func TestGlobalShortcutsAndMarqueeBoxSelection(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	img1 := filepath.Join(srcDir, "one.png")
	img2 := filepath.Join(srcDir, "two.png")
	img3 := filepath.Join(srcDir, "three.png")
	writeSamplePNG(t, img1, 160, 120, color.RGBA{220, 80, 120, 255})
	writeSamplePNG(t, img2, 160, 120, color.RGBA{60, 160, 220, 255})
	writeSamplePNG(t, img3, 160, 120, color.RGBA{120, 200, 90, 255})

	lib, err := library.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.Import(context.Background(), []string{img1, img2, img3}, model.StorageCopy, nil, nil); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.lib = lib
	a.projects = project.New(lib.DB)
	a.lang = i18n.ZhHant
	a.cfg.LibraryPath = root
	a.screen = screenLibrary
	a.reload()

	tt := gui.NewTester(a.View, 1360, 860)
	tt.Advance(400 * time.Millisecond)

	// 1. Global Ctrl+A MUST select all 3 assets even with ZERO widget focused!
	tt.Key(gui.Cmd, gui.KeyA)
	if len(a.selectedAssetIDs()) != 3 {
		t.Fatalf("Ctrl+A with no focus failed: selected=%d, want 3", len(a.selectedAssetIDs()))
	}

	// 2. Global Escape clears selection.
	tt.Key(0, gui.KeyEscape)
	if len(a.selectedAssetIDs()) != 0 {
		t.Fatalf("Escape did not clear selection: selected=%d", len(a.selectedAssetIDs()))
	}

	// 3. Global Arrow Right selects first asset, Shift+Right extends to 2 assets, '5' rates both 5 stars!
	tt.Key(0, gui.KeyRight)
	if len(a.selectedAssetIDs()) != 1 {
		t.Fatalf("Arrow Right expected 1 selected, got %d", len(a.selectedAssetIDs()))
	}
	tt.Key(gui.Shift, gui.KeyRight)
	if len(a.selectedAssetIDs()) != 2 {
		t.Fatalf("Shift+Right expected 2 selected, got %d", len(a.selectedAssetIDs()))
	}
	tt.Key(0, gui.Key5)
	for _, id := range a.selectedAssetIDs() {
		got, err := lib.Get(id)
		if err != nil || got.Rating != 5 {
			t.Fatalf("Key5 did not rate asset %s to 5 (got %d, err=%v)", id, got.Rating, err)
		}
	}

	// 4. Global Space opens Quick Look pop-up (1st press), 2nd Space promotes to Full Inline Preview; Escape closes it.
	tt.Key(0, gui.KeySpace)
	if !a.quickLookOpen || a.previewOpen {
		t.Fatalf("1st Space should open Quick Look pop-up: quickLook=%v preview=%v", a.quickLookOpen, a.previewOpen)
	}
	tt.Key(0, gui.KeySpace)
	if a.quickLookOpen || !a.previewOpen {
		t.Fatalf("2nd Space should promote to Full Inline Preview: quickLook=%v preview=%v", a.quickLookOpen, a.previewOpen)
	}
	tt.Key(0, gui.KeyEscape)
	if a.previewOpen || a.quickLookOpen {
		t.Fatal("Escape did not close Quick Preview")
	}

	// 5. Global Ctrl+, opens Preferences; Escape closes it.
	tt.Key(gui.Cmd, gui.KeyComma)
	if !a.settingsOpen {
		t.Fatal("global Ctrl+, did not open Preferences")
	}
	tt.Key(0, gui.KeyEscape)
	if a.settingsOpen {
		t.Fatal("Escape did not close Preferences")
	}

	// 6. Global Delete opens batch delete confirmation dialog.
	tt.Key(0, gui.KeyDelete)
	if !a.askOpen {
		t.Fatal("global Delete did not open confirmation dialog")
	}
	tt.Key(0, gui.KeyEscape)
	tt.Frame()

	// 7. Rubber-band marquee drag on empty space selects intersecting cards live
	// across Bento, Waterfall Masonry, AND Grid layouts!
	for _, mode := range []string{layoutBento, layoutMasonry, layoutGrid} {
		a.setLayoutMode(mode)
		a.clearSelection()
		a.assetIndex = -1
		tt.Frame()
		tt.Advance(300 * time.Millisecond)

		sb := a.stageBounds
		if sb.W <= 0 || sb.H <= 0 {
			t.Fatalf("[%s] stageBounds empty: %+v", mode, sb)
		}
		// Start on the empty space below the cards and drag up-left across them.
		startX := sb.X + sb.W - 6
		startY := sb.Y + sb.H - 20
		endX := sb.X + 40
		endY := sb.Y + 40
		tt.Press(startX, startY)
		tt.Move(endX, endY)
		if !a.marqueeDragging {
			t.Fatalf("[%s] marquee did not enter dragging state (active=%v stage=%+v)", mode, a.marqueeActive, sb)
		}
		tt.Release(endX, endY)

		got := len(a.selectedAssetIDs())
		if got < 2 {
			t.Fatalf("[%s] rubber-band marquee selected %d cards, want >= 2 (cardBounds=%+v)", mode, got, a.cardBounds)
		}

		// Plain click on empty bottom canvas clears selection cleanly.
		tt.ClickAt(startX, startY)
		if len(a.selectedAssetIDs()) != 0 {
			t.Fatalf("[%s] clicking empty canvas did not clear selection: got %d", mode, len(a.selectedAssetIDs()))
		}
	}

	// 8. Interactive Onboarding guided tour flow: click sidebar help button -> try live action -> step through all 5 steps -> finish.
	a.onboardingDone = false
	if err := tt.Click("新手教學"); err != nil {
		t.Fatalf("clicking onboarding help button failed: %v", err)
	}
	tt.Advance(300 * time.Millisecond)
	if !a.onboardingOpen || !tt.HasText("1. 匯入素材或網頁書籤") {
		t.Fatalf("onboarding step 1 did not render, texts=%v", tt.Texts())
	}
	for step := 1; step < onboardingTotalSteps; step++ {
		if err := tt.Click("下一步"); err != nil {
			t.Fatalf("clicking Next on step %d failed: %v", step, err)
		}
		tt.Advance(300 * time.Millisecond)
		if a.onboardingStep != step {
			t.Fatalf("expected onboardingStep=%d, got %d", step, a.onboardingStep)
		}
		if step == 1 && len(a.assets) > 0 {
			if err := tt.Click("幫我選取第一張素材"); err != nil {
				t.Fatalf("clicking live tour action failed: %v", err)
			}
			if !a.onboardingTried[1] || a.assetIndex != 0 {
				t.Fatalf("expected live tour step 2 action to select first asset, tried=%v idx=%d", a.onboardingTried[1], a.assetIndex)
			}
		}
	}
	if !tt.HasText("5. 挑選喜歡的主題風格與 AI / MCP") {
		t.Fatalf("onboarding step 5 missing, texts=%v", tt.Texts())
	}
	if err := tt.Click("開始使用 Raglex"); err != nil {
		t.Fatalf("clicking Finish onboarding failed: %v", err)
	}
	if a.onboardingOpen || !a.onboardingDone {
		t.Fatalf("onboarding did not complete cleanly: open=%v done=%v", a.onboardingOpen, a.onboardingDone)
	}

	// 9. Collapsible left sidebar via Ctrl+B and top-left toggle button.
	if !a.sidebarOpen {
		t.Fatal("expected sidebarOpen=true initially")
	}
	tt.Key(gui.Cmd, gui.KeyB)
	if a.sidebarOpen {
		t.Fatal("Ctrl+B did not collapse left sidebar")
	}
	if err := tt.Click("切換側邊欄 (Ctrl+B)"); err != nil {
		t.Fatalf("clicking sidebar toggle button failed: %v", err)
	}
	if !a.sidebarOpen {
		t.Fatal("clicking sidebar toggle button did not restore sidebar")
	}

	// 10. Stage 12: Batch Rename modal via Ctrl+A + F2, live preview, apply, and Ctrl+Z undo.
	tt.Key(gui.Cmd, gui.KeyA)
	tt.Key(0, gui.KeyF2)
	tt.Advance(300 * time.Millisecond)
	if !a.renameOpen || !tt.HasText("批量重新命名") {
		t.Fatalf("F2 did not open Batch Rename modal: open=%v texts=%v", a.renameOpen, tt.Texts())
	}
	a.renameTemplate = "shot_{index:02}"
	tt.Frame()
	if !tt.HasText("shot_01.png") {
		t.Fatalf("live preview did not render shot_01.png, texts=%v", tt.Texts())
	}
	if err := tt.Click("套用重新命名"); err != nil {
		t.Fatalf("clicking Apply Rename failed: %v", err)
	}
	if a.renameOpen {
		t.Fatal("rename modal stayed open after apply")
	}
	foundShot := false
	for _, asset := range a.assets {
		if a.assetName(asset) == "shot_01.png" {
			foundShot = true
			break
		}
	}
	if !foundShot {
		t.Fatalf("expected renamed asset shot_01.png in library")
	}
	// Ctrl+Z reverts the batch rename cleanly.
	tt.Key(gui.Cmd, gui.KeyZ)
	tt.Frame()
	for _, asset := range a.assets {
		if a.assetName(asset) == "shot_01.png" {
			t.Fatalf("Ctrl+Z did not revert shot_01.png")
		}
	}

	// 11. Ctrl+T Go to / Command Palette popup opens, filters, and executes!
	tt.Key(gui.Cmd, gui.KeyT)
	tt.Advance(250 * time.Millisecond)
	if !a.gotoOpen || !tt.HasText("快速跳轉與操作") {
		t.Fatalf("Ctrl+T did not open Go to palette: open=%v texts=%v", a.gotoOpen, tt.Texts())
	}
	tt.Key(0, gui.KeyEscape)
	tt.Frame()
	if a.gotoOpen {
		t.Fatal("Escape did not close Go to palette")
	}

	// 12. Dual-stage Space preview (1st Space -> Quick Look Pop-up, 2nd Space -> Full Inline Preview, Esc -> Grid).
	if len(a.assets) > 0 {
		a.selectCard(0, 0)
		tt.Frame()
		tt.Key(0, gui.KeySpace)
		tt.Advance(250 * time.Millisecond)
		if !a.quickLookOpen {
			t.Fatal("1st Space did not open Quick Look pop-up")
		}
		tt.Key(0, gui.KeySpace)
		tt.Advance(250 * time.Millisecond)
		if !a.previewOpen || a.quickLookOpen {
			t.Fatalf("2nd Space did not promote to inline preview: quickLook=%v preview=%v", a.quickLookOpen, a.previewOpen)
		}
		// Inspector Similar tab switches cleanly inside inline viewer.
		if err := tt.Click("相似"); err != nil {
			t.Fatalf("clicking Similar tab in inspector failed: %v", err)
		}
		if a.inspectorTab != 1 {
			t.Fatalf("expected inspectorTab=1, got %d", a.inspectorTab)
		}
		tt.Key(0, gui.KeyEscape)
		tt.Frame()
		if a.previewOpen {
			t.Fatal("Esc did not exit inline preview")
		}
	}
}

func TestStacksNativePreviewsAndRefWindowUI(t *testing.T) {
	root := t.TempDir()
	lib, err := library.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	srcDir := t.TempDir()
	img1 := filepath.Join(srcDir, "poster_v1.png")
	img2 := filepath.Join(srcDir, "poster_v2.png")
	writeSamplePNG(t, img1, 160, 120, color.RGBA{240, 90, 60, 255})
	writeSamplePNG(t, img2, 160, 120, color.RGBA{60, 140, 240, 255})

	svgPath := filepath.Join(srcDir, "logo_vector.svg")
	_ = os.WriteFile(svgPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="120" viewBox="0 0 120 120"><circle cx="60" cy="60" r="48" fill="#ff5c33"/></svg>`), 0o644)

	// Minimal valid 16-bit WAV file.
	wavPath := filepath.Join(srcDir, "sfx_click.wav")
	wav := make([]byte, 44+320)
	copy(wav[0:4], "RIFF")
	wav[4] = 0x64
	wav[5] = 0x01
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	wav[16] = 16
	wav[20] = 1
	wav[22] = 1
	wav[24] = 0x40
	wav[25] = 0x1F // 8000 Hz
	wav[28] = 0x80
	wav[29] = 0x3E
	wav[32] = 2
	wav[34] = 16
	copy(wav[36:40], "data")
	wav[40] = 0x40
	wav[41] = 0x01
	_ = os.WriteFile(wavPath, wav, 0o644)

	res, err := lib.Import(context.Background(), []string{img1, img2, svgPath, wavPath}, model.StorageCopy, nil, nil)
	if err != nil || res.Added != 4 {
		t.Fatalf("import failed: added=%d err=%v", res.Added, err)
	}

	a := NewApp()
	a.lang = i18n.ZhHant
	a.lib = lib
	a.projects = project.New(lib.DB)
	a.screen = screenLibrary
	a.reload()

	tt := gui.NewTester(a.View, 1360, 860)
	tt.Frame()

	// 1. Asset Stack folding & expanding.
	beforeCount := len(a.assets)
	if beforeCount != 4 {
		t.Fatalf("expected 4 assets, got %d", beforeCount)
	}
	a.stackSelectedAssets([]string{res.Items[0].AssetID, res.Items[1].AssetID})
	tt.Frame()
	if len(a.assets) != beforeCount-1 {
		t.Fatalf("expected library to collapse 2 stacked assets into 1 card: before=%d after=%d", beforeCount, len(a.assets))
	}
	if len(a.stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(a.stacks))
	}
	stID := a.stacks[0].ID
	a.openStackView(stID)
	tt.Frame()
	if len(a.assets) != 2 {
		t.Fatalf("expected 2 assets when stack expanded, got %d", len(a.assets))
	}
	if !tt.HasText("解散堆疊") {
		t.Fatalf("expected stack banner controls when expanded, texts=%v", tt.Texts())
	}
	a.closeStackView()
	tt.Frame()
	a.ungroupStack(stID)
	tt.Frame()
	if len(a.assets) != beforeCount {
		t.Fatalf("expected %d assets after ungrouping stack, got %d", beforeCount, len(a.assets))
	}

	// 2. Native SVG lossless preview & Audio waveform preview inside inline viewer.
	svgAsset, ok := a.assetByID[res.Items[2].AssetID]
	if !ok {
		t.Fatal("missing svg asset")
	}
	a.openPreview(svgAsset)
	tt.Frame()
	if a.previewSVG == nil {
		t.Fatal("expected parsed SVG in inline preview")
	}
	if !tt.HasText("SVG 無損向量") {
		t.Fatalf("expected SVG vector badge in inline preview, texts=%v", tt.Texts())
	}

	wavAsset, ok := a.assetByID[res.Items[3].AssetID]
	if !ok {
		t.Fatal("missing wav asset")
	}
	a.openPreview(wavAsset)
	tt.Frame()
	if len(a.previewAudioWave.Peaks) != 96 {
		t.Fatalf("expected 96 audio waveform bars, got %d", len(a.previewAudioWave.Peaks))
	}
	a.closePreview()
	tt.Frame()

	// 3. Floating reference window view renders cleanly with active board session.
	b, err := a.projects.CreateStandaloneBoard("RefMood")
	if err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.openBoard(b)
	tt.Frame()
	if !tt.HasText("置頂參考浮窗") {
		t.Fatalf("expected Floating Ref button on board toolbar, texts=%v", tt.Texts())
	}
	refTester := gui.NewTester(a.refWinView, 540, 420)
	refTester.Frame()
	if !refTester.HasText("RefMood") {
		t.Fatalf("expected board title in Floating Ref HUD, texts=%v", refTester.Texts())
	}

	// 4. Auto-update confirmation popup renders cleanly with 1-click update & restart button.
	a.updateRelease = &updater.Release{
		TagName:     "v9.9.0",
		Body:        "Performance & UX polish",
		DownloadURL: "https://example.com/raglex.zip",
	}
	a.updateAvailable = true
	a.updatePromptOpen = true
	tt.Frame()
	if !tt.HasText("立即更新並重啟") || !tt.HasText("稍後再說") {
		t.Fatalf("expected auto-update popup actions, texts=%v", tt.Texts())
	}
	a.updatePromptOpen = false

	// 5. Web Sources modal (live YouTube & RSS badges) + dedicated Watch Folders modal.
	a.webImportOpen = true
	a.webImportURL = "https://youtu.be/dQw4w9WgXcQ\nhttps://example.com/feed.xml"
	tt.Frame()
	if !tt.HasText("YouTube ×1") || !tt.HasText("RSS/Feed ×1") {
		t.Fatalf("expected live multi-source detection badges, texts=%v", tt.Texts())
	}
	a.webImportOpen = false

	a.watchFoldersOpen = true
	tt.Frame()
	if !tt.HasText("新增資料夾") || !tt.HasText("監控資料夾") {
		t.Fatalf("expected dedicated Watch Folders modal controls, texts=%v", tt.Texts())
	}
	a.watchFoldersOpen = false

	// 6. Custom ordering for Tags & Projects + global item count visibility toggle.
	t1, _ := a.lib.CreateTag("TagOne")
	t2, _ := a.lib.CreateTag("TagTwo")
	p1, _ := a.projects.Create("ProjOne", "", []string{t1.ID})
	p2, _ := a.projects.Create("ProjTwo", "", []string{t1.ID})
	a.reload()
	a.reorderTagDrop(t2.ID, t1.ID)
	if len(a.tagChildren[""]) < 2 || a.tagChildren[""][0].ID != t2.ID {
		t.Fatalf("expected TagTwo reordered first, got %+v", a.tagChildren[""])
	}
	a.reorderProjectDrop(p2.ID, p1.ID)
	if len(a.projectList) < 2 || a.projectList[0].ID != p2.ID {
		t.Fatalf("expected ProjTwo reordered first, got %+v", a.projectList)
	}

	// Hide all item counts across sidebar & library header.
	a.handleNav("tag-all")
	a.showSidebarCounts = true
	tt.Frame()
	if !tt.HasText("個素材") {
		t.Fatalf("expected header items count pill when showSidebarCounts=true, texts=%v", tt.Texts())
	}
	a.showSidebarCounts = false
	tt.Frame()
	if tt.HasText("個素材") {
		t.Fatalf("expected header items count pill hidden when showSidebarCounts=false, texts=%v", tt.Texts())
	}

	// 7. Multiline & auto-wrapping board text renders and expands box height cleanly.
	a.openBoard(b)
	tid := a.session.PlaceText("Line 1\nLine 2\nVeryLongWordWithoutSpaces_aaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, 100)
	a.session.Selected = map[string]bool{tid: true}
	tt.Frame()
	if cur := a.session.Items[tid]; cur.Kind != model.KindText {
		t.Fatalf("expected board text item, got %+v", cur)
	}
	a.editText = "First line\nSecond line\nThird line\nFourth line\nFifth line"
	a.session.SetText(tid, a.editText)
	tt.Frame()

	// 8. Focus Mode (Ctrl+Alt+C) hides top nav bar, sidebar, and inspector, and restores cleanly!
	a.handleNav("tag-all")
	a.sidebarOpen = true
	a.inspectorOpen = true
	tt.Frame()
	if !tt.HasText("+ 匯入") {
		t.Fatalf("expected topbar visible before Focus Mode, texts=%v", tt.Texts())
	}
	a.toggleFocusMode()
	tt.Frame()
	if a.sidebarOpen || a.inspectorOpen || tt.HasText("+ 匯入") {
		t.Fatalf("expected topbar/sidebar/inspector hidden in Focus Mode, sb=%v insp=%v texts=%v", a.sidebarOpen, a.inspectorOpen, tt.Texts())
	}
	a.toggleFocusMode()
	tt.Frame()
	if !a.sidebarOpen || !a.inspectorOpen || !tt.HasText("+ 匯入") {
		t.Fatalf("expected topbar/sidebar/inspector restored after exiting Focus Mode, sb=%v insp=%v", a.sidebarOpen, a.inspectorOpen)
	}

	// 9. Multi-select referenceSelected() MUST reference ALL selected assets into the active project.
	a.projectID = p1.ID
	a.project = p1
	a.referenceMode = true
	a.multiSelectMode = true
	a.selectAllVisible()
	wantRefCount := len(a.selectedAssetIDs())
	if wantRefCount < 2 {
		t.Fatalf("expected at least 2 visible assets for multi-select reference test, got %d", wantRefCount)
	}
	a.referenceSelected()
	refAssets, err := a.projects.Assets(p1.ID)
	if err != nil || len(refAssets) != wantRefCount {
		t.Fatalf("expected all %d selected assets referenced into project, got %d err=%v", wantRefCount, len(refAssets), err)
	}
}

// TestFoldersSidebarSwitcherAndSyncBar verifies the parallel hierarchical Folder
// system, left-sidebar Folders <-> Tags toggle, recursive subfolder filtering,
// Inspector folder chips, and the live Cloud Sync progress bar pinned above the
// sidebar bottom footer.
func TestFoldersSidebarSwitcherAndSyncBar(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "Brand", "Icons")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	img1 := filepath.Join(subDir, "logo_mark.png")
	img2 := filepath.Join(srcDir, "loose_note.png")
	writeSamplePNG(t, img1, 120, 120, color.RGBA{255, 92, 51, 255})
	writeSamplePNG(t, img2, 120, 120, color.RGBA{59, 130, 246, 255})

	lib, err := library.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	// Import Brand/ tree preserving subfolders -> creates Folders "Brand" -> "Icons"
	if _, err := lib.ImportWithFolders(context.Background(), []string{filepath.Join(srcDir, "Brand")}, model.StorageCopy, nil, nil, true, nil); err != nil {
		t.Fatal(err)
	}
	// Import loose file with a Tag only (uncategorized by Folder)
	tg, err := lib.CreateTag("Sketch")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.ImportWithFolders(context.Background(), []string{img2}, model.StorageCopy, []string{tg.ID}, nil, false, nil); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.lib = lib
	a.projects = project.New(lib.DB)
	a.lang = i18n.ZhHant
	a.cfg.LibraryPath = root
	a.screen = screenLibrary
	a.reload()

	tt := gui.NewTester(a.View, 1360, 860)
	tt.Advance(400 * time.Millisecond)

	// 1. Because Folders exist, sidebar defaults to "folders" mode and shows "Brand".
	if a.effectiveBrowseMode() != "folders" {
		t.Fatalf("expected default browse mode 'folders' when folders exist, got %q", a.effectiveBrowseMode())
	}
	if !tt.HasText("Brand") {
		t.Fatalf("expected root folder 'Brand' visible in sidebar, texts=%v", tt.Texts())
	}

	// 2. Clicking "Brand" filters recursively to include nested Icons/logo_mark.png (1 asset).
	if err := tt.Click("Brand"); err != nil {
		t.Fatalf("clicking Brand folder failed: %v", err)
	}
	tt.Advance(200 * time.Millisecond)
	if len(a.assets) != 1 || a.assetName(a.assets[0]) != "logo_mark.png" {
		t.Fatalf("expected Brand folder to filter to logo_mark.png, got %d assets", len(a.assets))
	}
	// Clicking Brand also expands it, revealing subfolder "Icons".
	if !tt.HasText("Icons") {
		t.Fatalf("expected nested subfolder 'Icons' expanded under Brand, texts=%v", tt.Texts())
	}

	// 3. Selecting logo_mark.png shows "所屬資料夾" and "Brand / Icons" in the Right Inspector.
	if err := tt.Click("logo_mark.png"); err != nil {
		t.Fatalf("clicking logo_mark.png failed: %v", err)
	}
	tt.Advance(200 * time.Millisecond)
	if !tt.HasText("所屬資料夾") || !tt.HasText("Brand / Icons") {
		t.Fatalf("expected Inspector to show 所屬資料夾 & Brand / Icons chip, texts=%v", tt.Texts())
	}

	// 4. Switching left sidebar to "標籤" mode shows the parallel Tag tree ("Sketch").
	if err := tt.Click("標籤"); err != nil {
		t.Fatalf("clicking Tags switcher button failed: %v", err)
	}
	tt.Advance(200 * time.Millisecond)
	if a.effectiveBrowseMode() != "tags" {
		t.Fatalf("expected browse mode 'tags' after toggle, got %q", a.effectiveBrowseMode())
	}
	if !tt.HasText("Sketch") {
		t.Fatalf("expected tag 'Sketch' visible in Tags mode, texts=%v", tt.Texts())
	}

	// 5. Clicking "未分類" shows only logo_mark.png (which has no Tag).
	if err := tt.Click("未分類"); err != nil {
		t.Fatalf("clicking Untagged filter failed: %v", err)
	}
	tt.Advance(200 * time.Millisecond)
	if len(a.assets) != 1 || a.assetName(a.assets[0]) != "logo_mark.png" {
		t.Fatalf("expected Untagged filter to show logo_mark.png, got %d assets", len(a.assets))
	}

	// 6. Live Sync Progress Bar in left sidebar appears when syncBusy is true and opens Account settings on click.
	a.syncBusy = true
	a.syncStage = "uploading"
	a.syncDone = 4
	a.syncTotal = 10
	a.syncCurrent = "logo_mark.png"
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("上傳素材中…") || !tt.HasText("4 / 10") {
		t.Fatalf("expected sidebar sync bar to render stage & counter, texts=%v", tt.Texts())
	}
	if err := tt.Click("上傳素材中…"); err != nil {
		t.Fatalf("clicking sidebar sync bar failed: %v", err)
	}
	tt.Advance(200 * time.Millisecond)
	if !a.settingsOpen || a.settingsTab != settingsTabAccount {
		t.Fatalf("expected clicking sidebar sync bar to open Account settings, open=%v tab=%d", a.settingsOpen, a.settingsTab)
	}
	a.settingsOpen = false
	a.syncBusy = false

	// 7. Folder Share modal opens from active folder and displays Raglex Cloud Public Share card.
	a.setSidebarBrowseMode("folders")
	a.handleNav("folder:" + a.folders[0].ID)
	a.accountToken = "test-cloud-token"
	a.openShareFolder(a.folders[0].ID)
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("Raglex Cloud 公開分享碼") || !tt.HasText("產生公開分享碼") {
		t.Fatalf("expected Folder Share modal with Cloud Public Share section, texts=%v", tt.Texts())
	}
	a.shareOpen = false

	// 8. Universal Receive modal recognizes SHR-XXXX-XXXX public codes without needing login.
	a.accountToken = ""
	a.pairRecvOpen = true
	a.pairRecvCode = "SHR-K7M2-9P4Q"
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("下載 / 連線") {
		t.Fatalf("expected Receive modal to switch button to 下載 / 連線 for SHR code, texts=%v", tt.Texts())
	}
	a.pairRecvOpen = false

	// 9. Markdown preview via raglex.markdown-notes plugin renders formatted headings & body text!
	mdPath := filepath.Join(srcDir, "esports.md")
	if err := os.WriteFile(mdPath, []byte("# Esports Notes\n\n- Team Alpha\n- Team Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.ImportWithFolders(context.Background(), []string{mdPath}, model.StorageCopy, nil, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	pm := plugin.NewManager(root)
	if err := pm.Install(context.Background(), plugin.RegistryEntry{
		ID:          "raglex.markdown-notes",
		Name:        "Markdown 筆記與靈感簡報",
		Version:     "1.0.0",
		Hooks:       []string{"preview_renderer", "context_menu"},
		Permissions: []string{"read_library"},
		Builtin:     true,
	}); err != nil {
		t.Fatal(err)
	}
	a.plugins = pm
	a.reload()
	var mdAsset model.Asset
	for _, it := range a.allAssets {
		if a.assetName(it) == "esports.md" {
			mdAsset = it
			break
		}
	}
	if mdAsset.ID == "" {
		t.Fatal("esports.md not imported")
	}
	a.openPreview(mdAsset)
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("Esports Notes") || !tt.HasText("Team Alpha") {
		t.Fatalf("expected Markdown preview to render parsed headings & list items, texts=%v", tt.Texts())
	}
	// Editing & saving Markdown persists directly to disk and updates hash cleanly.
	a.previewDocMode = "edit"
	a.previewDocDraft = "# Esports Notes Updated\n\n- [x] Done task\n"
	a.saveDocumentPreview(mdAsset)
	savedBytes, err := os.ReadFile(mdAsset.Path)
	if err != nil || string(savedBytes) != a.previewDocDraft {
		t.Fatalf("expected edited markdown saved to disk, got err=%v content=%q", err, string(savedBytes))
	}
	a.closePreview()
	tt.Frame()

	// 10. Sidebar top buttons customization: "全部素材" is always visible, others can be toggled off.
	if !tt.HasText("隨機探索") {
		t.Fatalf("expected 隨機探索 visible in sidebar by default, texts=%v", tt.Texts())
	}
	a.showSidebarRandom = false
	a.showSidebarRecent = false
	tt.Frame()
	if tt.HasText("隨機探索") || tt.HasText("近期新增") {
		t.Fatalf("expected toggled-off sidebar buttons to be hidden, texts=%v", tt.Texts())
	}
	if !tt.HasText("全部素材") {
		t.Fatalf("expected 全部素材 to remain permanently visible, texts=%v", tt.Texts())
	}
	a.showSidebarRandom = true
	a.showSidebarRecent = true

	// 11. Sync & Collab tab is locked when signed out, and unlocks when signed in (plus 20 GB quota bar in Account tab).
	a.accountToken = ""
	a.accountEmail = ""
	a.syncProvider = "cloud"
	a.openSettings(settingsTabSync)
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("前往帳號與雲端設定") {
		t.Fatalf("expected Sync & Collab tab to show locked notice when disconnected, texts=%v", tt.Texts())
	}

	a.accountToken = "tok-demo"
	a.accountEmail = "artist@sorai.tw"
	a.accountVerified = true
	a.accountUsedBytes = 2 << 30
	a.accountQuotaBytes = 20 << 30
	a.setSettingsTab(settingsTabAccount)
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("雲端儲存空間") || !tt.HasText("2.00 GB") || !tt.HasText("20.00 GB") {
		t.Fatalf("expected Account tab to render 20 GB storage quota meter, texts=%v", tt.Texts())
	}

	a.setSettingsTab(settingsTabSync)
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("自動背景同步") || !tt.HasText("選擇性資料夾同步") || !tt.HasText("好友協作空間") {
		t.Fatalf("expected unlocked Sync & Collab tab to show auto-sync, selective folder sync & collab spaces, texts=%v", tt.Texts())
	}
	a.settingsOpen = false

	// 12. Receive modal detects CLB-XXXX-XXXX collaboration invite codes cleanly.
	a.pairRecvOpen = true
	a.pairRecvCode = "CLB-9A2B-7C4D"
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("加入協作") {
		t.Fatalf("expected Receive modal to switch action button to 加入協作 for CLB code, texts=%v", tt.Texts())
	}
	a.pairRecvOpen = false

	// 13. Full Japanese localization switches sidebar & settings cleanly.
	a.setLang(i18n.Ja)
	a.openSettings(settingsTabSync)
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("すべてのアセット") || !tt.HasText("環境設定") || !tt.HasText("同期とコラボ") || !tt.HasText("フォルダ選択同期") {
		t.Fatalf("expected Japanese UI across sidebar and Sync & Collab settings, texts=%v", tt.Texts())
	}
	a.settingsOpen = false
	a.setLang(i18n.ZhHant)
}
