package cloud

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"raglex/internal/model"
	"raglex/internal/share"
	rsync "raglex/internal/sync"
)

// CollabMember represents a participant in a shared Raglex Collaboration Space.
type CollabMember struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     string    `json:"role"` // "owner" or "editor"
	JoinedAt time.Time `json:"joined_at"`
}

// CollabSpace stores a collaborative Project (including Boards & Placements) or
// shared Reference Folder synced among multiple Raglex Cloud users.
type CollabSpace struct {
	ID            string            `json:"id"`
	Code          string            `json:"code"` // e.g. CLB-XXXX-XXXX
	Kind          string            `json:"kind"` // "project" or "folder"
	Title         string            `json:"title"`
	OwnerUserID   string            `json:"owner_user_id"`
	OwnerName     string            `json:"owner_name"`
	Members       []CollabMember    `json:"members"`
	Revision      uint64            `json:"revision"`
	AllowedHashes map[string]bool   `json:"allowed_hashes,omitempty"`
	BoardOwners   map[string]string `json:"board_owners,omitempty"` // lowercase board name -> owner userID (users may only edit their own boards)
	Bundle        share.Bundle      `json:"bundle"`
	UpdatedBy     string            `json:"updated_by"`
	UpdatedAt     time.Time         `json:"updated_at"`
	CreatedAt     time.Time         `json:"created_at"`
}

// CollabSummary is the lightweight metadata returned when listing collab spaces.
type CollabSummary struct {
	ID          string         `json:"id"`
	Code        string         `json:"code"`
	Kind        string         `json:"kind"`
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

func toCollabSummary(c CollabSpace) CollabSummary {
	return CollabSummary{
		ID:          c.ID,
		Code:        c.Code,
		Kind:        c.Kind,
		Title:       c.Title,
		OwnerUserID: c.OwnerUserID,
		OwnerName:   c.OwnerName,
		Members:     c.Members,
		Revision:    c.Revision,
		AssetCount:  len(c.Bundle.Assets),
		BoardCount:  len(c.Bundle.Boards),
		FolderCount: len(c.Bundle.Folders),
		UpdatedBy:   c.UpdatedBy,
		UpdatedAt:   c.UpdatedAt,
		CreatedAt:   c.CreatedAt,
	}
}

func collabHasMember(c CollabSpace, userID string) bool {
	if c.OwnerUserID == userID {
		return true
	}
	for _, m := range c.Members {
		if m.UserID == userID {
			return true
		}
	}
	return false
}

func generateCollabCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return fmt.Sprintf("CLB-%s-%s", string(out[:4]), string(out[4:])), nil
}

