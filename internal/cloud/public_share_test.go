package cloud

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/share"
	rsync "raglex/internal/sync"
)

func TestPublicSharePublishAndAnonymousDownload(t *testing.T) {
	srv, err := Open(Options{
		DataDir:       t.TempDir(),
		BlobDir:       t.TempDir(),
		PublicHost:    "cloud.test.local",
		AllowRegister: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. Owner registers and uploads a blob.
	regBody := `{"email":"owner@example.com","password":"password123","name":"Owner"}`
	resp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewBufferString(regBody))
	if err != nil {
		t.Fatal(err)
	}
	var regOut struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&regOut)
	resp.Body.Close()
	if regOut.Token == "" {
		t.Fatal("missing owner token")
	}

	blobData := []byte("hello raglex public folder blob bytes")
	sum := sha256.Sum256(blobData)
	blobHash := hex.EncodeToString(sum[:])

	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/blobs/"+blobHash, bytes.NewReader(blobData))
	putReq.Header.Set("Authorization", "Bearer "+regOut.Token)
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("upload blob failed: err=%v status=%v", err, putResp)
	}
	putResp.Body.Close()

	// 2. Owner publishes a public folder share.
	bundle := share.Bundle{
		Kind:         "folder",
		RootFolderID: "f1",
		Folders: []share.FolderSnap{
			{ID: "f1", Name: "UI Kit", Color: "#3B82F6"},
			{ID: "f2", Name: "Icons", ParentID: "f1"},
		},
		Assets: []share.AssetSnap{
			{
				Asset:     model.Asset{ID: "a1", Hash: blobHash, Path: "icon.png", Mime: "image/png"},
				TagNames:  []string{"Design"},
				FolderIDs: []string{"f2"},
			},
		},
		Files: []share.FileSnap{
			{Hash: blobHash, Name: "icon.png", Size: int64(len(blobData))},
		},
	}
	pubPayload, _ := json.Marshal(map[string]any{"bundle": bundle})
	pubReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/public-shares", bytes.NewReader(pubPayload))
	pubReq.Header.Set("Authorization", "Bearer "+regOut.Token)
	pubReq.Header.Set("Content-Type", "application/json")
	pubResp, err := http.DefaultClient.Do(pubReq)
	if err != nil || pubResp.StatusCode != http.StatusCreated {
		t.Fatalf("publish public share failed: err=%v status=%v", err, pubResp)
	}
	var pubOut struct {
		OK   bool   `json:"ok"`
		Code string `json:"code"`
		URL  string `json:"url"`
	}
	_ = json.NewDecoder(pubResp.Body).Decode(&pubOut)
	pubResp.Body.Close()
	if !pubOut.OK || pubOut.Code == "" {
		t.Fatalf("expected share code, got %+v", pubOut)
	}

	// 3. Completely ANONYMOUS user (no Authorization header!) fetches the share manifest & blob!
	getResp, err := http.Get(ts.URL + "/api/v1/public-shares/" + pubOut.Code)
	if err != nil || getResp.StatusCode != http.StatusOK {
		t.Fatalf("anonymous GET share failed: err=%v status=%v", err, getResp)
	}
	var getOut struct {
		OK     bool         `json:"ok"`
		Bundle share.Bundle `json:"bundle"`
	}
	_ = json.NewDecoder(getResp.Body).Decode(&getOut)
	getResp.Body.Close()
	if !getOut.OK || getOut.Bundle.DisplayName() != "UI Kit" || len(getOut.Bundle.Folders) != 2 {
		t.Fatalf("unexpected anonymous bundle: %+v", getOut)
	}

	blobResp, err := http.Get(ts.URL + "/api/v1/public-shares/" + pubOut.Code + "/blobs/" + blobHash)
	if err != nil || blobResp.StatusCode != http.StatusOK {
		t.Fatalf("anonymous GET blob failed: err=%v status=%v", err, blobResp)
	}
	gotBytes, _ := io.ReadAll(blobResp.Body)
	blobResp.Body.Close()
	if !bytes.Equal(gotBytes, blobData) {
		t.Fatalf("blob bytes mismatch: got %q", string(gotBytes))
	}

	// 4. Attempting to fetch an unrelated blob hash through the share MUST fail with 404.
	otherSum := sha256.Sum256([]byte("private secret"))
	otherHash := hex.EncodeToString(otherSum[:])
	badResp, err := http.Get(ts.URL + "/api/v1/public-shares/" + pubOut.Code + "/blobs/" + otherHash)
	if err != nil {
		t.Fatal(err)
	}
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unshared blob hash, got %d", badResp.StatusCode)
	}

	// 5. Public landing page /s/{code} returns 200 HTML containing the code.
	pageResp, err := http.Get(ts.URL + "/s/" + pubOut.Code)
	if err != nil || pageResp.StatusCode != http.StatusOK {
		t.Fatalf("landing page failed: err=%v status=%v", err, pageResp)
	}
	pageBytes, _ := io.ReadAll(pageResp.Body)
	pageResp.Body.Close()
	if !bytes.Contains(pageBytes, []byte(pubOut.Code)) {
		t.Fatalf("landing page missing share code")
	}

	// 6. Owner revokes share -> anonymous GET now returns 404!
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/public-shares/"+pubOut.Code, nil)
	delReq.Header.Set("Authorization", "Bearer "+regOut.Token)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil || delResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke share failed: err=%v status=%v", err, delResp)
	}
	delResp.Body.Close()

	afterResp, err := http.Get(ts.URL + "/api/v1/public-shares/" + pubOut.Code)
	if err != nil {
		t.Fatal(err)
	}
	afterResp.Body.Close()
	if afterResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after revoke, got %d", afterResp.StatusCode)
	}
}

