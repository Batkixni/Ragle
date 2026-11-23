package share

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
)

// msgPipe abstracts the ordered message transport shared by both direct
// DTLS-encrypted WebRTC P2P DataChannels and the zero-storage X25519+AES-256-GCM
// E2E blind relay fallback.
type msgPipe interface {
	send(ctx context.Context, m dcMsg) error
	recv(ctx context.Context) (dcMsg, error)
}

// e2eKeyPair holds an ephemeral X25519 keypair for a single pairing session.
type e2eKeyPair struct {
	priv *ecdh.PrivateKey
	pub  string // base64url-encoded 32-byte X25519 public key
}

func newE2EKeyPair() (*e2eKeyPair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &e2eKeyPair{
		priv: priv,
		pub:  base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
	}, nil
}

// e2eCipher performs authenticated, replay-resistant AES-256-GCM framing using
// directional keys derived from an ephemeral X25519 shared secret and the room code.
type e2eCipher struct {
	sendAEAD cipher.AEAD
	recvAEAD cipher.AEAD
	sendDir  [4]byte
	recvDir  [4]byte

	sendMu  sync.Mutex
	sendSeq uint64
	recvMu  sync.Mutex
	recvSeq uint64
}

func newE2ECipher(priv *ecdh.PrivateKey, peerPubB64, code string, isSender bool) (*e2eCipher, error) {
	rawPeer, err := base64.RawURLEncoding.DecodeString(peerPubB64)
	if err != nil {
		return nil, fmt.Errorf("raglex: invalid peer public key: %w", err)
	}
	peerPub, err := ecdh.X25519().NewPublicKey(rawPeer)
	if err != nil {
		return nil, fmt.Errorf("raglex: bad X25519 peer key: %w", err)
	}
	shared, err := priv.ECDH(peerPub)
	if err != nil {
		return nil, err
	}
	norm := NormalizeCode(code)
	s2rKey := deriveDirectionalKey(shared, norm, "raglex-e2e-v1-s2r")
	r2sKey := deriveDirectionalKey(shared, norm, "raglex-e2e-v1-r2s")

	s2rBlock, err := aes.NewCipher(s2rKey[:])
	if err != nil {
		return nil, err
	}
	r2sBlock, err := aes.NewCipher(r2sKey[:])
	if err != nil {
		return nil, err
	}
	s2rAEAD, err := cipher.NewGCM(s2rBlock)
	if err != nil {
		return nil, err
	}
	r2sAEAD, err := cipher.NewGCM(r2sBlock)
	if err != nil {
		return nil, err
	}

	c := &e2eCipher{}
	if isSender {
		c.sendAEAD, c.recvAEAD = s2rAEAD, r2sAEAD
		c.sendDir = [4]byte{'S', '2', 'R', 1}
		c.recvDir = [4]byte{'R', '2', 'S', 1}
	} else {
		c.sendAEAD, c.recvAEAD = r2sAEAD, s2rAEAD
		c.sendDir = [4]byte{'R', '2', 'S', 1}
		c.recvDir = [4]byte{'S', '2', 'R', 1}
	}
	return c, nil
}

func deriveDirectionalKey(shared []byte, code, domain string) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(code))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(shared)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func makeNonce(dir [4]byte, seq uint64) []byte {
	var nonce [12]byte
	copy(nonce[:4], dir[:])
	binary.BigEndian.PutUint64(nonce[4:], seq)
	return nonce[:]
}

// Seal encrypts plaintext with a strictly monotonic nonce and binds the
// direction + sequence number as AAD so replayed or reordered frames are rejected.
func (c *e2eCipher) Seal(plaintext []byte) []byte {
	c.sendMu.Lock()
	seq := c.sendSeq
	c.sendSeq++
	c.sendMu.Unlock()

	nonce := makeNonce(c.sendDir, seq)
	out := make([]byte, 8, 8+len(plaintext)+c.sendAEAD.Overhead())
	binary.BigEndian.PutUint64(out[:8], seq)
	return c.sendAEAD.Seal(out, nonce, plaintext, nonce)
}

// Open authenticates and decrypts a frame, enforcing strictly ordered sequence numbers.
// The receive sequence counter only advances after GCM authentication succeeds.
func (c *e2eCipher) Open(frame []byte) ([]byte, error) {
	if len(frame) < 8+c.recvAEAD.Overhead() {
		return nil, errors.New("raglex: truncated e2e frame")
	}
	seq := binary.BigEndian.Uint64(frame[:8])

	c.recvMu.Lock()
	defer c.recvMu.Unlock()
	if seq != c.recvSeq {
		return nil, fmt.Errorf("raglex: e2e sequence mismatch (got %d, want %d)", seq, c.recvSeq)
	}
	nonce := makeNonce(c.recvDir, seq)
	plain, err := c.recvAEAD.Open(nil, nonce, frame[8:], nonce)
	if err != nil {
		return nil, fmt.Errorf("raglex: e2e decryption failed: %w", err)
	}
	c.recvSeq++
	return plain, nil
}

// relayPipe streams X25519+AES-256-GCM encrypted dcMsg frames through the
// SignalingServer's zero-storage in-memory blind relay when direct WebRTC
// hole-punching is blocked by strict symmetric NAT / CGNAT.
type relayPipe struct {
	client *SignalingClient
	code   string
	role   string // "sender" or "receiver"
	token  string
	cipher *e2eCipher
}

func newRelayPipe(client *SignalingClient, code, role, token string, c *e2eCipher) *relayPipe {
	return &relayPipe{
		client: client,
		code:   NormalizeCode(code),
		role:   role,
		token:  token,
		cipher: c,
	}
}

func (p *relayPipe) send(ctx context.Context, m dcMsg) error {
	plain, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	ciphertext := p.cipher.Seal(plain)
	return p.client.RelayPush(ctx, p.code, p.role, p.token, ciphertext)
}

func (p *relayPipe) recv(ctx context.Context) (dcMsg, error) {
	ciphertext, err := p.client.RelayPull(ctx, p.code, p.role, p.token)
	if err != nil {
		return dcMsg{}, err
	}
	plain, err := p.cipher.Open(ciphertext)
	if err != nil {
		return dcMsg{}, err
	}
	var m dcMsg
	if err := msgpack.Unmarshal(plain, &m); err != nil {
		return dcMsg{}, err
	}
	return m, nil
}
