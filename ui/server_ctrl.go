package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"raglex/internal/config"
	"raglex/internal/library"
	"raglex/internal/mcp"
	"raglex/internal/model"
	"raglex/internal/plugin"
	"raglex/internal/server"
	rsync "raglex/internal/sync"
)

func (a *App) serverPortValue() int {
	p, err := strconv.Atoi(strings.TrimSpace(a.serverPortStr))
	if err != nil || p < 1 || p > 65535 {
		return server.DefaultPort
	}
	return p
}

func (a *App) ensureServerToken() string {
	if strings.TrimSpace(a.serverToken) == "" {
		a.serverToken = server.GenerateToken()
		a.syncConfigFromState()
	}
	return a.serverToken
}

func (a *App) ensureSyncE2EEKey() string {
	if strings.TrimSpace(a.syncE2EEPassphrase) == "" {
		if gen, err := rsync.GenerateE2EEKey(); err == nil {
			a.syncE2EEPassphrase = gen
			a.syncConfigFromState()
		}
	}
	return strings.TrimSpace(a.syncE2EEPassphrase)
}

func (a *App) effectiveSyncE2EEKey() string {
	if !a.syncE2EEEnabled {
		return ""
	}
	return a.ensureSyncE2EEKey()
}

func (a *App) exportSyncE2EEKeyFile() {
	key := a.ensureSyncE2EEKey()
	if key == "" || a.win == nil {
		return
	}
	win := a.win
	title := a.t("sync.e2eeSaveFile")
	content := rsync.FormatE2EEKeyFile(key, a.accountEmail)
	defName := fmt.Sprintf("raglex-e2ee-key-%s.txt", time.Now().Format("20060102"))
	go func() {
		dst, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      win,
			Title:       title,
			DefaultPath: defName,
		})
		if err != nil || strings.TrimSpace(dst) == "" {
			return
		}
		writeErr := os.WriteFile(dst, []byte(content), 0o600)
		win.Update(func() {
			if a.isClosed() {
				return
			}
			if writeErr != nil {
				a.notify(writeErr.Error())
				return
			}
			a.notify(fmt.Sprintf(a.t("sync.e2eeSaved"), filepath.Base(dst)))
		})
	}()
}

func (a *App) importSyncE2EEKeyFile() {
	if a.win == nil {
		return
	}
	win := a.win
	title := a.t("sync.e2eeLoadFile")
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: win,
			Title:  title,
			Filters: []mygo.FileFilter{
				{Name: "Raglex Recovery Key (*.txt)", Extensions: []string{"txt", "key"}},
			},
		})
		if err != nil || len(paths) == 0 || strings.TrimSpace(paths[0]) == "" {
			return
		}
		raw, readErr := os.ReadFile(paths[0])
		win.Update(func() {
			if a.isClosed() {
				return
			}
			if readErr != nil {
				a.notify(readErr.Error())
				return
			}
			parsed := rsync.ParseE2EEKeyFile(string(raw))
			if parsed == "" {
				a.notify(a.t("sync.e2eeLoadErr"))
				return
			}
			a.syncE2EEEnabled = true
			a.syncE2EEPassphrase = parsed
			a.syncConfigFromState()
			a.notify(a.t("sync.e2eeLoaded"))
			a.scheduleAutoSync(1 * time.Second)
		})
	}()
}

func (a *App) regenerateSyncE2EEKeyPrompt() {
	a.ask(a.t("sync.e2eeRegenTitle"), a.t("sync.e2eeRegenBody"), a.t("sync.e2eeRegen"), func() {
		gen, err := rsync.GenerateE2EEKey()
		if err != nil {
			a.notify(err.Error())
			return
		}
		a.syncE2EEEnabled = true
		a.syncE2EEPassphrase = gen
		a.syncConfigFromState()
		a.notify(a.t("sync.e2eeRegenDone"))
	})
}

func (a *App) startBuiltInServer() {
	if a.lib == nil {
		return
	}
	if a.localSrv != nil && a.localSrv.Running() {
		a.serverRunning = true
		return
	}
	tok := a.ensureServerToken()
	port := a.serverPortValue()
	host := "127.0.0.1"
	if a.serverBindAll {
		host = "0.0.0.0"
	}

	win := a.win
	onChange := func() {
		if win == nil {
			return
		}
		win.Update(func() {
			if !a.isClosed() && a.lib != nil {
				a.reload()
			}
		})
	}
	mcpSrv := mcp.New(mcp.Options{
		Library:   a.lib,
		Projects:  a.projects,
		LayaURL:   a.layaURL,
		LayaModel: a.layaModel,
		OnChange:  onChange,
	})
	srv := server.New(server.Options{
		Library:  a.lib,
		Projects: a.projects,
		Token:    tok,
		OnChange: onChange,
		ExtraRoutes: func(mux *http.ServeMux) {
			mux.Handle("/mcp", mcpSrv)
		},
	})
	basePort := port
	var startErr error
	usedPort := port
	for offset := 0; offset <= 10; offset++ {
		tryPort := basePort + offset
		if tryPort > 65535 {
			break
		}
		tryAddr := fmt.Sprintf("%s:%d", host, tryPort)
		if _, err := srv.Start(tryAddr); err == nil {
			usedPort = tryPort
			startErr = nil
			break
		} else {
			startErr = err
			low := strings.ToLower(err.Error())
			if !strings.Contains(low, "address already in use") && !strings.Contains(low, "only one usage") && !strings.Contains(low, "bind:") {
				break
			}
		}
	}
	if startErr != nil {
		a.serverRunning = false
		low := strings.ToLower(startErr.Error())
		if strings.Contains(low, "address already in use") || strings.Contains(low, "only one usage") {
			a.notify(fmt.Sprintf(a.t("server.portBusy"), basePort))
		} else {
			a.notify(startErr.Error())
		}
		return
	}
	a.localSrv = srv
	a.serverRunning = true
	if usedPort != basePort {
		a.serverPortStr = strconv.Itoa(usedPort)
		a.notify(fmt.Sprintf(a.t("server.portFallback"), basePort, usedPort))
	}
	a.syncConfigFromState()
}

