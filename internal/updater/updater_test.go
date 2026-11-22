package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.2.0-beta.1", "0.2.0-beta.1", 0},
		{"v0.2.0-beta.2", "v0.2.0-beta.1", 1},
		{"v0.2.0-beta.10", "v0.2.0-beta.2", 1},
		{"v0.2.0", "v0.2.0-beta.9", 1},
		{"v0.2.0-beta.1", "v0.2.0", -1},
		{"v0.2.1", "v0.2.0", 1},
		{"v1.0.0", "v0.9.9", 1},
	}
	for _, tc := range cases {
		if got := CompareVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersion(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMatchAsset(t *testing.T) {
	assets := []string{
		"Ragle-windows-amd64.zip",
		"Ragle-windows-amd64.exe",
		"ragle-cli-windows-amd64.zip",
		"ragle-browser-extension.zip",
		"Ragle-darwin-arm64.zip",
		"SHA256SUMS.txt",
	}
	if got := MatchAsset(assets, "windows", "amd64"); got != "Ragle-windows-amd64.zip" {
		t.Fatalf("windows/amd64 matched %q, want Ragle-windows-amd64.zip", got)
	}
	if got := MatchAsset(assets, "darwin", "arm64"); got != "Ragle-darwin-arm64.zip" {
		t.Fatalf("darwin/arm64 matched %q, want Ragle-darwin-arm64.zip", got)
	}
}

func TestCheckAndApplyZipWithChecksum(t *testing.T) {
	// Build an in-memory release zip containing a fake updated Ragle binary.
	binName := "Ragle"
	if runtime.GOOS == "windows" {
		binName = "Ragle.exe"
	}
	newBinary := []byte("RAGLE_UPDATED_BINARY_V0_2_0_BETA_2")

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	fw, err := zw.Create(binName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(newBinary); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zipBytes := zipBuf.Bytes()
	sum := sha256.Sum256(zipBytes)
	hexSum := hex.EncodeToString(sum[:])

	assetName := fmt.Sprintf("Ragle-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	badChecksum := false

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Batkixni/Ragle/releases":
			rels := []ghRelease{
				{
					TagName:    "v0.2.0-beta.2",
					Name:       "Ragle v0.2.0-beta.2",
					Body:       "Smooth sidebar transitions and compact drag previews.",
					HTMLURL:    "https://github.com/Batkixni/Ragle/releases/tag/v0.2.0-beta.2",
					Prerelease: true,
					Assets: []ghAsset{
						{
							Name:               assetName,
							Size:               int64(len(zipBytes)),
							BrowserDownloadURL: srv.URL + "/download/" + assetName,
						},
						{
							Name:               "SHA256SUMS.txt",
							BrowserDownloadURL: srv.URL + "/download/SHA256SUMS.txt",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(rels)
		case "/download/" + assetName:
			_, _ = w.Write(zipBytes)
		case "/download/SHA256SUMS.txt":
			if badChecksum {
				_, _ = fmt.Fprintf(w, "%s  %s\n", "0000000000000000000000000000000000000000000000000000000000000000", assetName)
			} else {
				_, _ = fmt.Fprintf(w, "%s  %s\n", hexSum, assetName)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := &Client{
		Repo:              "Batkixni/Ragle",
		CurrentVersion:    "0.2.0-beta.1",
		IncludePrerelease: true,
		APIBase:           srv.URL,
		HTTPClient:        srv.Client(),
	}

	rel, newer, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !newer || rel.TagName != "v0.2.0-beta.2" {
		t.Fatalf("expected newer v0.2.0-beta.2, got newer=%v tag=%q", newer, rel.TagName)
	}

	tmpDir := t.TempDir()
	targetExe := filepath.Join(tmpDir, binName)
	if err := os.WriteFile(targetExe, []byte("OLD_BINARY"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Tampered checksum must be rejected without clobbering targetExe.
	badChecksum = true
	if err := client.DownloadAndApplyTo(context.Background(), rel, targetExe, nil); err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	got, _ := os.ReadFile(targetExe)
	if string(got) != "OLD_BINARY" {
		t.Fatalf("targetExe modified on checksum error: %q", string(got))
	}

	// Valid checksum installs atomically.
	badChecksum = false
	var sawDone bool
	if err := client.DownloadAndApplyTo(context.Background(), rel, targetExe, func(p Progress) {
		if p.Stage == "done" {
			sawDone = true
		}
	}); err != nil {
		t.Fatalf("DownloadAndApplyTo failed: %v", err)
	}
	if !sawDone {
		t.Fatal("expected progress stage 'done'")
	}
	got, err = os.ReadFile(targetExe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Fatalf("installed binary mismatch: got %q", string(got))
	}
}
