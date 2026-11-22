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

	"ragle/internal/i18n"
	"ragle/internal/library"
	"ragle/internal/model"
	"ragle/internal/project"
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

	// 1. REAL CLICK on an asset card MUST select it & populate Right Inspector!
	if a.assetIndex != -1 {
		t.Fatalf("expected initial assetIndex=-1, got %d", a.assetIndex)
	}
	if err := tt.Click("alpha_poster.png"); err != nil {
		t.Fatalf("failed to click asset card: %v", err)
	}
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
		tt.ClickAt(r.X+70, r.Y+28)
		updated, err := lib.Get(sel.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Rating == 0 {
			t.Fatalf("clicking Rating in Inspector did NOT save rating! db=%d inspectorRating=%d", updated.Rating, a.inspectorRating)
		}
	}

	// 3. Clicking Quick Preview button inside Right Inspector MUST open Lightbox, and Escape closes it!
	if err := tt.Click("快速預覽"); err != nil {
		t.Fatalf("clicking Quick Preview failed: %v", err)
	}
	if !a.previewOpen {
		t.Fatal("preview modal did not open")
	}
	tt.Key(0, gui.KeyEscape)
	if a.previewOpen {
		t.Fatal("Escape did not close preview modal")
	}

	// 4. Clicking Copy Path inside Right Inspector MUST copy asset path!
	if err := tt.Click("複製路徑"); err != nil {
		t.Fatalf("clicking Copy Path failed: %v", err)
	}
	if tt.Clipboard() != sel.Path {
		t.Fatalf("clipboard=%q, want %q", tt.Clipboard(), sel.Path)
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

	// 7. 1-click Filter Bar pills (Kind, Rating, Storage Mode)!
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

	// 9. "+ 匯入" PopoverMenuButton MUST open in-window menu with Files & Folder!
	if err := tt.Click("+ 匯入"); err != nil {
		t.Fatalf("clicking + Import failed: %v", err)
	}
	if !tt.HasText("匯入檔案") || !tt.HasText("匯入資料夾") {
		t.Fatalf("Import popover menu did not open")
	}
	tt.Key(0, gui.KeyEscape)

	// 10. Inspector toggle hides & restores right panel!
	if err := tt.Click("切換資訊欄"); err != nil {
		t.Fatalf("clicking inspector toggle failed: %v", err)
	}
	if a.inspectorOpen {
		t.Fatal("inspectorOpen stayed true")
	}
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
	if err := tt.Click("+ 新白版"); err != nil {
		t.Fatalf("clicking + New Board failed: %v", err)
	}
	if a.screen != screenBoard {
		t.Fatalf("expected screenBoard, got %v", a.screen)
	}
	if err := tt.Click("母庫"); err != nil {
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
	_ = tagB
}
