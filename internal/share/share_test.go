package share

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

func TestTransferCopiesProjectAndDedupsFile(t *testing.T) {
	bodyA := []byte("raglex-share-asset-a")
	bodyB := []byte("raglex-share-asset-b")
	src := openLib(t)
	dst := openLib(t)

	mood, err := src.CreateTag("Mood")
	if err != nil {
		t.Fatal(err)
	}
	extra, err := src.CreateTag("Extra")
	if err != nil {
		t.Fatal(err)
	}
	assetA := putFileAsset(t, src, "a.png", bodyA, 10, 20, 4)
	assetB := putFileAsset(t, src, "b.png", bodyB, 3, 4, 0)
	if err := src.DB.Update(func(tx *bbolt.Tx) error {
		if err := db.LinkAssetTag(tx, assetA.ID, mood.ID); err != nil {
			return err
		}
		return db.LinkAssetTag(tx, assetB.ID, extra.ID)
	}); err != nil {
		t.Fatal(err)
	}
	store := project.New(src.DB)
	proj, err := store.Create("Poster", "night", []string{mood.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(proj.ID, assetA.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(proj.ID, assetB.ID); err != nil {
		t.Fatal(err)
	}
	board, err := store.CreateBoard(proj.ID, "Main")
	if err != nil {
		t.Fatal(err)
	}
	groupID := model.NewID()
	if err := store.SavePlacements(board.ID, []model.Placement{
		{ID: groupID, Kind: model.KindGroup, X: 100, Y: 80, Width: 400, Height: 300, Scale: 1},
		{ID: model.NewID(), Kind: model.KindImage, AssetID: assetA.ID, ParentID: groupID, X: 40, Y: 50, Width: 320, Height: 200, Scale: 1, Rotation: 15, ZIndex: 2, Notes: "n"},
		{ID: model.NewID(), Kind: model.KindText, X: 10, Y: 20, Width: 200, Height: 40, Scale: 1, Text: "hello", FontSize: 18},
		{ID: model.NewID(), Kind: model.KindText, ParentID: groupID, X: 5, Y: 6, Width: 20, Height: 10, Scale: 1, Text: "child"},
	}); err != nil {
		t.Fatal(err)
	}

	moodDst, err := dst.CreateTag("mood")
	if err != nil {
		t.Fatal(err)
	}
	existing := putFileAsset(t, dst, "already.png", bodyA, 9, 8, 1)

	bundle, files, err := Export(src.DB, proj.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 2 {
		t.Fatalf("files in bundle: %d", len(bundle.Files))
	}
	for _, snap := range bundle.Assets {
		if snap.Asset.Path == "a.png" || snap.Asset.Path == "b.png" {
			continue
		}
		t.Fatalf("path on the wire: %s", snap.Asset.Path)
	}

	done := make(chan error, 1)
	var gotID string
	recv := startNode(t, Options{
		Name:      "Receiver",
		Ephemeral: true,
		Quiet:     true,
		Decide:    func(context.Context, string, Bundle) bool { return true },
		Take: func(hash, name string, r io.Reader) (string, error) {
			return SaveFile(dst.DB.FilesDir(), name, r)
		},
		Commit: func(b Bundle, paths map[string]string) (string, error) {
			id, err := Apply(dst.DB, b, paths)
			gotID = id
			done <- err
			return id, err
		},
	})
	send := startNode(t, Options{Name: "Sender", Ephemeral: true, Quiet: true})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := send.Send(ctx, recv.Addr(), bundle, files); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if gotID == "" || gotID == proj.ID {
		t.Fatalf("new project id %q", gotID)
	}

	got, err := project.New(dst.DB).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != gotID || got[0].Name != "Poster" || got[0].Description != "night" {
		t.Fatalf("project %+v", got)
	}
	tags, err := project.New(dst.DB).TagIDs(gotID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != moodDst.ID {
		t.Fatalf("project tags %+v want %s", tags, moodDst.ID)
	}
	allTags, err := dst.Tags()
	if err != nil {
		t.Fatal(err)
	}
	if len(allTags) != 2 {
		t.Fatalf("tags %+v", allTags)
	}

	refs, err := project.New(dst.DB).Assets(gotID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs %d", len(refs))
	}
	var kept, copied model.Asset
	for _, asset := range refs {
		switch asset.Hash {
		case sum(bodyA):
			kept = asset
		case sum(bodyB):
			copied = asset
		}
	}
	if kept.ID != existing.ID || kept.Width != 9 || kept.Rating != 1 {
		t.Fatalf("existing asset was replaced: %+v", kept)
	}
	if copied.ID == "" || copied.Missing || copied.StorageMode != model.StorageCopy {
		t.Fatalf("copied asset %+v", copied)
	}
	if _, err := os.Stat(copied.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst.DB.FilesDir(), "a.png")); !os.IsNotExist(err) {
		t.Fatalf("duplicate file still present: %v", err)
	}
	linked, err := tagsOf(dst, kept.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(linked) != 1 || linked[0] != moodDst.ID {
		t.Fatalf("tags on existing %+v", linked)
	}

	boards, err := project.New(dst.DB).Boards(gotID)
	if err != nil || len(boards) != 1 || boards[0].Name != "Main" {
		t.Fatalf("boards %+v %v", boards, err)
	}
	items, err := project.New(dst.DB).LoadPlacements(boards[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("placements %+v", items)
	}
	groups := map[string]model.Placement{}
	for _, item := range items {
		switch item.Kind {
		case model.KindImage:
			if item.AssetID != existing.ID || item.X != 40 || item.Rotation != 15 || item.Notes != "n" || item.ParentID == "" {
				t.Fatalf("image %+v", item)
			}
		case model.KindText:
			if item.Text != "hello" && item.Text != "child" {
				t.Fatalf("text %+v", item)
			}
			if item.Text == "hello" && (item.FontSize != 18 || item.ParentID != "") {
				t.Fatalf("hello %+v", item)
			}
			if item.Text == "child" && item.ParentID == "" {
				t.Fatalf("child %+v", item)
			}
		case model.KindGroup:
			groups[item.ID] = item
		}
	}
	if len(groups) != 1 {
		t.Fatalf("groups %+v", items)
	}
	for _, item := range items {
		if item.Text == "child" {
			if _, ok := groups[item.ParentID]; !ok {
				t.Fatalf("child parent %+v", item)
			}
		}
		if item.Kind == model.KindImage {
			if _, ok := groups[item.ParentID]; !ok {
				t.Fatalf("image parent %+v", item)
			}
		}
	}
}

func TestTransferWithoutFilesMarksMissing(t *testing.T) {
	src := openLib(t)
	dst := openLib(t)
	tag, err := src.CreateTag("Mood")
	if err != nil {
		t.Fatal(err)
	}
	asset := putFileAsset(t, src, "a.png", []byte("solo"), 2, 2, 0)
	store := project.New(src.DB)
	proj, err := store.Create("Loose", "", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(proj.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	board, err := store.CreateBoard(proj.ID, "One")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlacements(board.ID, []model.Placement{{
		ID: model.NewID(), Kind: model.KindImage, AssetID: asset.ID, X: 1, Y: 2, Width: 8, Height: 9, Scale: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := src.DB.Update(func(tx *bbolt.Tx) error {
		return db.UnlinkProjectTag(tx, proj.ID, tag.ID)
	}); err != nil {
		t.Fatal(err)
	}
	bundle, files, err := Export(src.DB, proj.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 0 || len(files) != 0 || len(bundle.Tags) != 0 {
		t.Fatalf("bundle files %d tags %d", len(bundle.Files), len(bundle.Tags))
	}
	takes := 0
	done := make(chan error, 1)
	recv := startNode(t, Options{
		Name:      "Receiver",
		Ephemeral: true,
		Quiet:     true,
		Decide:    func(context.Context, string, Bundle) bool { return true },
		Take: func(string, string, io.Reader) (string, error) {
			takes++
			return "", errors.New("unexpected file")
		},
		Commit: func(b Bundle, paths map[string]string) (string, error) {
			id, err := Apply(dst.DB, b, paths)
			done <- err
			return id, err
		},
	})
	send := startNode(t, Options{Name: "Sender", Ephemeral: true, Quiet: true})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := send.Send(ctx, recv.Addr(), bundle, files); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if takes != 0 {
		t.Fatalf("take calls %d", takes)
	}
	got, err := project.New(dst.DB).List()
	if err != nil || len(got) != 1 {
		t.Fatalf("projects %+v %v", got, err)
	}
	tags, err := dst.Tags()
	if err != nil || len(tags) != 1 || tags[0].Name != sharedTagName {
		t.Fatalf("tags %+v %v", tags, err)
	}
	linked, err := project.New(dst.DB).TagIDs(got[0].ID)
	if err != nil || len(linked) != 1 || linked[0] != tags[0].ID {
		t.Fatalf("project tags %+v %v", linked, err)
	}
	assets, err := project.New(dst.DB).Assets(got[0].ID)
	if err != nil || len(assets) != 1 {
		t.Fatalf("assets %+v %v", assets, err)
	}
	if !assets[0].Missing || assets[0].StorageMode != model.StorageIndex || assets[0].Path != "a.png" {
		t.Fatalf("asset %+v", assets[0])
	}
	entries, err := os.ReadDir(dst.DB.FilesDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("files landed: %d", len(entries))
	}
}

func TestDeclineDoesNotWrite(t *testing.T) {
	src := openLib(t)
	dst := openLib(t)
	tag, err := src.CreateTag("Mood")
	if err != nil {
		t.Fatal(err)
	}
	asset := putFileAsset(t, src, "a.png", []byte("nope"), 1, 1, 0)
	store := project.New(src.DB)
	proj, err := store.Create("Nope", "", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(proj.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	bundle, files, err := Export(src.DB, proj.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 1 {
		t.Fatalf("expected a file offer, got %d", len(bundle.Files))
	}
	takes := 0
	commits := 0
	recv := startNode(t, Options{
		Name:      "Receiver",
		Ephemeral: true,
		Quiet:     true,
		Decide:    func(context.Context, string, Bundle) bool { return false },
		Take: func(string, string, io.Reader) (string, error) {
			takes++
			return "", errors.New("file after decline")
		},
		Commit: func(Bundle, map[string]string) (string, error) {
			commits++
			return "", errors.New("commit after decline")
		},
	})
	send := startNode(t, Options{Name: "Sender", Ephemeral: true, Quiet: true})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = send.Send(ctx, recv.Addr(), bundle, files)
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("send err %v", err)
	}
	if takes != 0 || commits != 0 {
		t.Fatalf("take %d commit %d", takes, commits)
	}
	got, err := project.New(dst.DB).List()
	if err != nil || len(got) != 0 {
		t.Fatalf("projects %+v %v", got, err)
	}
	var assets []model.Asset
	if err := dst.DB.View(func(tx *bbolt.Tx) error {
		assets, err = db.ListAssets(tx)
		return err
	}); err != nil || len(assets) != 0 {
		t.Fatalf("assets %+v %v", assets, err)
	}
}

func TestBeaconRegistersPeer(t *testing.T) {
	n := startNode(t, Options{Name: "Me", Ephemeral: true, Quiet: true})
	if n.UDPPort() == 0 {
		t.Fatal("udp port")
	}
	sendBeacon(t, n.UDPPort(), beacon{Magic: magic, ID: n.ID(), Name: "Me", Port: 1})
	time.Sleep(150 * time.Millisecond)
	if peers := n.Peers(); len(peers) != 0 {
		t.Fatalf("own beacon registered: %+v", peers)
	}
	deadline := time.Now().Add(2 * time.Second)
	var found []Peer
	for time.Now().Before(deadline) {
		sendBeacon(t, n.UDPPort(), beacon{Magic: magic, ID: "other-peer", Name: "Other", Port: 9})
		found = n.Peers()
		if len(found) == 1 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if len(found) != 1 || found[0].Name != "Other" || found[0].ID != "other-peer" || found[0].Addr != "127.0.0.1:9" {
		t.Fatalf("peers %+v", found)
	}
}

func startNode(t *testing.T, opt Options) *Node {
	t.Helper()
	n, err := Start(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n
}

func openLib(t *testing.T) *library.Library {
	t.Helper()
	lib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lib.Close() })
	return lib
}

func putFileAsset(t *testing.T, lib *library.Library, name string, body []byte, width, height, rating int) model.Asset {
	t.Helper()
	path := filepath.Join(lib.DB.FilesDir(), name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	asset := model.Asset{
		ID:          model.NewID(),
		Path:        path,
		StorageMode: model.StorageCopy,
		Hash:        sum(body),
		Mime:        "image/png",
		Width:       width,
		Height:      height,
		Rating:      rating,
		CreatedAt:   time.Now(),
	}
	if err := lib.DB.Update(func(tx *bbolt.Tx) error {
		return db.PutAsset(tx, asset)
	}); err != nil {
		t.Fatal(err)
	}
	return asset
}

func tagsOf(lib *library.Library, assetID string) ([]string, error) {
	var ids []string
	err := lib.DB.View(func(tx *bbolt.Tx) error {
		var err error
		ids, err = db.TagsOfAsset(tx, assetID)
		return err
	})
	return ids, err
}

func sum(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

func sendBeacon(t *testing.T, port int, msg beacon) {
	t.Helper()
	raw, err := msgpack.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(raw); err != nil {
		t.Fatal(err)
	}
}
