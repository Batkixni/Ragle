package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/config"
	"raglex/internal/db"
	"raglex/internal/share"
)

// PublicShareResult is returned after publishing a Folder or Project to Raglex Cloud.
type PublicShareResult struct {
	Code        string `json:"code"`
	URL         string `json:"url"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	AssetCount  int    `json:"asset_count"`
	FolderCount int    `json:"folder_count"`
	FileCount   int    `json:"file_count"`
}

// DownloadShareResult summarizes a completed anonymous or signed-in Cloud Public Share import.
type DownloadShareResult struct {
	Code         string
	Kind         string // "folder" or "project"
	TargetID     string // newly created/resolved folderID or projectID
	Title        string
	AssetsCount  int
	FoldersCount int
	Downloaded   int
}

// NormalizePublicShareCode extracts & normalizes a Cloud Public Share code
// from either raw input ("shr-abcd-1234") or a full URL ("https://.../s/SHR-ABCD-1234").
func NormalizePublicShareCode(raw string) string {
	s := strings.TrimSpace(raw)
	if idx := strings.LastIndex(s, "/s/"); idx >= 0 {
		s = s[idx+3:]
	}
	if q := strings.IndexAny(s, "?#"); q >= 0 {
		s = s[:q]
	}
	s = strings.Trim(s, "/ \t\r\n")
	return strings.ToUpper(s)
}

// IsPublicShareCode returns true when raw looks like a Cloud Public Share code
// (e.g. "SHR-XXXX-XXXX") or a /s/SHR-... URL.
func IsPublicShareCode(raw string) bool {
	code := NormalizePublicShareCode(raw)
	return strings.HasPrefix(code, "SHR-") && len(code) >= 8
}

// PublishPublicShare exports a Folder (including all nested subfolders) or a Project,
// uploads any missing files concurrently to Raglex Cloud, and mints a public share code
// that ANY Raglex user (even without an account) can download.
func PublishPublicShare(
	ctx context.Context,
	cfg ClientConfig,
	database *db.DB,
	kind string,
	targetID string,
	includeFiles bool,
	onProgress func(SyncProgress),
) (PublicShareResult, error) {
	var out PublicShareResult
	if database == nil {
		return out, errors.New("library database is not open")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if base == "" {
		base = config.DefaultCloudEndpoint
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return out, errors.New("請先登入 Raglex Cloud 帳號才能建立公開分享碼")
	}

	report := func(stage string, done, total int, cur string) {
		if onProgress != nil {
			onProgress(SyncProgress{Stage: stage, Done: done, Total: total, Current: cur})
		}
	}

	report("scanning", 0, 0, "")
	var (
		bundle share.Bundle
		local  map[string]string
		err    error
	)
	if kind == "folder" {
		bundle, local, err = share.ExportFolder(database, targetID, includeFiles)
	} else {
		bundle, local, err = share.Export(database, targetID, includeFiles)
	}
	if err != nil {
		return out, err
	}

	client := newSyncHTTPClient()
	doReq := func(method, endpoint string, body io.Reader, contentType string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, base+endpoint, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		return client.Do(req)
	}

	// Upload any missing blobs concurrently (already-synced files are skipped instantly!).
	if includeFiles && len(bundle.Files) > 0 {
		hashes := make([]string, 0, len(bundle.Files))
		for _, f := range bundle.Files {
			hashes = append(hashes, f.Hash)
		}
		checkBody, _ := json.Marshal(map[string]any{"hashes": hashes})
		resp, err := doReq(http.MethodPost, "/v1/blobs/check", bytes.NewReader(checkBody), "application/json")
		if err != nil {
			return out, fmt.Errorf("connect cloud: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			msg := readCloudErr(resp)
			resp.Body.Close()
			return out, errors.New(msg)
		}
		var checkOut struct {
			Missing []string `json:"missing"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&checkOut)
		resp.Body.Close()

		if len(checkOut.Missing) > 0 {
			totalUp := len(checkOut.Missing)
			report("uploading", 0, totalUp, "")
			const maxWorkers = 6
			workers := maxWorkers
			if totalUp < workers {
				workers = totalUp
			}
			jobs := make(chan string, totalUp)
			for _, h := range checkOut.Missing {
				jobs <- h
			}
			close(jobs)

			var doneCount atomic.Int64
			var firstErr error
			var errOnce sync.Once
			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for h := range jobs {
						if ctx.Err() != nil {
							return
						}
						path := local[h]
						if path == "" {
							continue
						}
						f, ferr := os.Open(path)
						if ferr != nil {
							errOnce.Do(func() { firstErr = ferr })
							return
						}
						fi, _ := f.Stat()
						req, rerr := http.NewRequestWithContext(ctx, http.MethodPut, base+"/v1/blobs/"+h, f)
						if rerr != nil {
							f.Close()
							errOnce.Do(func() { firstErr = rerr })
							return
						}
						req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
						req.Header.Set("Content-Type", "application/octet-stream")
						if fi != nil {
							req.ContentLength = fi.Size()
						}
						upResp, uerr := client.Do(req)
						f.Close()
						if uerr != nil {
							errOnce.Do(func() { firstErr = uerr })
							return
						}
						if upResp.StatusCode < 200 || upResp.StatusCode >= 300 {
							msg := readCloudErr(upResp)
							upResp.Body.Close()
							errOnce.Do(func() { firstErr = errors.New(msg) })
							return
						}
						upResp.Body.Close()
						n := int(doneCount.Add(1))
						report("uploading", n, totalUp, filepath.Base(path))
					}
				}()
			}
			wg.Wait()
			if firstErr != nil {
				return out, firstErr
			}
		}
	}

	report("pushing", 0, 1, bundle.DisplayName())
	pubBody, err := json.Marshal(map[string]any{"bundle": bundle})
	if err != nil {
		return out, err
	}
	resp, err := doReq(http.MethodPost, "/api/v1/public-shares", bytes.NewReader(pubBody), "application/json")
	if err != nil {
		return out, fmt.Errorf("publish share: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, errors.New(readCloudErr(resp))
	}
	var pubResp struct {
		OK    bool   `json:"ok"`
		Code  string `json:"code"`
		URL   string `json:"url"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pubResp); err != nil {
		return out, err
	}
	if !pubResp.OK || pubResp.Code == "" {
		if pubResp.Error != "" {
			return out, errors.New(pubResp.Error)
		}
		return out, errors.New("publish failed")
	}
	out = PublicShareResult{
		Code:        pubResp.Code,
		URL:         pubResp.URL,
		Kind:        bundle.Kind,
		Title:       bundle.DisplayName(),
		AssetCount:  len(bundle.Assets),
		FolderCount: len(bundle.Folders),
		FileCount:   len(bundle.Files),
	}
	report("done", 1, 1, out.Code)
	return out, nil
}

// DownloadPublicShare downloads a public Folder or Project from Raglex Cloud using
// ONLY the share code — NO registration, login, or token is required!
func DownloadPublicShare(
	ctx context.Context,
	serverURL string,
	rawCode string,
	database *db.DB,
	onProgress func(SyncProgress),
) (DownloadShareResult, error) {
	var out DownloadShareResult
	if database == nil {
		return out, errors.New("library database is not open")
	}
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if base == "" {
		base = config.DefaultCloudEndpoint
	}
	code := NormalizePublicShareCode(rawCode)
	if code == "" {
		return out, errors.New("請輸入有效的公開分享碼")
	}

	report := func(stage string, done, total int, cur string) {
		if onProgress != nil {
			onProgress(SyncProgress{Stage: stage, Done: done, Total: total, Current: cur})
		}
	}

	client := newSyncHTTPClient()
	report("pulling", 0, 0, code)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/public-shares/"+code, nil)
	if err != nil {
		return out, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, fmt.Errorf("無法連線至 Raglex Cloud：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, errors.New(readCloudErr(resp))
	}
	var payload struct {
		OK     bool         `json:"ok"`
		Bundle share.Bundle `json:"bundle"`
		Error  string       `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&payload); err != nil {
		return out, err
	}
	if !payload.OK {
		if payload.Error != "" {
			return out, errors.New(payload.Error)
		}
		return out, errors.New("分享碼無效或已失效")
	}

	bundle := payload.Bundle
	// Check which files are already in our local library so we don't re-download duplicates.
	existingHashes := map[string]bool{}
	_ = database.View(func(tx *bbolt.Tx) error {
		for _, f := range bundle.Files {
			if _, ok := db.AssetIDByHash(tx, f.Hash); ok {
				existingHashes[f.Hash] = true
			}
		}
		return nil
	})

	var toDownload []share.FileSnap
	for _, f := range bundle.Files {
		if !existingHashes[f.Hash] {
			toDownload = append(toDownload, f)
		}
	}

	paths := map[string]string{}
	var pathsMu sync.Mutex
	var savedFiles []string

	if len(toDownload) > 0 {
		totalDl := len(toDownload)
		report("downloading", 0, totalDl, "")
		const maxWorkers = 6
		workers := maxWorkers
		if totalDl < workers {
			workers = totalDl
		}
		jobs := make(chan share.FileSnap, totalDl)
		for _, f := range toDownload {
			jobs <- f
		}
		close(jobs)

		var doneCount atomic.Int64
		var firstErr error
		var errOnce sync.Once
		var wg sync.WaitGroup
		filesDir := database.FilesDir()

		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for f := range jobs {
					if ctx.Err() != nil {
						return
					}
					dReq, rerr := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/public-shares/"+code+"/blobs/"+f.Hash, nil)
					if rerr != nil {
						errOnce.Do(func() { firstErr = rerr })
						return
					}
					dResp, derr := client.Do(dReq)
					if derr != nil {
						errOnce.Do(func() { firstErr = derr })
						return
					}
					if dResp.StatusCode != http.StatusOK {
						msg := readCloudErr(dResp)
						dResp.Body.Close()
						errOnce.Do(func() { firstErr = errors.New(msg) })
						return
					}
					dest, serr := share.SaveFile(filesDir, f.Name, dResp.Body)
					dResp.Body.Close()
					if serr != nil {
						errOnce.Do(func() { firstErr = serr })
						return
					}
					pathsMu.Lock()
					paths[f.Hash] = dest
					savedFiles = append(savedFiles, dest)
					pathsMu.Unlock()

					n := int(doneCount.Add(1))
					report("downloading", n, totalDl, f.Name)
				}
			}()
		}
		wg.Wait()
		if firstErr != nil {
			for _, p := range savedFiles {
				_ = os.Remove(p)
			}
			return out, firstErr
		}
	}

	targetID, err := share.Apply(database, bundle, paths)
	if err != nil {
		for _, p := range savedFiles {
			_ = os.Remove(p)
		}
		return out, err
	}

	kind := bundle.Kind
	if kind == "" {
		kind = "project"
	}
	out = DownloadShareResult{
		Code:         code,
		Kind:         kind,
		TargetID:     targetID,
		Title:        bundle.DisplayName(),
		AssetsCount:  len(bundle.Assets),
		FoldersCount: len(bundle.Folders),
		Downloaded:   len(paths),
	}
	report("done", len(toDownload), len(toDownload), out.Title)
	return out, nil
}

// RevokePublicShare revokes a previously published public share code (owner only).
func RevokePublicShare(ctx context.Context, cfg ClientConfig, rawCode string) error {
	base := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if base == "" {
		base = config.DefaultCloudEndpoint
	}
	code := NormalizePublicShareCode(rawCode)
	if code == "" || strings.TrimSpace(cfg.Token) == "" {
		return errors.New("missing share code or auth token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/v1/public-shares/"+code, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	resp, err := newSyncHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New(readCloudErr(resp))
	}
	return nil
}

func newSyncHTTPClient() *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        16,
		MaxIdleConnsPerHost: 12,
		MaxConnsPerHost:     8,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &http.Client{Transport: tr, Timeout: 15 * time.Minute}
}

func readCloudErr(resp *http.Response) string {
	var body struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&body) == nil && body.Error != "" {
		return body.Error
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode)
}
