package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// CollabMember mirrors cloud.CollabMember for desktop client rendering.
type CollabMember struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// CollabSummary describes a Raglex Cloud Collaboration Space (shared Project/Boards or Folder).
type CollabSummary struct {
	ID          string         `json:"id"`
	Code        string         `json:"code"`
	Kind        string         `json:"kind"` // "project" or "folder"
	Title       string         `json:"title"`
	OwnerUserID string         `json:"owner_user_id"`
	OwnerName   string         `json:"owner_name"`
	Members     []CollabMember `json:"members"`
	Revision    uint64         `json:"revision"`
	AssetCount  int            `json:"asset_count"`
	BoardCount  int            `json:"board_count"`
	FolderCount int            `json:"folder_count"`
	UpdatedBy   string         `json:"updated_by"`
	UpdatedAt   time.Time      `json:"updated_at"`
	CreatedAt   time.Time      `json:"created_at"`
}

// CollabSyncResult reports the outcome of a 2-way Collab Space sync.
type CollabSyncResult struct {
	Collab        CollabSummary
	LocalTargetID string
	Downloaded    int
	Pushed        bool
}

// IsCollabCode returns true when raw looks like a collaboration invite code (CLB-XXXX-XXXX).
func IsCollabCode(raw string) bool {
	s := strings.ToUpper(strings.TrimSpace(raw))
	return strings.HasPrefix(s, "CLB-") && len(s) >= 8
}

func collabBaseURL(cfg ClientConfig) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if base == "" {
		base = config.DefaultCloudEndpoint
	}
	return base
}

// CreateCollabSpace publishes a local Project (with all Boards) or Folder (with
// subfolders) as a multi-user Collaboration Space on Raglex Cloud.
func CreateCollabSpace(
	ctx context.Context,
	cfg ClientConfig,
	database *db.DB,
	kind string,
	targetID string,
	onProgress func(SyncProgress),
) (CollabSummary, error) {
	var out CollabSummary
	if database == nil {
		return out, errors.New("library database is not open")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return out, errors.New("請先登入 Raglex Cloud 帳號")
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
		bundle, local, err = share.ExportFolder(database, targetID, true)
	} else {
		bundle, local, err = share.Export(database, targetID, true)
	}
	if err != nil {
		return out, err
	}

	base := collabBaseURL(cfg)
	client := newSyncHTTPClient()
	if err := uploadMissingBundleBlobs(ctx, client, base, cfg.Token, bundle.Files, local, report); err != nil {
		return out, err
	}

	report("pushing", 0, 1, bundle.DisplayName())
	body, err := json.Marshal(map[string]any{"bundle": bundle})
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/collabs", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, errors.New(readCloudErr(resp))
	}
	var res struct {
		OK     bool          `json:"ok"`
		Collab CollabSummary `json:"collab"`
		Error  string        `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return out, err
	}
	if !res.OK {
		return out, errors.New(res.Error)
	}
	return res.Collab, nil
}

// ListCollabSpaces returns all collaboration spaces the signed-in user belongs to.
func ListCollabSpaces(ctx context.Context, cfg ClientConfig) ([]CollabSummary, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, collabBaseURL(cfg)+"/api/v1/collabs", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	resp, err := newSyncHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(readCloudErr(resp))
	}
	var res struct {
		OK      bool            `json:"ok"`
		Collabs []CollabSummary `json:"collabs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Collabs, nil
}

