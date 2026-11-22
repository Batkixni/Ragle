package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ragle/internal/mcp"
	"ragle/internal/server"
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
	addr := fmt.Sprintf("%s:%d", host, port)

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
	if _, err := srv.Start(addr); err != nil {
		a.serverRunning = false
		a.notify(err.Error())
		return
	}
	a.localSrv = srv
	a.serverRunning = true
	a.syncConfigFromState()
	a.notify(fmt.Sprintf(a.t("server.started"), srv.LocalURL()))
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
			"ragle": map[string]any{
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
