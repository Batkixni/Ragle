package share

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/vmihailenco/msgpack/v5"

	"ragle/internal/model"
)

const (
	beaconEvery = 2 * time.Second
	peerTTL     = 6 * time.Second
)

// Peer is another Ragle on the local network.
type Peer struct {
	ID   string
	Name string
	Addr string
}

type storedPeer struct {
	Peer
	Seen time.Time
}

type beacon struct {
	Magic string `msgpack:"magic"`
	ID    string `msgpack:"id"`
	Name  string `msgpack:"name"`
	Port  int    `msgpack:"port"`
}

// Options configures a share node. Ephemeral binds a free UDP port so tests
// do not need the shared discovery port. Quiet skips the broadcast.
type Options struct {
	Name      string
	Ephemeral bool
	Quiet     bool
	OnPeers   func()
	OnError   func(error)
	Decide    func(ctx context.Context, from string, bundle Bundle) bool
	Take      func(hash, name string, r io.Reader) (string, error)
	Commit    func(bundle Bundle, paths map[string]string) (string, error)
}

// Node announces this process and accepts project copies.
type Node struct {
	id      string
	name    string
	quiet   bool
	ln      net.Listener
	udp     *net.UDPConn
	tcpPort int
	udpPort int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once

	mu    sync.Mutex
	peers map[string]storedPeer

	onPeers func()
	onError func(error)
	decide  func(ctx context.Context, from string, bundle Bundle) bool
	take    func(hash, name string, r io.Reader) (string, error)
	commit  func(bundle Bundle, paths map[string]string) (string, error)
}

// Start listens for peers and for incoming projects. UDP discovery can fail
// while TCP still works; DiscoveryOK reports that.
func Start(opt Options) (*Node, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, errors.New("ragle: tcp address")
	}
	name := opt.Name
	if name == "" {
		name = "Ragle"
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &Node{
		id:      model.NewID(),
		name:    name,
		quiet:   opt.Quiet,
		ln:      ln,
		tcpPort: tcp.Port,
		ctx:     ctx,
		cancel:  cancel,
		peers:   map[string]storedPeer{},
		onPeers: opt.OnPeers,
		onError: opt.OnError,
		decide:  opt.Decide,
		take:    opt.Take,
		commit:  opt.Commit,
	}
	port := DiscoveryPort
	if opt.Ephemeral {
		port = 0
	}
	udp, udpErr := listenUDP(port)
	if udpErr == nil {
		addr, _ := udp.LocalAddr().(*net.UDPAddr)
		if addr != nil {
			n.udpPort = addr.Port
		}
		n.udp = udp
		n.wg.Add(2)
		go n.readUDP()
		go n.beaconLoop()
	}
	n.wg.Add(1)
	go n.loopAccept()
	return n, nil
}

// DiscoveryOK reports whether presence broadcasts can be sent and received.
func (n *Node) DiscoveryOK() bool {
	return n != nil && n.udp != nil
}

// ID is this node's presence id.
func (n *Node) ID() string { return n.id }

// Addr is the local TCP address other machines dial.
func (n *Node) Addr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(n.tcpPort))
}

// UDPPort is the discovery port this node bound.
func (n *Node) UDPPort() int { return n.udpPort }

