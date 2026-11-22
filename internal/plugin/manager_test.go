package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPluginDiscoveryAndExecution(t *testing.T) {
	libDir := t.TempDir()
	pdir := filepath.Join(libDir, "plugins", "hello-plugin")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}

	var cmd []string
	if runtime.GOOS == "windows" {
		cmd = []string{"powershell", "-NoProfile", "-Command", `Write-Output '{"ok":true,"toast":"Plugin ran!","reload":true}'`}
	} else {
		cmd = []string{"sh", "-c", `printf '{"ok":true,"toast":"Plugin ran!","reload":true}'`}
	}
	mf := Manifest{
		ID:          "hello-plugin",
		Name:        "Hello Plugin",
		Version:     "0.1.0",
		Description: "Test plugin",
		Command:     cmd,
		Hooks:       []string{"context_menu"},
		Permissions: []string{"read_library"},
	}
	raw, _ := json.MarshalIndent(mf, "", "  ")
	if err := os.WriteFile(filepath.Join(pdir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(libDir)
	list := mgr.List()
	if len(list) != 1 || list[0].Manifest.ID != "hello-plugin" {
		t.Fatalf("expected 1 plugin, got %+v", list)
	}
	if len(mgr.ForHook("context_menu")) != 1 {
		t.Fatalf("expected plugin registered for context_menu hook")
	}

	resp, err := mgr.Execute(context.Background(), "hello-plugin", Request{
		Hook:        "context_menu",
		LibraryPath: libDir,
		Assets:      []AssetPayload{{ID: "a1", Name: "poster.png"}},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !resp.OK || resp.Toast != "Plugin ran!" || !resp.Reload {
		t.Fatalf("unexpected response: %+v", resp)
	}

	mgr.SetEnabled("hello-plugin", false)
	if len(mgr.ForHook("context_menu")) != 0 {
		t.Fatalf("disabled plugin should not appear in ForHook")
	}
}
