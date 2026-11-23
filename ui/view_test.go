package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/canvas"
	"raglex/internal/i18n"
	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

func TestLanguageToggleChangesWelcome(t *testing.T) {
	a := NewApp()
	a.lang = i18n.En
	a.screen = screenWelcome
	tt := gui.NewTester(a.View, 960, 640)
	if !tt.HasText("Choose a folder") {
		t.Fatalf("english welcome missing, texts: %v", tt.Texts())
	}
	if err := tt.Click("中文"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("選擇資料夾") {
		t.Fatalf("chinese welcome missing, texts: %v", tt.Texts())
	}
	if tt.HasText("Choose a folder") {
		t.Fatal("english label still visible")
	}
	if err := tt.Click("EN"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Choose a folder") {
		t.Fatalf("english welcome missing after toggle, texts: %v", tt.Texts())
	}
}

func TestShellLibraryViewRenderingAndSearch(t *testing.T) {
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	tag1, err := lib.CreateTag("插畫")
	if err != nil {
		t.Fatal(err)
	}
	tag2, err := lib.CreateTag("UI設計")
	if err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.lib = lib
	a.projects = project.New(lib.DB)
	a.lang = i18n.ZhHant
	a.screen = screenLibrary
	a.cfg.LibraryPath = dir
	a.tags = []model.Tag{tag1, tag2}
	a.allAssets = []model.Asset{
		{
			ID:          "a1",
			Path:        "Concept_Hero.png",
			StorageMode: model.StorageIndex,
			Mime:        "image/png",
			Width:       1920,
			Height:      1080,
			Rating:      4,
		},
		{
			ID:          "a2",
			Path:        "Asset 2@4xd.png",
			StorageMode: model.StorageCopy,
			Mime:        "image/png",
			Width:       800,
			Height:      600,
			Rating:      5,
		},
	}
	a.allAssetsCount = 2
	a.assetByID = map[string]model.Asset{
		"a1": a.allAssets[0],
		"a2": a.allAssets[1],
	}
	a.assetTags = map[string][]model.Tag{
		"a1": {tag1},
		"a2": {tag2},
	}
	a.tagCounts = map[string]int{tag1.ID: 1, tag2.ID: 1}
	a.applyFilters()

	tt := gui.NewTester(a.View, 1360, 860)
	if !tt.HasText("全部素材") {
		t.Fatalf("library nav missing, texts: %v", tt.Texts())
	}
	if !tt.HasText("插畫") {
		t.Fatalf("tag name missing, texts: %v", tt.Texts())
	}
	if !tt.HasText("Asset 2@4xd.png") {
		t.Fatalf("asset card name missing, texts: %v", tt.Texts())
	}

	// Verify selecting an asset displays metadata in inspector.
	a.assetIndex = 0
	a.syncInspectorSelection(true)
	tt.Frame()
	if !tt.HasText("1920 × 1080") {
		t.Fatalf("inspector dimensions missing, texts: %v", tt.Texts())
	}

	// Live search filters down to matching item.
	a.searchQuery = "Hero"
	a.applyFilters()
	tt.Frame()
	if len(a.assets) != 1 || a.assets[0].ID != "a1" {
		t.Fatalf("search filter failed, got %d assets", len(a.assets))
	}
	a.clearFilters()
	tt.Frame()

	// Quick preview lightbox opens and closes cleanly.
	a.openPreview(a.allAssets[0])
	tt.Frame()
	if !a.previewOpen || !tt.HasText("Concept_Hero.png") {
		t.Fatalf("preview modal failed to render")
	}
	a.previewOpen = false
	tt.Frame()

	// Test clicking tag in sidebar.
	if err := tt.Click("UI設計"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tagID != tag2.ID {
		t.Fatalf("expected tagID to be %q, got %q", tag2.ID, a.tagID)
	}
}

func TestPairingShareUIControls(t *testing.T) {
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	tag, err := lib.CreateTag("Art")
	if err != nil {
		t.Fatal(err)
	}
	store := project.New(lib.DB)
	proj, err := store.Create("Demo", "desc", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.lib = lib
	a.projects = store
	a.lang = i18n.ZhHant
	a.cfg.LibraryPath = dir
	a.openProject(proj.ID)

	tt := gui.NewTester(a.View, 1360, 860)

	// Open receive-by-code modal via Preferences -> Sharing.
	a.openSettings(settingsTabSharing)
	tt.Advance(300 * time.Millisecond)
	if err := tt.Click("輸入配對碼接收專案…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("連線接收") {
		t.Fatalf("receive pair modal missing, texts: %v", tt.Texts())
	}

	// Close receive modal, then open Share modal.
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("分享"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("產生配對碼") {
		t.Fatalf("pairing code button missing, texts: %v", tt.Texts())
	}

	// Simulate active pairing code state.
	a.pairCode = "K7P-4M2"
	tt.Frame()
	if !tt.HasText("複製配對碼") {
		t.Fatalf("copy code button missing, texts: %v", tt.Texts())
	}
}

func makeArtBitmap(w, h int, c1, c2 color.RGBA) *gui.Bitmap {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		ty := float64(y) / float64(h)
		for x := 0; x < w; x++ {
			tx := float64(x) / float64(w)
			f := (tx + ty) * 0.5
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(float64(c1.R)*(1-f) + float64(c2.R)*f),
				G: uint8(float64(c1.G)*(1-f) + float64(c2.G)*f),
				B: uint8(float64(c1.B)*(1-f) + float64(c2.B)*f),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	bm, _ := gui.DecodeBitmap(buf.Bytes())
	return bm
}

func TestRenderScreenshots(t *testing.T) {
	outDir := os.Getenv("RAGLEX_SNAPSHOT_DIR")
	if outDir == "" {
		t.Skip("set RAGLEX_SNAPSHOT_DIR to render screenshots")
	}
	_ = os.MkdirAll(outDir, 0o755)

	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	t1, _ := lib.CreateTag("Editorial")
	t2, _ := lib.CreateTag("UI Motion")
	t3, _ := lib.CreateTag("Typography")
	t4, _ := lib.CreateTagWithParent("Poster", t1.ID)
	t5, _ := lib.CreateTagWithParent("Magazine", t1.ID)
	_ = lib.SetTagColor(t1.ID, "#22C55E")
	_ = lib.SetTagColor(t2.ID, "#EAB308")
	_ = lib.SetTagColor(t3.ID, "#EC4899")
	t1.Color = "#22C55E"
	t2.Color = "#EAB308"
	t3.Color = "#EC4899"
	store := project.New(lib.DB)
	proj, _ := store.Create("Brand Identity 2026", "Visual direction, typography & grid references", []string{t1.ID, t3.ID})
	b1, _ := store.CreateBoard(proj.ID, "Hero Moodboard")
	_, _ = store.CreateBoard(proj.ID, "Color Explorations")

	a := NewApp()
	a.lib = lib
	a.projects = store
	a.lang = i18n.ZhHant
	a.cfg.LibraryPath = filepath.Join(dir, "DesignVault")
	a.themeMode = "dark"

	save := func(name string, w, h int) {
		tt := gui.NewTester(a.View, w, h)
		tt.Advance(600 * time.Millisecond)
		f, err := os.Create(filepath.Join(outDir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Welcome screen (dark).
	a.screen = screenWelcome
	save("welcome_dark.png", 1280, 800)

	// Populate rich mock assets for visual verification.
	colors := [][2]color.RGBA{
		{{244, 114, 182, 255}, {99, 102, 241, 255}},
		{{52, 211, 153, 255}, {14, 165, 233, 255}},
		{{251, 146, 60, 255}, {239, 68, 68, 255}},
		{{167, 139, 250, 255}, {236, 72, 153, 255}},
		{{250, 204, 21, 255}, {249, 115, 22, 255}},
		{{45, 212, 191, 255}, {59, 130, 246, 255}},
	}
	names := []string{"Swiss_Poster_01.png", "Glass_Card_UI.png", "Editorial_Spread.jpg", "Kinetic_Type.png", "Warm_Lighting.png", "Minimal_Grid.png", "Architecture_Study.jpg", "Monochrome_Film.png"}
	dims := [][2]int{
		{1920, 1280}, // 3:2 landscape
		{1080, 1620}, // 2:3 portrait
		{2400, 1350}, // 16:9 wide
		{1200, 1200}, // 1:1 square
		{1080, 1920}, // 9:16 tall
		{1800, 1200}, // 3:2 landscape
		{1400, 1800}, // portrait
		{2000, 1200}, // wide
	}
	colors = append(colors,
		[2]color.RGBA{{148, 163, 184, 255}, {51, 65, 85, 255}},
		[2]color.RGBA{{212, 212, 216, 255}, {63, 63, 70, 255}},
	)
	a.tags = []model.Tag{t1, t2, t3, t4, t5}
	a.rebuildTagTree()
	a.expandedTags[t1.ID] = true
	a.projectList = []model.Project{proj}
	now := time.Now()
	for i, name := range names {
		id := "art-" + string(rune('a'+i))
		w, h := dims[i][0], dims[i][1]
		asset := model.Asset{
			ID:          id,
			Path:        filepath.Join("D:\\References", name),
			StorageMode: model.StorageCopy,
			Mime:        "image/png",
			Width:       w,
			Height:      h,
			Rating:      (i % 5) + 1,
			CreatedAt:   now.Add(-time.Duration(i) * time.Hour),
			Palette: []model.PaletteColor{
				{Hex: "#180A0C", Ratio: 0.30},
				{Hex: "#C91C22", Ratio: 0.25},
				{Hex: "#6B0F14", Ratio: 0.20},
				{Hex: "#F0322E", Ratio: 0.15},
				{Hex: "#F4A45B", Ratio: 0.10},
			},
		}
		a.allAssets = append(a.allAssets, asset)
		a.assetByID[id] = asset
		a.bitmaps[id] = makeArtBitmap(w/6, h/6, colors[i][0], colors[i][1])
		a.assetTags[id] = []model.Tag{t1, t2}
	}
	a.allAssetsCount = len(a.allAssets)
	a.tagCounts = map[string]int{t1.ID: len(names), t2.ID: len(names), t3.ID: 2, t4.ID: 3, t5.ID: 2}
	a.tagRecursiveCounts = map[string]int{t1.ID: len(names), t2.ID: len(names), t3.ID: 2, t4.ID: 3, t5.ID: 2}
	a.applyFilters()
	a.screen = screenLibrary
	a.assetIndex = 0
	a.inspectorFileSize = 2480128
	a.syncInspectorSelection(true)

	// 2. Library Bento (dark + toast), Narrow 980px responsive bar, Waterfall Masonry, Grid, Multi-Select & Bento (light).
	a.layoutMode = layoutBento
	a.notify("匯入完成：新增 11，已存在 0，失敗 0")
	save("library_bento_dark.png", 1360, 860)
	a.toastVisible = false
	a.previewOpen = true
	a.previewAssetID = "art-a"
	a.inspFileMenuOpen = true
	a.inspFileMenuPinned = true
	save("inspector_hover_dark.png", 1360, 860)
	a.inspFileMenuOpen = false
	a.inspFileMenuPinned = false
	a.previewOpen = false
	save("library_narrow_dark.png", 980, 700)
	a.multiSelectMode = true
	a.selectedIDs = map[string]bool{"art-a": true, "art-b": true, "art-c": true}
	save("library_multiselect_dark.png", 1360, 860)
	a.clearSelection()
	a.layoutMode = layoutMasonry
	save("library_masonry_dark.png", 1360, 860)
	a.layoutMode = layoutGrid
	save("library_dark.png", 1360, 860)
	a.layoutMode = layoutBento
	a.themeMode = "light"
	save("library_light.png", 1360, 860)

	// 2a. Tag Manager popup modal (dark).
	a.themeMode = "dark"
	a.screen = screenLibrary
	a.tagManagerOpen = true
	a.syncNavSelection()
	save("tags_manager_dark.png", 1360, 860)
	a.tagManagerOpen = false

	// 2b. Preferences modal screenshots (General, Database, Import — dark).
	a.themeMode = "dark"
	a.openSettings(settingsTabGeneral)
	save("settings_general_dark.png", 1360, 860)
	a.setSettingsTab(settingsTabDatabase)
	a.refreshDBStats()
	save("settings_database_dark.png", 1360, 860)
	a.setSettingsTab(settingsTabImport)
	save("settings_import_dark.png", 1360, 860)
	a.settingsOpen = false

	// 2c. First-run onboarding modal screenshots (Step 1, Step 3, Step 5 — dark).
	a.themeMode = "dark"
	a.openOnboarding()
	save("onboarding_step1_dark.png", 1360, 860)
	a.setOnboardingStep(2)
	save("onboarding_step3_dark.png", 1360, 860)
	a.setOnboardingStep(4)
	save("onboarding_step5_dark.png", 1360, 860)
	a.onboardingOpen = false

	// 3. Project view dark (Arc UI Editorial Hero + live visual Moodboard previews).
	a.themeMode = "dark"
	a.screen = screenProject
	a.projectID = proj.ID
	a.project = proj
	a.projectTags = map[string]bool{t1.ID: true, t3.ID: true}
	a.boards = []model.Board{b1, {ID: "b2", ProjectID: proj.ID, Name: "Color Explorations"}}
	a.boardCounts = map[string]int{b1.ID: 4, "b2": 0}
	a.boardPlacements = map[string][]model.Placement{
		b1.ID: {
			{ID: "p1", Kind: model.KindImage, AssetID: "art-a", X: 220, Y: 200, Width: 300, Height: 200, Scale: 1, ZIndex: 1},
			{ID: "p2", Kind: model.KindImage, AssetID: "art-b", X: 560, Y: 240, Width: 220, Height: 300, Scale: 1, Rotation: 6, ZIndex: 2},
			{ID: "p3", Kind: model.KindImage, AssetID: "art-c", X: 250, Y: 440, Width: 320, Height: 180, Scale: 1, Rotation: -3, ZIndex: 3},
			{ID: "p4", Kind: model.KindText, X: 540, Y: 460, Width: 240, Height: 48, Scale: 1, Text: "Hero Direction", FontSize: 20, ZIndex: 4},
		},
	}
	a.projectAssets = a.allAssets[:4]
	a.syncNavSelection()
	save("project_dark.png", 1360, 860)

	// 4. Moodboard view dark (include an item crossing top edge to verify strict clipping).
	a.screen = screenBoard
	a.boardID = b1.ID
	a.boardName = b1.Name
	a.session = canvas.NewSession([]model.Placement{
		{ID: "p0", Kind: model.KindImage, AssetID: "art-c", X: 320, Y: -20, Width: 280, Height: 180, Scale: 1, ZIndex: 0},
		{ID: "p1", Kind: model.KindImage, AssetID: "art-a", X: 260, Y: 220, Width: 300, Height: 200, Scale: 1, ZIndex: 1},
		{ID: "p2", Kind: model.KindImage, AssetID: "art-b", X: 600, Y: 280, Width: 260, Height: 180, Scale: 1, Rotation: 4, ZIndex: 2},
		{ID: "p3", Kind: model.KindText, X: 260, Y: 380, Width: 280, Height: 48, Scale: 1, Text: "Hero Direction — High Contrast", FontSize: 20, ZIndex: 3},
	})
	a.session.Selected = map[string]bool{"p1": true}
	save("board_dark.png", 1360, 860)
}
