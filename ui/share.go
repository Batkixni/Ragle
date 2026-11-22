package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/config"
	"ragle/internal/share"
)

func (a *App) startShare() {
	name, err := os.Hostname()
	if err != nil || name == "" {
		name = "Ragle"
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

func (a *App) sharePanel(c *gui.Context) {
	t := c.Theme()
	services := c.Services()
	gui.Column(c).Gap(8).Padding(12).Border(1, t.Border).Radius(8).Children(func() {
		gui.Text(c, a.t("share.title")).Bold()
		gui.Checkbox(c.Key("share-files"), &a.shareFiles, a.t("share.files"))
		node := a.shareNode()
		if node == nil || !node.DiscoveryOK() {
			gui.Text(c, a.t("share.unavailable")).TextColor(t.TextMuted)
		} else {
			peers := node.Peers()
			if len(peers) == 0 {
				gui.Text(c, a.t("share.none")).TextColor(t.TextMuted)
			}
			for _, peer := range peers {
				item := peer
				label := item.Name
				if label == "" {
					label = item.Addr
				}
				gui.Row(c.Key("share-row-" + item.ID)).Gap(8).AlignItems(gui.Center).Children(func() {
					gui.Text(c, label).Width(220)
					gui.Text(c, item.Addr).TextColor(t.TextMuted)
					gui.Button(c, a.t("share.sendTo")).Disabled(a.shareSending).OnClick(func() {
						a.sendProject(item.Addr)
					})
				})
			}
		}

		gui.Divider(c)
		gui.Text(c, a.t("share.pairTitle")).Bold()
		gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
			gui.TextInput(c.Key("signal-url"), &a.signalURL).Placeholder(a.t("share.signalURL")).Width(280)
			if a.pairCode == "" {
				gui.Button(c.Key("pair-create"), a.t("share.createCode")).Disabled(a.shareSending).OnClick(func() {
					a.startPairSend()
				})
			} else {
				gui.Button(c.Key("pair-cancel"), a.t("common.cancel")).OnClick(func() {
					a.cancelPair()
				})
			}
		})
		if a.pairCode != "" {
			code := a.pairCode
			gui.Row(c).Gap(8).AlignItems(gui.Center).Children(func() {
				gui.Text(c, fmt.Sprintf(a.t("share.codeWaiting"), code)).Bold().TextColor(t.Accent)
				gui.Button(c.Key("pair-copy"), a.t("share.copyCode")).OnClick(func() {
					services.WriteClipboard(code)
					a.status = a.t("share.codeCopied")
				})
			})
		}
	})
}

func (a *App) receivePairBar(c *gui.Context) {
	t := c.Theme()
	gui.Row(c).Padding(8, 16).Gap(8).AlignItems(gui.Center).Background(t.Surface).Border(1, t.Border).Children(func() {
		gui.Text(c, a.t("share.pairTitle")).Bold().FontSize(12)
		gui.TextInput(c.Key("recv-signal-url"), &a.signalURL).Placeholder(a.t("share.signalURL")).Width(240)
		gui.TextInput(c.Key("recv-pair-code"), &a.pairRecvCode).Placeholder(a.t("share.enterCode")).Width(180)
		gui.Button(c.Key("recv-pair-go"), a.t("share.connect")).Disabled(a.pairReceiving).OnClick(func() {
			a.startPairReceive()
		})
		gui.Button(c.Key("recv-pair-close"), a.t("common.cancel")).OnClick(func() {
			a.cancelPair()
			a.pairRecvOpen = false
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
		name = "Ragle"
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
