package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/config"
	"raglex/internal/share"
	rsync "raglex/internal/sync"
)

func (a *App) startShare() {
	name := strings.TrimSpace(a.deviceName)
	if name == "" {
		if host, err := os.Hostname(); err == nil && host != "" {
			name = host
		} else {
			name = "Raglex"
		}
	}
	win := a.win
	node, err := share.Start(share.Options{
		Name: name,
		OnPeers: func() {
			if win != nil {
				win.Invalidate()
			}
		},
		OnError: func(shareErr error) {
			if win == nil {
				return
			}
			win.Update(func() {
				if a.isClosed() {
					return
				}
				a.status = a.shareErrText(shareErr)
			})
		},
		Decide: a.decide,
		Take:   a.shareTake,
		Commit: a.shareCommit,
	})
	if node != nil {
		a.mu.Lock()
		a.node = node
		a.mu.Unlock()
	}
	if err != nil || node == nil || !node.DiscoveryOK() {
		a.shareDown = true
		if a.status == "" {
			a.status = a.t("share.unavailable")
		}
	}
}

func (a *App) shareNode() *share.Node {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.node
}

// openShareProject opens the Share modal targeting a Project.
func (a *App) openShareProject(projectID string) {
	if projectID == "" {
		return
	}
	a.shareKind = "project"
	a.shareTargetID = projectID
	a.shareTargetTitle = a.project.Name
	a.cloudShareCode = ""
	a.cloudShareURL = ""
	a.cloudShareProgress = ""
	a.collabLastCode = ""
	a.pubShareURL = ""
	a.shareOpen = true
}

// openShareFolder opens the Share modal targeting a Folder (and all nested subfolders).
func (a *App) openShareFolder(folderID string) {
	if folderID == "" {
		return
	}
	a.ensureFolderTree()
	f, ok := a.folderByID[folderID]
	if !ok {
		return
	}
	a.shareKind = "folder"
	a.shareTargetID = folderID
	a.shareTargetTitle = f.Name
	a.cloudShareCode = ""
	a.cloudShareURL = ""
	a.cloudShareProgress = ""
	a.collabLastCode = ""
	a.pubShareURL = ""
	a.shareOpen = true
}

func (a *App) activeShareTarget() (kind, id, title string) {
	if a.shareKind == "folder" && a.shareTargetID != "" {
		title = a.shareTargetTitle
		if f, ok := a.folderByID[a.shareTargetID]; ok {
			title = f.Name
		}
		return "folder", a.shareTargetID, title
	}
	if a.shareKind == "project" && a.shareTargetID != "" {
		title = a.shareTargetTitle
		if title == "" {
			title = a.project.Name
		}
		return "project", a.shareTargetID, title
	}
	if a.projectID != "" {
		return "project", a.projectID, a.project.Name
	}
	return "", "", ""
}

