package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func buildTestPluginZip(t *testing.T, id, name string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mf := Manifest{
		ID:      id,
		Name:    name,
		Version: "1.2.0",
		Command: []string{"echo", `{"ok":true}`},
		Hooks:   []string{"context_menu"},
	}
	rawMf, _ := json.Marshal(mf)
	w, err := zw.Create("pkg-root/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(rawMf)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func TestPluginRegistryFetchInstallVerifyAndUninstall(t *testing.T) {
	zipBytes, zipSHA := buildTestPluginZip(t, "community.demo-tool", "Community Demo Tool")

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.json":
			cat := RegistryCatalog{
				SchemaVersion: 1,
				Plugins: []RegistryEntry{
					{
						ID:          "community.demo-tool",
						Name:        "Community Demo Tool",
						Version:     "1.2.0",
						Author:      "Tester",
						Description: "Test remote plugin",
						Category:    "utility",
						DownloadURL: srvURL + "/demo.zip",
						SHA256:      zipSHA,
						Hooks:       []string{"context_menu"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(cat)
		case "/demo.zip":
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	entries, err := FetchRegistry(context.Background(), srv.URL+"/registry.json")
	if err != nil {
		t.Fatalf("FetchRegistry failed: %v", err)
	}
	var remote RegistryEntry
	found := false
	for _, e := range entries {
		if e.ID == "community.demo-tool" {
			remote = e
			found = true
		}
	}
	if !found {
		t.Fatalf("expected community.demo-tool in merged catalog, got %+v", entries)
	}

	mgr := NewManager(t.TempDir())

	// Tampered checksum MUST fail install!
	bad := remote
	bad.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := mgr.Install(context.Background(), bad); err == nil {
		t.Fatal("expected tampered sha256 install to fail")
	}

	// Valid checksum installs cleanly!
	if err := mgr.Install(context.Background(), remote); err != nil {
		t.Fatalf("Install valid zip failed: %v", err)
	}
	ver, ok := mgr.IsInstalled("community.demo-tool")
	if !ok || ver != "1.2.0" {
		t.Fatalf("expected installed v1.2.0, got ok=%v ver=%q", ok, ver)
	}

	// Built-in starter plugin installs cleanly offline!
	starter := BuiltinStarterCatalog()[0]
	if err := mgr.Install(context.Background(), starter); err != nil {
		t.Fatalf("Install builtin starter failed: %v", err)
	}
	if _, ok := mgr.IsInstalled(starter.ID); !ok {
		t.Fatalf("expected starter %s to be installed", starter.ID)
	}

	// Uninstall cleans up directory & reloads!
	if err := mgr.Uninstall("community.demo-tool"); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if _, ok := mgr.IsInstalled("community.demo-tool"); ok {
		t.Fatal("plugin still reported installed after Uninstall")
	}
}