func (a *App) stopBuiltInServer() {
	if a.localSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.localSrv.Stop(ctx)
		cancel()
		a.localSrv = nil
	}
	a.serverRunning = false
}

func (a *App) toggleBuiltInServer() {
	if a.serverRunning {
		a.startBuiltInServer()
		if a.localSrv != nil && a.localSrv.Running() {
			a.notify(fmt.Sprintf(a.t("server.started"), a.localSrv.LocalURL()))
		}
	} else {
		a.stopBuiltInServer()
		a.notify(a.t("server.stopped"))
	}
}

func (a *App) buildMCPConfigJSON() string {
	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", a.serverPortValue())
	if a.localSrv != nil && a.localSrv.Running() {
		url = a.localSrv.LocalURL() + "/mcp"
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"raglex": map[string]any{
				"url": url,
				"headers": map[string]string{
					"Authorization": "Bearer " + a.ensureServerToken(),
				},
			},
		},
	}
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	return string(raw)
}

// runPlugin executes an installed external plugin against assetIDs off-thread.
func (a *App) runPlugin(pluginID, hook string, assetIDs []string) {
	if a.lib == nil || a.plugins == nil || pluginID == "" {
		return
	}
	payloadAssets := make([]plugin.AssetPayload, 0, len(assetIDs))
	for _, id := range assetIDs {
		if asset, err := a.lib.Get(id); err == nil {
			payloadAssets = append(payloadAssets, plugin.AssetPayload{
				ID:        asset.ID,
				Name:      asset.Label(),
				Path:      asset.Path,
				Mime:      asset.Mime,
				Width:     asset.Width,
				Height:    asset.Height,
				Rating:    asset.Rating,
				Notes:     asset.Notes,
				SourceURL: asset.SourceURL,
				Palette:   asset.Palette,
			})
		}
	}
	req := plugin.Request{
		Hook:        hook,
		LibraryPath: a.cfg.LibraryPath,
		Token:       a.serverToken,
		Assets:      payloadAssets,
	}
	if a.localSrv != nil && a.localSrv.Running() {
		req.ServerURL = a.localSrv.LocalURL()
	}
	mgr := a.plugins
	lib := a.lib
	win := a.win

	run := func() {
		resp, err := mgr.Execute(context.Background(), pluginID, req)
		if err == nil && len(resp.AddTags) > 0 {
			tags, _ := lib.Tags()
			byName := map[string]string{}
			for _, tg := range tags {
				byName[strings.ToLower(tg.Name)] = tg.ID
			}
			for aid, names := range resp.AddTags {
				for _, name := range names {
					name = strings.TrimSpace(name)
					if name == "" {
						continue
					}
					tid, ok := byName[strings.ToLower(name)]
					if !ok {
						if created, cerr := lib.CreateTag(name); cerr == nil {
							tid = created.ID
							byName[strings.ToLower(name)] = tid
						}
					}
					if tid != "" {
						_ = lib.SetAssetTag(aid, tid, true)
					}
				}
			}
		}
		finish := func() {
			if err != nil {
				a.notify(err.Error())
				return
			}
			if resp.Reload || len(resp.AddTags) > 0 {
				a.reload()
			}
			if resp.Toast != "" {
				a.notify(resp.Toast)
			} else {
				a.notify(fmt.Sprintf(a.t("plugins.ran"), pluginID))
			}
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

// refreshPluginMarketplace fetches the community GitHub PR registry.json
// merged with Raglex's built-in official starter plugins off the UI thread.
func (a *App) refreshPluginMarketplace() {
	if a.pluginMarketLoading {
		return
	}
	a.pluginMarketLoading = true
	a.pluginMarketErr = ""
	win := a.win
	run := func() {
		entries, err := plugin.FetchRegistry(context.Background(), plugin.DefaultRegistryURL)
		finish := func() {
			a.pluginMarketLoading = false
			a.pluginMarketLoaded = true
			a.pluginMarket = entries
			if err != nil {
				a.pluginMarketErr = err.Error()
			} else {
				a.pluginMarketErr = ""
			}
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) installMarketplacePlugin(entry plugin.RegistryEntry) {
	if a.plugins == nil || a.pluginMarketBusyID != "" {
		return
	}
	a.pluginMarketBusyID = entry.ID
	mgr := a.plugins
	win := a.win
	run := func() {
		err := mgr.Install(context.Background(), entry)
		finish := func() {
			a.pluginMarketBusyID = ""
			if err != nil {
				a.notify(err.Error())
				return
			}
			a.notify(fmt.Sprintf(a.t("plugins.installedDone"), entry.Name))
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) uninstallPlugin(id, name string) {
	if a.plugins == nil {
		return
	}
	if err := a.plugins.Uninstall(id); err != nil {
		a.notify(err.Error())
		return
	}
	a.notify(fmt.Sprintf(a.t("plugins.uninstalledDone"), name))
}

func (a *App) isPluginEnabled(id string) bool {
	if a.plugins == nil {
		return false
	}
	for _, p := range a.plugins.List() {
		if p.Manifest.ID == id && p.Enabled {
			return true
		}
	}
	return false
}

func pluginHasHook(hooks []string, want string) bool {
	for _, h := range hooks {
		if strings.EqualFold(strings.TrimSpace(h), want) {
			return true
		}
	}
	return false
}

// runLibraryPlugin executes library-wide official plugins (such as the native
// .library migrator and smart duplicate scanner) or custom library_action plugins.
func (a *App) runLibraryPlugin(pluginID string) {
	switch pluginID {
	case "raglex.library-migrator":
		a.startMigrateExternalLibrary()
	case "raglex.duplicate-finder":
		a.runDuplicateFinderPlugin()
	default:
		if len(a.allAssets) > 0 {
			ids := make([]string, 0, len(a.allAssets))
			for _, it := range a.allAssets {
				ids = append(ids, it.ID)
			}
			a.runPlugin(pluginID, "library_action", ids)
		}
	}
}

func (a *App) startMigrateExternalLibrary() {
	if a.lib == nil || a.win == nil || a.importing {
		return
	}
	win := a.win
	lib := a.lib
	mode := model.StorageIndex
	if a.storageMode >= 0 && a.storageMode < len(storageModes) {
		mode = storageModes[a.storageMode]
	}
	title := a.t("plugins.migrateTitle")
	go func() {
		dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     title,
			Directory: true,
		})
		if err != nil || len(dirs) == 0 || strings.TrimSpace(dirs[0]) == "" {
			return
		}
		picked := filepath.Clean(dirs[0])
		ctx, cancel := context.WithCancel(context.Background())
		win.Update(func() {
			if a.isClosed() {
				cancel()
				return
			}
			a.settingsOpen = false
			a.importing = true
			a.importStage = "scanning"
			a.importDone = 0
			a.importTotal = 0
			a.importAdded = 0
			a.importExisting = 0
			a.importFailed = 0
			a.importCurrent = filepath.Base(picked)
			a.importCancel = cancel
		})
		defer cancel()

		var lastPush time.Time
		res, impErr := lib.ImportExternalPackageWithProgress(ctx, picked, mode, func(p library.ImportProgress) {
			now := time.Now()
			if p.Total > 0 && p.Done < p.Total && now.Sub(lastPush) < 70*time.Millisecond {
				return
			}
			lastPush = now
			prog := p
			win.Update(func() {
				if a.isClosed() {
					return
				}
				if a.importStage != "canceling" {
					a.importStage = "importing"
				}
				a.importDone = prog.Done
				a.importTotal = prog.Total
				a.importAdded = prog.Added
				a.importExisting = prog.Existing
				a.importFailed = prog.Failed
				if prog.Current != "" {
					a.importCurrent = prog.Current
				}
			})
		})
		win.Update(func() {
			a.importing = false
			a.importCancel = nil
			a.importStage = ""
			if a.isClosed() {
				return
			}
			if impErr != nil && !errors.Is(impErr, context.Canceled) {
				a.notify(impErr.Error())
				return
			}
			if res.FoldersMapped > 0 {
				a.sidebarBrowseMode = "folders"
			}
			a.reload()
			if errors.Is(impErr, context.Canceled) {
				a.notify(fmt.Sprintf(a.t("import.canceled"), res.AssetsImported, res.AssetsExisting))
				return
			}
			if res.AssetsImported == 0 && res.AssetsExisting == 0 && res.FoldersMapped == 0 {
				a.notify(a.t("plugins.migrateNone"))
				return
			}
			a.notify(fmt.Sprintf(a.t("plugins.migrateDone"), res.AssetsImported, res.FoldersMapped, res.TagsMapped))
		})
	}()
}

// startMigrateExternalPack opens a file picker for external asset package archives
// and imports all contained assets, nested folders (as Raglex Folders), tags,
// ratings, notes, and source URLs into the active library with live progress.
func (a *App) startMigrateExternalPack() {
	if a.lib == nil || a.win == nil || a.importing {
		return
	}
	win := a.win
	lib := a.lib
	title := a.t("plugins.migratePackTitle")
	go func() {
		files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:   win,
			Title:    title,
			Multiple: true,
			Filters: []mygo.FileFilter{
				{Name: "Asset Package Archive", Extensions: []string{"eaglepack", "zip"}},
			},
		})
		if err != nil || len(files) == 0 {
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		win.Update(func() {
			if a.isClosed() {
				cancel()
				return
			}
			a.settingsOpen = false
			a.importing = true
			a.importStage = "scanning"
			a.importDone = 0
			a.importTotal = 0
			a.importAdded = 0
			a.importExisting = 0
			a.importFailed = 0
			a.importCurrent = filepath.Base(files[0])
			a.importCancel = cancel
		})
		defer cancel()

		var totalAssets, totalExisting, totalFolders, totalTags int
		var lastErr error
		var lastPush time.Time
		for _, f := range files {
			if ctx.Err() != nil {
				break
			}
			if strings.TrimSpace(f) == "" {
				continue
			}
			res, impErr := lib.ImportExternalPackageWithProgress(ctx, filepath.Clean(f), model.StorageCopy, func(p library.ImportProgress) {
				now := time.Now()
				if p.Total > 0 && p.Done < p.Total && now.Sub(lastPush) < 70*time.Millisecond {
					return
				}
				lastPush = now
				prog := p
				win.Update(func() {
					if a.isClosed() {
						return
					}
					if a.importStage != "canceling" {
						a.importStage = "importing"
					}
					a.importDone = prog.Done
					a.importTotal = prog.Total
					a.importAdded = totalAssets + prog.Added
					a.importExisting = totalExisting + prog.Existing
					a.importFailed = prog.Failed
					if prog.Current != "" {
						a.importCurrent = prog.Current
					}
				})
			})
			if impErr != nil {
				lastErr = impErr
			}
			totalAssets += res.AssetsImported
			totalExisting += res.AssetsExisting
			totalFolders += res.FoldersMapped
			totalTags += res.TagsMapped
		}
		win.Update(func() {
			a.importing = false
			a.importCancel = nil
			a.importStage = ""
			if a.isClosed() {
				return
			}
			if totalFolders > 0 {
				a.sidebarBrowseMode = "folders"
			}
			a.reload()
			if errors.Is(lastErr, context.Canceled) {
				a.notify(fmt.Sprintf(a.t("import.canceled"), totalAssets, totalExisting))
				return
			}
			if lastErr != nil && totalAssets == 0 && totalFolders == 0 {
				a.notify(lastErr.Error())
				return
			}
			if totalAssets == 0 && totalExisting == 0 && totalFolders == 0 {
				a.notify(a.t("plugins.migrateNone"))
				return
			}
			a.notify(fmt.Sprintf(a.t("plugins.migrateDone"), totalAssets, totalFolders, totalTags))
		})
	}()
}

func (a *App) runDuplicateFinderPlugin() {
	if a.lib == nil {
		return
	}
	// Group by normalized filename stem + exact dimensions (catches renamed copies,
	// re-encoded variants, or identical screenshots from different folders).
	type dupKey struct {
		stem string
		w, h int
	}
	groups := map[dupKey][]model.Asset{}
	for _, it := range a.allAssets {
		if it.Missing {
			continue
		}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path)))
		// Strip trailing " (1)", "_1", "-copy" suffixes so near-duplicate filenames match.
		stem := strings.TrimSpace(strings.TrimRight(base, "0123456789_- ()"))
		if stem == "" {
			stem = base
		}
		k := dupKey{stem: stem, w: it.Width, h: it.Height}
		groups[k] = append(groups[k], it)
	}
	var dupIDs []string
	for _, list := range groups {
		if len(list) >= 2 {
			for _, it := range list {
				dupIDs = append(dupIDs, it.ID)
			}
		}
	}
	if len(dupIDs) == 0 {
		a.notify(a.t("plugins.dupNone"))
		return
	}
	tid := a.lib.EnsureTagID("Duplicate-Review")
	if tid != "" {
		for _, id := range dupIDs {
			_ = a.lib.SetAssetTag(id, tid, true)
		}
		a.reload()
		a.settingsOpen = false
		a.handleNav("tag:" + tid)
	}
	a.notify(fmt.Sprintf(a.t("plugins.dupDone"), len(dupIDs)))
}

// effectiveSyncEndpoint returns the active sync base URL & auth token based on
// whether the user chose Raglex Cloud (default) or Self-Host.
func (a *App) effectiveSyncEndpoint() (string, string) {
	if a.syncProvider == "selfhost" {
		return strings.TrimSpace(a.syncServerURL), strings.TrimSpace(a.syncToken)
	}
	return config.DefaultCloudEndpoint, strings.TrimSpace(a.accountToken)
}

// submitCloudAuth attempts real login or registration against the Raglex Cloud
// placeholder endpoint (https://cloud.raglex.app/api/v1/auth/...). While Cloud
// is not yet deployed, it surfaces an honest unreachable status cleanly.
func (a *App) submitCloudAuth(register bool) {
	if a.authBusy {
		return
	}
	email := strings.TrimSpace(a.authEmailBuf)
	pass := a.authPassBuf
	if email == "" || pass == "" {
		a.authStatusMsg = a.t("account.needCreds")
		return
	}
	a.authBusy = true
	a.authStatusMsg = a.t("account.signingIn")
	endpoint := config.DefaultCloudEndpoint + "/api/v1/auth/login"
	if register {
		endpoint = config.DefaultCloudEndpoint + "/api/v1/auth/register"
	}
	inviteCode := strings.TrimSpace(a.authInviteBuf)
	win := a.win
	run := func() {
		payload := map[string]string{
			"email":    email,
			"password": pass,
		}
		if register && inviteCode != "" {
			payload["invite_code"] = inviteCode
		}
		body, _ := json.Marshal(payload)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
		var resp *http.Response
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "Raglex-Desktop/0.3")
			resp, err = http.DefaultClient.Do(req)
		}
		var out struct {
			OK               bool   `json:"ok"`
			Token            string `json:"token"`
			Email            string `json:"email"`
			Name             string `json:"name"`
			EmailVerified    bool   `json:"email_verified"`
			VerificationSent bool   `json:"verification_sent"`
			Error            string `json:"error"`
		}
		var errMsg string
		if err != nil {
			errMsg = fmt.Sprintf(a.t("account.unreachable"), config.DefaultCloudEndpoint)
		} else {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&out)
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || out.Token == "" {
				if out.Error != "" {
					errMsg = out.Error
				} else {
					errMsg = fmt.Sprintf(a.t("account.unreachable"), fmt.Sprintf("HTTP %d", resp.StatusCode))
				}
			}
		}
		finish := func() {
			a.authBusy = false
			if errMsg != "" {
				a.authStatusMsg = errMsg
				return
			}
			a.accountEmail = out.Email
			if a.accountEmail == "" {
				a.accountEmail = email
			}
			a.accountName = out.Name
			a.accountToken = out.Token
			a.accountVerified = out.EmailVerified
			a.authPassBuf = ""
			a.authInviteBuf = ""
			a.authStatusMsg = fmt.Sprintf(a.t("account.signedInAs"), a.accountEmail)
			if out.VerificationSent {
				a.authStatusMsg = a.t("account.verifySent")
			}
			a.syncConfigFromState()
			a.notify(a.authStatusMsg)
			a.refreshAccountProfile()
			a.scheduleAutoSync(2 * time.Second)
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) submitVerifyEmail() {
	code := strings.TrimSpace(a.authVerifyCodeBuf)
	if a.authBusy || a.accountToken == "" || code == "" {
		return
	}
	a.authBusy = true
	a.authStatusMsg = a.t("account.signingIn")
	tok := a.accountToken
	email := a.accountEmail
	win := a.win
	run := func() {
		body, _ := json.Marshal(map[string]string{"email": email, "code": code})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.DefaultCloudEndpoint+"/api/v1/auth/verify-email", strings.NewReader(string(body)))
		var resp *http.Response
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+tok)
			resp, err = http.DefaultClient.Do(req)
		}
		var out struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		errMsg := ""
		if err != nil {
			errMsg = fmt.Sprintf(a.t("account.unreachable"), config.DefaultCloudEndpoint)
		} else {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&out)
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || !out.OK {
				errMsg = out.Error
				if errMsg == "" {
					errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}
		}
		finish := func() {
			a.authBusy = false
			if errMsg != "" {
				a.authStatusMsg = errMsg
				return
			}
			a.accountVerified = true
			a.authVerifyCodeBuf = ""
			a.authStatusMsg = a.t("account.verifyOK")
			a.syncConfigFromState()
			a.notify(a.authStatusMsg)
			a.refreshAccountProfile()
			a.scheduleAutoSync(2 * time.Second)
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) resendVerifyEmail() {
	if a.authBusy || a.accountToken == "" {
		return
	}
	a.authBusy = true
	a.authStatusMsg = a.t("account.signingIn")
	tok := a.accountToken
	win := a.win
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.DefaultCloudEndpoint+"/api/v1/auth/resend-verify", nil)
		var resp *http.Response
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+tok)
			resp, err = http.DefaultClient.Do(req)
		}
		var out struct {
			OK            bool   `json:"ok"`
			EmailVerified bool   `json:"email_verified"`
			Error         string `json:"error"`
		}
		errMsg := ""
		if err != nil {
			errMsg = fmt.Sprintf(a.t("account.unreachable"), config.DefaultCloudEndpoint)
		} else {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&out)
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || !out.OK {
				errMsg = out.Error
				if errMsg == "" {
					errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}
		}
		finish := func() {
			a.authBusy = false
			if errMsg != "" {
				a.authStatusMsg = errMsg
				return
			}
			if out.EmailVerified {
				a.accountVerified = true
				a.authStatusMsg = a.t("account.verifyOK")
				a.syncConfigFromState()
			} else {
				a.authStatusMsg = a.t("account.verifySent")
			}
			a.notify(a.authStatusMsg)
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) sendPasswordResetCode() {
	email := strings.TrimSpace(a.authEmailBuf)
	if a.authBusy || email == "" {
		a.authStatusMsg = a.t("account.needCreds")
		return
	}
	a.authBusy = true
	a.authStatusMsg = a.t("account.signingIn")
	win := a.win
	run := func() {
		body, _ := json.Marshal(map[string]string{"email": email})
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.DefaultCloudEndpoint+"/api/v1/auth/forgot-password", strings.NewReader(string(body)))
		var resp *http.Response
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			resp, err = http.DefaultClient.Do(req)
		}
		var out struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		errMsg := ""
		if err != nil {
			errMsg = fmt.Sprintf(a.t("account.unreachable"), config.DefaultCloudEndpoint)
		} else {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&out)
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || !out.OK {
				errMsg = out.Error
				if errMsg == "" {
					errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}
		}
		finish := func() {
			a.authBusy = false
			if errMsg != "" {
				a.authStatusMsg = errMsg
				return
			}
			a.authStatusMsg = a.t("account.resetSent")
			a.notify(a.authStatusMsg)
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) submitPasswordReset() {
	email := strings.TrimSpace(a.authEmailBuf)
	code := strings.TrimSpace(a.authResetCodeBuf)
	newPass := a.authResetNewPassBuf
	if a.authBusy || email == "" || code == "" || newPass == "" {
		a.authStatusMsg = a.t("account.needCreds")
		return
	}
	a.authBusy = true
	a.authStatusMsg = a.t("account.signingIn")
	win := a.win
	run := func() {
		body, _ := json.Marshal(map[string]string{
			"email":        email,
			"code":         code,
			"new_password": newPass,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.DefaultCloudEndpoint+"/api/v1/auth/reset-password", strings.NewReader(string(body)))
		var resp *http.Response
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			resp, err = http.DefaultClient.Do(req)
		}
		var out struct {
			OK            bool   `json:"ok"`
			Token         string `json:"token"`
			Email         string `json:"email"`
			Name          string `json:"name"`
			EmailVerified bool   `json:"email_verified"`
			Error         string `json:"error"`
		}
		errMsg := ""
		if err != nil {
			errMsg = fmt.Sprintf(a.t("account.unreachable"), config.DefaultCloudEndpoint)
		} else {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&out)
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || out.Token == "" {
				errMsg = out.Error
				if errMsg == "" {
					errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}
		}
		finish := func() {
			a.authBusy = false
			if errMsg != "" {
				a.authStatusMsg = errMsg
				return
			}
			a.accountEmail = out.Email
			if a.accountEmail == "" {
				a.accountEmail = email
			}
			a.accountName = out.Name
			a.accountToken = out.Token
			a.accountVerified = out.EmailVerified
			a.authResetMode = false
			a.authResetCodeBuf = ""
			a.authResetNewPassBuf = ""
			a.authPassBuf = ""
			a.authStatusMsg = a.t("account.resetOK")
			a.syncConfigFromState()
			a.notify(a.authStatusMsg)
			a.refreshAccountProfile()
			a.scheduleAutoSync(2 * time.Second)
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) signOutAccount() {
	a.accountEmail = ""
	a.accountName = ""
	a.accountToken = ""
	a.accountVerified = false
	a.authPassBuf = ""
	a.authVerifyCodeBuf = ""
	a.authStatusMsg = ""
	a.syncConfigFromState()
	a.notify(a.t("account.signedOut"))
}

// refreshAccountProfile silently queries GET /api/v1/auth/me in the background
// so email_verified and profile metadata stay in sync across app restarts & upgrades.
func (a *App) refreshAccountProfile() {
	if a.win == nil || a.accountToken == "" || a.syncProvider == "selfhost" {
		return
	}
	tok := strings.TrimSpace(a.accountToken)
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.DefaultCloudEndpoint+"/api/v1/auth/me", nil)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", "Raglex-Desktop/0.4")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}
		var out struct {
			OK            bool   `json:"ok"`
			Email         string `json:"email"`
			Name          string `json:"name"`
			EmailVerified bool   `json:"email_verified"`
			UsedBytes     uint64 `json:"used_bytes"`
			QuotaBytes    int64  `json:"quota_bytes"`
		}
		if json.NewDecoder(resp.Body).Decode(&out) != nil || !out.OK {
			return
		}
		win.Update(func() {
			if a.isClosed() || strings.TrimSpace(a.accountToken) != tok {
				return
			}
			changed := a.accountVerified != out.EmailVerified
			if out.Email != "" && a.accountEmail != out.Email {
				a.accountEmail = out.Email
				changed = true
			}
			if out.Name != "" && a.accountName != out.Name {
				a.accountName = out.Name
				changed = true
			}
			a.accountVerified = out.EmailVerified
			a.accountUsedBytes = out.UsedBytes
			if out.QuotaBytes > 0 {
				a.accountQuotaBytes = out.QuotaBytes
			} else if a.accountQuotaBytes <= 0 {
				a.accountQuotaBytes = 20 << 30
			}
			if changed {
				a.syncConfigFromState()
			}
		})
	}()
}