func (a *App) shareModal(c *gui.Context) {
	if !a.shareOpen {
		return
	}
	kind, targetID, targetTitle := a.activeShareTarget()
	if targetID == "" {
		return
	}
	t := c.Theme()
	services := c.Services()
	_, cloudTok := a.effectiveSyncEndpoint()
	cloudSignedIn := cloudTok != ""

	a.styledDialog(c, "share-modal", &a.shareOpen, 540, 0, func(_ gui.Element) {
		gui.Column(c).Padding(18, 18).Gap(4).Children(func() {
			gui.Row(c).Padding(0, 6, 8, 6).Gap(10).AlignItems(gui.Center).Children(func() {
				headerIcon := iconShare
				if kind == "folder" {
					headerIcon = iconFolder
				}
				gui.Icon(c, headerIcon).Size(17, 17).TextColor(a.pal.ink)
				gui.Text(c, a.t("share.send")+" · "+targetTitle).FontSize(15).Bold().Grow(1).SingleLine()
				a.iconButton(c, "share-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.shareOpen = false
				})
			})

			gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
			gui.Row(c).FillWidth().Padding(8, 6).AlignItems(gui.Center).Children(func() {
				gui.Checkbox(c.Key("share-files"), &a.shareFiles, a.t("share.files"))
			})

			// 1. Raglex Cloud Public Share Code (anyone—even without an account—can download by code).
			a.settingsSection(c, a.t("share.cloudCard"), a.t("share.cloudCardTip"), true, func() {
				if !cloudSignedIn {
					gui.Row(c).FillWidth().Padding(6, 10).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Icon(c, iconCloud).Size(15, 15).TextColor(a.pal.inkSecondary)
						gui.Text(c, a.t("share.cloudSignInNeed")).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1)
						a.arcButton(c, "cloud-share-signin", a.t("share.cloudSignInBtn"), true).OnClick(func() {
							a.shareOpen = false
							a.openSettings(settingsTabAccount)
						})
					})
				} else if a.cloudShareCode != "" {
					code := a.cloudShareCode
					shareURL := a.cloudShareURL
					gui.Column(c).FillWidth().Padding(4, 8).Gap(8).Children(func() {
						gui.Row(c).
							FillWidth().
							Padding(10, 12).
							Gap(8).
							Radius(10).
							AlignItems(gui.Center).
							Background(a.settingsRowHoverBg()).
							Children(func() {
								gui.Icon(c, iconCloud).Size(15, 15).TextColor(a.pal.controlOn)
								gui.Text(c, code).FontSize(14).Bold().TextColor(a.pal.ink).Grow(1).SingleLine()
								a.arcButton(c, "cloud-share-copy-code", a.t("share.cloudCopyCode"), true).OnClick(func() {
									services.WriteClipboard(code)
									a.notify(a.t("share.codeCopied"))
								})
								if shareURL != "" {
									a.arcButton(c, "cloud-share-copy-url", a.t("share.cloudCopyURL"), false).OnClick(func() {
										services.WriteClipboard(shareURL)
										a.notify(a.t("share.cloudURLCopied"))
									})
								}
								a.toolButton(c, "cloud-share-revoke", iconTrash, a.t("share.cloudRevoke"), false).
									Padding(4, 8).
									OnClick(func() { a.revokeCloudPublicShare() })
							})
						if shareURL != "" {
							gui.Text(c, shareURL).FontSize(11).TextColor(a.pal.inkMuted).Padding(0, 4).SingleLine()
						}
					})
				} else {
					gui.Row(c).FillWidth().Padding(6, 10).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Icon(c, iconCloud).Size(15, 15).TextColor(a.pal.inkSecondary)
						if a.cloudShareBusy {
							gui.Spinner(c)
							msg := a.t("share.cloudPublishing")
							if a.cloudShareProgress != "" {
								msg = a.cloudShareProgress
							}
							gui.Text(c, msg).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
						} else {
							gui.PrimaryButton(c.Key("cloud-share-publish"), a.t("share.cloudPublish")).
								OnClick(func() { a.startCloudPublicShare(kind, targetID) })
							gui.Spacer(c)
						}
					})
				}
			})

			// 1.5 Team Collaboration Space (CLB-XXXX-XXXX; shared refs, per-user whiteboards).
			a.settingsSection(c, a.t("share.collabCard"), a.t("share.collabCardTip"), true, func() {
				if !cloudSignedIn {
					gui.Row(c).FillWidth().Padding(6, 10).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Icon(c, iconUser).Size(15, 15).TextColor(a.pal.inkSecondary)
						gui.Text(c, a.t("collab.needCloud")).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1)
						a.arcButton(c, "collab-share-signin", a.t("share.cloudSignInBtn"), true).OnClick(func() {
							a.shareOpen = false
							a.openSettings(settingsTabAccount)
						})
					})
				} else if a.collabLastCode != "" {
					code := a.collabLastCode
					gui.Row(c).
						FillWidth().
						Padding(8, 12).
						Gap(8).
						Radius(10).
						AlignItems(gui.Center).
						Background(a.settingsRowHoverBg()).
						Children(func() {
							gui.Icon(c, iconUser).Size(15, 15).TextColor(a.pal.controlOn)
							gui.Text(c, code).FontSize(14).Bold().TextColor(a.pal.ink).Grow(1).SingleLine()
							a.arcButton(c, "collab-share-copy", a.t("collab.copyCode"), true).OnClick(func() {
								services.WriteClipboard(code)
								a.notify(a.t("share.codeCopied"))
							})
						})
				} else {
					gui.Row(c).FillWidth().Padding(6, 10).Gap(10).AlignItems(gui.Center).Children(func() {
						gui.Icon(c, iconUser).Size(15, 15).TextColor(a.pal.inkSecondary)
						if a.collabBusy {
							gui.Spinner(c)
							gui.Text(c, a.t("share.cloudPublishing")).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
						} else {
							a.arcButton(c, "collab-share-create", a.t("share.collabCreate"), false).
								Disabled(a.collabBusy).
								OnClick(func() { a.createCollabFromTarget(kind, targetID) })
							gui.Spacer(c)
						}
					})
				}
			})

			// 2. Same Wi-Fi transfer (auto discovery, zero setup).
			a.settingsSection(c, a.t("share.lanCard"), "", true, func() {
				node := a.shareNode()
				if node == nil || !node.DiscoveryOK() {
					gui.Text(c, a.t("share.unavailable")).FontSize(12).TextColor(t.TextMuted).Padding(4, 10)
				} else {
					peers := node.Peers()
					if len(peers) == 0 {
						gui.Text(c, a.t("share.none")).FontSize(12).TextColor(t.TextMuted).Padding(4, 10)
					}
					for _, peer := range peers {
						item := peer
						label := item.Name
						if label == "" {
							label = item.Addr
						}
						a.settingsItemRow(c, "share-row-"+item.ID, iconGlobe, label, "", func() {
							gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
								gui.Text(c, item.Addr).FontSize(11).TextColor(t.TextMuted)
								gui.PrimaryButton(c, a.t("share.sendTo")).Disabled(a.shareSending).OnClick(func() {
									a.sendShareTarget(item.Addr, kind, targetID)
								})
							})
						})
					}
				}
			})

			// 3. Cross-network pairing code transfer (Direct DTLS P2P + E2E encrypted blind relay fallback).
			a.settingsSection(c, a.t("share.codeCard"), a.t("share.codeCardTip"), true, func() {
				gui.Row(c).FillWidth().Padding(6, 10).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconLink).Size(15, 15).TextColor(a.pal.inkSecondary)
					if a.pairCode == "" {
						gui.PrimaryButton(c.Key("pair-create"), a.t("share.createCode")).
							Disabled(a.shareSending).
							OnClick(func() { a.startPairSend(kind, targetID) })
					} else {
						gui.Button(c.Key("pair-cancel"), a.t("common.cancel")).OnClick(func() {
							a.cancelPair()
						})
					}
					if a.pairTransport == "p2p" {
						a.pillBadge(c, a.t("share.modeP2P"), gui.Hex("#10B981"), gui.RGBA(16, 185, 129, 0.14))
					} else if a.pairTransport == "e2e-relay" {
						a.pillBadge(c, a.t("share.modeRelay"), gui.Hex("#38BDF8"), gui.RGBA(56, 189, 248, 0.14))
					}
					gui.Spacer(c)
					a.toolButton(c, "pair-adv-toggle", iconSliders, a.t("pref.advConn"), a.pairAdvRelayOpen).OnClick(func() {
						a.pairAdvRelayOpen = !a.pairAdvRelayOpen
					})
				})
				if a.pairAdvRelayOpen {
					a.settingsItemRow(c, "pair-relay", iconGlobe, a.t("pref.advRelay"), a.t("pref.advRelayTip"), func() {
						gui.TextInput(c.Key("signal-url"), &a.signalURL).
							Placeholder("http://127.0.0.1:47595").
							Width(240)
					})
					gui.Row(c).FillWidth().Padding(2, 10, 6, 35).Children(func() {
						gui.Checkbox(c.Key("pair-force-relay"), &a.pairForceRelay, a.t("share.forceRelay")).OnChange(func() {
							a.syncConfigFromState()
						})
					})
				}
				if a.pairCode != "" {
					code := a.pairCode
					gui.Row(c).
						Padding(8, 10).
						Gap(10).
						Radius(8).
						AlignItems(gui.Center).
						Background(a.settingsRowHoverBg()).
						Children(func() {
							gui.Text(c, fmt.Sprintf(a.t("share.codeWaiting"), code)).
								FontSize(13.5).
								Bold().
								TextColor(a.pal.ink).
								Grow(1)
							a.arcButton(c, "pair-copy", a.t("share.copyCode"), true).OnClick(func() {
								services.WriteClipboard(code)
								a.status = a.t("share.codeCopied")
							})
						})
				}
			})

			// 4. Local Web Share Link (for Projects via built-in Raglex server /s/<id>).
			if kind == "project" {
				a.settingsSection(c, a.t("share.webCard"), a.t("share.webCardTip"), true, func() {
					gui.Row(c).FillWidth().Padding(6, 10).Gap(8).AlignItems(gui.Center).Children(func() {
						gui.Icon(c, iconExternal).Size(15, 15).TextColor(a.pal.inkSecondary)
						gui.TextInput(c.Key("pub-share-pass"), &a.pubSharePass).
							Placeholder(a.t("share.publicPass")).
							Grow(1)
						a.arcButton(c, "pub-share-create", a.t("share.publicCreate"), false).OnClick(func() {
							if u, err := a.createPublicShareLink("project", targetID, targetTitle, a.pubSharePass, a.shareFiles); err == nil {
								a.pubShareURL = u
								services.WriteClipboard(u)
								a.notify(fmt.Sprintf(a.t("share.publicCopied"), u))
							} else {
								a.notify(err.Error())
							}
						})
					})
					if a.pubShareURL != "" {
						gui.Text(c, a.pubShareURL).FontSize(11).TextColor(a.pal.inkMuted).Padding(0, 10, 4, 35).SingleLine()
					}
				})
			}

			if a.status != "" {
				gui.Text(c, a.status).FontSize(11.5).TextColor(t.TextMuted).Padding(6, 10, 0, 10)
			}
		})
	})
}