// Peers lists instances heard from within the last few seconds.
func (n *Node) Peers() []Peer {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.prune(time.Now())
	out := make([]Peer, 0, len(n.peers))
	for _, p := range n.peers {
		out = append(out, p.Peer)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Close stops discovery and waits for transfers that were already running.
// A transfer waiting for the user unblocks because the node context is canceled.
func (n *Node) Close() error {
	if n == nil {
		return nil
	}
	n.once.Do(func() {
		n.cancel()
		_ = n.ln.Close()
		if n.udp != nil {
			_ = n.udp.Close()
		}
		n.wg.Wait()
	})
	return nil
}

// Send dials addr, offers bundle, and streams files only after the receiver accepts.
// files maps a hash to a local path. Closing the node cancels the send.
func (n *Node) Send(ctx context.Context, addr string, b Bundle, files map[string]string) error {
	if n.ctx.Err() != nil {
		return ErrClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(n.ctx, cancel)
	defer stop()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		if n.ctx.Err() != nil {
			return ErrClosed
		}
		return err
	}
	defer conn.Close()
	stopConn := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopConn()

	if err := writeMagic(conn); err != nil {
		return sendErr(err)
	}
	raw, err := msgpack.Marshal(hello{Name: n.name, ID: n.id})
	if err != nil {
		return err
	}
	if err := writeFrame(conn, frameHello, raw); err != nil {
		return sendErr(err)
	}
	raw, err = msgpack.Marshal(b)
	if err != nil {
		return err
	}
	if err := writeFrame(conn, frameMeta, raw); err != nil {
		return sendErr(err)
	}
	kind, payload, err := readFrame(conn)
	if err != nil {
		return sendErr(err)
	}
	if kind != frameAccept {
		return errors.New("ragle: expected accept")
	}
	yes, err := accepted(payload)
	if err != nil {
		return err
	}
	if !yes {
		return ErrDeclined
	}
	for _, file := range b.Files {
		path := files[file.Hash]
		if path == "" {
			return fmt.Errorf("ragle: missing file %s", file.Name)
		}
		if err := sendFile(conn, file, path); err != nil {
			return sendErr(err)
		}
	}
	if err := writeFrame(conn, frameEnd, nil); err != nil {
		return sendErr(err)
	}
	return nil
}

func sendFile(w io.Writer, file FileSnap, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := msgpack.Marshal(fileHeader{Hash: file.Hash, Name: file.Name, Size: fi.Size()})
	if err != nil {
		return err
	}
	if err := writeFrame(w, frameFile, hdr); err != nil {
		return err
	}
	n, err := io.Copy(w, f)
	if err != nil {
		return err
	}
	if n != fi.Size() {
		return ErrInterrupted
	}
	return nil
}

func sendErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return ErrInterrupted
	}
	return err
}

func (n *Node) loopAccept() {
	defer n.wg.Done()
	for {
		conn, err := n.ln.Accept()
		if err != nil {
			return
		}
		if n.ctx.Err() != nil {
			_ = conn.Close()
			return
		}
		n.wg.Add(1)
		go n.handle(conn)
	}
}

func (n *Node) handle(conn net.Conn) {
	defer n.wg.Done()
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-n.ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	err := n.receive(conn)
	if err == nil || errors.Is(err, ErrDeclined) || errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = ErrInterrupted
	}
	if n.onError != nil && n.ctx.Err() == nil {
		n.onError(err)
	}
}

func (n *Node) receive(conn net.Conn) error {
	if err := readMagic(conn); err != nil {
		return err
	}
	kind, payload, err := readFrame(conn)
	if err != nil {
		return err
	}
	if kind != frameHello {
		return errors.New("ragle: expected hello")
	}
	var greet hello
	if err := msgpack.Unmarshal(payload, &greet); err != nil {
		return err
	}
	kind, payload, err = readFrame(conn)
	if err != nil {
		return err
	}
	if kind != frameMeta {
		return errors.New("ragle: expected meta")
	}
	var bundle Bundle
	if err := msgpack.Unmarshal(payload, &bundle); err != nil {
		return err
	}
	from := greet.Name
	if from == "" {
		from = greet.ID
	}
	yes := false
	if n.decide != nil {
		yes = n.decide(n.ctx, from, bundle)
	}
	if n.ctx.Err() != nil {
		return context.Canceled
	}
	if err := writeAccept(conn, yes); err != nil {
		return err
	}
	if !yes {
		return ErrDeclined
	}
	paths := map[string]string{}
	cleanup := func() {
		for _, path := range paths {
			_ = os.Remove(path)
		}
	}
	for {
		if n.ctx.Err() != nil {
			cleanup()
			return context.Canceled
		}
		kind, payload, err = readFrame(conn)
		if err != nil {
			cleanup()
			return err
		}
		switch kind {
		case frameEnd:
			if n.commit == nil {
				cleanup()
				return errors.New("ragle: nowhere to store the project")
			}
			if _, err := n.commit(bundle, paths); err != nil {
				cleanup()
				return err
			}
			return nil
		case frameFile:
			var hdr fileHeader
			if err := msgpack.Unmarshal(payload, &hdr); err != nil {
				cleanup()
				return err
			}
			path, err := n.takeFile(conn, hdr)
			if err != nil {
				cleanup()
				return err
			}
			if path != "" {
				paths[hdr.Hash] = path
			}
		default:
			cleanup()
			return errors.New("ragle: unexpected frame")
		}
	}
}

