package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"raglex/internal/mcp"
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

// runCloudSyncNow executes a bidirectional delta + SHA-256 blob sync with the
// configured Raglex Sync API v1 server (self-hosted Docker or Raglex Cloud).
func (a *App) runCloudSyncNow() {
	if a.lib == nil || a.syncBusy || strings.TrimSpace(a.syncServerURL) == "" {
		return
	}
	a.syncConfigFromState()
	a.syncBusy = true
	a.syncStatusMsg = a.t("sync.syncing")
	lib := a.lib
	projs := a.projects
	cfg := rsync.ClientConfig{
		ServerURL: strings.TrimSpace(a.syncServerURL),
		Token:     strings.TrimSpace(a.syncToken),
		DeviceID:  strings.TrimSpace(a.deviceName),
	}
	win := a.win
	run := func() {
		rep, err := rsync.SyncNow(context.Background(), cfg, lib, projs)
		finish := func() {
			a.syncBusy = false
			if err != nil {
				a.syncStatusMsg = err.Error()
				a.notify(err.Error())
				return
			}
			msg := fmt.Sprintf(a.t("sync.done"), rep.PushedOps, rep.PulledOps, rep.UploadedBlobs, rep.DownloadedBlobs)
			a.syncStatusMsg = msg
			if rep.PulledOps > 0 || rep.DownloadedBlobs > 0 {
				a.reload()
			}
			a.notify(msg)
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