// runCloudSyncNow executes a user-initiated bidirectional delta + concurrent
// SHA-256 blob sync with live sidebar progress and toast feedback.
func (a *App) runCloudSyncNow() {
	a.runCloudSync(false, false)
}

// runCloudSync executes a bidirectional sync. When quiet is true (smart
// background auto-sync), no-op scans stay completely invisible and offline
// errors back off silently without spamming toasts. When pullOnly is true
// (periodic idle poll), it skips local DB snapshotting and push entirely,
// sending only a ~150B GET /v1/sync/pull request.
func (a *App) runCloudSync(quiet, pullOnly bool) {
	serverURL, token := a.effectiveSyncEndpoint()
	if a.lib == nil || a.syncBusy || serverURL == "" {
		return
	}
	if quiet && (!a.autoSync || a.importing || a.collabBusy || !a.hasSyncConnection()) {
		return
	}
	if !quiet {
		a.syncConfigFromState()
		a.autoSyncFailCount = 0
	}
	a.syncBusy = true
	a.syncHidden = quiet
	a.syncStage = "scanning"
	a.syncDone = 0
	a.syncTotal = 0
	a.syncCurrent = ""
	if !quiet {
		a.syncStatusMsg = a.t("sync.syncing")
	}
	lib := a.lib
	projs := a.projects
	exMap := make(map[string]bool, len(a.syncExcludeFolders))
	for id, off := range a.syncExcludeFolders {
		if off {
			exMap[id] = true
		}
	}
	cfg := rsync.ClientConfig{
		ServerURL:            serverURL,
		Token:                token,
		DeviceID:             "", // Always use sync.db's persistent unique per-library DeviceID so Windows/Mac never collide
		E2EEPassphrase:       a.effectiveSyncE2EEKey(),
		ExcludedFolderIDs:    exMap,
		ExcludeUncategorized: !a.syncIncludeUncat,
		PullOnly:             pullOnly,
	}
	win := a.win
	run := func() {
		var lastPush time.Time
		onProgress := func(p rsync.SyncProgress) {
			now := time.Now()
			if p.Total > 0 && p.Done < p.Total && now.Sub(lastPush) < 80*time.Millisecond {
				return
			}
			lastPush = now
			prog := p
			apply := func() {
				a.syncStage = prog.Stage
				a.syncDone = prog.Done
				a.syncTotal = prog.Total
				a.syncCurrent = prog.Current
				if prog.Stage == "uploading" || prog.Stage == "downloading" || prog.Total > 0 {
					a.syncHidden = false
				}
			}
			if win == nil {
				apply()
				return
			}
			win.Update(func() {
				if !a.isClosed() {
					apply()
				}
			})
		}
		rep, err := rsync.SyncNowWithProgress(context.Background(), cfg, lib, projs, onProgress)
		finish := func() {
			a.syncBusy = false
			a.syncHidden = false
			a.syncStage = ""
			a.syncDone = 0
			a.syncTotal = 0
			a.syncCurrent = ""
			if err != nil {
				a.syncStatusMsg = err.Error()
				if quiet {
					a.autoSyncFailCount++
				} else {
					a.notify(err.Error())
				}
				return
			}
			a.autoSyncFailCount = 0
			a.lastSyncAt = time.Now()
			msg := fmt.Sprintf(a.t("sync.done"), rep.PushedOps, rep.PulledOps, rep.UploadedBlobs, rep.DownloadedBlobs)
			if rep.DecryptFailedOps > 0 {
				msg = fmt.Sprintf(a.t("sync.e2eeDecryptFailed"), rep.DecryptFailedOps)
			}
			a.syncStatusMsg = msg
			pulled := rep.PulledOps + rep.DownloadedBlobs
			pushed := rep.PushedOps + rep.UploadedBlobs
			if pulled > 0 {
				a.autoSyncSuppress = true
				a.reload()
				a.autoSyncSuppress = false
			}
			if !quiet || pushed > 0 || pulled > 0 {
				a.refreshAccountProfile()
			}
			if !quiet || rep.DecryptFailedOps > 0 {
				a.notify(msg)
			} else if pulled > 0 {
				a.notify(fmt.Sprintf(a.t("sync.pulledNotice"), pulled))
			}
			if a.autoSyncPending {
				a.autoSyncPending = false
				a.scheduleAutoSync(2 * time.Second)
			}
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

// scheduleAutoSync coalesces rapid local edits (imports, tags, folders, boards)
// into a single quiet background sync after delay.
func (a *App) scheduleAutoSync(delay time.Duration) {
	if a.win == nil || !a.autoSync || a.autoSyncSuppress || a.importing || a.lib == nil || !a.hasSyncConnection() {
		return
	}
	if a.syncBusy {
		a.autoSyncPending = true
		return
	}
	if delay <= 0 {
		delay = 4 * time.Second
	}
	win := a.win
	a.mu.Lock()
	if a.autoSyncTimer != nil {
		a.autoSyncTimer.Stop()
	}
	a.autoSyncTimer = time.AfterFunc(delay, func() {
		if win == nil {
			return
		}
		win.Update(func() {
			if a.isClosed() || !a.autoSync || a.lib == nil {
				return
			}
			a.runCloudSync(true, false)
		})
	})
	a.mu.Unlock()
}

// startAutoSyncLoop runs a lightweight background sync loop: an initial quiet
// sync shortly after opening a library, plus periodic 90s pulls with automatic
// backoff when offline.
func (a *App) startAutoSyncLoop() {
	a.stopAutoSyncLoop()
	if a.win == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.autoSyncCancel = cancel
	a.mu.Unlock()
	win := a.win
	go func() {
		// Initial quiet sync 3s after library open.
		initTimer := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			initTimer.Stop()
			return
		case <-initTimer.C:
		}
		win.Update(func() {
			if a.isClosed() || a.lib == nil {
				return
			}
			if a.autoSync && a.hasSyncConnection() {
				a.runCloudSync(true, false)
			}
		})

		ticker := time.NewTicker(90 * time.Second)
		defer ticker.Stop()
		ticksSkipped := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				win.Update(func() {
					if a.isClosed() || a.lib == nil || !a.autoSync || !a.hasSyncConnection() {
						return
					}
					if a.autoSyncFailCount > 0 {
						skipLimit := 1 << min(a.autoSyncFailCount, 3)
						if ticksSkipped < skipLimit-1 {
							ticksSkipped++
							return
						}
					}
					ticksSkipped = 0
					if !a.syncBusy && !a.importing {
						a.runCloudSync(true, true)
					}
					if !a.collabBusy && len(a.collabLocalMap) > 0 {
						a.syncLinkedCollabsQuietly()
					}
				})
			}
		}
	}()
}

