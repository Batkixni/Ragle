package share

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	dcLabel              = "raglex-share"
	dcChunkSize          = 32 << 10  // 32 KiB per SCTP/relay message
	dcLowWater           = 256 << 10 // 256 KiB
	dcHighWater          = 1 << 20   // 1 MiB
	maxMetaBytes         = 32 << 20  // 32 MiB
	defaultSTUN          = "stun:stun.l.google.com:19302"
	defaultDirectTimeout = 8 * time.Second
	dcFlushGrace         = 150 * time.Millisecond
)

// PairOptions configures a WebRTC pairing-code transfer with automatic
// X25519 + AES-256-GCM zero-storage E2E blind relay fallback.
type PairOptions struct {
	SignalingURL string
	Name         string
	// ICEServers overrides the default STUN server when non-nil.
	// Pass an empty slice []webrtc.ICEServer{} for offline / local-only tests.
	ICEServers []webrtc.ICEServer
	// DirectTimeout controls how long to wait for direct WebRTC P2P ICE before
	// automatically falling back to the X25519+AES-256-GCM blind E2E relay.
	DirectTimeout time.Duration
	// ForceRelay skips direct WebRTC ICE and uses the E2E blind relay directly
	// (useful behind strict symmetric corporate firewalls or in tests).
	ForceRelay bool
	// OnTransport optionally reports which encrypted transport won ("p2p" or "e2e-relay").
	OnTransport func(mode string)
}

func (o PairOptions) directTimeout() time.Duration {
	if o.DirectTimeout > 0 {
		return o.DirectTimeout
	}
	return defaultDirectTimeout
}

func (o PairOptions) webrtcConfig() webrtc.Configuration {
	if o.ICEServers != nil {
		return webrtc.Configuration{ICEServers: o.ICEServers}
	}
	return webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{defaultSTUN}},
		},
	}
}

type dcMsg struct {
	Type     string `msgpack:"t"`
	Name     string `msgpack:"n,omitempty"`
	Accept   bool   `msgpack:"a,omitempty"`
	Hash     string `msgpack:"h,omitempty"`
	FileName string `msgpack:"fn,omitempty"`
	Size     int64  `msgpack:"sz,omitempty"`
	Offset   int64  `msgpack:"off,omitempty"`
	Data     []byte `msgpack:"d,omitempty"`
	Error    string `msgpack:"err,omitempty"`
}

const (
	dcHello     = "hello"
	dcMetaChunk = "meta_chunk"
	dcMetaDone  = "meta_done"
	dcAccept    = "accept"
	dcFileChunk = "file_chunk"
	dcFileDone  = "file_done"
	dcEnd       = "end"
	dcAck       = "ack"
	dcError     = "error"
)

type dcPipe struct {
	dc      *webrtc.DataChannel
	openCh  chan struct{}
	closeCh chan struct{}
	lowCh   chan struct{}
	recvCh  chan dcMsg
	errCh   chan error
	once    sync.Once
}

func newDCPipe(dc *webrtc.DataChannel) *dcPipe {
	p := &dcPipe{
		dc:      dc,
		openCh:  make(chan struct{}),
		closeCh: make(chan struct{}),
		lowCh:   make(chan struct{}, 1),
		recvCh:  make(chan dcMsg, 64),
		errCh:   make(chan error, 1),
	}
	dc.SetBufferedAmountLowThreshold(dcLowWater)
	dc.OnBufferedAmountLow(func() {
		select {
		case p.lowCh <- struct{}{}:
		default:
		}
	})
	dc.OnOpen(func() {
		select {
		case <-p.openCh:
		default:
			close(p.openCh)
		}
	})
	dc.OnClose(func() {
		p.once.Do(func() { close(p.closeCh) })
	})
	dc.OnError(func(err error) {
		select {
		case p.errCh <- err:
		default:
		}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var m dcMsg
		if err := msgpack.Unmarshal(msg.Data, &m); err != nil {
			select {
			case p.errCh <- err:
			default:
			}
			return
		}
		select {
		case p.recvCh <- m:
		case <-p.closeCh:
		}
	})
	return p
}

