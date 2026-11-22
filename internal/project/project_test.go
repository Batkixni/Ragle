package project

import (
	"testing"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
)

func TestUnreferenceDropsPlacementsAndDeleteKeepsAsset(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	store := New(d)
	tag := model.Tag{ID: model.NewID(), Name: "t", CreatedAt: time.Now()}
	asset := model.Asset{ID: model.NewID(), Path: "/a.png", Hash: "h", StorageMode: model.StorageIndex, CreatedAt: time.Now()}
	if err := d.Update(func(tx *bbolt.Tx) error {
		if err := db.PutTag(tx, tag); err != nil {
			return err
		}
		return db.PutAsset(tx, asset)
	}); err != nil {
		t.Fatal(err)
	}
	p, err := store.Create("Poster", "", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("No tag", "", nil); err != ErrNeedTag {
		t.Fatalf("need tag err %v", err)
	}
	other, err := store.Create("Other", "", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(p.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(other.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateBoard(p.ID, "Main")
	if err != nil {
		t.Fatal(err)
	}
	ob, err := store.CreateBoard(other.ID, "Main")
	if err != nil {
		t.Fatal(err)
	}
	err = store.SavePlacements(b.ID, []model.Placement{{
		ID: model.NewID(), Kind: model.KindImage, AssetID: asset.ID, X: 1, Y: 2, Scale: 1, Width: 10, Height: 10,
	}})
	if err != nil {
		t.Fatal(err)
	}
	keptID := model.NewID()
	err = store.SavePlacements(ob.ID, []model.Placement{{
		ID: keptID, Kind: model.KindImage, AssetID: asset.ID, X: 3, Y: 4, Scale: 1, Width: 10, Height: 10,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Unreference(p.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	left, err := store.LoadPlacements(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("placements left on the project: %+v", left)
	}
	still, err := store.LoadPlacements(ob.ID)
	if err != nil || len(still) != 1 || still[0].ID != keptID {
		t.Fatalf("other board %+v %v", still, err)
	}
	if err := store.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	err = d.View(func(tx *bbolt.Tx) error {
		if _, err := db.GetAsset(tx, asset.ID); err != nil {
			return err
		}
		if _, err := db.GetProject(tx, other.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStandaloneBoardsAndMoveToProject(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	store := New(d)
	var tagID string
	var assetID string
	err = d.Update(func(tx *bbolt.Tx) error {
		tg := model.Tag{ID: model.NewID(), Name: "Visual"}
		tagID = tg.ID
		if err := db.PutTag(tx, tg); err != nil {
			return err
		}
		a := model.Asset{ID: model.NewID(), Path: "/tmp/ref.png", Hash: "h1", Width: 600, Height: 400}
		assetID = a.ID
		return db.PutAsset(tx, a)
	})
	if err != nil {
		t.Fatal(err)
	}

	sb, err := store.CreateStandaloneBoard("Quick Mood")
	if err != nil {
		t.Fatal(err)
	}
	if sb.ProjectID != "" {
		t.Fatalf("expected empty ProjectID, got %q", sb.ProjectID)
	}
	list, err := store.StandaloneBoards()
	if err != nil || len(list) != 1 || list[0].ID != sb.ID {
		t.Fatalf("StandaloneBoards got %+v err=%v", list, err)
	}

	added, err := store.AddAssetsToBoard(sb.ID, []string{assetID})
	if err != nil || added != 1 {
		t.Fatalf("AddAssetsToBoard got added=%d err=%v", added, err)
	}

	proj, err := store.Create("Brand 2026", "", []string{tagID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MoveBoardToProject(sb.ID, proj.ID); err != nil {
		t.Fatal(err)
	}
	listAfter, _ := store.StandaloneBoards()
	if len(listAfter) != 0 {
		t.Fatalf("expected 0 standalone boards after move, got %d", len(listAfter))
	}
	projBoards, _ := store.Boards(proj.ID)
	if len(projBoards) != 1 || projBoards[0].ID != sb.ID {
		t.Fatalf("expected board inside project, got %+v", projBoards)
	}
	projAssets, _ := store.Assets(proj.ID)
	if len(projAssets) != 1 || projAssets[0].ID != assetID {
		t.Fatalf("expected placed asset referenced in project, got %+v", projAssets)
	}
}
