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
		gui.Column(c).Padding(22, 24).Gap(16).Children(func() {
			gui.Row(c).Gap(10).AlignItems(gui.Center).Children(func() {
				gui.Icon(c, iconShare).Size(18, 18).TextColor(a.pal.ink)
				gui.Text(c, a.t("share.send")+" · "+a.project.Name).FontSize(16).Bold().Grow(1).SingleLine()
				a.iconButton(c, "share-close", iconX, a.t("common.close"), false).OnClick(func() {
					a.shareOpen = false
				})
			})

			gui.Checkbox(c.Key("share-files"), &a.shareFiles, a.t("share.files"))

			// LAN section.
			gui.Column(c).Padding(16).Gap(10).Radius(radiusPanel).Background(a.pal.surface).Children(func() {
				a.sectionLabel(c, a.t("share.title"))
				node := a.shareNode()
				if node == nil || !node.DiscoveryOK() {
					gui.Text(c, a.t("share.unavailable")).FontSize(12).TextColor(t.TextMuted)
				} else {
					peers := node.Peers()
					if len(peers) == 0 {
						gui.Text(c, a.t("share.none")).FontSize(12).TextColor(t.TextMuted)
					}
					for _, peer := range peers {
						item := peer
						label := item.Name
						if label == "" {
							label = item.Addr
						}
						gui.Row(c.Key("share-row-" + item.ID)).Gap(8).AlignItems(gui.Center).Children(func() {
							gui.Text(c, label).Bold().FontSize(13).Grow(1).SingleLine()
							gui.Text(c, item.Addr).FontSize(11).TextColor(t.TextMuted)
							gui.PrimaryButton(c, a.t("share.sendTo")).Disabled(a.shareSending).OnClick(func() {
								a.sendProject(item.Addr)
							})
						})
					}
				}
			})

			// Pairing code E2E section.
			gui.Column(c).Padding(16).Gap(10).Radius(radiusPanel).Background(a.pal.surface).Children(func() {
				a.sectionLabel(c, a.t("share.pairTitle"))
				gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.TextInput(c.Key("signal-url"), &a.signalURL).
						Placeholder(a.t("share.signalURL")).
						Grow(1)
					if a.pairCode == "" {
						gui.PrimaryButton(c.Key("pair-create"), a.t("share.createCode")).
							Disabled(a.shareSending).
							OnClick(func() { a.startPairSend() })
					} else {
						gui.Button(c.Key("pair-cancel"), a.t("common.cancel")).OnClick(func() {
							a.cancelPair()
						})
					}
				})
				if a.pairCode != "" {
					code := a.pairCode
					gui.Row(c).
						Padding(12, 14).
						Gap(10).
						Radius(radiusControl).
						AlignItems(gui.Center).
						Background(a.pal.surfaceMuted).
						Children(func() {
							gui.Text(c, fmt.Sprintf(a.t("share.codeWaiting"), code)).
								FontSize(14).
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

			if a.status != "" {
				gui.Text(c, a.status).FontSize(12).TextColor(t.TextMuted)
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
				gui.Text(c, a.t("share.pairTitle")).FontSize(15).Bold().Grow(1)
				a.iconButton(c, "recv-pair-x", iconX, a.t("common.close"), false).OnClick(func() {
					a.cancelPair()
					a.pairRecvOpen = false
				})
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("share.signalURL"))
				gui.TextInput(c.Key("recv-signal-url"), &a.signalURL).
					Placeholder("http://127.0.0.1:8787").
					FillWidth()
			})

			gui.Column(c).Gap(6).Children(func() {
				a.sectionLabel(c, a.t("share.receiveByCode"))
				gui.TextInput(c.Key("recv-pair-code"), &a.pairRecvCode).
					Placeholder(a.t("share.enterCode")).
					AutoFocus().
					FillWidth().
					OnSubmit(func() { a.startPairReceive() })
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
	a.status = a.t("share.sending")
	include := a.shareFiles
	projectID := a.projectID
	win := a.win
	go func() {
		defer cancel()
		bundle, files, err := share.Export(database, projectID, include)
		if err == nil {
			err = share.PairSend(ctx, share.PairOptions{
				SignalingURL: sigURL,
				Name:         name,
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
	a.status = a.t("share.connecting")
	win := a.win
	go func() {
		defer cancel()
		_, recvErr := share.PairReceive(ctx, share.PairOptions{
			SignalingURL: sigURL,
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