// stopAutoSyncLoop cancels any active periodic loop and pending debounce timer.
func (a *App) stopAutoSyncLoop() {
	a.mu.Lock()
	cancel := a.autoSyncCancel
	a.autoSyncCancel = nil
	timer := a.autoSyncTimer
	a.autoSyncTimer = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if timer != nil {
		timer.Stop()
	}
	a.autoSyncPending = false
}

// syncLinkedCollabsQuietly refreshes joined collaboration spaces in the
// background so teammates' newly shared reference assets arrive automatically.
func (a *App) syncLinkedCollabsQuietly() {
	database := a.shareDB.Load()
	if database == nil && a.lib != nil {
		database = a.lib.DB
	}
	serverURL, token := a.effectiveSyncEndpoint()
	if database == nil || token == "" || a.collabBusy || a.win == nil || len(a.collabLocalMap) == 0 {
		return
	}
	links := make(map[string]string, len(a.collabLocalMap))
	for k, v := range a.collabLocalMap {
		links[k] = v
	}
	cfg := rsync.ClientConfig{ServerURL: serverURL, Token: token}
	win := a.win
	a.collabBusy = true
	go func() {
		list, err := rsync.ListCollabSpaces(context.Background(), cfg)
		changedAny := false
		if err == nil {
			for _, cs := range list {
				localID := links[cs.ID]
				if localID == "" {
					continue
				}
				if res, serr := rsync.SyncCollabSpace(context.Background(), cfg, database, cs.ID, cs.Kind, localID, nil); serr == nil {
					if res.Downloaded > 0 {
						changedAny = true
					}
				}
			}
		}
		win.Update(func() {
			a.collabBusy = false
			if a.isClosed() {
				return
			}
			if err == nil {
				a.collabList = list
				a.collabLoaded = true
			}
			if changedAny {
				a.autoSyncSuppress = true
				a.reload()
				a.autoSyncSuppress = false
			}
		})
	}()
}