func (p *dcPipe) waitOpen(ctx context.Context, failCh <-chan struct{}) error {
	if p.dc.ReadyState() == webrtc.DataChannelStateOpen {
		return nil
	}
	select {
	case <-p.openCh:
		return nil
	case err := <-p.errCh:
		return err
	case <-p.closeCh:
		return ErrInterrupted
	case <-failCh:
		return ErrInterrupted
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *dcPipe) send(ctx context.Context, m dcMsg) error {
	raw, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	for p.dc.BufferedAmount() > dcHighWater {
		select {
		case <-p.lowCh:
		case err := <-p.errCh:
			return err
		case <-p.closeCh:
			return ErrInterrupted
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := p.dc.Send(raw); err != nil {
		return ErrInterrupted
	}
	return nil
}

func (p *dcPipe) recv(ctx context.Context) (dcMsg, error) {
	select {
	case m := <-p.recvCh:
		return m, nil
	case err := <-p.errCh:
		return dcMsg{}, err
	case <-p.closeCh:
		select {
		case m := <-p.recvCh:
			return m, nil
		default:
			return dcMsg{}, ErrInterrupted
		}
	case <-ctx.Done():
		return dcMsg{}, ctx.Err()
	}
}

func watchPCFailure(pc *webrtc.PeerConnection) <-chan struct{} {
	failCh := make(chan struct{})
	var once sync.Once
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			once.Do(func() { close(failCh) })
		}
	})
	return failCh
}

// PairSend registers a short pairing code with the signaling server, attempts a
// direct DTLS-encrypted WebRTC DataChannel P2P connection first, and automatically
// falls back to an X25519 + AES-256-GCM zero-storage E2E blind relay if strict
// NAT/CGNAT blocks direct hole-punching.
func PairSend(ctx context.Context, opt PairOptions, bundle Bundle, files map[string]string, onCode func(code string)) error {
	client := NewSignalingClient(opt.SignalingURL)
	if client.baseURL == "" {
		return ErrNoSignaling
	}
	senderName := opt.Name
	if senderName == "" {
		senderName = "Raglex"
	}
	kp, err := newE2EKeyPair()
	if err != nil {
		return err
	}

	var pc *webrtc.PeerConnection
	var dcP *dcPipe
	var failCh <-chan struct{}
	offerSDP := "e2e-relay-only"

	if !opt.ForceRelay {
		pc, err = webrtc.NewPeerConnection(opt.webrtcConfig())
		if err != nil {
			return err
		}
		defer pc.Close()
		failCh = watchPCFailure(pc)

		ordered := true
		dc, err := pc.CreateDataChannel(dcLabel, &webrtc.DataChannelInit{Ordered: &ordered})
		if err != nil {
			return err
		}
		dcP = newDCPipe(dc)

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			return err
		}
		gatherDone := webrtc.GatheringCompletePromise(pc)
		if err := pc.SetLocalDescription(offer); err != nil {
			return err
		}
		select {
		case <-gatherDone:
		case <-ctx.Done():
			return ctx.Err()
		}
		localDesc := pc.LocalDescription()
		if localDesc == nil {
			return errors.New("raglex: missing local description")
		}
		offerSDP = localDesc.SDP
	}

	code, token, err := client.CreateRoomWithKey(ctx, offerSDP, senderName, kp.pub)
	if err != nil {
		return err
	}
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.DeleteRoom(cleanCtx, code, token)
	}()
	if onCode != nil {
		onCode(code)
	}

	answerSDP, peerPub, err := client.WaitAnswerWithKey(ctx, code, token)
	if err != nil {
		return err
	}

	var pipe msgPipe
	usedRelay := opt.ForceRelay || answerSDP == "e2e-relay-only" || pc == nil
	if !usedRelay {
		if err := pc.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  answerSDP,
		}); err != nil {
			usedRelay = true
		} else {
			directCtx, cancel := context.WithTimeout(ctx, opt.directTimeout())
			waitErr := dcP.waitOpen(directCtx, failCh)
			cancel()
			if waitErr == nil {
				pipe = dcP
			} else if ctx.Err() != nil {
				return ctx.Err()
			} else {
				usedRelay = true
			}
		}
	}

	if usedRelay {
		if peerPub == "" {
			return ErrInterrupted
		}
		ciph, err := newE2ECipher(kp.priv, peerPub, code, true)
		if err != nil {
			return err
		}
		pipe = newRelayPipe(client, code, "sender", token, ciph)
		if opt.OnTransport != nil {
			opt.OnTransport("e2e-relay")
		}
	} else if opt.OnTransport != nil {
		opt.OnTransport("p2p")
	}

	// 1. Hello + metadata frames (chunked to respect SCTP/relay message limits).
	if err := pipe.send(ctx, dcMsg{Type: dcHello, Name: senderName}); err != nil {
		return err
	}
	metaRaw, err := msgpack.Marshal(bundle)
	if err != nil {
		return err
	}
	if len(metaRaw) > maxMetaBytes {
		return errors.New("raglex: bundle metadata too large")
	}
	for off := 0; off < len(metaRaw); off += dcChunkSize {
		end := off + dcChunkSize
		if end > len(metaRaw) {
			end = len(metaRaw)
		}
		if err := pipe.send(ctx, dcMsg{
			Type:   dcMetaChunk,
			Size:   int64(len(metaRaw)),
			Offset: int64(off),
			Data:   metaRaw[off:end],
		}); err != nil {
			return err
		}
	}
	if err := pipe.send(ctx, dcMsg{Type: dcMetaDone, Size: int64(len(metaRaw))}); err != nil {
		return err
	}

	// 2. Wait for receiver accept/decline.
	resp, err := pipe.recv(ctx)
	if err != nil {
		return err
	}
	if resp.Type == dcError {
		return errors.New(resp.Error)
	}
	if resp.Type != dcAccept {
		return errors.New("raglex: expected accept frame")
	}
	if !resp.Accept {
		return ErrDeclined
	}

	// 3. Stream each file as chunk frames with hash and byte offset.
	for _, file := range bundle.Files {
		path := files[file.Hash]
		if path == "" {
			continue
		}
		if err := sendPairFile(ctx, pipe, file, path); err != nil {
			return err
		}
	}

	if err := pipe.send(ctx, dcMsg{Type: dcEnd}); err != nil {
		return err
	}

	// 4. Wait for receiver commit acknowledgement.
	ack, err := pipe.recv(ctx)
	if err != nil {
		return err
	}
	if ack.Type == dcError {
		return errors.New(ack.Error)
	}
	if ack.Type != dcAck {
		return errors.New("raglex: expected ack frame")
	}
	return nil
}

