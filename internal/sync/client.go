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
	E2EEPassphrase       string          // Optional client-side zero-knowledge AES-256-GCM passphrase (never leaves device)
	ExcludedFolderIDs    map[string]bool // Folder IDs (and their nested descendants) skipped during push/upload
	ExcludeUncategorized bool            // Skip assets that do not belong to any folder
	PullOnly             bool            // Skip local catalog scan / blob check / push; only poll /v1/sync/pull
}

func remoteScopeKey(base, token, e2eePass string) string {
	sum := sha256.Sum256([]byte(base + "\n" + strings.TrimSpace(token) + "\n" + strings.TrimSpace(e2eePass)))
	return hex.EncodeToString(sum[:8])
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
	PushedOps        int    `json:"pushed_ops"`
	PulledOps        int    `json:"pulled_ops"`
	UploadedBlobs    int    `json:"uploaded_blobs"`
	DownloadedBlobs  int    `json:"downloaded_blobs"`
	DecryptFailedOps int    `json:"decrypt_failed_ops,omitempty"`
	LatestSeq        uint64 `json:"latest_seq"`
}

type assetSyncPayload struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Ext         string     `json:"ext,omitempty"`
	Hash        string     `json:"hash"`
	BlobHash    string     `json:"blob_hash,omitempty"` // Ciphertext SHA-256 on server when E2EE is enabled
	Mime        string     `json:"mime"`
	Rating      int        `json:"rating"`
	Notes       string     `json:"notes,omitempty"`
	SourceURL   string     `json:"source_url,omitempty"`
	SourceTitle string     `json:"source_title,omitempty"`
	IsBookmark  bool       `json:"is_bookmark,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Folders     [][]string `json:"folders,omitempty"`
}

func extFromMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "image/avif":
		return ".avif"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "application/pdf":
		return ".pdf"
	case "text/markdown":
		return ".md"
	case "text/plain":
		return ".txt"
	default:
		return ".png"
	}
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

	devID := strings.TrimSpace(cfg.DeviceID)
	if devID == "" {
		devID = localStore.EnsureDeviceID()
	}

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

	scope := remoteScopeKey(base, cfg.Token, cfg.E2EEPassphrase)
	e2eeKey, err := deriveE2EEKey(cfg.E2EEPassphrase)
	if err != nil {
		return rep, fmt.Errorf("sync e2ee key derivation failed: %w", err)
	}
	var (
		snapLoaded  bool
		snap        library.CatalogSnapshot
		localByHash map[string]model.Asset
		tagByName   map[string]string
	)
	ensureSnap := func() error {
		if snapLoaded {
			return nil
		}
		var err error
		snap, err = lib.LoadCatalogSnapshot()
		if err != nil {
			return err
		}
		localByHash = make(map[string]model.Asset, len(snap.Assets))
		for _, a := range snap.Assets {
			if a.Hash != "" {
				localByHash[strings.ToLower(a.Hash)] = a
			}
		}
		tagByName = make(map[string]string, len(snap.Tags))
		for _, tg := range snap.Tags {
			tagByName[strings.ToLower(tg.Name)] = tg.ID
		}
		snapLoaded = true
		return nil
	}

	// When PullOnly is false, compute ONLY the delta against locally cached
	// confirmed OpIDs & SHA-256 hashes so unchanged items send zero bytes.
	if !cfg.PullOnly {
		// 1. Load entire catalog in ONE single bbolt read transaction (zero N+1 queries!).
		if err := ensureSnap(); err != nil {
			return rep, err
		}
		knownOps, knownHashes := localStore.LoadRemoteKnown(scope)

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

		type encBlobRef struct {
			path      string
			plainHash string
		}
		hashToPath := map[string]string{}
		hashToName := map[string]string{}
		encHashToPlain := map[string]string{}
		encHashToFile := map[string]encBlobRef{}
		var plainToCT map[string]string
		var newE2EEHashes map[string]string
		if len(e2eeKey) > 0 {
			plainToCT = localStore.LoadE2EEBlobHashes(scope)
			newE2EEHashes = map[string]string{}
		}
		var candidateHashes []string
		pushOps := make([]SyncOp, 0, 16)

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
			validHash := false
			if _, err := ValidateSHA256(h); err == nil && a.Path != "" && !a.Missing {
				validHash = true
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
				Ext:         filepath.Ext(a.Path),
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
			// Deterministic OpID from payload hash makes pushes 100% idempotent
			// and lets us skip unchanged assets locally with zero disk/network work!
			opSum := sha256.Sum256(rawPL)
			opPrefix := "asset:"
			if len(e2eeKey) > 0 {
				opPrefix = "e2ee:"
			}
			opID := opPrefix + hex.EncodeToString(opSum[:16])

			needBlob := validHash && !knownHashes[h]
			needOp := !knownOps[opID]
			if !needBlob && !needOp {
				continue
			}

			blobHash := h
			if len(e2eeKey) > 0 && validHash {
				ctHash := plainToCT[h]
				if ctHash == "" {
					plainBytes, rerr := os.ReadFile(a.Path)
					if rerr == nil {
						_, computedHash, cerr := encryptBlob(e2eeKey, h, plainBytes)
						plainBytes = nil
						if cerr == nil {
							ctHash = computedHash
							plainToCT[h] = ctHash
							newE2EEHashes[h] = ctHash
						}
					}
				}
				if ctHash != "" {
					blobHash = ctHash
					pl.BlobHash = ctHash
					rawPL, _ = json.Marshal(pl)
					if needBlob {
						if _, seen := encHashToFile[ctHash]; !seen {
							encHashToFile[ctHash] = encBlobRef{path: a.Path, plainHash: h}
							encHashToPlain[ctHash] = h
							hashToName[ctHash] = a.Label()
							candidateHashes = append(candidateHashes, ctHash)
						}
					}
				}
			} else if needBlob {
				if _, seen := hashToPath[h]; !seen {
					hashToPath[h] = a.Path
					hashToName[h] = a.Label()
					candidateHashes = append(candidateHashes, h)
				}
			}

			if !needOp {
				continue
			}
			opKind := "asset.upsert"
			entityID := a.ID
			wirePayload := json.RawMessage(rawPL)
			if len(e2eeKey) > 0 {
				encEnv, cerr := encryptOpPayload(e2eeKey, opID, rawPL)
				if cerr != nil {
					continue
				}
				opKind = e2eeOpKind
				entityID = "e2ee"
				wirePayload = encEnv
			}
			_ = blobHash
			pushOps = append(pushOps, SyncOp{
				OpID:      opID,
				DeviceID:  devID,
				Timestamp: time.Now().UnixMilli(),
				Kind:      opKind,
				EntityID:  entityID,
				Payload:   wirePayload,
			})
		}

		if len(newE2EEHashes) > 0 {
			_ = localStore.SaveE2EEBlobHashes(scope, newE2EEHashes)
		}

		var confirmedHashes []string
		var confirmedOps []string

		// 2. Check missing blobs & upload concurrently (up to 6 parallel workers!).
		if len(candidateHashes) > 0 {
			emit(SyncProgress{Stage: "checking", Done: 0, Total: len(candidateHashes)})
			chkBody, _ := json.Marshal(map[string]any{"hashes": candidateHashes})
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

			missingSet := make(map[string]bool, len(chkOut.Missing))
			for _, mh := range chkOut.Missing {
				missingSet[mh] = true
			}
			for _, ch := range candidateHashes {
				if !missingSet[ch] {
					if ph := encHashToPlain[ch]; ph != "" {
						confirmedHashes = append(confirmedHashes, ph)
					} else {
						confirmedHashes = append(confirmedHashes, ch)
					}
				}
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
				if len(e2eeKey) > 0 {
					// Encrypting large RAW/video files on-the-fly: cap at 2 concurrent workers
					// so memory stays bounded to ~2 files and CPU stays gentle.
					workers = 2
				}
				if totalMissing < workers {
					workers = totalMissing
				}

				var (
					wg          sync.WaitGroup
					doneCount   atomic.Int64
					okCount     atomic.Int64
					firstErr    error
					errOnce     sync.Once
					uploadedMu  sync.Mutex
					uploadedOK  []string
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
							var bodyReader io.Reader
							var closer io.Closer
							if ref, ok := encHashToFile[mh]; ok {
								plainBytes, rerr := os.ReadFile(ref.path)
								if rerr != nil {
									d := int(doneCount.Add(1))
									emit(SyncProgress{Stage: "uploading", Done: d, Total: totalMissing})
									continue
								}
								ctBytes, _, cerr := encryptBlob(e2eeKey, ref.plainHash, plainBytes)
								plainBytes = nil
								if cerr != nil {
									d := int(doneCount.Add(1))
									emit(SyncProgress{Stage: "uploading", Done: d, Total: totalMissing})
									continue
								}
								bodyReader = bytes.NewReader(ctBytes)
							} else {
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
								bodyReader = f
								closer = f
							}
							upResp, err := doReq(upCtx, http.MethodPut, "/v1/blobs/"+mh, bodyReader, "application/octet-stream")
							if closer != nil {
								_ = closer.Close()
							}
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
								confirmedKey := mh
								if ph := encHashToPlain[mh]; ph != "" {
									confirmedKey = ph
								}
								uploadedMu.Lock()
								uploadedOK = append(uploadedOK, confirmedKey)
								uploadedMu.Unlock()
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
				if len(uploadedOK) > 0 {
					confirmedHashes = append(confirmedHashes, uploadedOK...)
				}
				if firstErr != nil {
					_ = localStore.MarkRemoteKnown(scope, nil, confirmedHashes)
					return rep, firstErr
				}
				rep.UploadedBlobs = int(okCount.Load())
			}
		}

		// 3. Push only delta local ops idempotently in batches.
		if len(pushOps) > 0 {
			emit(SyncProgress{Stage: "pushing", Done: 0, Total: len(pushOps)})
			const opBatch = 400
			for start := 0; start < len(pushOps); start += opBatch {
				end := min(start+opBatch, len(pushOps))
				batch := pushOps[start:end]
				pushBody, _ := json.Marshal(map[string]any{"ops": batch})
				resp, err := doReq(ctx, http.MethodPost, "/v1/sync/push", bytes.NewReader(pushBody), "application/json")
				if err != nil {
					_ = localStore.MarkRemoteKnown(scope, confirmedOps, confirmedHashes)
					return rep, err
				}
				var pushOut struct {
					LatestSeq uint64 `json:"latest_seq"`
					Accepted  int    `json:"accepted"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&pushOut)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					_ = localStore.MarkRemoteKnown(scope, confirmedOps, confirmedHashes)
					return rep, fmt.Errorf("sync push failed: HTTP %d", resp.StatusCode)
				}
				rep.PushedOps += pushOut.Accepted
				rep.LatestSeq = pushOut.LatestSeq
				for _, op := range batch {
					confirmedOps = append(confirmedOps, op.OpID)
				}
				emit(SyncProgress{Stage: "pushing", Done: end, Total: len(pushOps)})
			}
		}

		_ = localStore.MarkRemoteKnown(scope, confirmedOps, confirmedHashes)
	}

	// 4. Pull remote ops since last cursor and apply missing assets + metadata locally.
	// Scope cursor by server+account+E2EE key ("v2:"+scope) so:
	// - switching accounts or loading an E2EE key on another device cleanly re-pulls from 0,
	// - clients upgrading from older builds with shared default DeviceID auto-heal and pull missed ops.
	emit(SyncProgress{Stage: "pulling"})
	cursorKey := "v2:" + scope
	since := localStore.GetCursor(cursorKey)
	safeSince := since
	cursorBlocked := false
	var pulledKnownOps []string
	var pulledKnownHashes []string
	var knownOpsForPull map[string]bool

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

		if len(pullOut.Ops) > 0 {
			if knownOpsForPull == nil {
				knownOpsForPull, _ = localStore.LoadRemoteKnown(scope)
			}
			hasRemote := false
			for _, op := range pullOut.Ops {
				if !(op.DeviceID == devID && knownOpsForPull[op.OpID]) {
					hasRemote = true
					break
				}
			}
			if hasRemote {
				if err := ensureSnap(); err != nil {
					return rep, err
				}
			}
		}

		for idx, op := range pullOut.Ops {
			if op.Seq > since {
				since = op.Seq
			}
			// Only skip our own echo when BOTH DeviceID matches AND this library
			// already knows it pushed this OpID; never drop assets missing locally!
			if op.DeviceID == devID && knownOpsForPull[op.OpID] {
				if !cursorBlocked && op.Seq > safeSince {
					safeSince = op.Seq
				}
				continue
			}
			if op.Kind == "asset.upsert" || op.Kind == e2eeOpKind {
				isEncryptedOp := op.Kind == e2eeOpKind
				rawPayload := []byte(op.Payload)
				if isEncryptedOp {
					dec, derr := decryptOpPayload(e2eeKey, op.OpID, op.Payload)
					if derr != nil {
						rep.DecryptFailedOps++
						cursorBlocked = true
						continue
					}
					rawPayload = dec
				}
				var pl assetSyncPayload
				if err := json.Unmarshal(rawPayload, &pl); err != nil {
					if !cursorBlocked && op.Seq > safeSince {
						safeSince = op.Seq
					}
					continue
				}
				var target model.Asset
				if ex, ok := localByHash[strings.ToLower(pl.Hash)]; ok {
					target = ex
					rep.PulledOps++
				} else if pl.Hash != "" {
					fetchHash := pl.Hash
					if isEncryptedOp && pl.BlobHash != "" {
						fetchHash = pl.BlobHash
					}
					emit(SyncProgress{Stage: "downloading", Done: idx + 1, Total: len(pullOut.Ops), Current: pl.Name})
					bResp, err := doReq(ctx, http.MethodGet, "/v1/blobs/"+fetchHash, nil, "")
					if err != nil {
						cursorBlocked = true
						continue
					}
					data, rerr := io.ReadAll(io.LimitReader(bResp.Body, maxBlobUploadBytes))
					_ = bResp.Body.Close()
					if rerr != nil || bResp.StatusCode != http.StatusOK || len(data) == 0 {
						cursorBlocked = true
						continue
					}
					if isEncryptedOp {
						plainData, derr := decryptBlob(e2eeKey, pl.Hash, data)
						if derr != nil {
							rep.DecryptFailedOps++
							cursorBlocked = true
							continue
						}
						data = plainData
					}
					name := pl.Name
					if filepath.Ext(name) == "" {
						ext := strings.TrimSpace(pl.Ext)
						if ext == "" || !strings.HasPrefix(ext, ".") {
							ext = extFromMime(pl.Mime)
						}
						name = name + ext
					}
					imported, _, ierr := lib.ImportBytes(name, data, nil, pl.SourceURL, pl.SourceTitle)
					if ierr != nil {
						cursorBlocked = true
						continue
					}
					target = imported
					localByHash[strings.ToLower(pl.Hash)] = imported
					rep.PulledOps++
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
					pulledKnownOps = append(pulledKnownOps, op.OpID)
					if h := strings.ToLower(strings.TrimSpace(pl.Hash)); h != "" {
						pulledKnownHashes = append(pulledKnownHashes, h)
					}
				}
			}
			if !cursorBlocked && op.Seq > safeSince {
				safeSince = op.Seq
			}
		}
		if !pullOut.HasMore || len(pullOut.Ops) == 0 {
			break
		}
	}
	if len(pulledKnownOps) > 0 || len(pulledKnownHashes) > 0 {
		_ = localStore.MarkRemoteKnown(scope, pulledKnownOps, pulledKnownHashes)
	}
	if safeSince > 0 {
		_ = localStore.SetCursor(cursorKey, safeSince)
	}
	emit(SyncProgress{Stage: "done", Done: rep.UploadedBlobs + rep.DownloadedBlobs, Total: rep.UploadedBlobs + rep.DownloadedBlobs})
	return rep, nil
}

// ResetRemoteSync clears the authenticated user's remote sync oplog and blobs
// via POST /v1/sync/reset and wipes local cached cursors/known-sets for that
// endpoint so the next sync starts cleanly from scratch (without touching any
// local library files).
func ResetRemoteSync(ctx context.Context, cfg ClientConfig, lib *library.Library) (int, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if base == "" {
		return 0, fmt.Errorf("sync server URL is empty")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, base+"/v1/sync/reset", nil)
	if err != nil {
		return 0, err
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return 0, fmt.Errorf("reset remote sync failed: %s", msg)
	}
	var out struct {
		RemovedLinks int `json:"removed_links"`
		DeletedBlobs int `json:"deleted_blobs"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if lib != nil {
		if st, err := OpenStore(lib.DB.Root()); err == nil {
			scope := remoteScopeKey(base, cfg.Token, cfg.E2EEPassphrase)
			_ = st.ResetRemoteState(scope, base)
			_ = st.Close()
		}
	}
	return out.RemovedLinks, nil
}