// createPublicShareLink ensures the built-in Raglex server is running and mints
// a shareable web URL (/s/<id>) with optional password & download toggle.
func (a *App) createPublicShareLink(kind, targetID, title, password string, allowDownload bool) (string, error) {
	if a.localSrv == nil || !a.localSrv.Running() {
		a.startBuiltInServer()
	}
	if a.localSrv == nil || !a.localSrv.Running() {
		return "", fmt.Errorf("built-in server is not running")
	}
	_, relPath, err := a.localSrv.CreatePublicShare(rsync.CreateShareRequest{
		Kind:          kind,
		TargetID:      targetID,
		Title:         title,
		Password:      strings.TrimSpace(password),
		AllowDownload: allowDownload,
	})
	if err != nil {
		return "", err
	}
	base := a.localSrv.LocalURL()
	if a.serverBindAll {
		if lan := a.localSrv.LANURL(); lan != "" {
			base = lan
		}
	}
	return base + relPath, nil
}

// hasSyncConnection reports whether the user has signed into Raglex Cloud OR
// configured a Self-Host sync server URL (gates the Sync & Collab settings tab).
func (a *App) hasSyncConnection() bool {
	if a.syncProvider == "selfhost" {
		return strings.TrimSpace(a.syncServerURL) != ""
	}
	return strings.TrimSpace(a.accountToken) != ""
}

