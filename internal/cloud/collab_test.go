package cloud

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
	rsync "raglex/internal/sync"
)

func TestTwoUsersCollaboratePerUserBoardOwnershipAndSharedRefs(t *testing.T) {
	srv, err := Open(Options{
		DataDir:       t.TempDir(),
		BlobDir:       t.TempDir(),
		PublicHost:    "cloud.test.local",
		AllowRegister: true,
		QuotaBytes:    20 << 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	register := func(email, name string) string {
		body, _ := json.Marshal(map[string]string{"email": email, "password": "password123", "name": name})
		resp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		return out.Token
	}

	aliceTok := register("alice@example.com", "Alice")
	bobTok := register("bob@example.com", "Bob")

	// Alice sets up a Project with 1 asset + 1 board ("Alice Board") with a note.
	aliceLib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer aliceLib.Close()
	aliceStore := project.New(aliceLib.DB)

	imgA := filepath.Join(t.TempDir(), "ref_a.png")
	_ = os.WriteFile(imgA, []byte("alice-ref-image-bytes"), 0o644)
	tgA, _ := aliceLib.CreateTag("Concept")
	resA, _ := aliceLib.ImportWithFolders(t.Context(), []string{imgA}, model.StorageCopy, []string{tgA.ID}, nil, false, nil)
	projA, _ := aliceStore.Create("Studio MV", "Shared refs", []string{tgA.ID})
	_ = aliceStore.Reference(projA.ID, resA.Items[0].AssetID)
	boardA, _ := aliceStore.CreateBoard(projA.ID, "Alice Board")
	_ = aliceStore.SavePlacements(boardA.ID, []model.Placement{
		{ID: model.NewID(), BoardID: boardA.ID, Kind: model.KindText, Text: "Alice original note", X: 10, Y: 20},
	})

	// Alice creates Collab Space -> gets CLB-XXXX-XXXX.
	aliceCfg := rsync.ClientConfig{ServerURL: ts.URL, Token: aliceTok}
	cs, err := rsync.CreateCollabSpace(t.Context(), aliceCfg, aliceLib.DB, "project", projA.ID, nil)
	if err != nil {
		t.Fatalf("CreateCollabSpace failed: %v", err)
	}
	if !rsync.IsCollabCode(cs.Code) || cs.BoardCount != 1 {
		t.Fatalf("unexpected collab summary: %+v", cs)
	}

	// Bob joins via CLB code and pulls into his empty library.
	bobLib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer bobLib.Close()
	bobStore := project.New(bobLib.DB)
	bobCfg := rsync.ClientConfig{ServerURL: ts.URL, Token: bobTok}

	joined, err := rsync.JoinCollabSpace(t.Context(), bobCfg, cs.Code)
	if err != nil {
		t.Fatalf("JoinCollabSpace failed: %v", err)
	}
	sync1, err := rsync.SyncCollabSpace(t.Context(), bobCfg, bobLib.DB, joined.ID, joined.Kind, "", nil)
	if err != nil {
		t.Fatalf("Bob initial SyncCollabSpace failed: %v", err)
	}
	bobProjID := sync1.LocalTargetID
	if bobProjID == "" || sync1.Downloaded != 1 {
		t.Fatalf("unexpected Bob initial sync: %+v", sync1)
	}

	// Bob tries to tamper with Alice's board AND adds his own "Bob Board" + new ref image!
	bobBoards, _ := bobStore.Boards(bobProjID)
	for _, b := range bobBoards {
		if b.Name == "Alice Board" {
			_ = bobStore.SavePlacements(b.ID, []model.Placement{
				{ID: model.NewID(), BoardID: b.ID, Kind: model.KindText, Text: "BOB VANDALISM", X: 0, Y: 0},
			})
		}
	}
	boardB, _ := bobStore.CreateBoard(bobProjID, "Bob Board")
	_ = bobStore.SavePlacements(boardB.ID, []model.Placement{
		{ID: model.NewID(), BoardID: boardB.ID, Kind: model.KindText, Text: "Bob own board note", X: 50, Y: 50},
	})
	imgB := filepath.Join(t.TempDir(), "ref_b.png")
	_ = os.WriteFile(imgB, []byte("bob-ref-image-bytes"), 0o644)
	resB, _ := bobLib.ImportWithFolders(t.Context(), []string{imgB}, model.StorageCopy, nil, nil, false, nil)
	_ = bobStore.Reference(bobProjID, resB.Items[0].AssetID)

	// Bob pushes -> server MUST reject Bob's edit to Alice Board, keep Bob Board + ref_b.png!
	if _, err := rsync.SyncCollabSpace(t.Context(), bobCfg, bobLib.DB, joined.ID, joined.Kind, bobProjID, nil); err != nil {
		t.Fatalf("Bob push sync failed: %v", err)
	}

	// Alice syncs -> gets Bob Board + ref_b.png, AND Alice Board is untouched!
	syncA, err := rsync.SyncCollabSpace(t.Context(), aliceCfg, aliceLib.DB, cs.ID, cs.Kind, projA.ID, nil)
	if err != nil {
		t.Fatalf("Alice pull sync failed: %v", err)
	}
	if syncA.Downloaded != 1 {
		t.Fatalf("expected Alice to download Bob's 1 new ref image, got %d", syncA.Downloaded)
	}
	aliceBoards, _ := aliceStore.Boards(projA.ID)
	if len(aliceBoards) != 2 {
		t.Fatalf("expected 2 boards (Alice Board + Bob Board), got %+v", aliceBoards)
	}
	for _, b := range aliceBoards {
		pls, _ := aliceStore.LoadPlacements(b.ID)
		if b.Name == "Alice Board" {
			if len(pls) != 1 || pls[0].Text != "Alice original note" {
				t.Fatalf("Alice Board was modified by Bob! got placements=%+v", pls)
			}
		}
		if b.Name == "Bob Board" {
			if len(pls) != 1 || pls[0].Text != "Bob own board note" {
				t.Fatalf("Bob Board missing or wrong placements=%+v", pls)
			}
		}
	}

	// Privacy check: when Bob lists collab spaces, Alice's email MUST be masked while Bob's own stays intact!
	bobSpaces, err := rsync.ListCollabSpaces(t.Context(), bobCfg)
	if err != nil || len(bobSpaces) != 1 {
		t.Fatalf("expected 1 collab space for Bob, got len=%d err=%v", len(bobSpaces), err)
	}
	for _, m := range bobSpaces[0].Members {
		if m.Role == "owner" && m.Email == "alice@example.com" {
			t.Fatalf("expected Alice's email to be masked for Bob, got raw %q", m.Email)
		}
		if m.Role == "editor" && m.Email != "bob@example.com" {
			t.Fatalf("expected Bob's own email intact, got %q", m.Email)
		}
	}
}