// JoinCollabSpace joins a friend's Collaboration Space using its CLB-XXXX-XXXX code.
func JoinCollabSpace(ctx context.Context, cfg ClientConfig, code string) (CollabSummary, error) {
	var out CollabSummary
	if strings.TrimSpace(cfg.Token) == "" {
		return out, errors.New("請先登入 Raglex Cloud 帳號才能加入好友協作空間")
	}
	body, _ := json.Marshal(map[string]string{"code": strings.ToUpper(strings.TrimSpace(code))})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, collabBaseURL(cfg)+"/api/v1/collabs/join", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := newSyncHTTPClient().Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, errors.New(readCloudErr(resp))
	}
	var res struct {
		OK     bool          `json:"ok"`
		Collab CollabSummary `json:"collab"`
		Error  string        `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return out, err
	}
	if !res.OK {
		return out, errors.New(res.Error)
	}
	return res.Collab, nil
}

// SyncCollabSpace performs a full 2-way sync of a Collaboration Space:
// if localTargetID is non-empty, pushes local changes (new reference files,
// folders, or whiteboard placements) first, then downloads any new blobs from
// collaborators and merges them into the local library.
func SyncCollabSpace(
	ctx context.Context,
	cfg ClientConfig,
	database *db.DB,
	collabID string,
	kind string,
	localTargetID string,
	onProgress func(SyncProgress),
) (CollabSyncResult, error) {
	var out CollabSyncResult
	if database == nil {
		return out, errors.New("library database is not open")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return out, errors.New("請先登入 Raglex Cloud 帳號")
	}
	report := func(stage string, done, total int, cur string) {
		if onProgress != nil {
			onProgress(SyncProgress{Stage: stage, Done: done, Total: total, Current: cur})
		}
	}
	base := collabBaseURL(cfg)
	client := newSyncHTTPClient()

	var (
		remoteCollab CollabSummary
		remoteBundle share.Bundle
	)

	// 1. If we already have a linked local Project/Folder, export & push our changes first.
	if localTargetID != "" {
		var (
			localBundle share.Bundle
			localFiles  map[string]string
			expErr      error
		)
		if kind == "folder" {
			localBundle, localFiles, expErr = share.ExportFolder(database, localTargetID, true)
		} else {
			localBundle, localFiles, expErr = share.Export(database, localTargetID, true)
		}
		if expErr == nil {
			if err := uploadMissingBundleBlobs(ctx, client, base, cfg.Token, localBundle.Files, localFiles, report); err != nil {
				return out, err
			}
			report("pushing", 0, 1, localBundle.DisplayName())
			pb, _ := json.Marshal(map[string]any{"bundle": localBundle})
			pReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/collabs/"+collabID+"/push", bytes.NewReader(pb))
			if err != nil {
				return out, err
			}
			pReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
			pReq.Header.Set("Content-Type", "application/json")
			pResp, err := client.Do(pReq)
			if err != nil {
				return out, err
			}
			if pResp.StatusCode != http.StatusOK {
				msg := readCloudErr(pResp)
				pResp.Body.Close()
				return out, errors.New(msg)
			}
			var pushRes struct {
				OK     bool          `json:"ok"`
				Collab CollabSummary `json:"collab"`
				Bundle share.Bundle  `json:"bundle"`
			}
			_ = json.NewDecoder(io.LimitReader(pResp.Body, 32<<20)).Decode(&pushRes)
			pResp.Body.Close()
			remoteCollab = pushRes.Collab
			remoteBundle = pushRes.Bundle
			out.Pushed = true
		}
	}

	// 2. Otherwise pull latest bundle from server.
	if remoteCollab.ID == "" {
		report("pulling", 0, 0, collabID)
		gReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/collabs/"+collabID, nil)
		if err != nil {
			return out, err
		}
		gReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
		gResp, err := client.Do(gReq)
		if err != nil {
			return out, err
		}
		defer gResp.Body.Close()
		if gResp.StatusCode != http.StatusOK {
			return out, errors.New(readCloudErr(gResp))
		}
		var pullRes struct {
			OK     bool          `json:"ok"`
			Collab CollabSummary `json:"collab"`
			Bundle share.Bundle  `json:"bundle"`
		}
		if err := json.NewDecoder(io.LimitReader(gResp.Body, 32<<20)).Decode(&pullRes); err != nil {
			return out, err
		}
		remoteCollab = pullRes.Collab
		remoteBundle = pullRes.Bundle
	}

	// 3. Download any missing blobs from collaborators concurrently.
	existingHashes := map[string]bool{}
	_ = database.View(func(tx *bbolt.Tx) error {
		for _, f := range remoteBundle.Files {
			if _, ok := db.AssetIDByHash(tx, f.Hash); ok {
				existingHashes[f.Hash] = true
			}
		}
		return nil
	})
	var toDownload []share.FileSnap
	for _, f := range remoteBundle.Files {
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
		workers := 6
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
					dReq, rerr := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/collabs/"+collabID+"/blobs/"+f.Hash, nil)
					if rerr != nil {
						errOnce.Do(func() { firstErr = rerr })
						return
					}
					dReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
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

	// 4. Apply merged bundle into local DB (for folders, share.Apply reuses matching
	// folder names cleanly; for projects, reuse existing localTargetID if already linked!).
	appliedID, err := share.ApplyInto(database, remoteBundle, paths, localTargetID)
	if err != nil {
		for _, p := range savedFiles {
			_ = os.Remove(p)
		}
		return out, err
	}

	out.Collab = remoteCollab
	out.LocalTargetID = appliedID
	out.Downloaded = len(paths)
	report("done", len(toDownload), len(toDownload), remoteCollab.Title)
	return out, nil
}

// LeaveOrDeleteCollab leaves a collab space (or deletes it if current user is owner).
func LeaveOrDeleteCollab(ctx context.Context, cfg ClientConfig, collabID string) error {
	if strings.TrimSpace(cfg.Token) == "" || collabID == "" {
		return errors.New("missing token or collab id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, collabBaseURL(cfg)+"/api/v1/collabs/"+collabID, nil)
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

func uploadMissingBundleBlobs(
	ctx context.Context,
	client *http.Client,
	base, token string,
	files []share.FileSnap,
	local map[string]string,
	report func(stage string, done, total int, cur string),
) error {
	if len(files) == 0 {
		return nil
	}
	hashes := make([]string, 0, len(files))
	for _, f := range files {
		hashes = append(hashes, f.Hash)
	}
	checkBody, _ := json.Marshal(map[string]any{"hashes": hashes})
	cReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/blobs/check", bytes.NewReader(checkBody))
	if err != nil {
		return err
	}
	cReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	cReq.Header.Set("Content-Type", "application/json")
	cResp, err := client.Do(cReq)
	if err != nil {
		return err
	}
	if cResp.StatusCode != http.StatusOK {
		msg := readCloudErr(cResp)
		cResp.Body.Close()
		return errors.New(msg)
	}
	var checkOut struct {
		Missing []string `json:"missing"`
	}
	_ = json.NewDecoder(cResp.Body).Decode(&checkOut)
	cResp.Body.Close()
	if len(checkOut.Missing) == 0 {
		return nil
	}
	totalUp := len(checkOut.Missing)
	report("uploading", 0, totalUp, "")
	workers := 6
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
				p := local[h]
				if p == "" {
					continue
				}
				f, ferr := os.Open(p)
				if ferr != nil {
					errOnce.Do(func() { firstErr = ferr })
					return
				}
				fi, _ := f.Stat()
				uReq, rerr := http.NewRequestWithContext(ctx, http.MethodPut, base+"/v1/blobs/"+h, f)
				if rerr != nil {
					f.Close()
					errOnce.Do(func() { firstErr = rerr })
					return
				}
				uReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
				uReq.Header.Set("Content-Type", "application/octet-stream")
				if fi != nil {
					uReq.ContentLength = fi.Size()
				}
				uResp, uerr := client.Do(uReq)
				f.Close()
				if uerr != nil {
					errOnce.Do(func() { firstErr = uerr })
					return
				}
				if uResp.StatusCode < 200 || uResp.StatusCode >= 300 {
					msg := readCloudErr(uResp)
					uResp.Body.Close()
					errOnce.Do(func() { firstErr = errors.New(msg) })
					return
				}
				uResp.Body.Close()
				n := int(doneCount.Add(1))
				report("uploading", n, totalUp, filepath.Base(p))
			}
		}()
	}
	wg.Wait()
	return firstErr
}
