package share

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"go.etcd.io/bbolt"

	"raglex/internal/db"
	"raglex/internal/model"
	"raglex/internal/project"
)

func newTestSignaling(t *testing.T, ttl time.Duration) (*SignalingServer, *httptest.Server) {
	t.Helper()
	sig := NewSignalingServer(ttl)
	sig.pollTimeout = 200 * time.Millisecond
	srv := httptest.NewServer(sig)
	t.Cleanup(func() {
		srv.Close()
		sig.Close()
	})
	return sig, srv
}

func TestSignalingLifecycle(t *testing.T) {
	_, srv := newTestSignaling(t, time.Minute)
	client := NewSignalingClient(srv.URL)
	ctx := context.Background()

	code, token, err := client.CreateRoom(ctx, "offer-sdp", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(NormalizeCode(code)) != 6 || !strings.Contains(code, "-") {
		t.Fatalf("unexpected code format: %q", code)
	}

	// Receiver fetches offer using lowercase unformatted code.
	offer, name, err := client.GetOffer(ctx, strings.ToLower(NormalizeCode(code)))
	if err != nil {
		t.Fatal(err)
	}
	if offer != "offer-sdp" || name != "Alice" {
		t.Fatalf("got offer=%q name=%q", offer, name)
	}

	ansCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		ans, waitErr := client.WaitAnswer(ctx, code, token)
		if waitErr != nil {
			errCh <- waitErr
			return
		}
		ansCh <- ans
	}()

	time.Sleep(50 * time.Millisecond)
	if err := client.SubmitAnswer(ctx, code, "answer-sdp"); err != nil {
		t.Fatal(err)
	}

	// Second claim must fail with ErrRoomClaimed (or ErrRoomNotFound once popped).
	if err := client.SubmitAnswer(ctx, code, "answer-2"); !errors.Is(err, ErrRoomClaimed) && !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected claimed/not found, got %v", err)
	}

	select {
	case ans := <-ansCh:
		if ans != "answer-sdp" {
			t.Fatalf("answer=%q", ans)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for answer")
	}
}

