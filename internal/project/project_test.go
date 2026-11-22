package project

import (
	"testing"
	"time"

	"go.etcd.io/bbolt"

	"ragle/internal/db"
	"ragle/internal/model"
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
