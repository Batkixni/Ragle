package library

import (
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

	res, err := lib.ImportExternalLibrary(context.Background(), extRoot, model.StorageIndex)
	if err != nil {
		t.Fatalf("ImportExternalLibrary: %v", err)
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

	tags, _ := lib.Tags()
	byName := map[string]model.Tag{}
	for _, tg := range tags {
		byName[tg.Name] = tg
	}
	parent, ok1 := byName["UI Inspiration"]
	child, ok2 := byName["Typography"]
	if !ok1 || !ok2 || child.ParentID != parent.ID {
		t.Fatalf("expected nested tag hierarchy UI Inspiration -> Typography, got %+v", tags)
	}
	if _, ok := byName["Editorial"]; !ok {
		t.Fatalf("expected item tag Editorial created, got %+v", tags)
	}
}
