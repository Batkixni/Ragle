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
	"time"

	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/project"
)

// ClientConfig configures a desktop library sync run against any Raglex Sync API v1
// server (self-hosted Docker instance or Raglex Cloud).
type ClientConfig struct {
	ServerURL string
	Token     string
	DeviceID  string
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
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Hash        string   `json:"hash"`
	Mime        string   `json:"mime"`
	Rating      int      `json:"rating"`
	Notes       string   `json:"notes,omitempty"`
	SourceURL   string   `json:"source_url,omitempty"`
	SourceTitle string   `json:"source_title,omitempty"`
	IsBookmark  bool     `json:"is_bookmark,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// SyncNow executes a full bidirectional delta + content-addressed blob sync
// between local library (lib/projects) and the remote Raglex Sync API v1 server.
func SyncNow(ctx context.Context, cfg ClientConfig, lib *library.Library, projects *project.Store) (SyncReport, error) {
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

	localStore, err := OpenStore(lib.DB.Root())
	if err != nil {
		return rep, err
	}
	defer localStore.Close()

	hc := &http.Client{Timeout: 60 * time.Second}
	doReq := func(method, path string, body io.Reader, ctype string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, base+path, body)
		if err != nil {
			return nil, err
		}
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		return hc.Do(req)
	}

	// 1. Collect local assets and check which SHA-256 blobs are missing remotely.
	assets, err := lib.List(library.Filter{})
	if err != nil {
		return rep, err
	}
	hashToPath := map[string]string{}
	var allHashes []string
	var pushOps []SyncOp

	for _, a := range assets {
		h := strings.ToLower(strings.TrimSpace(a.Hash))
		if _, err := ValidateSHA256(h); err == nil && a.Path != "" && !a.Missing {
			if _, seen := hashToPath[h]; !seen {
				hashToPath[h] = a.Path
				allHashes = append(allHashes, h)
			}
		}
		tgs, _ := lib.AssetTags(a.ID)
		tagNames := make([]string, 0, len(tgs))
		for _, tg := range tgs {
			tagNames = append(tagNames, tg.Name)
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

	// 2. Check missing blobs & upload only deduplicated missing SHA-256 blobs.
	if len(allHashes) > 0 {
		chkBody, _ := json.Marshal(map[string]any{"hashes": allHashes})
		resp, err := doReq(http.MethodPost, "/v1/blobs/check", bytes.NewReader(chkBody), "application/json")
		if err != nil {
			return rep, err
		}
		var chkOut struct {
			Missing []string `json:"missing"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&chkOut)
		_ = resp.Body.Close()

		for _, mh := range chkOut.Missing {
			p := hashToPath[mh]
			if p == "" {
				continue
			}
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			upResp, err := doReq(http.MethodPut, "/v1/blobs/"+mh, f, "application/octet-stream")
			_ = f.Close()
			if err != nil {
				return rep, err
			}
			_, _ = io.Copy(io.Discard, upResp.Body)
			_ = upResp.Body.Close()
			if upResp.StatusCode == http.StatusOK {
				rep.UploadedBlobs++
			}
		}
	}

	// 3. Push local ops idempotently.
	if len(pushOps) > 0 {
		pushBody, _ := json.Marshal(map[string]any{"ops": pushOps})
		resp, err := doReq(http.MethodPost, "/v1/sync/push", bytes.NewReader(pushBody), "application/json")
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
		rep.PushedOps = pushOut.Accepted
		rep.LatestSeq = pushOut.LatestSeq
	}

	// 4. Pull remote ops since last cursor and apply missing assets + metadata locally.
	cursorKey := base
	since := localStore.GetCursor(cursorKey)
	localByHash := map[string]model.Asset{}
	for _, a := range assets {
		if a.Hash != "" {
			localByHash[strings.ToLower(a.Hash)] = a
		}
	}
	tagByName := map[string]string{}
	if tags, err := lib.Tags(); err == nil {
		for _, tg := range tags {
			tagByName[strings.ToLower(tg.Name)] = tg.ID
		}
	}

	for {
		resp, err := doReq(http.MethodGet, fmt.Sprintf("/v1/sync/pull?since=%d&limit=500", since), nil, "")
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

		for _, op := range pullOut.Ops {
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
					// Download blob from remote server and import into local library.
					bResp, err := doReq(http.MethodGet, "/v1/blobs/"+pl.Hash, nil, "")
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
				}
			}
		}
		if !pullOut.HasMore || len(pullOut.Ops) == 0 {
			break
		}
	}
	_ = localStore.SetCursor(cursorKey, since)
	_ = projects
	return rep, nil
}