func sendPairFile(ctx context.Context, pipe msgPipe, file FileSnap, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	size := fi.Size()
	buf := make([]byte, dcChunkSize)
	var offset int64
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := pipe.send(ctx, dcMsg{
				Type:     dcFileChunk,
				Hash:     file.Hash,
				FileName: file.Name,
				Size:     size,
				Offset:   offset,
				Data:     chunk,
			}); err != nil {
				return err
			}
			offset += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if offset != size {
		return ErrInterrupted
	}
	return pipe.send(ctx, dcMsg{
		Type:     dcFileDone,
		Hash:     file.Hash,
		FileName: file.Name,
		Size:     size,
	})
}

// PairReceive connects to a sender using a short pairing code, tries direct
// DTLS-encrypted WebRTC P2P first, automatically falls back to X25519 + AES-256-GCM
// zero-storage E2E blind relay when direct NAT traversal is blocked, asks decide
// before accepting files, verifies file hashes, and commits the project bundle.
func PairReceive(
	ctx context.Context,
	opt PairOptions,
	code string,
	decide func(ctx context.Context, from string, bundle Bundle) bool,
	take func(hash, name string, r io.Reader) (string, error),
	commit func(bundle Bundle, paths map[string]string) (string, error),
) (string, error) {
	client := NewSignalingClient(opt.SignalingURL)
	if client.baseURL == "" {
		return "", ErrNoSignaling
	}
	offerSDP, senderName, peerPub, err := client.GetOfferWithKey(ctx, code)
	if err != nil {
		return "", err
	}
	kp, err := newE2EKeyPair()
	if err != nil {
		return "", err
	}

	var pc *webrtc.PeerConnection
	var failCh <-chan struct{}
	var pipeCh chan *dcPipe
	answerSDP := "e2e-relay-only"

	useRelay := opt.ForceRelay || offerSDP == "e2e-relay-only"
	if !useRelay {
		pc, err = webrtc.NewPeerConnection(opt.webrtcConfig())
		if err != nil {
			return "", err
		}
		defer pc.Close()
		failCh = watchPCFailure(pc)

		pipeCh = make(chan *dcPipe, 1)
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			if dc.Label() != dcLabel {
				return
			}
			select {
			case pipeCh <- newDCPipe(dc):
			default:
			}
		})

		if err := pc.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  offerSDP,
		}); err != nil {
			useRelay = true
		} else {
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				return "", err
			}
			gatherDone := webrtc.GatheringCompletePromise(pc)
			if err := pc.SetLocalDescription(answer); err != nil {
				return "", err
			}
			select {
			case <-gatherDone:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			localDesc := pc.LocalDescription()
			if localDesc == nil {
				return "", errors.New("raglex: missing local answer")
			}
			answerSDP = localDesc.SDP
		}
	}

	recvToken, err := client.SubmitAnswerWithKey(ctx, code, answerSDP, kp.pub)
	if err != nil {
		return "", err
	}

	var pipe msgPipe
	if !useRelay {
		directCtx, cancel := context.WithTimeout(ctx, opt.directTimeout())
		var dcP *dcPipe
		select {
		case dcP = <-pipeCh:
		case <-failCh:
			useRelay = true
		case <-directCtx.Done():
			if ctx.Err() != nil {
				cancel()
				return "", ctx.Err()
			}
			useRelay = true
		}
		if !useRelay && dcP != nil {
			if err := dcP.waitOpen(directCtx, failCh); err != nil {
				if ctx.Err() != nil {
					cancel()
					return "", ctx.Err()
				}
				useRelay = true
			} else {
				pipe = dcP
			}
		}
		cancel()
	}

	if useRelay {
		if peerPub == "" {
			return "", ErrInterrupted
		}
		ciph, err := newE2ECipher(kp.priv, peerPub, code, false)
		if err != nil {
			return "", err
		}
		pipe = newRelayPipe(client, code, "receiver", recvToken, ciph)
		if opt.OnTransport != nil {
			opt.OnTransport("e2e-relay")
		}
	} else if opt.OnTransport != nil {
		opt.OnTransport("p2p")
	}

	// 1. Receive hello and chunked metadata.
	helloMsg, err := pipe.recv(ctx)
	if err != nil {
		return "", err
	}
	if helloMsg.Type != dcHello {
		return "", errors.New("raglex: expected hello frame")
	}
	from := helloMsg.Name
	if from == "" {
		from = senderName
	}
	if from == "" {
		from = "Raglex"
	}

	var metaBuf bytes.Buffer
	var expectedMeta int64 = -1
	for {
		m, err := pipe.recv(ctx)
		if err != nil {
			return "", err
		}
		switch m.Type {
		case dcMetaChunk:
			if expectedMeta < 0 {
				if m.Size < 0 || m.Size > maxMetaBytes {
					return "", errors.New("raglex: bad metadata size")
				}
				expectedMeta = m.Size
			}
			if m.Offset != int64(metaBuf.Len()) {
				return "", errors.New("raglex: metadata offset mismatch")
			}
			if int64(metaBuf.Len())+int64(len(m.Data)) > maxMetaBytes {
				return "", errors.New("raglex: metadata too large")
			}
			metaBuf.Write(m.Data)
		case dcMetaDone:
			if expectedMeta >= 0 && int64(metaBuf.Len()) != expectedMeta {
				return "", ErrInterrupted
			}
			if m.Size != int64(metaBuf.Len()) {
				return "", ErrInterrupted
			}
			goto metaReady
		default:
			return "", fmt.Errorf("raglex: unexpected frame %q during metadata", m.Type)
		}
	}