func TestEndToEndFolderPublicShareAcrossTwoLibraries(t *testing.T) {
	srv, err := Open(Options{
		DataDir:       t.TempDir(),
		BlobDir:       t.TempDir(),
		PublicHost:    "cloud.test.local",
		AllowRegister: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Owner registers on Cloud.
	resp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json",
		bytes.NewBufferString(`{"email":"alice@example.com","password":"password123","name":"Alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	resp.Body.Close()

	// Alice's local library with nested Folder "Brand" -> "Icons" + 1 file + 1 tag.
	aliceLib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer aliceLib.Close()

	srcDir := t.TempDir()
	iconsDir := filepath.Join(srcDir, "Brand", "Icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	samplePath := filepath.Join(iconsDir, "badge.png")
	sampleBytes := []byte("PNG-fake-bytes-for-public-folder-share")
	if err := os.WriteFile(samplePath, sampleBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	tg, err := aliceLib.CreateTag("Vector")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := aliceLib.ImportWithFolders(t.Context(), []string{filepath.Join(srcDir, "Brand")}, model.StorageCopy, []string{tg.ID}, nil, true, nil); err != nil {
		t.Fatal(err)
	}
	folders, _ := aliceLib.Folders()
	var brandID string
	for _, f := range folders {
		if f.Name == "Brand" && f.ParentID == "" {
			brandID = f.ID
			break
		}
	}
	if brandID == "" {
		t.Fatalf("expected Brand folder, got %+v", folders)
	}
	_ = aliceLib.SetFolderColor(brandID, "#FF5C33")

	// Alice publishes "Brand" folder to Cloud -> gets SHR-XXXX-XXXX code!
	pubRes, err := rsync.PublishPublicShare(t.Context(), rsync.ClientConfig{
		ServerURL: ts.URL,
		Token:     reg.Token,
	}, aliceLib.DB, "folder", brandID, true, nil)
	if err != nil {
		t.Fatalf("PublishPublicShare failed: %v", err)
	}
	if !rsync.IsPublicShareCode(pubRes.Code) || pubRes.AssetCount != 1 || pubRes.FolderCount != 2 {
		t.Fatalf("unexpected pubRes: %+v", pubRes)
	}

	// Bob (unregistered user, NO token!) downloads the folder using ONLY the code!
	bobLib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer bobLib.Close()

	dlRes, err := rsync.DownloadPublicShare(t.Context(), ts.URL, pubRes.Code, bobLib.DB, nil)
	if err != nil {
		t.Fatalf("DownloadPublicShare failed: %v", err)
	}
	if dlRes.Kind != "folder" || dlRes.Title != "Brand" || dlRes.AssetsCount != 1 || dlRes.FoldersCount != 2 || dlRes.Downloaded != 1 {
		t.Fatalf("unexpected dlRes: %+v", dlRes)
	}

	bobSnap, err := bobLib.LoadCatalogSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(bobSnap.Folders) != 2 || len(bobSnap.Assets) != 1 || len(bobSnap.Tags) != 1 {
		t.Fatalf("bob library missing items: folders=%d assets=%d tags=%d", len(bobSnap.Folders), len(bobSnap.Assets), len(bobSnap.Tags))
	}
	if bobSnap.Assets[0].Missing {
		t.Fatal("downloaded asset should not be marked missing")
	}
	gotData, err := os.ReadFile(bobSnap.Assets[0].Path)
	if err != nil || !bytes.Equal(gotData, sampleBytes) {
		t.Fatalf("downloaded file mismatch: err=%v data=%q", err, string(gotData))
	}
}
