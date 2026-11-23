package share

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

func TestPairE2EBlindRelayFallbackAndCiphertextPrivacy(t *testing.T) {
	sig, srv := newTestSignaling(t, time.Minute)
	src := openLib(t)
	dst := openLib(t)

	const secretProject = "TopSecret_Campaign_2026"
	const secretPayload = "ULTRA_CONFIDENTIAL_ARTWORK_PAYLOAD_BYTES_99887766"

	tag, err := src.CreateTag("ClientWork")
	if err != nil {
		t.Fatal(err)
	}
	store := project.New(src.DB)
	proj, err := store.Create(secretProject, "Private brief", []string{tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	art, _, err := src.ImportBytes("confidential_poster.txt", []byte(secretPayload), []string{tag.ID}, "", "")
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if err := store.Reference(proj.ID, art.ID); err != nil {
		t.Fatal(err)
	}

	bundle, files, err := Export(src.DB, proj.ID, true)
	if err != nil {
		t.Fatal(err)
	}

	// Observe every single byte passing through the relay server to prove zero plaintext leaks.
	var relayMu sync.Mutex
	var relayedFrames [][]byte
	sig.onRelay = func(_ string, _ string, ciphertext []byte) {
		cp := make([]byte, len(ciphertext))
		copy(cp, ciphertext)
		relayMu.Lock()
		relayedFrames = append(relayedFrames, cp)
		relayMu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var sendMode, recvMode string
	sendOpt := PairOptions{
		SignalingURL: srv.URL,
		Name:         "Studio-A",
		ForceRelay:   true,
		OnTransport:  func(m string) { sendMode = m },
	}
	recvOpt := PairOptions{
		SignalingURL: srv.URL,
		Name:         "Studio-B",
		ForceRelay:   true,
		OnTransport:  func(m string) { recvMode = m },
	}

	codeCh := make(chan string, 1)
	sendErrCh := make(chan error, 1)
	go func() {
		sendErrCh <- PairSend(ctx, sendOpt, bundle, files, func(code string) {
			codeCh <- code
		})
	}()

	code := <-codeCh
	projID, recvErr := PairReceive(ctx, recvOpt, code,
		func(_ context.Context, from string, b Bundle) bool {
			return from == "Studio-A" && b.Project.Name == secretProject
		},
		func(_, name string, r io.Reader) (string, error) {
			return SaveFile(dst.DB.FilesDir(), name, r)
		},
		func(b Bundle, paths map[string]string) (string, error) {
			return Apply(dst.DB, b, paths)
		},
	)
	if recvErr != nil {
		t.Fatalf("PairReceive via E2E relay failed: %v", recvErr)
	}
	if err := <-sendErrCh; err != nil {
		t.Fatalf("PairSend via E2E relay failed: %v", err)
	}
	if sendMode != "e2e-relay" || recvMode != "e2e-relay" {
		t.Fatalf("expected e2e-relay transport on both peers, got send=%q recv=%q", sendMode, recvMode)
	}

	// Verify destination received exact decrypted project and file bytes.
	dstStore := project.New(dst.DB)
	assets, err := dstStore.Assets(projID)
	if err != nil || len(assets) != 1 {
		t.Fatalf("expected 1 received asset, got %d err=%v", len(assets), err)
	}
	gotBytes, err := os.ReadFile(assets[0].Path)
	if err != nil || string(gotBytes) != secretPayload {
		t.Fatalf("decrypted file mismatch: got %q err=%v", string(gotBytes), err)
	}

	// Prove ZERO plaintext strings ever appeared in any relayed ciphertext frame!
	relayMu.Lock()
	defer relayMu.Unlock()
	if len(relayedFrames) < 5 {
		t.Fatalf("expected relayed frames, got %d", len(relayedFrames))
	}
	for i, frame := range relayedFrames {
		if bytes.Contains(frame, []byte(secretProject)) ||
			bytes.Contains(frame, []byte(secretPayload)) ||
			bytes.Contains(frame, []byte("confidential_poster")) {
			t.Fatalf("plaintext leaked in relayed frame %d!", i)
		}
	}
}

func TestE2ECipherTamperAndReplayRejected(t *testing.T) {
	kA, err := newE2EKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	kB, err := newE2EKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cA, err := newE2ECipher(kA.priv, kB.pub, "ABC-123", true)
	if err != nil {
		t.Fatal(err)
	}
	cB, err := newE2ECipher(kB.priv, kA.pub, "ABC-123", false)
	if err != nil {
		t.Fatal(err)
	}

	f0 := cA.Seal([]byte("hello"))
	f1 := cA.Seal([]byte("world"))

	// Out-of-order replay rejected.
	if _, err := cB.Open(f1); err == nil {
		t.Fatal("expected out-of-order frame to be rejected")
	}

	// Tampered ciphertext rejected.
	bad := append([]byte(nil), f0...)
	bad[len(bad)-1] ^= 0xFF
	if _, err := cB.Open(bad); err == nil {
		t.Fatal("expected tampered frame to fail GCM auth")
	}

	// Valid ordered frames decrypt cleanly.
	p0, err := cB.Open(f0)
	if err != nil || string(p0) != "hello" {
		t.Fatalf("expected hello, got %q err=%v", string(p0), err)
	}
	// Replay of f0 must fail.
	if _, err := cB.Open(f0); err == nil {
		t.Fatal("expected replayed frame to fail")
	}
	p1, err := cB.Open(f1)
	if err != nil || string(p1) != "world" {
		t.Fatalf("expected world, got %q err=%v", string(p1), err)
	}
}
