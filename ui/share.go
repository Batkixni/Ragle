package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	gui "github.com/egoist/mygo/ui"

	"raglex/internal/config"
	"raglex/internal/share"
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

func (a *App) shareModal(c *gui.Context) {
	if !a.shareOpen || a.projectID == "" {
		return
	}
	t := c.Theme()
	services := c.Services()
	a.styledDialog(c, "share-modal", &a.shareOpen, 520, 0, func(_ gui.Element) {
		gui.Column(c).Padding(18, 18).Gap(4).Children(func() {
			gui.Row(c).Padding(0, 6, 8, 6).Gap(10).AlignItems(gui.Center).Children(func() {
				gui.Icon(c, iconShare).Size(17, 17).TextColor(a.pal.ink)
				gui.Text(c, a.t("share.send")+" · "+a.project.Name).FontSize(15).Bold().Grow(1).SingleLine()
				a.iconButton(c, "share-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.shareOpen = false
				})
			})

			gui.Box(c).FillWidth().Height(1).Background(a.chromeHairline())
			gui.Row(c).FillWidth().Padding(8, 6).AlignItems(gui.Center).Children(func() {
				gui.Checkbox(c.Key("share-files"), &a.shareFiles, a.t("share.files"))
			})

			// 1. Same Wi-Fi transfer (auto discovery, zero setup).
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
									a.sendProject(item.Addr)
								})
							})
						})
					}
				}
			})

			// 2. Cross-network pairing code transfer (Direct DTLS P2P + E2E encrypted blind relay fallback).
			a.settingsSection(c, a.t("share.codeCard"), a.t("share.codeCardTip"), true, func() {
				gui.Row(c).FillWidth().Padding(6, 10).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconLink).Size(15, 15).TextColor(a.pal.inkSecondary)
					if a.pairCode == "" {
						gui.PrimaryButton(c.Key("pair-create"), a.t("share.createCode")).
							Disabled(a.shareSending).
							OnClick(func() { a.startPairSend() })
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
							Placeholder("http://127.0.0.1:41595").
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

			// 3. Public Web Share Link (powered by built-in Raglex server /s/<id>).
			a.settingsSection(c, a.t("share.webCard"), a.t("share.webCardTip"), true, func() {
				gui.Row(c).FillWidth().Padding(6, 10).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Icon(c, iconExternal).Size(15, 15).TextColor(a.pal.inkSecondary)
					gui.TextInput(c.Key("pub-share-pass"), &a.pubSharePass).
						Placeholder(a.t("share.publicPass")).
						Grow(1)
					a.arcButton(c, "pub-share-create", a.t("share.publicCreate"), false).OnClick(func() {
						if u, err := a.createPublicShareLink("project", a.projectID, a.project.Name, a.pubSharePass, a.shareFiles); err == nil {
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
	a.styledDialog(c, "recv-pair-modal", &a.pairRecvOpen, 440, 0, func(_ gui.Element) {
		gui.Column(c).Padding(22, 24).Gap(14).Children(func() {
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Icon(c, iconLink).Size(17, 17).TextColor(a.pal.ink)
				gui.Text(c, a.t("pref.grpRecv")).FontSize(15).Bold().SingleLine()
				a.helpTip(c, a.t("pref.recvTip"))
				gui.Spacer(c)
				a.iconButton(c, "recv-pair-x", iconX, a.t("common.close"), false).OnClick(func() {
					a.cancelPair()
					a.pairRecvOpen = false
				})
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("share.receiveByCode"))
				gui.TextInput(c.Key("recv-pair-code"), &a.pairRecvCode).
					Placeholder(a.t("share.enterCode")).
					AutoFocus().
					FillWidth().
					OnSubmit(func() { a.startPairReceive() })
			})

			gui.Column(c).Gap(6).Children(func() {
				gui.Row(c).Gap(6).AlignItems(gui.Center).Children(func() {
					a.sectionLabel(c, a.t("pref.advRelay"))
					a.helpTip(c, a.t("pref.advRelayTip"))
				})
				gui.TextInput(c.Key("recv-signal-url"), &a.signalURL).
					Placeholder("http://127.0.0.1:41595").
					FillWidth()
			})

			if a.status != "" {
				gui.Text(c, a.status).FontSize(12).TextColor(t.TextMuted)
			}

			gui.Divider(c)
			gui.Row(c).Gap(8).Justify(gui.End).Children(func() {
				gui.Button(c.Key("recv-pair-close"), a.t("common.cancel")).OnClick(func() {
					a.cancelPair()
					a.pairRecvOpen = false
				})
				gui.PrimaryButton(c.Key("recv-pair-go"), a.t("share.connect")).
					Disabled(a.pairReceiving).
					OnClick(func() { a.startPairReceive() })
			})
		})
	})
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
}

func (a *App) startPairSend() {
	database := a.shareDB.Load()
	if database == nil || a.shareSending || a.win == nil || a.projectID == "" {
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
	projectID := a.projectID
	win := a.win
	go func() {
		defer cancel()
		bundle, files, err := share.Export(database, projectID, include)
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

func (a *App) sendProject(addr string) {
	node := a.shareNode()
	database := a.shareDB.Load()
	if node == nil || database == nil || a.shareSending || a.win == nil || addr == "" {
		return
	}
	a.shareSending = true
	a.status = a.t("share.sending")
	include := a.shareFiles
	projectID := a.projectID
	win := a.win
	go func() {
		bundle, files, err := share.Export(database, projectID, include)
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
		body := fmt.Sprintf(a.t("share.incomingMeta"), from, bundle.Project.Name)
		if len(bundle.Files) > 0 {
			body = fmt.Sprintf(a.t("share.incomingFiles"), from, bundle.Project.Name, len(bundle.Files))
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
		win.Update(func() {
			if a.isClosed() || a.lib == nil {
				return
			}
			a.openProject(id)
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