func TestPairTransferWithFilesAndDedup(t *testing.T) {
	_, srv := newTestSignaling(t, time.Minute)
	src := openLib(t)
	dst := openLib(t)

	// Multi-chunk file (> 32 KiB) to verify chunked offset transfer + SHA-256 verification.
	bigBody := bytes.Repeat([]byte("RAGLEX-E2E-CHUNK-DATA-0123456789!"), 3500) // ~112 KiB
	dupBody := []byte("already-in-receiver-library")

	tag, err := src.CreateTag("Concept")
	if err != nil {
		t.Fatal(err)
	}
	assetBig := putFileAsset(t, src, "big.bin", bigBody, 100, 100, 5)
	assetDup := putFileAsset(t, src, "dup.png", dupBody, 20, 20, 3)

	// Receiver already has dupBody under a different name.
	existing := putFileAsset(t, dst, "local-dup.png", dupBody, 20, 20, 1)

	store := project.New(src.DB)
	proj, err := store.Create("RemoteBoard", "via pairing code", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(proj.ID, assetBig.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Reference(proj.ID, assetDup.ID); err != nil {
		t.Fatal(err)
	}
	board, err := store.CreateBoard(proj.ID, "Canvas 1")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlacements(board.ID, []model.Placement{
		{ID: model.NewID(), Kind: model.KindImage, AssetID: assetBig.ID, X: 10, Y: 20, Width: 300, Height: 200, Scale: 1},
		{ID: model.NewID(), Kind: model.KindImage, AssetID: assetDup.ID, X: 50, Y: 60, Width: 100, Height: 100, Scale: 1},
	}); err != nil {
		t.Fatal(err)
	}

	bundle, files, err := Export(src.DB, proj.ID, true)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	opt := PairOptions{
		SignalingURL: srv.URL,
		Name:         "SenderBox",
		ICEServers:   []webrtc.ICEServer{}, // pure local ICE, no external STUN needed
	}

	codeCh := make(chan string, 1)
	sendErrCh := make(chan error, 1)
	go func() {
		sendErrCh <- PairSend(ctx, opt, bundle, files, func(code string) {
			codeCh <- code
		})
	}()

	var code string
	select {
	case code = <-codeCh:
	case err := <-sendErrCh:
		t.Fatalf("send failed before code: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for pairing code")
	}

	var sawFrom string
	projID, recvErr := PairReceive(ctx, opt, code,
		func(_ context.Context, from string, b Bundle) bool {
			sawFrom = from
			return b.Project.Name == "RemoteBoard"
		},
		func(_, name string, r io.Reader) (string, error) {
			return SaveFile(dst.DB.FilesDir(), name, r)
		},
		func(b Bundle, paths map[string]string) (string, error) {
			return Apply(dst.DB, b, paths)
		},
	)
	if recvErr != nil {
		t.Fatalf("receive failed: %v", recvErr)
	}
	if err := <-sendErrCh; err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if sawFrom != "SenderBox" {
		t.Fatalf("expected from=SenderBox, got %q", sawFrom)
	}
	if projID == "" || projID == proj.ID {
		t.Fatalf("expected new project ID, got %q", projID)
	}

	// Verify deduplication and exact file contents on receiver.
	dstStore := project.New(dst.DB)
	refs, err := dstStore.Assets(projID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 referenced assets, got %d", len(refs))
	}
	var gotDup, gotBig bool
	for _, a := range refs {
		switch a.Hash {
		case existing.Hash:
			gotDup = true
			if a.ID != existing.ID {
				t.Fatalf("expected dedup to reuse existing asset ID %s, got %s", existing.ID, a.ID)
			}
		case assetBig.Hash:
			gotBig = true
			data, readErr := os.ReadFile(a.Path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(data, bigBody) {
				t.Fatalf("big file content mismatch: got %d bytes", len(data))
			}
			if a.StorageMode != model.StorageCopy || a.Missing {
				t.Fatalf("unexpected big asset state: mode=%s missing=%v", a.StorageMode, a.Missing)
			}
		}
	}
	if !gotDup || !gotBig {
		t.Fatalf("missing assets: gotDup=%v gotBig=%v", gotDup, gotBig)
	}
}

func TestPairTransferDeclined(t *testing.T) {
	_, srv := newTestSignaling(t, time.Minute)
	src := openLib(t)
	dst := openLib(t)

	tag, err := src.CreateTag("Tag")
	if err != nil {
		t.Fatal(err)
	}
	store := project.New(src.DB)
	proj, err := store.Create("Secret", "", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	bundle, files, err := Export(src.DB, proj.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	opt := PairOptions{
		SignalingURL: srv.URL,
		Name:         "Alice",
		ICEServers:   []webrtc.ICEServer{},
	}

	codeCh := make(chan string, 1)
	sendErrCh := make(chan error, 1)
	go func() {
		sendErrCh <- PairSend(ctx, opt, bundle, files, func(code string) {
			codeCh <- code
		})
	}()

	code := <-codeCh
	_, recvErr := PairReceive(ctx, opt, code,
		func(_ context.Context, _ string, _ Bundle) bool { return false },
		nil,
		func(b Bundle, paths map[string]string) (string, error) {
			return Apply(dst.DB, b, paths)
		},
	)
	if !errors.Is(recvErr, ErrDeclined) {
		t.Fatalf("expected ErrDeclined on receiver, got %v", recvErr)
	}
	if err := <-sendErrCh; !errors.Is(err, ErrDeclined) {
		t.Fatalf("expected ErrDeclined on sender, got %v", err)
	}

	// Receiver DB must remain untouched.
	var count int
	_ = dst.DB.View(func(tx *bbolt.Tx) error {
		ps, err := db.ListProjects(tx)
		if err != nil {
			return err
		}
		count = len(ps)
		return nil
	})
	if count != 0 {
		t.Fatalf("expected 0 projects in receiver DB, got %d", count)
	}
}

func TestPairSendCancelCleansRoom(t *testing.T) {
	_, srv := newTestSignaling(t, time.Minute)
	opt := PairOptions{
		SignalingURL: srv.URL,
		Name:         "Alice",
		ICEServers:   []webrtc.ICEServer{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- PairSend(ctx, opt, Bundle{Project: model.Project{Name: "P"}}, nil, func(code string) {
			codeCh <- code
		})
	}()

	code := <-codeCh
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Room should be cleaned up on the signaling server.
	client := NewSignalingClient(srv.URL)
	if _, _, err := client.GetOffer(context.Background(), code); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound after cancel, got %v", err)
	}
}
