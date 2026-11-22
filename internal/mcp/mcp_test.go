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

	"ragle/internal/library"
	"ragle/internal/project"
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
		`{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"ragle://library/summary"}}`,
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