func (a *App) receivePairModal(c *gui.Context) {
	if !a.pairRecvOpen {
		return
	}
	t := c.Theme()
	isCloudCode := rsync.IsPublicShareCode(a.pairRecvCode)
	isCollabCode := rsync.IsCollabCode(a.pairRecvCode)
	a.styledDialog(c, "recv-pair-modal", &a.pairRecvOpen, 460, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Icon(c, iconLink).Size(17, 17).TextColor(a.pal.ink)
				gui.Text(c, a.t("pref.grpRecv")).FontSize(15).Bold().SingleLine()
				a.helpTip(c, a.t("share.recvUnifiedTip"))
				gui.Spacer(c)
				a.iconButton(c, "recv-pair-x", iconX, a.t("common.close"), false).OnClick(func() {
					a.cancelPair()
					a.pairRecvOpen = false
				})
			})

			lanText := a.t("settings.lanOff")
			lanDot := a.pal.inkFaint
			if node := a.shareNode(); node != nil && node.DiscoveryOK() {
				lanText = a.t("settings.lanOK")
				lanDot = gui.Hex("#10B981")
			}
			gui.Row(c).FillWidth().Padding(6, 10).Gap(8).Radius(8).Background(a.settingsRowHoverBg()).AlignItems(gui.Center).Children(func() {
				gui.Box(c).Size(7, 7).Radius(radiusPill).Background(lanDot)
				gui.Text(c, a.t("pref.lanState")).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
				gui.Text(c, lanText).FontSize(11.5).TextColor(a.pal.inkMuted).SingleLine()
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("share.receiveByCode"))
				gui.TextInput(c.Key("recv-pair-code"), &a.pairRecvCode).
					Placeholder(a.t("share.recvUnifiedPlaceholder")).
					AutoFocus().
					FillWidth().
					OnSubmit(func() { a.startPairReceive() })
				gui.Text(c, a.t("share.recvUnifiedTip")).FontSize(11).TextColor(a.pal.inkMuted).LineHeight(1.4)
			})

			if !isCloudCode && !isCollabCode {
				gui.Column(c).Gap(6).Children(func() {
					gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
						a.sectionLabel(c, a.t("pref.advRelay"))
						a.helpTip(c, a.t("pref.advRelayTip"))
					})
					gui.TextInput(c.Key("recv-signal-url"), &a.signalURL).
						Placeholder("http://127.0.0.1:47595").
						FillWidth()
				})
			}

			if a.pairReceiving && a.cloudShareProgress != "" {
				gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Spinner(c)
					gui.Text(c, a.cloudShareProgress).FontSize(12).TextColor(a.pal.inkSecondary).Grow(1).SingleLine()
				})
			} else if a.status != "" {
				gui.Text(c, a.status).FontSize(12).TextColor(t.TextMuted)
			}

			gui.Divider(c)
			gui.Row(c).Gap(8).Justify(gui.End).Children(func() {
				gui.Button(c.Key("recv-pair-close"), a.t("common.cancel")).OnClick(func() {
					a.cancelPair()
					a.pairRecvOpen = false
				})
				goLabel := a.t("share.connect")
				if isCollabCode {
					goLabel = a.t("collab.joinBtn")
				} else if isCloudCode {
					goLabel = a.t("share.recvDownloadBtn")
				}
				gui.PrimaryButton(c.Key("recv-pair-go"), goLabel).
					Disabled(a.pairReceiving || a.collabBusy).
					OnClick(func() { a.startPairReceive() })
			})
		})
	})
}

