package ffmpeg

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func makeDummyBin(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho ffmpeg-dummy-binary\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDetectPriorityBundledManagedSystem(t *testing.T) {
	origExe := execPathFn
	origCfg := userConfigDirFn
	origLook := lookPathFn
	defer func() {
		execPathFn = origExe
		userConfigDirFn = origCfg
		lookPathFn = origLook
		ResetCache()
	}()

	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "app")
	cfgDir := filepath.Join(tmp, "cfg")
	fakeExe := filepath.Join(appDir, binaryName("Raglex", runtime.GOOS))
	makeDummyBin(t, fakeExe)

	execPathFn = func() (string, error) { return fakeExe, nil }
	userConfigDirFn = func() (string, error) { return cfgDir, nil }
	lookPathFn = func(file string) (string, error) { return "/usr/bin/" + file, nil }

	// 1. Only system PATH exists.
	ResetCache()
	p, src := Detect()
	if src != SourceSystem || p != "/usr/bin/ffmpeg" {
		t.Fatalf("expected system fallback, got %q (%q)", p, src)
	}

	// 2. Managed binary overrides system PATH.
	managedBin := filepath.Join(cfgDir, "Raglex", "bin", binaryName("ffmpeg", runtime.GOOS))
	makeDummyBin(t, managedBin)
	ResetCache()
	p, src = Detect()
	if src != SourceManaged || p != managedBin {
		t.Fatalf("expected managed binary %q, got %q (%q)", managedBin, p, src)
	}

	// 3. Bundled binary next to executable overrides managed & system.
	bundledBin := filepath.Join(appDir, binaryName("ffmpeg", runtime.GOOS))
	makeDummyBin(t, bundledBin)
	ResetCache()
	p, src = Detect()
	if src != SourceBundled || p != bundledBin {
		t.Fatalf("expected bundled binary %q, got %q (%q)", bundledBin, p, src)
	}

	// 4. macOS .app/Contents/Resources/ffmpeg layout works too.
	macExe := filepath.Join(tmp, "Raglex.app", "Contents", "MacOS", "Raglex")
	macResBin := filepath.Join(tmp, "Raglex.app", "Contents", "Resources", binaryName("ffmpeg", runtime.GOOS))
	makeDummyBin(t, macExe)
	makeDummyBin(t, macResBin)
	execPathFn = func() (string, error) { return macExe, nil }
	ResetCache()
	p, src = Detect()
	if src != SourceBundled || p != macResBin {
		t.Fatalf("expected macOS Resources bundled binary %q, got %q (%q)", macResBin, p, src)
	}

	// 5. None available.
	execPathFn = func() (string, error) { return filepath.Join(tmp, "empty", "Raglex"), nil }
	userConfigDirFn = func() (string, error) { return filepath.Join(tmp, "empty-cfg"), nil }
	lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
	ResetCache()
	p, src = Detect()
	if src != SourceNone || p != "" || Available() {
		t.Fatalf("expected unavailable, got %q (%q)", p, src)
	}
}

func TestInstallFromSpecsGzipAndZip(t *testing.T) {
	origExe := execPathFn
	origCfg := userConfigDirFn
	origLook := lookPathFn
	origClient := httpClient
	defer func() {
		execPathFn = origExe
		userConfigDirFn = origCfg
		lookPathFn = origLook
		httpClient = origClient
		ResetCache()
	}()

	payload := []byte("FAKE-FFMPEG-BINARY-PAYLOAD-0123456789")

	// Prepare gzip payload.
	var gzBuf bytes.Buffer
	gzw := gzip.NewWriter(&gzBuf)
	_, _ = gzw.Write(payload)
	_ = gzw.Close()

	// Prepare zip payload with nested bin/ffmpeg(.exe).
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	fw, err := zw.Create("ffmpeg-master-winarm64/bin/" + binaryName("ffmpeg", runtime.GOOS))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(payload)
	_ = zw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ffmpeg.gz":
			_, _ = w.Write(gzBuf.Bytes())
		case "/ffmpeg.zip":
			_, _ = w.Write(zipBuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	execPathFn = func() (string, error) { return filepath.Join(tmp, "empty", "Raglex"), nil }
	userConfigDirFn = func() (string, error) { return filepath.Join(tmp, "cfg"), nil }
	lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
	httpClient = srv.Client()

	// Test gzip download.
	ResetCache()
	var progressCalls int
	installed, err := installFromSpecs(context.Background(), []DownloadSpec{
		{URL: srv.URL + "/ffmpeg.gz", Format: "gz"},
	}, func(done, total int64) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("gzip install failed: %v", err)
	}
	if progressCalls == 0 {
		t.Fatalf("expected progress callback to fire")
	}
	got, err := os.ReadFile(installed)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("unexpected installed payload: err=%v len=%d", err, len(got))
	}
	p, src := Detect()
	if src != SourceManaged || p != installed {
		t.Fatalf("Detect after install: got %q (%q)", p, src)
	}

	// Test zip download overwriting managed binary.
	installedZip, err := installFromSpecs(context.Background(), []DownloadSpec{
		{URL: srv.URL + "/ffmpeg.zip", Format: "zip"},
	}, nil)
	if err != nil {
		t.Fatalf("zip install failed: %v", err)
	}
	gotZip, err := os.ReadFile(installedZip)
	if err != nil || !bytes.Equal(gotZip, payload) {
		t.Fatalf("unexpected zip installed payload: err=%v len=%d", err, len(gotZip))
	}
}
