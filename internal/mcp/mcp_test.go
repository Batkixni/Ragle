package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

func testPNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMCPStdioToolsAndResources(t *testing.T) {
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	projs := project.New(lib.DB)

	a, _, err := lib.ImportBytes("studio_hero.png", testPNG(t, color.RGBA{255, 92, 51, 255}), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = lib.EnsureVisualFeatures(a.ID)

	srv := New(Options{Library: lib, Projects: projs})

	reqs := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_assets","arguments":{"color":"#FF5C33"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_board","arguments":{"name":"MCP Mood","asset_ids":["` + a.ID + `"]}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"raglex://library/summary"}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), strings.NewReader(reqs), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 JSON-RPC responses, got %d:\n%s", len(lines), out.String())
	}
	for i, line := range lines {
		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("line %d invalid json: %v", i, err)
		}
		if resp.Error != nil {
			t.Fatalf("line %d returned error: %+v", i, resp.Error)
		}
	}
	if !strings.Contains(lines[2], "studio_hero.png") {
		t.Fatalf("color search response missing studio_hero.png: %s", lines[2])
	}
	if !strings.Contains(lines[3], "MCP Mood") {
		t.Fatalf("create_board response missing MCP Mood: %s", lines[3])
	}
}

func TestMCPv02NewTools(t *testing.T) {
	dir := t.TempDir()
	lib, err := library.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	projs := project.New(lib.DB)

	a, _, err := lib.ImportBytes("hero.png", testPNG(t, color.RGBA{40, 120, 220, 255}), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := projs.CreateStandaloneBoard("Agent Board")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(Options{Library: lib, Projects: projs})
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
		res, err := srv.callTool(context.Background(), raw)
		if err != nil {
			t.Fatalf("%s failed: %v", name, err)
		}
		return res.(map[string]any)
	}

	upd := call("update_asset", map[string]any{
		"asset_id":   a.ID,
		"rating":     5,
		"notes":      "Key visual reference",
		"source_url": "https://example.com/ref",
	})
	if got := upd["asset"].(model.Asset); got.Rating != 5 || got.Notes != "Key visual reference" {
		t.Fatalf("update_asset mismatch: %+v", got)
	}

	projRes := call("create_project", map[string]any{
		"name":        "Brand Refresh",
		"description": "2026 identity",
		"asset_ids":   []string{a.ID},
	})
	if projRes["referenced"].(int) != 1 {
		t.Fatalf("expected 1 referenced asset, got %v", projRes["referenced"])
	}

	elRes := call("add_board_elements", map[string]any{
		"board_id": b.ID,
		"elements": []map[string]any{
			{"kind": "frame", "x": 0, "y": 0, "width": 600, "height": 420, "text": "Mood Section", "color": "#4ADE80"},
			{"kind": "arrow", "x": 40, "y": 40, "x2": 240, "y2": 180, "dashed": true, "color": "#FF5C33"},
		},
	})
	if elRes["count"].(int) != 2 {
		t.Fatalf("expected 2 created board elements, got %v", elRes["count"])
	}

	gb := call("get_board", map[string]any{"board_id": b.ID})
	if gb["count"].(int) != 2 {
		t.Fatalf("expected get_board count=2, got %v", gb["count"])
	}

	ga := call("get_asset", map[string]any{"asset_id": a.ID})
	if ga["asset"].(model.Asset).Rating != 5 {
		t.Fatalf("expected get_asset rating=5")
	}
}
