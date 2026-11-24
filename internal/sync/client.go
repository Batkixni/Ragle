package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

// ClientConfig configures a desktop library sync run against any Raglex Sync API v1
// server (self-hosted Docker instance or Raglex Cloud).
type ClientConfig struct {
	ServerURL            string
	Token                string
	DeviceID             string
	ExcludedFolderIDs    map[string]bool // Folder IDs (and their nested descendants) skipped during push/upload
	ExcludeUncategorized bool            // Skip assets that do not belong to any folder
}

// SyncProgress reports live stage and item progress during a cloud sync.
type SyncProgress struct {
	Stage   string // "scanning", "checking", "uploading", "pushing", "pulling", "downloading", "done"
	Done    int
	Total   int
	Current string
}

// SyncReport summarizes what happened during a SyncNow round-trip.
type SyncReport struct {
	PushedOps       int    `json:"pushed_ops"`
	PulledOps       int    `json:"pulled_ops"`
	UploadedBlobs   int    `json:"uploaded_blobs"`
	DownloadedBlobs int    `json:"downloaded_blobs"`
	LatestSeq       uint64 `json:"latest_seq"`
}

type assetSyncPayload struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Hash        string     `json:"hash"`
	Mime        string     `json:"mime"`
	Rating      int        `json:"rating"`
	Notes       string     `json:"notes,omitempty"`
	SourceURL   string     `json:"source_url,omitempty"`
	SourceTitle string     `json:"source_title,omitempty"`
	IsBookmark  bool       `json:"is_bookmark,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Folders     [][]string `json:"folders,omitempty"`
}

// SyncNow executes a full bidirectional delta + content-addressed blob sync
// between local library (lib/projects) and the remote Raglex Sync API v1 server.
func SyncNow(ctx context.Context, cfg ClientConfig, lib *library.Library, projects *project.Store) (SyncReport, error) {
	return SyncNowWithProgress(ctx, cfg, lib, projects, nil)
}

// SyncNowWithProgress executes a concurrent bidirectional sync while emitting
// live progress snapshots via onProgress (if non-nil).
func SyncNowWithProgress(ctx context.Context, cfg ClientConfig, lib *library.Library, projects *project.Store, onProgress func(SyncProgress)) (SyncReport, error) {
	var rep SyncReport
	base := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if base == "" {
		return rep, fmt.Errorf("sync server URL is empty")
	}
	if lib == nil {
		return rep, fmt.Errorf("library is not open")
	}
	devID := strings.TrimSpace(cfg.DeviceID)
	if devID == "" {
		devID = "raglex-desktop"
	}

	emit := func(p SyncProgress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	emit(SyncProgress{Stage: "scanning"})
	localStore, err := OpenStore(lib.DB.Root())
	if err != nil {
		return rep, err
	}
	defer localStore.Close()

	// Pooled HTTP transport with HTTP/2 & keep-alive for fast multi-worker transfers.
	transport := &http.Transport{
		MaxIdleConns:        16,
		MaxIdleConnsPerHost: 12,
		MaxConnsPerHost:     8,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	defer transport.CloseIdleConnections()

	metaHC := &http.Client{Transport: transport, Timeout: 60 * time.Second}
	blobHC := &http.Client{Transport: transport, Timeout: 15 * time.Minute}
	doReq := func(reqCtx context.Context, method, path string, body io.Reader, ctype string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(reqCtx, method, base+path, body)
		if err != nil {
			return nil, err
		}
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		if strings.HasPrefix(path, "/v1/blobs/") {
			return blobHC.Do(req)
		}
		return metaHC.Do(req)
	}

	// 1. Load entire catalog in ONE single bbolt read transaction (zero N+1 queries!).
	snap, err := lib.LoadCatalogSnapshot()
	if err != nil {
		return rep, err
	}
	folderByID := make(map[string]model.Folder, len(snap.Folders))
	for _, f := range snap.Folders {
		folderByID[f.ID] = f
	}
	folderPathOf := func(fid string) []string {
		var rev []string
		seen := map[string]bool{}
		cur := fid
		for cur != "" && !seen[cur] {
			seen[cur] = true
			f, ok := folderByID[cur]
			if !ok {
				break
			}
			rev = append(rev, f.Name)
			cur = f.ParentID
		}
		for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
			rev[i], rev[j] = rev[j], rev[i]
		}
		return rev
	}

	isFolderExcluded := func(fid string) bool {
		if len(cfg.ExcludedFolderIDs) == 0 {
			return false
		}
		seen := map[string]bool{}
		cur := fid
		for cur != "" && !seen[cur] {
			seen[cur] = true
			if cfg.ExcludedFolderIDs[cur] {
				return true
			}
			f, ok := folderByID[cur]
			if !ok {
				break
			}
			cur = f.ParentID
		}
		return false
	}

	hashToPath := map[string]string{}
	hashToName := map[string]string{}
	var allHashes []string
	pushOps := make([]SyncOp, 0, len(snap.Assets))

	for _, a := range snap.Assets {
		flds := snap.AssetFolders[a.ID]
		if len(flds) == 0 {
			if cfg.ExcludeUncategorized {
				continue
			}
		} else if len(cfg.ExcludedFolderIDs) > 0 {
			anyIncluded := false
			for _, fl := range flds {
				if !isFolderExcluded(fl.ID) {
					anyIncluded = true
					break
				}
			}
			if !anyIncluded {
				continue
			}
		}

		h := strings.ToLower(strings.TrimSpace(a.Hash))
		if _, err := ValidateSHA256(h); err == nil && a.Path != "" && !a.Missing {
			if _, seen := hashToPath[h]; !seen {
				hashToPath[h] = a.Path
				hashToName[h] = a.Label()
				allHashes = append(allHashes, h)
			}
		}
		tgs := snap.AssetTags[a.ID]
		tagNames := make([]string, 0, len(tgs))
		for _, tg := range tgs {
			tagNames = append(tagNames, tg.Name)
		}
		var folderPaths [][]string
		for _, fl := range flds {
			if isFolderExcluded(fl.ID) {
				continue
			}
			if fp := folderPathOf(fl.ID); len(fp) > 0 {
				folderPaths = append(folderPaths, fp)
			}
		}
		pl := assetSyncPayload{
			ID:          a.ID,
			Name:        a.Label(),
			Hash:        h,
			Mime:        a.Mime,
			Rating:      a.Rating,
			Notes:       a.Notes,
			SourceURL:   a.SourceURL,
			SourceTitle: a.SourceTitle,
			IsBookmark:  a.IsBookmark,
			Tags:        tagNames,
			Folders:     folderPaths,
		}
		rawPL, _ := json.Marshal(pl)
		// Deterministic OpID from payload hash makes pushes 100% idempotent!
		opSum := sha256.Sum256(rawPL)
		pushOps = append(pushOps, SyncOp{
			OpID:      "asset:" + hex.EncodeToString(opSum[:16]),
			DeviceID:  devID,
			Timestamp: time.Now().UnixMilli(),
			Kind:      "asset.upsert",
			EntityID:  a.ID,
			Payload:   rawPL,
		})
	}

	// 2. Check missing blobs & upload concurrently (up to 6 parallel workers!).
	if len(allHashes) > 0 {
		emit(SyncProgress{Stage: "checking", Done: 0, Total: len(allHashes)})
		chkBody, _ := json.Marshal(map[string]any{"hashes": allHashes})
		resp, err := doReq(ctx, http.MethodPost, "/v1/blobs/check", bytes.NewReader(chkBody), "application/json")
		if err != nil {
			return rep, err
		}
		var chkOut struct {
			Missing []string `json:"missing"`
			Error   string   `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&chkOut)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if chkOut.Error != "" {
				return rep, fmt.Errorf("sync check failed: %s", chkOut.Error)
			}
			return rep, fmt.Errorf("sync check failed: HTTP %d", resp.StatusCode)
		}

		totalMissing := len(chkOut.Missing)
		if totalMissing > 0 {
			emit(SyncProgress{Stage: "uploading", Done: 0, Total: totalMissing})
			upCtx, cancelUp := context.WithCancel(ctx)
			defer cancelUp()

			jobs := make(chan string, totalMissing)
			for _, mh := range chkOut.Missing {
				jobs <- mh
			}
			close(jobs)

			workers := 6
			if totalMissing < workers {
				workers = totalMissing
			}

			var (
				wg        sync.WaitGroup
				doneCount atomic.Int64
				okCount   atomic.Int64
				firstErr  error
				errOnce   sync.Once
			)
			setErr := func(e error) {
				errOnce.Do(func() {
					firstErr = e
					cancelUp()
				})
			}

			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for mh := range jobs {
						if upCtx.Err() != nil {
							return
						}
						p := hashToPath[mh]
						if p == "" {
							d := int(doneCount.Add(1))
							emit(SyncProgress{Stage: "uploading", Done: d, Total: totalMissing})
							continue
						}
						f, err := os.Open(p)
						if err != nil {
							d := int(doneCount.Add(1))
							emit(SyncProgress{Stage: "uploading", Done: d, Total: totalMissing})
							continue
						}
						upResp, err := doReq(upCtx, http.MethodPut, "/v1/blobs/"+mh, f, "application/octet-stream")
						_ = f.Close()
						if err != nil {
							if upCtx.Err() == nil {
								setErr(err)
							}
							return
						}
						var upOut struct {
							Error string `json:"error"`
						}
						_ = json.NewDecoder(io.LimitReader(upResp.Body, 16<<10)).Decode(&upOut)
						_, _ = io.Copy(io.Discard, upResp.Body)
						_ = upResp.Body.Close()

						if upResp.StatusCode == http.StatusOK {
							okCount.Add(1)
						} else if upResp.StatusCode == http.StatusRequestEntityTooLarge || upResp.StatusCode == http.StatusUnauthorized {
							if upOut.Error != "" {
								setErr(fmt.Errorf("blob upload failed: %s", upOut.Error))
							} else {
								setErr(fmt.Errorf("blob upload failed: HTTP %d", upResp.StatusCode))
							}
							return
						}
						d := int(doneCount.Add(1))
						emit(SyncProgress{Stage: "uploading", Done: d, Total: totalMissing, Current: hashToName[mh]})
					}
				}()
			}
			wg.Wait()
			if firstErr != nil {
				return rep, firstErr
			}
			rep.UploadedBlobs = int(okCount.Load())
		}
	}

	// 3. Push local ops idempotently in batches.
	if len(pushOps) > 0 {
		emit(SyncProgress{Stage: "pushing", Done: 0, Total: len(pushOps)})
		const opBatch = 400
		for start := 0; start < len(pushOps); start += opBatch {
			end := min(start+opBatch, len(pushOps))
			pushBody, _ := json.Marshal(map[string]any{"ops": pushOps[start:end]})
			resp, err := doReq(ctx, http.MethodPost, "/v1/sync/push", bytes.NewReader(pushBody), "application/json")
			if err != nil {
				return rep, err
			}
			var pushOut struct {
				LatestSeq uint64 `json:"latest_seq"`
				Accepted  int    `json:"accepted"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&pushOut)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return rep, fmt.Errorf("sync push failed: HTTP %d", resp.StatusCode)
			}
			rep.PushedOps += pushOut.Accepted
			rep.LatestSeq = pushOut.LatestSeq
			emit(SyncProgress{Stage: "pushing", Done: end, Total: len(pushOps)})
		}
	}

	// 4. Pull remote ops since last cursor and apply missing assets + metadata locally.
	emit(SyncProgress{Stage: "pulling"})
	cursorKey := base
	since := localStore.GetCursor(cursorKey)
	localByHash := map[string]model.Asset{}
	for _, a := range snap.Assets {
		if a.Hash != "" {
			localByHash[strings.ToLower(a.Hash)] = a
		}
	}
	tagByName := map[string]string{}
	for _, tg := range snap.Tags {
		tagByName[strings.ToLower(tg.Name)] = tg.ID
	}

	for {
		resp, err := doReq(ctx, http.MethodGet, fmt.Sprintf("/v1/sync/pull?since=%d&limit=500", since), nil, "")
		if err != nil {
			return rep, err
		}
		var pullOut struct {
			Ops       []SyncOp `json:"ops"`
			LatestSeq uint64   `json:"latest_seq"`
			HasMore   bool     `json:"has_more"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&pullOut)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return rep, fmt.Errorf("sync pull failed: HTTP %d", resp.StatusCode)
		}
		if decErr != nil {
			return rep, decErr
		}
		rep.LatestSeq = pullOut.LatestSeq

		for idx, op := range pullOut.Ops {
			if op.Seq > since {
				since = op.Seq
			}
			if op.DeviceID == devID {
				continue
			}
			if op.Kind == "asset.upsert" {
				var pl assetSyncPayload
				if err := json.Unmarshal(op.Payload, &pl); err != nil {
					continue
				}
				rep.PulledOps++
				var target model.Asset
				if ex, ok := localByHash[strings.ToLower(pl.Hash)]; ok {
					target = ex
				} else if pl.Hash != "" {
					emit(SyncProgress{Stage: "downloading", Done: idx + 1, Total: len(pullOut.Ops), Current: pl.Name})
					bResp, err := doReq(ctx, http.MethodGet, "/v1/blobs/"+pl.Hash, nil, "")
					if err != nil {
						continue
					}
					data, rerr := io.ReadAll(io.LimitReader(bResp.Body, maxBlobUploadBytes))
					_ = bResp.Body.Close()
					if rerr != nil || bResp.StatusCode != http.StatusOK || len(data) == 0 {
						continue
					}
					name := pl.Name
					if filepath.Ext(name) == "" {
						name = name + ".bin"
					}
					imported, _, ierr := lib.ImportBytes(name, data, nil, pl.SourceURL, pl.SourceTitle)
					if ierr != nil {
						continue
					}
					target = imported
					localByHash[strings.ToLower(pl.Hash)] = imported
					rep.DownloadedBlobs++
				}
				if target.ID != "" {
					if pl.Rating > 0 && target.Rating != pl.Rating {
						_ = lib.SetRating(target.ID, pl.Rating)
					}
					if pl.Notes != target.Notes || pl.SourceURL != target.SourceURL || pl.SourceTitle != target.SourceTitle {
						_, _ = lib.UpdateAssetMeta(target.ID, pl.Notes, pl.SourceURL, pl.SourceTitle)
					}
					for _, tn := range pl.Tags {
						key := strings.ToLower(strings.TrimSpace(tn))
						if key == "" {
							continue
						}
						tid, ok := tagByName[key]
						if !ok {
							if created, cerr := lib.CreateTag(tn); cerr == nil {
								tid = created.ID
								tagByName[key] = tid
							}
						}
						if tid != "" {
							_ = lib.SetAssetTag(target.ID, tid, true)
						}
					}
					for _, fp := range pl.Folders {
						if fid := lib.EnsureFolderPath("", fp); fid != "" {
							_ = lib.SetAssetFolder(target.ID, fid, true)
						}
					}
				}
			}
		}
		if !pullOut.HasMore || len(pullOut.Ops) == 0 {
			break
		}
	}
	if since > 0 {
		_ = localStore.SetCursor(cursorKey, since)
	}
	emit(SyncProgress{Stage: "done", Done: rep.UploadedBlobs + rep.DownloadedBlobs, Total: rep.UploadedBlobs + rep.DownloadedBlobs})
	return rep, nil
}
