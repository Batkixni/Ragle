package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"raglex/internal/model"
)

func TestAssetStacksLifecycleAndAutoGroup(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	srcDir := t.TempDir()
	names := []string{"Poster_v1.png", "Poster_v2.png", "Poster_final.png", "Standalone.png"}
	paths := make([]string, 0, len(names))
	for i, name := range names {
		p := filepath.Join(srcDir, name)
		if err := os.WriteFile(p, []byte("fake-image-content-"+name+string(rune('A'+i))), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}

	res, err := lib.Import(context.Background(), paths, model.StorageCopy, nil, nil)
	if err != nil || res.Added != 4 {
		t.Fatalf("import failed: added=%d err=%v", res.Added, err)
	}

	// Verify VersionStem extraction.
	if got := VersionStem("Poster_v1.png"); got != "Poster" {
		t.Fatalf("VersionStem(Poster_v1.png) = %q, want Poster", got)
	}
	if got := VersionStem("Poster_final.png"); got != "Poster" {
		t.Fatalf("VersionStem(Poster_final.png) = %q, want Poster", got)
	}
	if got := VersionStem("Hero_side (2).jpg"); got != "Hero" {
		t.Fatalf("VersionStem(Hero_side (2).jpg) = %q, want Hero", got)
	}

	// Auto-stack should fold the 3 Poster versions into 1 stack with Poster_final as cover.
	n, err := lib.AutoStackByVersionName()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 auto-created stack, got %d", n)
	}
	stacks, err := lib.ListStacks()
	if err != nil || len(stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d err=%v", len(stacks), err)
	}
	st := stacks[0]
	if len(st.AssetIDs) != 3 {
		t.Fatalf("expected 3 assets in stack, got %d", len(st.AssetIDs))
	}
	if st.Name != "Poster" {
		t.Fatalf("expected stack name Poster, got %q", st.Name)
	}

	// Switch cover.
	st, err = lib.SetStackCover(st.ID, st.AssetIDs[0])
	if err != nil || st.CoverID != st.AssetIDs[0] {
		t.Fatalf("SetStackCover failed: %v", err)
	}

	// Removing 1 leaves 2 (stack stays alive); removing another leaves 1 (auto-dissolves).
	if err := lib.RemoveFromStack([]string{st.AssetIDs[2]}); err != nil {
		t.Fatal(err)
	}
	stacks, _ = lib.ListStacks()
	if len(stacks) != 1 || len(stacks[0].AssetIDs) != 2 {
		t.Fatalf("expected 1 stack of 2 after removing 1, got %+v", stacks)
	}
	if err := lib.RemoveFromStack([]string{st.AssetIDs[1]}); err != nil {
		t.Fatal(err)
	}
	stacks, _ = lib.ListStacks()
	if len(stacks) != 0 {
		t.Fatalf("expected stack auto-dissolved when <2 left, got %d", len(stacks))
	}

	// Manual CreateStack & UngroupStack.
	st2, err := lib.CreateStack("Manual Group", []string{res.Items[0].AssetID, res.Items[3].AssetID}, res.Items[3].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if st2.CoverID != res.Items[3].AssetID {
		t.Fatalf("expected explicit coverID, got %s", st2.CoverID)
	}
	if err := lib.UngroupStack(st2.ID); err != nil {
		t.Fatal(err)
	}
	stacks, _ = lib.ListStacks()
	if len(stacks) != 0 {
		t.Fatalf("expected 0 stacks after ungroup, got %d", len(stacks))
	}
}
