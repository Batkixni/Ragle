package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"raglex/internal/library"
	"raglex/internal/project"
)

func samplePNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTwoDeviceDeltaAndBlobSync(t *testing.T) {
	srvDir := t.TempDir()
	st, err := OpenStore(srvDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	blobs, err := NewLocalBlobStore(srvDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	const tok = "secret-sync-token"
	h := &Handler{
		Store: st,
		Blobs: blobs,
		Auth: func(r *http.Request) bool {
			return r.Header.Get("Authorization") == "Bearer "+tok
		},
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Device A creates an asset with tag, rating, and notes.
	libA, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer libA.Close()
	tg, err := libA.CreateTag("Poster")
	if err != nil {
		t.Fatal(err)
	}
	a1, _, err := libA.ImportBytes("neon_poster.png", samplePNG(t, color.RGBA{255, 80, 40, 255}), []string{tg.ID}, "https://example.com/art", "Neon Art")
	if err != nil {
		t.Fatal(err)
	}
	_ = libA.SetRating(a1.ID, 5)
	_, _ = libA.UpdateAssetMeta(a1.ID, "Hero reference", "https://example.com/art", "Neon Art")

	// Device A pushes to sync server.
	repA, err := SyncNow(context.Background(), ClientConfig{ServerURL: srv.URL, Token: tok, DeviceID: "dev-A"}, libA, project.New(libA.DB))
	if err != nil {
		t.Fatalf("Device A SyncNow failed: %v", err)
	}
	if repA.PushedOps != 1 || repA.UploadedBlobs != 1 {
		t.Fatalf("expected 1 pushed op & 1 uploaded blob, got %+v", repA)
	}

	// Re-syncing Device A must be 100% idempotent (0 new ops, 0 new blobs).
	repA2, err := SyncNow(context.Background(), ClientConfig{ServerURL: srv.URL, Token: tok, DeviceID: "dev-A"}, libA, project.New(libA.DB))
	if err != nil {
		t.Fatalf("Device A 2nd SyncNow failed: %v", err)
	}
	if repA2.PushedOps != 0 || repA2.UploadedBlobs != 0 {
		t.Fatalf("expected idempotent 0/0 on 2nd sync, got %+v", repA2)
	}

	// Device B starts completely empty and pulls from sync server!
	libB, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer libB.Close()
	repB, err := SyncNow(context.Background(), ClientConfig{ServerURL: srv.URL, Token: tok, DeviceID: "dev-B"}, libB, project.New(libB.DB))
	if err != nil {
		t.Fatalf("Device B SyncNow failed: %v", err)
	}
	if repB.PulledOps != 1 || repB.DownloadedBlobs != 1 {
		t.Fatalf("expected Device B to pull 1 op & 1 blob, got %+v", repB)
	}
	listB, err := libB.List(library.Filter{})
	if err != nil || len(listB) != 1 {
		t.Fatalf("expected 1 synced asset on Device B, got %d err=%v", len(listB), err)
	}
	got := listB[0]
	if got.Rating != 5 || got.Notes != "Hero reference" || got.SourceURL != "https://example.com/art" {
		t.Fatalf("synced metadata mismatch: %+v", got)
	}
	tagsB, _ := libB.AssetTags(got.ID)
	if len(tagsB) != 1 || tagsB[0].Name != "Poster" {
		t.Fatalf("synced tags mismatch: %+v", tagsB)
	}
}

func TestBlobTamperRejectedAndR2Presign(t *testing.T) {
	blobs, err := NewLocalBlobStore(t.TempDir(), &R2Presigner{
		Endpoint:  "https://acct.r2.cloudflarestorage.com",
		Bucket:    "raglex-cloud",
		Region:    "auto",
		AccessKey: "AKIA_TEST",
		SecretKey: "SECRET_TEST",
	})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("valid-content")
	sum := sha256.Sum256(data)
	goodHash := hex.EncodeToString(sum[:])

	// Tampered upload MUST be rejected.
	if _, err := blobs.Put(context.Background(), goodHash, bytes.NewReader([]byte("tampered"))); err == nil {
		t.Fatal("expected sha256 mismatch error")
	}
	if _, err := blobs.Put(context.Background(), goodHash, bytes.NewReader(data)); err != nil {
		t.Fatalf("valid Put failed: %v", err)
	}
	rc, sz, err := blobs.Open(context.Background(), goodHash)
	if err != nil || sz != int64(len(data)) {
		t.Fatalf("Open failed: sz=%d err=%v", sz, err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("Open content mismatch")
	}

	u, ok := blobs.PresignURL("PUT", goodHash, 10*time.Minute)
	if !ok || !strings.Contains(u, "X-Amz-Algorithm=AWS4-HMAC-SHA256") || !strings.Contains(u, "X-Amz-Signature=") {
		t.Fatalf("unexpected R2 presigned URL: ok=%v url=%q", ok, u)
	}
}

func TestPublicShareLinkPasswordAndViewer(t *testing.T) {
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	a, _, err := lib.ImportBytes("share_me.png", samplePNG(t, color.RGBA{30, 140, 250, 255}), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	blobs, _ := NewLocalBlobStore(dir, nil)

	link, err := st.CreateShare(CreateShareRequest{
		Kind:          "asset",
		TargetID:      a.ID,
		Title:         "Client Preview Deck",
		Password:      "studio2026",
		AllowDownload: true,
		TTLHours:      24,
	})
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{Store: st, Blobs: blobs, Lib: lib, Projects: project.New(lib.DB)}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Without password -> 401 with unlock HTML form.
	r1, err := http.Get(srv.URL + "/s/" + link.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without password, got %d", r1.StatusCode)
	}

	// With correct password -> 200 HTML gallery containing title and asset name.
	r2, err := http.Get(srv.URL + "/s/" + link.ID + "?p=studio2026")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r2.Body)
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusOK || !strings.Contains(string(body), "Client Preview Deck") || !strings.Contains(string(body), "share_me.png") {
		t.Fatalf("expected 200 public viewer with asset, status=%d body=%s", r2.StatusCode, string(body))
	}
}

func TestOpenStoreConcurrentSameProcess(t *testing.T) {
	dir := t.TempDir()
	s1, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("first OpenStore failed: %v", err)
	}
	defer s1.Close()

	// Simulates built-in Companion Server holding sync.db open while desktop
	// SyncNow opens the exact same library sync.db concurrently.
	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("second OpenStore in same process failed (expected shared ref): %v", err)
	}
	if err := s2.SetCursor("https://cloud.raglex.sorai.tw", 42); err != nil {
		t.Fatalf("SetCursor on s2 failed: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatalf("Close s2 failed: %v", err)
	}
	// Closing s2 MUST NOT close s1's underlying bbolt handle.
	if got := s1.GetCursor("https://cloud.raglex.sorai.tw"); got != 42 {
		t.Fatalf("GetCursor on s1 after s2.Close() = %d, want 42", got)
	}
}

func TestSyncDeltaCacheAndPullOnlyZeroWaste(t *testing.T) {
	srvDir := t.TempDir()
	srvStore, err := OpenStore(srvDir)
	if err != nil {
		t.Fatal(err)
	}
	defer srvStore.Close()
	srvBlobs, _ := NewLocalBlobStore(srvDir, nil)
	h := &Handler{Store: srvStore, Blobs: srvBlobs}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	var checkHits, pushHits, pullHits int
	counted := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/blobs/check":
			checkHits++
		case "/v1/sync/push":
			pushHits++
		case "/v1/sync/pull":
			pullHits++
		}
		mux.ServeHTTP(w, r)
	})
	ts := httptest.NewServer(counted)
	defer ts.Close()

	cliDir := t.TempDir()
	lib, err := library.Open(cliDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	projs := project.New(lib.DB)

	a1, _, err := lib.ImportBytes("one.png", samplePNG(t, color.RGBA{200, 50, 50, 255}), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.ImportBytes("two.png", samplePNG(t, color.RGBA{50, 200, 50, 255}), nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.ImportBytes("three.png", samplePNG(t, color.RGBA{50, 50, 200, 255}), nil, "", ""); err != nil {
		t.Fatal(err)
	}

	cfg := ClientConfig{ServerURL: ts.URL, Token: "tok-1", DeviceID: "dev-a"}

	// 1. Initial sync uploads 3 blobs & pushes 3 ops.
	rep1, err := SyncNow(context.Background(), cfg, lib, projs)
	if err != nil {
		t.Fatal(err)
	}
	if rep1.UploadedBlobs != 3 || rep1.PushedOps != 3 || checkHits != 1 || pushHits != 1 {
		t.Fatalf("initial sync mismatch: rep=%+v check=%d push=%d pull=%d", rep1, checkHits, pushHits, pullHits)
	}

	// 2. Immediate repeat sync with NO edits sends ZERO /v1/blobs/check and ZERO /v1/sync/push!
	checkHits, pushHits, pullHits = 0, 0, 0
	rep2, err := SyncNow(context.Background(), cfg, lib, projs)
	if err != nil {
		t.Fatal(err)
	}
	if checkHits != 0 || pushHits != 0 || pullHits != 1 || rep2.UploadedBlobs != 0 || rep2.PushedOps != 0 {
		t.Fatalf("repeat idle sync wasted network! rep=%+v check=%d push=%d pull=%d", rep2, checkHits, pushHits, pullHits)
	}

	// 3. Edit 1 asset's rating -> sends ZERO blob checks and pushes ONLY 1 delta op!
	if err := lib.SetRating(a1.ID, 5); err != nil {
		t.Fatal(err)
	}
	checkHits, pushHits, pullHits = 0, 0, 0
	rep3, err := SyncNow(context.Background(), cfg, lib, projs)
	if err != nil {
		t.Fatal(err)
	}
	if checkHits != 0 || pushHits != 1 || rep3.PushedOps != 1 || rep3.UploadedBlobs != 0 {
		t.Fatalf("1-asset edit should push exactly 1 op with 0 blob checks: rep=%+v check=%d push=%d", rep3, checkHits, pushHits)
	}

	// 4. PullOnly periodic poll sends ONLY /v1/sync/pull even when local assets exist.
	checkHits, pushHits, pullHits = 0, 0, 0
	pullCfg := cfg
	pullCfg.PullOnly = true
	if _, err := SyncNow(context.Background(), pullCfg, lib, projs); err != nil {
		t.Fatal(err)
	}
	if checkHits != 0 || pushHits != 0 || pullHits != 1 {
		t.Fatalf("PullOnly poll sent unexpected requests: check=%d push=%d pull=%d", checkHits, pushHits, pullHits)
	}
}

func TestSyncZeroKnowledgeE2EE(t *testing.T) {
	srvDir := t.TempDir()
	srvStore, err := OpenStore(srvDir)
	if err != nil {
		t.Fatal(err)
	}
	defer srvStore.Close()
	srvBlobs, _ := NewLocalBlobStore(srvDir, nil)
	h := &Handler{Store: srvStore, Blobs: srvBlobs}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Device A imports a secret concept art PNG with sensitive notes/tags and syncs with E2EE passphrase.
	dirA := t.TempDir()
	libA, err := library.Open(dirA)
	if err != nil {
		t.Fatal(err)
	}
	defer libA.Close()
	projsA := project.New(libA.DB)

	origPNG := samplePNG(t, color.RGBA{255, 92, 51, 255})
	a1, _, err := libA.ImportBytes("secret_nda_character.png", origPNG, nil, "https://internal.studio/nda", "Top Secret NDA")
	if err != nil {
		t.Fatal(err)
	}
	_ = libA.SetRating(a1.ID, 5)
	_, _ = libA.UpdateAssetMeta(a1.ID, "Unannounced 2027 Boss Design", "https://internal.studio/nda", "Top Secret NDA")
	if tg, err := libA.CreateTag("NDA-Confidential"); err == nil {
		_ = libA.SetAssetTag(a1.ID, tg.ID, true)
	}

	cfgA := ClientConfig{
		ServerURL:      ts.URL,
		Token:          "tok-e2ee",
		DeviceID:       "studio-mac",
		E2EEPassphrase: "correct-horse-battery-staple",
	}
	repA, err := SyncNow(context.Background(), cfgA, libA, projsA)
	if err != nil {
		t.Fatal(err)
	}
	if repA.UploadedBlobs != 1 || repA.PushedOps != 1 {
		t.Fatalf("expected 1 encrypted blob + 1 encrypted op, got %+v", repA)
	}

	// Verify ZERO plaintext leaks on the server oplog or blob store!
	ops, _, _, err := srvStore.PullOps(0, 100)
	if err != nil || len(ops) != 1 {
		t.Fatalf("expected 1 op on server, got len=%d err=%v", len(ops), err)
	}
	if ops[0].Kind != e2eeOpKind || ops[0].EntityID != "e2ee" {
		t.Fatalf("expected opaque e2ee.v1 op without real entity_id, got kind=%q entity=%q", ops[0].Kind, ops[0].EntityID)
	}
	rawOpStr := string(ops[0].Payload)
	for _, secret := range []string{"secret_nda_character", "Unannounced 2027", "NDA-Confidential", "internal.studio"} {
		if strings.Contains(rawOpStr, secret) {
			t.Fatalf("server oplog leaked plaintext %q in %s", secret, rawOpStr)
		}
	}
	// Verify stored blob on server is ciphertext (not raw PNG!).
	if srvBlobs.Has(context.Background(), a1.Hash) {
		t.Fatal("server stored plaintext hash instead of ciphertext hash")
	}

	// Device B with WRONG passphrase cannot decrypt or import anything.
	dirWrong := t.TempDir()
	libWrong, err := library.Open(dirWrong)
	if err != nil {
		t.Fatal(err)
	}
	defer libWrong.Close()
	cfgWrong := ClientConfig{
		ServerURL:      ts.URL,
		Token:          "tok-e2ee",
		DeviceID:       "wrong-dev",
		E2EEPassphrase: "wrong-passphrase",
	}
	repWrong, err := SyncNow(context.Background(), cfgWrong, libWrong, project.New(libWrong.DB))
	if err != nil {
		t.Fatal(err)
	}
	if repWrong.PulledOps != 0 || repWrong.DownloadedBlobs != 0 {
		t.Fatalf("wrong passphrase should NOT decrypt any op/blob, got %+v", repWrong)
	}

	// Device C with SAME passphrase decrypts the exact PNG bytes, notes, rating & tag!
	dirC := t.TempDir()
	libC, err := library.Open(dirC)
	if err != nil {
		t.Fatal(err)
	}
	defer libC.Close()
	cfgC := ClientConfig{
		ServerURL:      ts.URL,
		Token:          "tok-e2ee",
		DeviceID:       "home-win",
		E2EEPassphrase: "correct-horse-battery-staple",
	}
	repC, err := SyncNow(context.Background(), cfgC, libC, project.New(libC.DB))
	if err != nil {
		t.Fatal(err)
	}
	if repC.PulledOps != 1 || repC.DownloadedBlobs != 1 {
		t.Fatalf("expected Device C to decrypt 1 op + 1 blob, got %+v", repC)
	}
	snapC, err := libC.LoadCatalogSnapshot()
	if err != nil || len(snapC.Assets) != 1 {
		t.Fatalf("expected 1 decrypted asset on Device C, got %d err=%v", len(snapC.Assets), err)
	}
	got := snapC.Assets[0]
	if got.Hash != a1.Hash || got.Rating != 5 || got.Notes != "Unannounced 2027 Boss Design" {
		t.Fatalf("decrypted asset metadata mismatch: got=%+v wantHash=%s", got, a1.Hash)
	}
	if len(snapC.AssetTags[got.ID]) != 1 || snapC.AssetTags[got.ID][0].Name != "NDA-Confidential" {
		t.Fatalf("decrypted asset tags mismatch: %+v", snapC.AssetTags[got.ID])
	}
}

func TestGenerateAndParseE2EEKeyAndSync(t *testing.T) {
	k1, err := GenerateE2EEKey()
	if err != nil {
		t.Fatalf("GenerateE2EEKey failed: %v", err)
	}
	k2, err := GenerateE2EEKey()
	if err != nil {
		t.Fatalf("GenerateE2EEKey 2nd failed: %v", err)
	}
	if !strings.HasPrefix(k1, E2EEKeyPrefix) || k1 == k2 {
		t.Fatalf("expected distinct prefixed random keys, got k1=%q k2=%q", k1, k2)
	}
	rawKey, err := deriveE2EEKey(k1)
	if err != nil || len(rawKey) != 32 {
		t.Fatalf("expected 32-byte raw AES-256 key, got len=%d err=%v", len(rawKey), err)
	}
	exported := FormatE2EEKeyFile(k1, "artist@sorai.tw")
	parsed := ParseE2EEKeyFile(exported)
	if parsed != k1 {
		t.Fatalf("ParseE2EEKeyFile roundtrip mismatch: got %q want %q", parsed, k1)
	}

	plain := []byte("top-secret-storyboard-frame-bytes")
	ct, ctHash, err := encryptBlob(rawKey, "plainhash123", plain)
	if err != nil || ctHash == "" || bytes.Contains(ct, plain) {
		t.Fatalf("encryptBlob failed or leaked plaintext: err=%v", err)
	}
	dec, err := decryptBlob(rawKey, "plainhash123", ct)
	if err != nil || !bytes.Equal(dec, plain) {
		t.Fatalf("decryptBlob mismatch: err=%v", err)
	}
}

func TestCrossDeviceSameAccountBlankDeviceIDAndKeySwitch(t *testing.T) {
	srvDir := t.TempDir()
	srvStore, err := OpenStore(srvDir)
	if err != nil {
		t.Fatal(err)
	}
	defer srvStore.Close()
	blobs, err := NewLocalBlobStore(srvDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Store: srvStore, Blobs: blobs}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Device A (Windows) with BLANK DeviceID (default user config) imports a PNG and pushes.
	dirWin := t.TempDir()
	libWin, err := library.Open(dirWin)
	if err != nil {
		t.Fatal(err)
	}
	defer libWin.Close()
	aWin, _, err := libWin.ImportBytes("windows-ref.png", samplePNG(t, color.RGBA{255, 92, 51, 255}), nil, "", "")
	if err != nil {
		t.Fatalf("win import failed: %v", err)
	}
	_ = libWin.SetRating(aWin.ID, 4)

	keyWin, err := GenerateE2EEKey()
	if err != nil {
		t.Fatal(err)
	}
	repWin, err := SyncNow(context.Background(), ClientConfig{
		ServerURL:      ts.URL,
		Token:          "tok-same-account",
		DeviceID:       "", // default blank
		E2EEPassphrase: keyWin,
	}, libWin, project.New(libWin.DB))
	if err != nil || repWin.PushedOps != 1 || repWin.UploadedBlobs != 1 {
		t.Fatalf("windows push failed: rep=%+v err=%v", repWin, err)
	}

	// Device B (Mac) ALSO has BLANK DeviceID and starts with its own auto-generated different key.
	dirMac := t.TempDir()
	libMac, err := library.Open(dirMac)
	if err != nil {
		t.Fatal(err)
	}
	defer libMac.Close()
	keyMacWrong, _ := GenerateE2EEKey()
	repWrong, err := SyncNow(context.Background(), ClientConfig{
		ServerURL:      ts.URL,
		Token:          "tok-same-account",
		DeviceID:       "", // default blank
		E2EEPassphrase: keyMacWrong,
	}, libMac, project.New(libMac.DB))
	if err != nil {
		t.Fatalf("mac sync with wrong key errored: %v", err)
	}
	if repWrong.DecryptFailedOps != 1 || repWrong.DownloadedBlobs != 0 {
		t.Fatalf("expected 1 decrypt failure with mismatched key, got %+v", repWrong)
	}

	// Now Mac loads Windows's key file -> MUST re-pull from 0 and download the Windows asset!
	repMac, err := SyncNow(context.Background(), ClientConfig{
		ServerURL:      ts.URL,
		Token:          "tok-same-account",
		DeviceID:       "", // still blank!
		E2EEPassphrase: keyWin,
	}, libMac, project.New(libMac.DB))
	if err != nil {
		t.Fatalf("mac sync with windows key failed: %v", err)
	}
	if repMac.PulledOps != 1 || repMac.DownloadedBlobs != 1 || repMac.DecryptFailedOps != 0 {
		t.Fatalf("expected Mac to pull 1 op + 1 blob, got %+v", repMac)
	}
	snapMac, err := libMac.LoadCatalogSnapshot()
	if err != nil || len(snapMac.Assets) != 1 || snapMac.Assets[0].Hash != aWin.Hash || snapMac.Assets[0].Rating != 4 {
		t.Fatalf("mac library missing synced asset: %+v err=%v", snapMac.Assets, err)
	}

	// Also verify plaintext cross-device sync with blank DeviceIDs (no E2EE).
	if _, _, err := libWin.ImportBytes("plain-win.png", samplePNG(t, color.RGBA{30, 140, 220, 255}), nil, "", ""); err != nil {
		t.Fatalf("plain import failed: %v", err)
	}
	if _, err := SyncNow(context.Background(), ClientConfig{
		ServerURL: ts.URL,
		Token:     "tok-same-account",
		DeviceID:  "",
	}, libWin, project.New(libWin.DB)); err != nil {
		t.Fatalf("win plain push failed: %v", err)
	}
	repMacPlain, err := SyncNow(context.Background(), ClientConfig{
		ServerURL: ts.URL,
		Token:     "tok-same-account",
		DeviceID:  "",
	}, libMac, project.New(libMac.DB))
	if err != nil || repMacPlain.DownloadedBlobs != 1 {
		t.Fatalf("expected Mac to download plaintext asset from Windows with blank DeviceID, got rep=%+v err=%v", repMacPlain, err)
	}
}