func (s *Server) handleCollabsCollection(w http.ResponseWriter, r *http.Request, user User) {
	switch r.Method {
	case http.MethodGet:
		var list []CollabSummary
		_ = s.db.View(func(tx *bbolt.Tx) error {
			return tx.Bucket(bucketCollabs).ForEach(func(_, v []byte) error {
				var cs CollabSpace
				if json.Unmarshal(v, &cs) == nil && collabHasMember(cs, user.ID) {
					list = append(list, toCollabSummary(cs))
				}
				return nil
			})
		})
		sort.Slice(list, func(i, j int) bool {
			return list[i].UpdatedAt.After(list[j].UpdatedAt)
		})
		if list == nil {
			list = []CollabSummary{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "collabs": list})

	case http.MethodPost:
		var req struct {
			Bundle share.Bundle `json:"bundle"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 24<<20)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON payload")
			return
		}
		kind := strings.ToLower(strings.TrimSpace(req.Bundle.Kind))
		if kind == "" {
			kind = "project"
		}
		if kind != "folder" && kind != "project" {
			writeErr(w, http.StatusBadRequest, "invalid collab kind")
			return
		}
		req.Bundle.Kind = kind
		title := strings.TrimSpace(req.Bundle.DisplayName())
		if title == "" {
			title = "Shared Workspace"
		}

		allowed := make(map[string]bool, len(req.Bundle.Files))
		for i, f := range req.Bundle.Files {
			norm, err := rsync.ValidateSHA256(f.Hash)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid file sha256")
				return
			}
			if !s.userOwnsBlob(user.ID, norm) || !s.blobs.Has(r.Context(), norm) {
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("missing blob %s — upload files first", norm[:12]))
				return
			}
			req.Bundle.Files[i].Hash = norm
			allowed[norm] = true
		}

		ownerName := strings.TrimSpace(user.Name)
		if ownerName == "" {
			ownerName = user.Email
		}

		var code string
		for attempt := 0; attempt < 8; attempt++ {
			cand, err := generateCollabCode()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			exists := false
			_ = s.db.View(func(tx *bbolt.Tx) error {
				exists = tx.Bucket(bucketCollabCodes).Get([]byte(cand)) != nil
				return nil
			})
			if !exists {
				code = cand
				break
			}
		}
		if code == "" {
			writeErr(w, http.StatusInternalServerError, "failed to allocate collab code")
			return
		}

		now := time.Now().UTC()
		boardOwners := map[string]string{}
		for _, b := range req.Bundle.Boards {
			bk := strings.ToLower(strings.TrimSpace(b.Name))
			if bk != "" {
				boardOwners[bk] = user.ID
			}
		}
		cs := CollabSpace{
			ID:          model.NewID(),
			Code:        code,
			Kind:        kind,
			Title:       title,
			OwnerUserID: user.ID,
			OwnerName:   ownerName,
			Members: []CollabMember{{
				UserID:   user.ID,
				Email:    user.Email,
				Name:     ownerName,
				Role:     "owner",
				JoinedAt: now,
			}},
			Revision:      1,
			AllowedHashes: allowed,
			BoardOwners:   boardOwners,
			Bundle:        req.Bundle,
			UpdatedBy:     ownerName,
			UpdatedAt:     now,
			CreatedAt:     now,
		}

		err := s.db.Update(func(tx *bbolt.Tx) error {
			raw, err := json.Marshal(cs)
			if err != nil {
				return err
			}
			if err := tx.Bucket(bucketCollabs).Put([]byte(cs.ID), raw); err != nil {
				return err
			}
			return tx.Bucket(bucketCollabCodes).Put([]byte(cs.Code), []byte(cs.ID))
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"ok":     true,
			"collab": toCollabSummary(cs),
		})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleCollabJoin(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		writeErr(w, http.StatusBadRequest, "請輸入協作邀請碼")
		return
	}

	var joined CollabSpace
	var notFound bool
	err := s.db.Update(func(tx *bbolt.Tx) error {
		idBytes := tx.Bucket(bucketCollabCodes).Get([]byte(code))
		if idBytes == nil {
			notFound = true
			return nil
		}
		raw := tx.Bucket(bucketCollabs).Get(idBytes)
		if raw == nil {
			notFound = true
			return nil
		}
		if err := json.Unmarshal(raw, &joined); err != nil {
			return err
		}
		if !collabHasMember(joined, user.ID) {
			name := strings.TrimSpace(user.Name)
			if name == "" {
				name = user.Email
			}
			joined.Members = append(joined.Members, CollabMember{
				UserID:   user.ID,
				Email:    user.Email,
				Name:     name,
				Role:     "editor",
				JoinedAt: time.Now().UTC(),
			})
			upd, err := json.Marshal(joined)
			if err != nil {
				return err
			}
			if err := tx.Bucket(bucketCollabs).Put([]byte(joined.ID), upd); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if notFound {
		writeErr(w, http.StatusNotFound, "協作邀請碼不存在或已關閉")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"collab": toCollabSummary(joined),
	})
}

func (s *Server) handleCollabByID(w http.ResponseWriter, r *http.Request, user User) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/collabs/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, "missing collab id")
		return
	}
	collabID := parts[0]

	loadSpace := func() (CollabSpace, bool) {
		var cs CollabSpace
		var ok bool
		_ = s.db.View(func(tx *bbolt.Tx) error {
			raw := tx.Bucket(bucketCollabs).Get([]byte(collabID))
			if raw != nil && json.Unmarshal(raw, &cs) == nil {
				ok = true
			}
			return nil
		})
		return cs, ok
	}

	// Subroute: GET /api/v1/collabs/{id}/blobs/{sha256}
	if len(parts) == 3 && parts[1] == "blobs" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		cs, ok := loadSpace()
		if !ok || !collabHasMember(cs, user.ID) {
			writeErr(w, http.StatusNotFound, "collab space not found")
			return
		}
		norm, err := rsync.ValidateSHA256(parts[2])
		if err != nil || !cs.AllowedHashes[norm] {
			writeErr(w, http.StatusNotFound, "blob not in collab space")
			return
		}
		rc, size, err := s.blobs.Open(r.Context(), norm)
		if err != nil {
			writeErr(w, http.StatusNotFound, "blob not found")
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, rc)
		}
		return
	}

	// Subroute: POST /api/v1/collabs/{id}/push
	if len(parts) == 2 && parts[1] == "push" {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Bundle share.Bundle `json:"bundle"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 24<<20)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON payload")
			return
		}
		cs, ok := loadSpace()
		if !ok {
			writeErr(w, http.StatusNotFound, "collab space not found")
			return
		}
		if !collabHasMember(cs, user.ID) {
			writeErr(w, http.StatusForbidden, "not a member of this collab space")
			return
		}
		if cs.AllowedHashes == nil {
			cs.AllowedHashes = map[string]bool{}
		}
		// Verify every file hash in incoming bundle is either already in AllowedHashes OR owned by pushing user.
		for i, f := range req.Bundle.Files {
			norm, err := rsync.ValidateSHA256(f.Hash)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid file sha256")
				return
			}
			if !cs.AllowedHashes[norm] {
				if !s.userOwnsBlob(user.ID, norm) || !s.blobs.Has(r.Context(), norm) {
					writeErr(w, http.StatusBadRequest, fmt.Sprintf("missing blob %s", norm[:12]))
					return
				}
				cs.AllowedHashes[norm] = true
			}
			req.Bundle.Files[i].Hash = norm
		}

		if cs.BoardOwners == nil {
			cs.BoardOwners = map[string]string{}
			for _, b := range cs.Bundle.Boards {
				if bk := strings.ToLower(strings.TrimSpace(b.Name)); bk != "" {
					cs.BoardOwners[bk] = cs.OwnerUserID
				}
			}
		}
		merged := mergeCollabBundles(cs.Bundle, req.Bundle, cs.BoardOwners, user.ID)
		updaterName := strings.TrimSpace(user.Name)
		if updaterName == "" {
			updaterName = user.Email
		}
		cs.Bundle = merged
		cs.Revision++
		cs.UpdatedBy = updaterName
		cs.UpdatedAt = time.Now().UTC()
		if title := strings.TrimSpace(merged.DisplayName()); title != "" {
			cs.Title = title
		}

		err := s.db.Update(func(tx *bbolt.Tx) error {
			raw, err := json.Marshal(cs)
			if err != nil {
				return err
			}
			return tx.Bucket(bucketCollabs).Put([]byte(cs.ID), raw)
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"collab": toCollabSummary(cs),
			"bundle": cs.Bundle,
		})
		return
	}

	if len(parts) != 1 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		cs, ok := loadSpace()
		if !ok || !collabHasMember(cs, user.ID) {
			writeErr(w, http.StatusNotFound, "collab space not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"collab": toCollabSummary(cs),
			"bundle": cs.Bundle,
		})

	case http.MethodDelete:
		var notFound bool
		err := s.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketCollabs)
			raw := b.Get([]byte(collabID))
			if raw == nil {
				notFound = true
				return nil
			}
			var cs CollabSpace
			if err := json.Unmarshal(raw, &cs); err != nil {
				return b.Delete([]byte(collabID))
			}
			if !collabHasMember(cs, user.ID) {
				notFound = true
				return nil
			}
			if cs.OwnerUserID == user.ID {
				_ = tx.Bucket(bucketCollabCodes).Delete([]byte(cs.Code))
				return b.Delete([]byte(collabID))
			}
			// Non-owner leaves the collab space.
			next := make([]CollabMember, 0, len(cs.Members))
			for _, m := range cs.Members {
				if m.UserID != user.ID {
					next = append(next, m)
				}
			}
			cs.Members = next
			upd, err := json.Marshal(cs)
			if err != nil {
				return err
			}
			return b.Put([]byte(collabID), upd)
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if notFound {
			writeErr(w, http.StatusNotFound, "collab space not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// mergeCollabBundles merges an incoming member push into the existing collab
// bundle. Shared reference assets & folders merge additively across members,
// while Whiteboards are strictly per-user owned: each member may create or edit
// only their own boards and can NEVER modify another member's board.
func mergeCollabBundles(base, inc share.Bundle, boardOwners map[string]string, pushingUserID string) share.Bundle {
	out := base
	if out.Kind == "" {
		out.Kind = inc.Kind
	}

	// 1. Merge Folders (by ID or parent+name).
	folderByID := map[string]share.FolderSnap{}
	folderKeyToID := map[string]string{}
	fkey := func(pid, name string) string {
		return pid + "\x00" + strings.ToLower(strings.TrimSpace(name))
	}
	for _, f := range out.Folders {
		folderByID[f.ID] = f
		folderKeyToID[fkey(f.ParentID, f.Name)] = f.ID
	}
	incFolderMap := map[string]string{}
	// Map root folder if both have one.
	if out.RootFolderID != "" && inc.RootFolderID != "" {
		incFolderMap[inc.RootFolderID] = out.RootFolderID
	}
	remFolders := append([]share.FolderSnap(nil), inc.Folders...)
	for pass := 0; len(remFolders) > 0 && pass < 24; pass++ {
		var next []share.FolderSnap
		prog := false
		for _, f := range remFolders {
			if mapped, ok := incFolderMap[f.ID]; ok {
				if f.Color != "" {
					cur := folderByID[mapped]
					cur.Color = f.Color
					folderByID[mapped] = cur
				}
				prog = true
				continue
			}
			mp := ""
			if f.ParentID != "" {
				var ok bool
				mp, ok = incFolderMap[f.ParentID]
				if !ok {
					next = append(next, f)
					continue
				}
			}
			k := fkey(mp, f.Name)
			if exID, ok := folderKeyToID[k]; ok {
				incFolderMap[f.ID] = exID
			} else {
				nf := f
				nf.ParentID = mp
				out.Folders = append(out.Folders, nf)
				folderByID[nf.ID] = nf
				folderKeyToID[k] = nf.ID
				incFolderMap[f.ID] = nf.ID
			}
			prog = true
		}
		remFolders = next
		if !prog {
			break
		}
	}

	// 2. Merge Assets (dedup by SHA-256 hash; union tags & folder links).
	assetByHash := map[string]int{}
	incAssetMap := map[string]string{}
	for i, a := range out.Assets {
		if a.Asset.Hash != "" {
			assetByHash[a.Asset.Hash] = i
		}
	}
	for _, ia := range inc.Assets {
		mappedFids := make([]string, 0, len(ia.FolderIDs))
		for _, fid := range ia.FolderIDs {
			if m := incFolderMap[fid]; m != "" {
				mappedFids = append(mappedFids, m)
			} else {
				mappedFids = append(mappedFids, fid)
			}
		}
		if idx, ok := assetByHash[ia.Asset.Hash]; ok {
			ex := &out.Assets[idx]
			incAssetMap[ia.Asset.ID] = ex.Asset.ID
			ex.TagNames = unionStrings(ex.TagNames, ia.TagNames)
			ex.FolderIDs = unionStrings(ex.FolderIDs, mappedFids)
			if ia.Asset.Rating > ex.Asset.Rating {
				ex.Asset.Rating = ia.Asset.Rating
			}
			if ia.Asset.Notes != "" && ex.Asset.Notes == "" {
				ex.Asset.Notes = ia.Asset.Notes
			}
		} else {
			na := ia
			na.FolderIDs = mappedFids
			out.Assets = append(out.Assets, na)
			if na.Asset.Hash != "" {
				assetByHash[na.Asset.Hash] = len(out.Assets) - 1
			}
			incAssetMap[ia.Asset.ID] = na.Asset.ID
		}
	}

	// 3. Merge Files (dedup by SHA-256 hash).
	fileSeen := map[string]bool{}
	for _, f := range out.Files {
		fileSeen[f.Hash] = true
	}
	for _, f := range inc.Files {
		if !fileSeen[f.Hash] {
			fileSeen[f.Hash] = true
			out.Files = append(out.Files, f)
		}
	}

	// 4. Merge Project References, Boards & Placements (for project collabs).
	if out.Kind != "folder" {
		if strings.TrimSpace(inc.Project.Description) != "" {
			out.Project.Description = inc.Project.Description
		}
		refSeen := map[string]bool{}
		for _, r := range out.References {
			refSeen[r] = true
		}
		for _, r := range inc.References {
			mr := incAssetMap[r]
			if mr == "" {
				mr = r
			}
			if !refSeen[mr] {
				refSeen[mr] = true
				out.References = append(out.References, mr)
			}
		}

		boardByName := map[string]string{}
		boardIDToName := map[string]string{}
		for _, b := range out.Boards {
			bk := strings.ToLower(strings.TrimSpace(b.Name))
			boardByName[bk] = b.ID
			boardIDToName[b.ID] = bk
		}
		incBoardMap := map[string]string{}
		editableBoardIDs := map[string]bool{}
		for _, ib := range inc.Boards {
			bk := strings.ToLower(strings.TrimSpace(ib.Name))
			if bk == "" {
				continue
			}
			if exID, ok := boardByName[bk]; ok {
				incBoardMap[ib.ID] = exID
				// Only the board's owner may overwrite/update placements on this board.
				if ownerID := boardOwners[bk]; ownerID == "" || ownerID == pushingUserID {
					boardOwners[bk] = pushingUserID
					editableBoardIDs[exID] = true
				}
			} else {
				// New board created by pushingUserID -> claim ownership for pushingUserID.
				out.Boards = append(out.Boards, ib)
				boardByName[bk] = ib.ID
				boardIDToName[ib.ID] = bk
				boardOwners[bk] = pushingUserID
				incBoardMap[ib.ID] = ib.ID
				editableBoardIDs[ib.ID] = true
			}
		}

		// Replace placements ONLY on boards owned by pushingUserID; keep other members'
		// boards 100% untouched!
		keptPlacements := make([]model.Placement, 0, len(out.Placements))
		for _, pl := range out.Placements {
			if editableBoardIDs[pl.BoardID] {
				continue
			}
			keptPlacements = append(keptPlacements, pl)
		}
		for _, ipl := range inc.Placements {
			mb := incBoardMap[ipl.BoardID]
			if mb == "" || !editableBoardIDs[mb] {
				continue
			}
			np := ipl
			np.BoardID = mb
			if ipl.AssetID != "" {
				if ma := incAssetMap[ipl.AssetID]; ma != "" {
					np.AssetID = ma
				}
			}
			keptPlacements = append(keptPlacements, np)
		}
		out.Placements = keptPlacements
	}

	return out
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