func (n *Node) takeFile(r io.Reader, hdr fileHeader) (string, error) {
	if hdr.Size < 0 {
		return "", errors.New("ragle: bad file size")
	}
	if n.take == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(r, hdr.Size))
		return "", errors.New("ragle: nowhere to store the file")
	}
	sum := sha256.New()
	body := io.TeeReader(io.LimitReader(r, hdr.Size), sum)
	path, err := n.take(hdr.Hash, hdr.Name, body)
	_, _ = io.Copy(io.Discard, body)
	if err != nil {
		if path != "" {
			_ = os.Remove(path)
		}
		return "", err
	}
	got := hex.EncodeToString(sum.Sum(nil))
	fi, statErr := os.Stat(path)
	if statErr != nil || fi.Size() != hdr.Size || got != hdr.Hash {
		if path != "" {
			_ = os.Remove(path)
		}
		return "", errors.New("ragle: file hash mismatch")
	}
	return path, nil
}

func (n *Node) readUDP() {
	defer n.wg.Done()
	buf := make([]byte, 2048)
	for {
		k, addr, err := n.udp.ReadFrom(buf)
		if err != nil {
			return
		}
		var msg beacon
		if err := msgpack.Unmarshal(buf[:k], &msg); err != nil {
			continue
		}
		if msg.Magic != magic || msg.ID == "" || msg.ID == n.id || msg.Port <= 0 {
			continue
		}
		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok || udpAddr.IP == nil {
			continue
		}
		ip := udpAddr.IP
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		n.note(storedPeer{
			Peer: Peer{
				ID:   msg.ID,
				Name: msg.Name,
				Addr: net.JoinHostPort(ip.String(), strconv.Itoa(msg.Port)),
			},
			Seen: time.Now(),
		})
	}
}

func (n *Node) beaconLoop() {
	defer n.wg.Done()
	if !n.quiet {
		n.announce()
	}
	tick := time.NewTicker(beaconEvery)
	defer tick.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-tick.C:
			if !n.quiet {
				n.announce()
			}
			n.expire()
		}
	}
}

func (n *Node) announce() {
	if n.udp == nil || n.udpPort == 0 {
		return
	}
	raw, err := msgpack.Marshal(beacon{Magic: magic, ID: n.id, Name: n.name, Port: n.tcpPort})
	if err != nil {
		return
	}
	targets := []string{
		fmt.Sprintf("255.255.255.255:%d", n.udpPort),
		fmt.Sprintf("127.0.0.1:%d", n.udpPort),
	}
	for _, target := range targets {
		addr, err := net.ResolveUDPAddr("udp4", target)
		if err != nil {
			continue
		}
		_, _ = n.udp.WriteTo(raw, addr)
	}
}

func (n *Node) note(p storedPeer) {
	n.mu.Lock()
	prev, ok := n.peers[p.ID]
	changed := !ok || prev.Addr != p.Addr || prev.Name != p.Name
	n.peers[p.ID] = p
	n.mu.Unlock()
	if changed && n.onPeers != nil {
		n.onPeers()
	}
}

func (n *Node) expire() {
	n.mu.Lock()
	before := len(n.peers)
	n.prune(time.Now())
	after := len(n.peers)
	n.mu.Unlock()
	if after != before && n.onPeers != nil {
		n.onPeers()
	}
}

func (n *Node) prune(now time.Time) {
	for id, p := range n.peers {
		if now.Sub(p.Seen) > peerTTL {
			delete(n.peers, id)
		}
	}
}

func listenUDP(port int) (*net.UDPConn, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var sockErr error
			err := c.Control(func(fd uintptr) {
				sockErr = setReuse(fd)
			})
			if err != nil {
				return err
			}
			return sockErr
		},
	}
	pc, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, errors.New("ragle: udp address")
	}
	return conn, nil
}
