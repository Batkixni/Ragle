package ui

import (
	"testing"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/i18n"
	"ragle/internal/library"
	"ragle/internal/model"
	"ragle/internal/project"
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

func TestShellLibraryViewRendering(t *testing.T) {
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	a := NewApp()
	a.lib = lib
	a.projects = project.New(lib.DB)
	a.lang = i18n.ZhHant
	a.screen = screenLibrary
	a.cfg.LibraryPath = dir
	a.tags = []model.Tag{
		{ID: "t1", Name: "插畫"},
		{ID: "t2", Name: "UI設計"},
	}
	a.assets = []model.Asset{
		{
			ID:          "a1",
			Path:        "Asset 1.png",
			StorageMode: model.StorageIndex,
			Rating:      4,
		},
		{
			ID:          "a2",
			Path:        "Asset 2@4xd.png",
			StorageMode: model.StorageCopy,
			Rating:      5,
		},
	}
	a.assetByID = map[string]model.Asset{
		"a1": a.assets[0],
		"a2": a.assets[1],
	}

	tt := gui.NewTester(a.View, 1280, 800)
	if !tt.HasText("母庫") {
		t.Fatalf("library nav missing, texts: %v", tt.Texts())
	}
	if !tt.HasText("插畫") {
		t.Fatalf("tag name missing, texts: %v", tt.Texts())
	}
	if !tt.HasText("Asset 2@4xd.png") {
		t.Fatalf("asset card name missing, texts: %v", tt.Texts())
	}
	// Verify selecting an asset displays it in inspector
	a.assetIndex = 1
	tt.Frame()
	if !tt.HasText("複製") {
		t.Fatalf("inspector storage mode missing, texts: %v", tt.Texts())
	}

	// Test clicking tag in sidebar
	if err := tt.Click("UI設計"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.tagID != "t2" {
		t.Fatalf("expected tagID to be t2, got %q", a.tagID)
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

	tt := gui.NewTester(a.View, 1280, 800)

	// Toggle top-bar receive-by-code bar.
	if err := tt.Click("輸入配對碼"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("連線接收") {
		t.Fatalf("receive pair bar missing, texts: %v", tt.Texts())
	}

	// Open project share panel and verify pairing section renders.
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