func (a *App) refreshCollabSpaces() {
	serverURL, token := a.effectiveSyncEndpoint()
	if token == "" || a.collabBusy {
		return
	}
	cfg := rsync.ClientConfig{ServerURL: serverURL, Token: token}
	win := a.win
	run := func() {
		list, err := rsync.ListCollabSpaces(context.Background(), cfg)
		finish := func() {
			if err == nil {
				a.collabList = list
				a.collabLoaded = true
			}
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) createCollabFromTarget(kind, targetID string) {
	database := a.shareDB.Load()
	if database == nil && a.lib != nil {
		database = a.lib.DB
	}
	serverURL, token := a.effectiveSyncEndpoint()
	if database == nil || a.collabBusy || token == "" || targetID == "" {
		if token == "" {
			a.notify(a.t("collab.needCloud"))
		}
		return
	}
	a.collabBusy = true
	a.syncBusy = true
	a.syncStage = "uploading"
	a.syncDone = 0
	a.syncTotal = 0
	a.syncCurrent = ""
	cfg := rsync.ClientConfig{ServerURL: serverURL, Token: token}
	win := a.win
	run := func() {
		var lastPush time.Time
		cs, err := rsync.CreateCollabSpace(context.Background(), cfg, database, kind, targetID, func(p rsync.SyncProgress) {
			now := time.Now()
			if p.Total > 0 && p.Done < p.Total && now.Sub(lastPush) < 80*time.Millisecond {
				return
			}
			lastPush = now
			prog := p
			if win != nil {
				win.Update(func() {
					if !a.isClosed() {
						a.syncStage = prog.Stage
						a.syncDone = prog.Done
						a.syncTotal = prog.Total
						a.syncCurrent = prog.Current
					}
				})
			}
		})
		finish := func() {
			a.collabBusy = false
			a.syncBusy = false
			a.syncStage = ""
			a.syncDone = 0
			a.syncTotal = 0
			a.syncCurrent = ""
			if err != nil {
				a.notify(err.Error())
				return
			}
			if a.collabLocalMap == nil {
				a.collabLocalMap = map[string]string{}
			}
			a.collabLocalMap[cs.ID] = targetID
			a.collabLastCode = cs.Code
			a.syncConfigFromState()
			a.notify(fmt.Sprintf(a.t("collab.created"), cs.Title, cs.Code))
			a.refreshCollabSpaces()
			a.refreshAccountProfile()
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) joinCollabByCode(rawCode string) {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	serverURL, token := a.effectiveSyncEndpoint()
	if code == "" || a.collabBusy {
		return
	}
	if token == "" {
		a.notify(a.t("collab.needCloud"))
		return
	}
	a.collabBusy = true
	cfg := rsync.ClientConfig{ServerURL: serverURL, Token: token}
	win := a.win
	run := func() {
		cs, err := rsync.JoinCollabSpace(context.Background(), cfg, code)
		finish := func() {
			a.collabBusy = false
			if err != nil {
				a.notify(err.Error())
				return
			}
			a.collabJoinCode = ""
			a.pairRecvCode = ""
			a.pairRecvOpen = false
			a.notify(fmt.Sprintf(a.t("collab.joined"), cs.Title))
			a.refreshCollabSpaces()
			a.syncCollabSpaceByID(cs.ID, cs.Kind)
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) syncCollabSpaceByID(collabID, kind string) {
	database := a.shareDB.Load()
	if database == nil && a.lib != nil {
		database = a.lib.DB
	}
	serverURL, token := a.effectiveSyncEndpoint()
	if database == nil || a.collabBusy || token == "" || collabID == "" {
		return
	}
	a.collabBusy = true
	a.syncBusy = true
	a.syncStage = "pulling"
	a.syncDone = 0
	a.syncTotal = 0
	a.syncCurrent = ""
	localID := ""
	if a.collabLocalMap != nil {
		localID = a.collabLocalMap[collabID]
	}
	cfg := rsync.ClientConfig{ServerURL: serverURL, Token: token}
	win := a.win
	run := func() {
		var lastPush time.Time
		res, err := rsync.SyncCollabSpace(context.Background(), cfg, database, collabID, kind, localID, func(p rsync.SyncProgress) {
			now := time.Now()
			if p.Total > 0 && p.Done < p.Total && now.Sub(lastPush) < 80*time.Millisecond {
				return
			}
			lastPush = now
			prog := p
			if win != nil {
				win.Update(func() {
					if !a.isClosed() {
						a.syncStage = prog.Stage
						a.syncDone = prog.Done
						a.syncTotal = prog.Total
						a.syncCurrent = prog.Current
					}
				})
			}
		})
		finish := func() {
			a.collabBusy = false
			a.syncBusy = false
			a.syncStage = ""
			a.syncDone = 0
			a.syncTotal = 0
			a.syncCurrent = ""
			if err != nil {
				a.notify(err.Error())
				return
			}
			if a.collabLocalMap == nil {
				a.collabLocalMap = map[string]string{}
			}
			if res.LocalTargetID != "" {
				a.collabLocalMap[collabID] = res.LocalTargetID
				a.syncConfigFromState()
			}
			a.reload()
			a.notify(fmt.Sprintf(a.t("collab.synced"), res.Collab.Title))
			a.refreshCollabSpaces()
			a.refreshAccountProfile()
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}

func (a *App) leaveOrDeleteCollabSpace(collabID string) {
	serverURL, token := a.effectiveSyncEndpoint()
	if token == "" || collabID == "" || a.collabBusy {
		return
	}
	a.collabBusy = true
	cfg := rsync.ClientConfig{ServerURL: serverURL, Token: token}
	win := a.win
	run := func() {
		err := rsync.LeaveOrDeleteCollab(context.Background(), cfg, collabID)
		finish := func() {
			a.collabBusy = false
			if err != nil {
				a.notify(err.Error())
				return
			}
			delete(a.collabLocalMap, collabID)
			a.syncConfigFromState()
			a.notify(a.t("collab.left"))
			a.refreshCollabSpaces()
		}
		if win == nil {
			finish()
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				finish()
			}
		})
	}
	if win == nil {
		run()
		return
	}
	go run()
}