metaReady:
	var bundle Bundle
	if err := msgpack.Unmarshal(metaBuf.Bytes(), &bundle); err != nil {
		return "", err
	}

	yes := false
	if decide != nil {
		yes = decide(ctx, from, bundle)
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err := pipe.send(ctx, dcMsg{Type: dcAccept, Accept: yes}); err != nil {
		return "", err
	}
	if !yes {
		time.Sleep(dcFlushGrace)
		return "", ErrDeclined
	}

	// 2. Receive file chunks with hash + offset and verify before committing.
	paths := map[string]string{}
	cleanup := func() {
		for _, p := range paths {
			_ = os.Remove(p)
		}
	}

	type activeFile struct {
		hash    string
		name    string
		size    int64
		written int64
		tmp     *os.File
		tmpPath string
		sum     io.Writer
		hasher  func() string
	}
	var cur *activeFile
	closeCur := func() {
		if cur != nil {
			_ = cur.tmp.Close()
			_ = os.Remove(cur.tmpPath)
			cur = nil
		}
	}
	defer closeCur()

	openActive := func(hash, name string, size int64) error {
		if size < 0 {
			return errors.New("raglex: bad file size")
		}
		tmp, err := os.CreateTemp("", "raglex-pair-*")
		if err != nil {
			return err
		}
		h := sha256.New()
		cur = &activeFile{
			hash:    hash,
			name:    name,
			size:    size,
			tmp:     tmp,
			tmpPath: tmp.Name(),
			sum:     h,
			hasher:  func() string { return hex.EncodeToString(h.Sum(nil)) },
		}
		return nil
	}

	for {
		m, err := pipe.recv(ctx)
		if err != nil {
			cleanup()
			return "", err
		}
		switch m.Type {
		case dcFileChunk:
			if cur == nil {
				if err := openActive(m.Hash, m.FileName, m.Size); err != nil {
					cleanup()
					return "", err
				}
			}
			if cur.hash != m.Hash {
				cleanup()
				return "", errors.New("raglex: interleaved file hash")
			}
			if m.Offset != cur.written {
				cleanup()
				return "", errors.New("raglex: file offset mismatch")
			}
			if len(m.Data) > 0 {
				if _, err := cur.tmp.Write(m.Data); err != nil {
					cleanup()
					return "", err
				}
				_, _ = cur.sum.Write(m.Data)
				cur.written += int64(len(m.Data))
			}
		case dcFileDone:
			if cur == nil {
				if err := openActive(m.Hash, m.FileName, m.Size); err != nil {
					cleanup()
					return "", err
				}
			}
			if cur.hash != m.Hash || cur.written != m.Size || cur.hasher() != cur.hash {
				cleanup()
				_ = pipe.send(ctx, dcMsg{Type: dcError, Error: "raglex: file hash mismatch"})
				time.Sleep(dcFlushGrace)
				return "", errors.New("raglex: file hash mismatch")
			}
			if take == nil {
				cleanup()
				return "", errors.New("raglex: nowhere to store the file")
			}
			if _, err := cur.tmp.Seek(0, io.SeekStart); err != nil {
				cleanup()
				return "", err
			}
			savedPath, err := take(cur.hash, cur.name, cur.tmp)
			closeCur()
			if err != nil {
				if savedPath != "" {
					_ = os.Remove(savedPath)
				}
				cleanup()
				_ = pipe.send(ctx, dcMsg{Type: dcError, Error: err.Error()})
				time.Sleep(dcFlushGrace)
				return "", err
			}
			if savedPath != "" {
				paths[m.Hash] = savedPath
			}
		case dcEnd:
			if cur != nil {
				cleanup()
				return "", ErrInterrupted
			}
			if commit == nil {
				cleanup()
				return "", errors.New("raglex: nowhere to store the project")
			}
			id, err := commit(bundle, paths)
			if err != nil {
				cleanup()
				_ = pipe.send(ctx, dcMsg{Type: dcError, Error: err.Error()})
				time.Sleep(dcFlushGrace)
				return "", err
			}
			_ = pipe.send(ctx, dcMsg{Type: dcAck})
			time.Sleep(dcFlushGrace)
			return id, nil
		default:
			cleanup()
			return "", fmt.Errorf("raglex: unexpected frame %q", m.Type)
		}
	}
}