// startCloudPublicShare publishes the selected Folder or Project to Raglex Cloud
// and displays a public share code that ANY Raglex user can download without an account.
func (a *App) startCloudPublicShare(kind, targetID string) {
	database := a.shareDB.Load()
	if database == nil && a.lib != nil {
		database = a.lib.DB
	}
	serverURL, token := a.effectiveSyncEndpoint()
	if database == nil || a.cloudShareBusy || token == "" || targetID == "" {
		return
	}
	a.cloudShareBusy = true
	a.cloudShareCode = ""
	a.cloudShareURL = ""
	a.cloudShareProgress = a.t("share.cloudPublishing")
	includeFiles := a.shareFiles
	cfg := rsync.ClientConfig{
		ServerURL: serverURL,
		Token:     token,
		DeviceID:  strings.TrimSpace(a.deviceName),
	}
	win := a.win
	run := func() {
		var lastPush time.Time
		res, err := rsync.PublishPublicShare(context.Background(), cfg, database, kind, targetID, includeFiles, func(p rsync.SyncProgress) {
			now := time.Now()
			if p.Total > 0 && p.Done < p.Total && now.Sub(lastPush) < 80*time.Millisecond {
				return
			}
			lastPush = now
			msg := a.t("share.cloudPublishing")
			if p.Stage == "uploading" && p.Total > 0 {
				msg = fmt.Sprintf("%s (%d / %d) %s", a.t("sync.stage.uploading"), p.Done, p.Total, p.Current)
			}
			apply := func() { a.cloudShareProgress = msg }
			if win == nil {
				apply()
				return
			}
			win.Update(func() {
				if !a.isClosed() {
					apply()
				}
			})
		})
		finish := func() {
			a.cloudShareBusy = false
			a.cloudShareProgress = ""
			if err != nil {
				a.status = err.Error()
				a.notify(err.Error())
				return
			}
			a.cloudShareCode = res.Code
			a.cloudShareURL = res.URL
			a.status = fmt.Sprintf(a.t("share.cloudPublished"), res.Code)
			a.notify(a.status)
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

func (a *App) revokeCloudPublicShare() {
	code := a.cloudShareCode
	serverURL, token := a.effectiveSyncEndpoint()
	if code == "" || token == "" {
		return
	}
	cfg := rsync.ClientConfig{ServerURL: serverURL, Token: token}
	win := a.win
	run := func() {
		err := rsync.RevokePublicShare(context.Background(), cfg, code)
		finish := func() {
			if err != nil {
				a.notify(err.Error())
				return
			}
			a.cloudShareCode = ""
			a.cloudShareURL = ""
			a.notify(a.t("share.cloudRevoked"))
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

// startCloudPublicDownload downloads a public share from Raglex Cloud using ONLY
// its share code — works even for users who have NEVER registered or signed in!
func (a *App) startCloudPublicDownload(rawCode string) {
	database := a.shareDB.Load()
	if database == nil && a.lib != nil {
		database = a.lib.DB
	}
	if database == nil || a.pairReceiving {
		return
	}
	serverURL := config.DefaultCloudEndpoint
	if a.syncProvider == "selfhost" && strings.TrimSpace(a.syncServerURL) != "" {
		serverURL = strings.TrimSpace(a.syncServerURL)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.pairCancel != nil {
		a.pairCancel()
	}
	a.pairCancel = cancel
	a.mu.Unlock()

	a.pairReceiving = true
	a.cloudShareProgress = a.t("share.cloudDownloading")
	a.status = a.cloudShareProgress
	a.syncBusy = true
	a.syncStage = "downloading"
	a.syncDone = 0
	a.syncTotal = 0
	a.syncCurrent = rsync.NormalizePublicShareCode(rawCode)
	win := a.win

	run := func() {
		defer cancel()
		var lastPush time.Time
		res, err := rsync.DownloadPublicShare(ctx, serverURL, rawCode, database, func(p rsync.SyncProgress) {
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
				if prog.Stage == "downloading" && prog.Total > 0 {
					a.cloudShareProgress = fmt.Sprintf("%s (%d / %d) %s", a.t("sync.stage.downloading"), prog.Done, prog.Total, prog.Current)
				} else {
					a.cloudShareProgress = a.t("share.cloudDownloading")
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
		})
		finish := func() {
			a.mu.Lock()
			a.pairCancel = nil
			a.mu.Unlock()
			a.pairReceiving = false
			a.cloudShareProgress = ""
			a.syncBusy = false
			a.syncStage = ""
			a.syncDone = 0
			a.syncTotal = 0
			a.syncCurrent = ""
			if err != nil {
				if errors.Is(err, context.Canceled) {
					a.status = ""
					return
				}
				a.status = err.Error()
				a.notify(err.Error())
				return
			}
			a.pairRecvCode = ""
			a.pairRecvOpen = false
			a.reload()
			if res.Kind == "folder" && res.TargetID != "" {
				a.sidebarBrowseMode = "folders"
				a.handleNav("folder:" + res.TargetID)
			} else if res.TargetID != "" {
				a.openProject(res.TargetID)
			}
			msg := fmt.Sprintf(a.t("share.cloudDownloaded"), res.Title, res.AssetsCount)
			a.status = msg
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

func (a *App) saveSignalURL() string {
	url := trim(a.signalURL)
	if url == "" && trim(a.syncServerURL) != "" {
		url = trim(a.syncServerURL)
		a.signalURL = url
	}
	if url != a.cfg.SignalingURL {
		a.cfg.SignalingURL = url
		if a.win != nil {
			_ = config.Save(a.cfg)
		}
	}
	return url
}

func (a *App) cancelPair() {
	a.mu.Lock()
	cancel := a.pairCancel
	a.pairCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.pairCode = ""
	a.shareSending = false
	a.pairReceiving = false
	a.cloudShareProgress = ""
}

func (a *App) startPairSend(kind, targetID string) {
	database := a.shareDB.Load()
	if database == nil || a.shareSending || a.win == nil || targetID == "" {
		return
	}
	sigURL := a.saveSignalURL()
	if sigURL == "" {
		a.pairAdvRelayOpen = true
		a.status = a.t("share.needSignal")
		return
	}
	name, err := os.Hostname()
	if err != nil || name == "" {
		name = "Raglex"
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.pairCancel != nil {
		a.pairCancel()
	}
	a.pairCancel = cancel
	a.mu.Unlock()

	a.shareSending = true
	a.pairCode = ""
	a.pairTransport = ""
	a.status = a.t("share.sending")
	include := a.shareFiles
	forceRelay := a.pairForceRelay
	win := a.win
	go func() {
		defer cancel()
		var (
			bundle share.Bundle
			files  map[string]string
			err    error
		)
		if kind == "folder" {
			bundle, files, err = share.ExportFolder(database, targetID, include)
		} else {
			bundle, files, err = share.Export(database, targetID, include)
		}
		if err == nil {
			err = share.PairSend(ctx, share.PairOptions{
				SignalingURL: sigURL,
				Name:         name,
				ForceRelay:   forceRelay,
				OnTransport: func(mode string) {
					win.Update(func() {
						if !a.isClosed() {
							a.pairTransport = mode
						}
					})
				},
			}, bundle, files, func(code string) {
				win.Update(func() {
					if a.isClosed() {
						return
					}
					a.pairCode = code
				})
			})
		}
		sendErr := err
		win.Update(func() {
			a.mu.Lock()
			a.pairCancel = nil
			a.mu.Unlock()
			a.shareSending = false
			a.pairCode = ""
			if a.isClosed() {
				return
			}
			if sendErr == nil {
				a.status = a.t("share.sent")
				return
			}
			if errors.Is(sendErr, context.Canceled) {
				a.status = ""
				return
			}
			a.status = a.shareErrText(sendErr)
		})
	}()
}

func (a *App) startPairReceive() {
	// Automatically detect Collaboration invite codes (CLB-XXXX-XXXX).
	if rsync.IsCollabCode(a.pairRecvCode) {
		a.joinCollabByCode(a.pairRecvCode)
		return
	}
	// Automatically detect Raglex Cloud Public Share codes (SHR-XXXX-XXXX or /s/SHR-...)
	// and download directly without requiring any account or login!
	if rsync.IsPublicShareCode(a.pairRecvCode) {
		a.startCloudPublicDownload(a.pairRecvCode)
		return
	}

	database := a.shareDB.Load()
	if database == nil || a.pairReceiving || a.win == nil {
		return
	}
	sigURL := a.saveSignalURL()
	if sigURL == "" {
		a.status = a.t("share.needSignal")
		return
	}
	code := share.NormalizeCode(a.pairRecvCode)
	if len(code) < 4 {
		a.status = a.t("share.badCode")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.pairCancel != nil {
		a.pairCancel()
	}
	a.pairCancel = cancel
	a.mu.Unlock()

	a.pairReceiving = true
	a.pairTransport = ""
	a.status = a.t("share.connecting")
	forceRelay := a.pairForceRelay
	win := a.win
	go func() {
		defer cancel()
		_, recvErr := share.PairReceive(ctx, share.PairOptions{
			SignalingURL: sigURL,
			ForceRelay:   forceRelay,
			OnTransport: func(mode string) {
				win.Update(func() {
					if !a.isClosed() {
						a.pairTransport = mode
					}
				})
			},
		}, code, a.decide, a.shareTake, a.shareCommit)
		win.Update(func() {
			a.mu.Lock()
			a.pairCancel = nil
			a.mu.Unlock()
			a.pairReceiving = false
			if a.isClosed() {
				return
			}
			if recvErr == nil {
				a.pairRecvCode = ""
				a.pairRecvOpen = false
				return
			}
			if errors.Is(recvErr, context.Canceled) {
				a.status = ""
				return
			}
			a.status = a.shareErrText(recvErr)
		})
	}()
}

func (a *App) sendShareTarget(addr, kind, targetID string) {
	node := a.shareNode()
	database := a.shareDB.Load()
	if node == nil || database == nil || a.shareSending || a.win == nil || addr == "" || targetID == "" {
		return
	}
	a.shareSending = true
	a.status = a.t("share.sending")
	include := a.shareFiles
	win := a.win
	go func() {
		var (
			bundle share.Bundle
			files  map[string]string
			err    error
		)
		if kind == "folder" {
			bundle, files, err = share.ExportFolder(database, targetID, include)
		} else {
			bundle, files, err = share.Export(database, targetID, include)
		}
		if err == nil {
			err = node.Send(context.Background(), addr, bundle, files)
		}
		sendErr := err
		win.Update(func() {
			a.shareSending = false
			if a.isClosed() {
				return
			}
			if sendErr == nil {
				a.status = a.t("share.sent")
				return
			}
			a.status = a.shareErrText(sendErr)
		})
	}()
}

func (a *App) decide(ctx context.Context, from string, bundle share.Bundle) bool {
	if a.win == nil || a.shareDB.Load() == nil {
		return false
	}
	a.mu.Lock()
	if a.closed || a.shareAsking {
		a.mu.Unlock()
		return false
	}
	a.shareAsking = true
	win := a.win
	a.mu.Unlock()

	result := make(chan bool, 1)
	win.Update(func() {
		a.mu.Lock()
		asking := a.shareAsking && !a.closed
		a.mu.Unlock()
		if !asking || a.shareDB.Load() == nil || a.askOpen {
			a.clearShareAsking()
			select {
			case result <- false:
			default:
			}
			return
		}
		title := bundle.DisplayName()
		body := fmt.Sprintf(a.t("share.incomingMeta"), from, title)
		if len(bundle.Files) > 0 {
			body = fmt.Sprintf(a.t("share.incomingFiles"), from, title, len(bundle.Files))
		}
		a.askBoth(a.t("share.incoming"), body, a.t("share.accept"), func() {
			a.status = a.t("share.receiving")
			a.answerShare(result, true)
		}, func() {
			a.answerShare(result, false)
		})
	})
	select {
	case ok := <-result:
		return ok
	case <-ctx.Done():
		a.clearShareAsking()
		win.Update(func() {
			a.mu.Lock()
			asking := a.shareAsking
			a.mu.Unlock()
			if !asking {
				a.askOpen = false
				a.askFn = nil
				a.askCancel = nil
			}
		})
		return false
	}
}

func (a *App) answerShare(result chan bool, ok bool) {
	a.clearShareAsking()
	select {
	case result <- ok:
	default:
	}
}

func (a *App) clearShareAsking() {
	a.mu.Lock()
	a.shareAsking = false
	a.mu.Unlock()
}

func (a *App) shareTake(hash, name string, r io.Reader) (string, error) {
	database := a.shareDB.Load()
	if database == nil || a.isClosed() {
		_, _ = io.Copy(io.Discard, r)
		return "", share.ErrClosed
	}
	return share.SaveFile(database.FilesDir(), name, r)
}

func (a *App) shareCommit(bundle share.Bundle, paths map[string]string) (string, error) {
	database := a.shareDB.Load()
	if database == nil || a.isClosed() {
		return "", share.ErrClosed
	}
	id, err := share.Apply(database, bundle, paths)
	if err != nil {
		return "", err
	}
	if a.win != nil {
		win := a.win
		isFolder := bundle.Kind == "folder"
		win.Update(func() {
			if a.isClosed() || a.lib == nil {
				return
			}
			if isFolder {
				a.reload()
				a.sidebarBrowseMode = "folders"
				a.handleNav("folder:" + id)
			} else {
				a.openProject(id)
			}
			a.status = a.t("share.received")
		})
	}
	return id, nil
}

func (a *App) shareErrText(err error) string {
	switch {
	case errors.Is(err, share.ErrDeclined):
		return a.t("share.declined")
	case errors.Is(err, share.ErrInterrupted):
		return a.t("share.interrupted")
	case errors.Is(err, share.ErrNoSignaling):
		return a.t("share.needSignal")
	case errors.Is(err, share.ErrRoomNotFound), errors.Is(err, share.ErrBadCode):
		return a.t("share.badCode")
	case errors.Is(err, share.ErrRoomClaimed):
		return a.t("share.codeClaimed")
	case err == nil:
		return ""
	default:
		return err.Error()
	}
}
