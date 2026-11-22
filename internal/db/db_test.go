package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/model"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestAssetHashAndPath(t *testing.T) {
	d := openTest(t)
	a := model.Asset{
		ID: model.NewID(), Path: `C:\photos\a.png`, StorageMode: model.StorageIndex,
		Hash: "abc", Mime: "image/png", Width: 10, Height: 20, Rating: 3,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := d.Update(func(tx *bbolt.Tx) error { return PutAsset(tx, a) }); err != nil {
		t.Fatal(err)
	}
	err := d.View(func(tx *bbolt.Tx) error {
		got, err := GetAsset(tx, a.ID)
		if err != nil {
			return err
		}
		if got.Path != a.Path || got.Rating != 3 || got.Width != 10 {
			t.Fatalf("got %+v", got)
		}
		id, ok := AssetIDByHash(tx, "abc")
		if !ok || id != a.ID {
			t.Fatalf("hash index %q %v", id, ok)
		}
		id, ok = AssetIDByPath(tx, a.Path)
		if !ok || id != a.ID {
			t.Fatalf("path index %q %v", id, ok)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	a.Missing = true
	if err := d.Update(func(tx *bbolt.Tx) error { return PutAsset(tx, a) }); err != nil {
		t.Fatal(err)
	}
	_ = d.View(func(tx *bbolt.Tx) error {
		if _, ok := AssetIDByPath(tx, a.Path); ok {
			t.Fatal("missing asset kept its path index")
		}
		got, err := GetAsset(tx, a.ID)
		if err != nil {
			return err
		}
		if !got.Missing || got.Path != a.Path {
			t.Fatalf("path should stay visible, got %+v", got)
		}
		return nil
	})
}

func TestTagDeleteKeepsProjectAndAsset(t *testing.T) {
	d := openTest(t)
	tag := model.Tag{ID: model.NewID(), Name: "refs", CreatedAt: time.Now()}
	asset := model.Asset{ID: model.NewID(), Path: "/a", Hash: "h1", StorageMode: model.StorageIndex, CreatedAt: time.Now()}
	project := model.Project{ID: model.NewID(), Name: "p", CreatedAt: time.Now()}
	err := d.Update(func(tx *bbolt.Tx) error {
		if err := PutTag(tx, tag); err != nil {
			return err
		}
		if err := PutAsset(tx, asset); err != nil {
			return err
		}
		if err := PutProject(tx, project); err != nil {
			return err
		}
		if err := LinkAssetTag(tx, asset.ID, tag.ID); err != nil {
			return err
		}
		return LinkProjectTag(tx, project.ID, tag.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Update(func(tx *bbolt.Tx) error { return DeleteTag(tx, tag.ID) }); err != nil {
		t.Fatal(err)
	}
	err = d.View(func(tx *bbolt.Tx) error {
		if _, err := GetTag(tx, tag.ID); err != ErrNotFound {
			t.Fatalf("tag err %v", err)
		}
		if _, err := GetAsset(tx, asset.ID); err != nil {
			return err
		}
		if _, err := GetProject(tx, project.ID); err != nil {
			return err
		}
		tags, err := TagsOfAsset(tx, asset.ID)
		if err != nil {
			return err
		}
		if len(tags) != 0 {
			t.Fatalf("asset tags %v", tags)
		}
		pt, err := TagsOfProject(tx, project.ID)
		if err != nil {
			return err
		}
		if len(pt) != 0 {
			t.Fatalf("project tags %v", pt)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectDeleteKeepsAssetAndDropsPlacements(t *testing.T) {
	d := openTest(t)
	asset := model.Asset{ID: model.NewID(), Path: "/a", Hash: "h", StorageMode: model.StorageCopy, CreatedAt: time.Now()}
	other := model.Project{ID: model.NewID(), Name: "other", CreatedAt: time.Now()}
	project := model.Project{ID: model.NewID(), Name: "p", CreatedAt: time.Now()}
	board := model.Board{ID: model.NewID(), ProjectID: project.ID, Name: "b", CreatedAt: time.Now()}
	otherBoard := model.Board{ID: model.NewID(), ProjectID: other.ID, Name: "ob", CreatedAt: time.Now()}
	place := model.Placement{ID: model.NewID(), BoardID: board.ID, Kind: model.KindImage, AssetID: asset.ID, X: 1, Y: 2, Scale: 1}
	kept := model.Placement{ID: model.NewID(), BoardID: otherBoard.ID, Kind: model.KindImage, AssetID: asset.ID, X: 3, Y: 4, Scale: 1}
	err := d.Update(func(tx *bbolt.Tx) error {
		for _, step := range []error{
			PutAsset(tx, asset),
			PutProject(tx, project),
			PutProject(tx, other),
			PutBoard(tx, board),
			PutBoard(tx, otherBoard),
			PutPlacement(tx, place),
			PutPlacement(tx, kept),
			LinkProjectAsset(tx, project.ID, asset.ID),
			LinkProjectAsset(tx, other.ID, asset.ID),
		} {
			if step != nil {
				return step
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Update(func(tx *bbolt.Tx) error { return DeleteProject(tx, project.ID) }); err != nil {
		t.Fatal(err)
	}
	err = d.View(func(tx *bbolt.Tx) error {
		if _, err := GetAsset(tx, asset.ID); err != nil {
			return err
		}
		if _, err := GetProject(tx, project.ID); err != ErrNotFound {
			t.Fatalf("project %v", err)
		}
		if _, err := GetPlacement(tx, place.ID); err != ErrNotFound {
			t.Fatalf("placement %v", err)
		}
		if _, err := GetPlacement(tx, kept.ID); err != nil {
			return err
		}
		ids, err := AssetsOfProject(tx, other.ID)
		if err != nil {
			return err
		}
		if len(ids) != 1 || ids[0] != asset.ID {
			t.Fatalf("other project assets %v", ids)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemoveReferenceDropsOnlyThatProjectsPlacements(t *testing.T) {
	d := openTest(t)
	asset := model.Asset{ID: model.NewID(), Path: "/a", Hash: "h", StorageMode: model.StorageIndex, CreatedAt: time.Now()}
	p1 := model.Project{ID: model.NewID(), Name: "one", CreatedAt: time.Now()}
	p2 := model.Project{ID: model.NewID(), Name: "two", CreatedAt: time.Now()}
	b1 := model.Board{ID: model.NewID(), ProjectID: p1.ID, Name: "b", CreatedAt: time.Now()}
	b2 := model.Board{ID: model.NewID(), ProjectID: p2.ID, Name: "b", CreatedAt: time.Now()}
	pl1 := model.Placement{ID: model.NewID(), BoardID: b1.ID, Kind: model.KindImage, AssetID: asset.ID, Scale: 1}
	pl2 := model.Placement{ID: model.NewID(), BoardID: b2.ID, Kind: model.KindImage, AssetID: asset.ID, Scale: 1}
	err := d.Update(func(tx *bbolt.Tx) error {
		steps := []error{
			PutAsset(tx, asset), PutProject(tx, p1), PutProject(tx, p2),
			PutBoard(tx, b1), PutBoard(tx, b2), PutPlacement(tx, pl1), PutPlacement(tx, pl2),
			LinkProjectAsset(tx, p1.ID, asset.ID), LinkProjectAsset(tx, p2.ID, asset.ID),
		}
		for _, step := range steps {
			if step != nil {
				return step
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = d.Update(func(tx *bbolt.Tx) error {
		if err := UnlinkProjectAsset(tx, p1.ID, asset.ID); err != nil {
			return err
		}
		ids, err := PlacementsOfBoard(tx, b1.ID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			p, err := GetPlacement(tx, id)
			if err != nil {
				return err
			}
			if p.AssetID == asset.ID {
				if err := DeletePlacement(tx, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = d.View(func(tx *bbolt.Tx) error {
		if _, err := GetPlacement(tx, pl1.ID); err != ErrNotFound {
			t.Fatalf("p1 placement %v", err)
		}
		if _, err := GetPlacement(tx, pl2.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := GetAsset(tx, asset.ID); err != nil {
			t.Fatal(err)
		}
		return nil
	})
}

func TestGroupDeleteRemovesDescendants(t *testing.T) {
	d := openTest(t)
	boardID := model.NewID()
	projectID := model.NewID()
	group := model.Placement{ID: model.NewID(), BoardID: boardID, Kind: model.KindGroup, Width: 100, Height: 80, Scale: 1}
	child := model.Placement{ID: model.NewID(), BoardID: boardID, Kind: model.KindText, ParentID: group.ID, Text: "hi", Scale: 1}
	grand := model.Placement{ID: model.NewID(), BoardID: boardID, Kind: model.KindImage, ParentID: child.ID, AssetID: "asset", Scale: 1}
	outside := model.Placement{ID: model.NewID(), BoardID: boardID, Kind: model.KindText, Text: "stay", Scale: 1}
	err := d.Update(func(tx *bbolt.Tx) error {
		if err := PutProject(tx, model.Project{ID: projectID, Name: "p", CreatedAt: time.Now()}); err != nil {
			return err
		}
		if err := PutBoard(tx, model.Board{ID: boardID, ProjectID: projectID, Name: "b", CreatedAt: time.Now()}); err != nil {
			return err
		}
		for _, p := range []model.Placement{group, child, grand, outside} {
			if err := PutPlacement(tx, p); err != nil {
				return err
			}
		}
		return DeletePlacementTree(tx, group.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = d.View(func(tx *bbolt.Tx) error {
		for _, id := range []string{group.ID, child.ID, grand.ID} {
			if _, err := GetPlacement(tx, id); err != ErrNotFound {
				t.Fatalf("%s still present: %v", id, err)
			}
		}
		if _, err := GetPlacement(tx, outside.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBackupAndRestore(t *testing.T) {
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	asset := model.Asset{ID: model.NewID(), Path: "/kept", Hash: "hash", StorageMode: model.StorageIndex, CreatedAt: time.Now()}
	if err := d.Update(func(tx *bbolt.Tx) error { return PutAsset(tx, asset) }); err != nil {
		t.Fatal(err)
	}
	if err := d.Backup(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "raglex.db"), []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err == nil {
		t.Fatal("opened a corrupt database")
	}
	if err := Restore(root); err != nil {
		t.Fatal(err)
	}
	d, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	err = d.View(func(tx *bbolt.Tx) error {
		got, err := GetAsset(tx, asset.ID)
		if err != nil {
			return err
		}
		if got.Hash != "hash" {
			t.Fatalf("hash %s", got.Hash)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
