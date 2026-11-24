package library

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"raglex/internal/model"
)

func TestImportExternalLibraryPreservesHierarchyAndMetadata(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	extRoot := filepath.Join(t.TempDir(), "Sample.library")
	infoDir := filepath.Join(extRoot, "images", "ITEM01.info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rootMeta := `{
		"folders": [
			{
				"id": "F1",
				"name": "UI Inspiration",
				"iconColor": "blue",
				"children": [
					{"id": "F2", "name": "Typography", "iconColor": "orange"}
				]
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(extRoot, "metadata.json"), []byte(rootMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	writePNG(t, filepath.Join(infoDir, "Poster Ref.png"), 77)
	itemMeta := `{
		"id": "ITEM01",
		"name": "Poster Ref",
		"ext": "png",
		"tags": ["Editorial", "Grid"],
		"folders": ["F2"],
		"url": "https://example.com/poster",
		"annotation": "Great kerning",
		"star": 5,
		"isDeleted": false
	}`
	if err := os.WriteFile(filepath.Join(infoDir, "metadata.json"), []byte(itemMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := lib.ImportExternalPackage(context.Background(), extRoot, model.StorageIndex)
	if err != nil {
		t.Fatalf("ImportExternalPackage: %v", err)
	}
	if res.AssetsImported != 1 || res.FoldersMapped != 2 {
		t.Fatalf("unexpected migration result: %+v", res)
	}

	assets, err := lib.List(Filter{})
	if err != nil || len(assets) != 1 {
		t.Fatalf("expected 1 migrated asset, got %d err=%v", len(assets), err)
	}
	a := assets[0]
	if a.Rating != 5 || a.Notes != "Great kerning" || a.SourceURL != "https://example.com/poster" {
		t.Fatalf("expected metadata preserved, got %+v", a)
	}

	// Verify nested Folders hierarchy (UI Inspiration -> Typography).
	folders, _ := lib.Folders()
	fByName := map[string]model.Folder{}
	for _, f := range folders {
		fByName[f.Name] = f
	}
	parent, ok1 := fByName["UI Inspiration"]
	child, ok2 := fByName["Typography"]
	if !ok1 || !ok2 || child.ParentID != parent.ID {
		t.Fatalf("expected nested folder hierarchy UI Inspiration -> Typography, got %+v", folders)
	}
	af, _ := lib.AssetFolders(a.ID)
	if len(af) != 1 || af[0].ID != child.ID {
		t.Fatalf("expected asset linked to Typography folder, got %+v", af)
	}

	// Verify item Tags preserved independently.
	tags, _ := lib.Tags()
	tByName := map[string]model.Tag{}
	for _, tg := range tags {
		tByName[tg.Name] = tg
	}
	if _, ok := tByName["Editorial"]; !ok {
		t.Fatalf("expected item tag Editorial created, got %+v", tags)
	}
}

func TestImportEaglePackArchiveAndNestedFolders(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	// Create a synthetic .eaglepack ZIP archive with pack.json + images/PK01.info/.
	srcPNG := filepath.Join(t.TempDir(), "hero.png")
	writePNG(t, srcPNG, 42)
	pngBytes, _ := os.ReadFile(srcPNG)

	packPath := filepath.Join(t.TempDir(), "DesignKit.eaglepack")
	zf, err := os.Create(packPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	writeEntry := func(name string, data []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(data)
	}
	writeEntry("pack.json", []byte(`{
		"folders": [
			{"id": "PF1", "name": "3D Pack", "iconColor": "purple", "children": [
				{"id": "PF2", "name": "Materials", "iconColor": "aqua"}
			]}
		]
	}`))
	writeEntry("images/PK01.info/metadata.json", []byte(`{
		"id": "PK01",
		"name": "Chrome Shader",
		"ext": "png",
		"tags": ["Metallic"],
		"folders": ["PF2"],
		"annotation": "PBR ref",
		"star": 4
	}`))
	writeEntry("images/PK01.info/Chrome Shader.png", pngBytes)
	_ = zw.Close()
	_ = zf.Close()

	res, err := lib.ImportExternalPackage(context.Background(), packPath, model.StorageIndex)
	if err != nil {
		t.Fatalf("ImportExternalPackage(.eaglepack) failed: %v", err)
	}
	if res.AssetsImported != 1 || res.FoldersMapped != 2 {
		t.Fatalf("unexpected eaglepack result: %+v", res)
	}

	assets, _ := lib.List(Filter{})
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset from eaglepack, got %d", len(assets))
	}
	// Extracted .eaglepack assets MUST be copied into library files/ so they persist!
	if _, err := os.Stat(assets[0].Path); err != nil {
		t.Fatalf("extracted eaglepack asset file missing: %v", err)
	}
	af, _ := lib.AssetFolders(assets[0].ID)
	if len(af) != 1 || af[0].Name != "Materials" {
		t.Fatalf("expected asset in Materials folder, got %+v", af)
	}
	at, _ := lib.AssetTags(assets[0].ID)
	if len(at) != 1 || at[0].Name != "Metallic" {
		t.Fatalf("expected asset tagged Metallic, got %+v", at)
	}
}

func TestImportDirectoryMirrorsSubfoldersAsFolders(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	rootImport := filepath.Join(t.TempDir(), "PhotoShoot")
	selDir := filepath.Join(rootImport, "Selects")
	rawDir := filepath.Join(rootImport, "RAW")
	_ = os.MkdirAll(selDir, 0o755)
	_ = os.MkdirAll(rawDir, 0o755)
	writePNG(t, filepath.Join(selDir, "a.png"), 10)
	writePNG(t, filepath.Join(rawDir, "b.png"), 20)

	res, err := lib.Import(context.Background(), []string{rootImport}, model.StorageIndex, nil, nil)
	if err != nil || res.Added != 2 {
		t.Fatalf("import dir failed: res=%+v err=%v", res, err)
	}

	folders, _ := lib.Folders()
	byName := map[string]model.Folder{}
	for _, f := range folders {
		byName[f.Name] = f
	}
	pRoot, ok0 := byName["PhotoShoot"]
	pSel, ok1 := byName["Selects"]
	pRaw, ok2 := byName["RAW"]
	if !ok0 || !ok1 || !ok2 || pSel.ParentID != pRoot.ID || pRaw.ParentID != pRoot.ID {
		t.Fatalf("expected PhotoShoot -> {Selects, RAW} folders, got %+v", folders)
	}
}

func TestImportRealEaglePackSingularFolderAndURLBookmarks(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	packPath := filepath.Join(t.TempDir(), "MV_Collection.eaglepack")
	zf, err := os.Create(packPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)

	// Real .eaglepack pack.json uses SINGULAR "folder": {...} object!
	packJSON := `{
		"folder": {
			"id": "F_ROOT",
			"name": "MV分類",
			"children": [
				{"id": "F_CUTE", "name": "可愛", "iconColor": "pink", "children": []},
				{"id": "F_PV", "name": "PV集中", "iconColor": "aqua", "children": []}
			]
		},
		"images": []
	}`
	w, _ := zw.Create("pack.json")
	_, _ = w.Write([]byte(packJSON))

	// Item 1: YouTube URL bookmark with 72-byte .url file + real *_thumbnail.png cover art.
	m1, _ := zw.Create("ITEM1.info/metadata.json")
	_, _ = m1.Write([]byte(`{"id":"ITEM1","name":"Teaser PV - YouTube","ext":"url","tags":["MV"],"folders":["F_CUTE"],"url":"https://www.youtube.com/watch?v=abc123","star":5}`))
	u1, _ := zw.Create("ITEM1.info/Teaser PV - YouTube.url")
	_, _ = u1.Write([]byte("[InternetShortcut]\nURL=https://www.youtube.com/watch?v=abc123\n"))
	tmpThumb := filepath.Join(t.TempDir(), "cover.png")
	writePNG(t, tmpThumb, 128)
	thumbBytes, _ := os.ReadFile(tmpThumb)
	t1, _ := zw.Create("ITEM1.info/Teaser PV - YouTube_thumbnail.png")
	_, _ = t1.Write(thumbBytes)

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = zf.Close()

	var progressCalls int
	res, err := lib.ImportExternalPackageWithProgress(context.Background(), packPath, model.StorageCopy, func(p ImportProgress) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("ImportExternalPackageWithProgress failed: %v", err)
	}
	if res.AssetsImported != 1 || res.FoldersMapped != 3 || progressCalls == 0 {
		t.Fatalf("unexpected result: res=%+v progressCalls=%d", res, progressCalls)
	}

	snap, err := lib.LoadCatalogSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Folders) != 3 {
		t.Fatalf("expected 3 folders (MV分類, 可愛, PV集中), got %+v", snap.Folders)
	}
	if len(snap.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(snap.Assets))
	}
	asset := snap.Assets[0]
	if !asset.IsBookmark {
		t.Fatalf("expected URL item imported as IsBookmark=true, got %+v", asset)
	}
	if asset.SourceURL != "https://www.youtube.com/watch?v=abc123" || asset.DisplayName != "Teaser PV - YouTube" {
		t.Fatalf("unexpected bookmark metadata: url=%q name=%q", asset.SourceURL, asset.DisplayName)
	}
	if model.MediaKind(asset.Mime) != model.MediaImage {
		t.Fatalf("expected bookmark cover image mime (image/*), got %q (path=%q)", asset.Mime, asset.Path)
	}
	af := snap.AssetFolders[asset.ID]
	if len(af) != 1 || af[0].Name != "可愛" {
		t.Fatalf("expected asset linked to folder 可愛, got %+v", af)
	}

	// Verify DeleteFoldersBatch with includeSubfolders=true deletes parent + all children atomically.
	var rootFID string
	for _, f := range snap.Folders {
		if f.Name == "MV分類" {
			rootFID = f.ID
			break
		}
	}
	n, err := lib.DeleteFoldersBatch([]string{rootFID}, true)
	if err != nil || n != 3 {
		t.Fatalf("expected DeleteFoldersBatch recursive to delete 3 folders, got n=%d err=%v", n, err)
	}
	rem, _ := lib.Folders()
	if len(rem) != 0 {
		t.Fatalf("expected 0 folders remaining after recursive delete, got %+v", rem)
	}
}
