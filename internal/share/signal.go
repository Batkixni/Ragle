package share

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Pairing error sentinels.
var (
	ErrRoomNotFound = errors.New("raglex: pairing code not found or expired")
	ErrRoomClaimed  = errors.New("raglex: pairing code already used")
	ErrBadCode      = errors.New("raglex: invalid pairing code")
	ErrNoSignaling  = errors.New("raglex: signaling server URL is empty")
)

// codeAlphabet omits ambiguous characters (0, 1, I, L, O).
const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

const (
	defaultRoomTTL     = 5 * time.Minute
	defaultPollTimeout = 25 * time.Second
	maxSignalBody      = 256 << 10 // 256 KiB, ample for SDP payloads
	maxRelayFrame      = 256 << 10 // 256 KiB max single encrypted relay frame
	relayChanCap       = 32        // bounded in-memory backpressure queue
)

// NormalizeCode strips whitespace and dashes and uppercases a pairing code.
func NormalizeCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if r == '-' || r == ' ' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// FormatCode formats a 6-character pairing code as XXX-XXX.
func FormatCode(code string) string {
	norm := NormalizeCode(code)
	if len(norm) == 6 {
		return norm[:3] + "-" + norm[3:]
	}
	return norm
}

func generateCode() (string, error) {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	out := make([]byte, 6)
	for i, v := range raw {
		out[i] = codeAlphabet[int(v)%len(codeAlphabet)]
	}
	return string(out), nil
}

func generateToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

type signalRoom struct {
	code           string
	token          string
	recvToken      string
	name           string
	offer          string
	answer         string
	senderPubKey   string
	receiverPubKey string
	claimed        bool
	expiresAt      time.Time
	answered       chan struct{}
	toReceiver     chan []byte
	toSender       chan []byte
	closed         chan struct{}
	closeOnce      sync.Once
}

func (r *signalRoom) closeRoom() {
	r.closeOnce.Do(func() {
		if r.closed != nil {
			close(r.closed)
		}
	})
}

// SignalingServer is a lightweight, self-hostable HTTP signaling server for
// WebRTC pairing-code handshakes and zero-storage E2E blind ciphertext relay.
// It holds only ephemeral SDP handshakes and opaque encrypted RAM frames and
// never writes project data or files to disk.
type SignalingServer struct {
	ttl         time.Duration
	pollTimeout time.Duration

	mu       sync.Mutex
	rooms    map[string]*signalRoom
	onRelay  func(code, role string, ciphertext []byte) // optional test observer hook
	stopCh   chan struct{}
	once     sync.Once
}

// NewSignalingServer creates an in-memory signaling server with the given room TTL.
func NewSignalingServer(ttl time.Duration) *SignalingServer {
	if ttl <= 0 {
		ttl = defaultRoomTTL
	}
	s := &SignalingServer{
		ttl:         ttl,
		pollTimeout: defaultPollTimeout,
		rooms:       map[string]*signalRoom{},
		stopCh:      make(chan struct{}),
	}
	go s.reapLoop()
	return s
}

// Close stops background expiration cleanup.
func (s *SignalingServer) Close() {
	s.once.Do(func() {
		close(s.stopCh)
	})
}

func (s *SignalingServer) reapLoop() {
	interval := s.ttl / 2
	if interval < time.Second {
		interval = time.Second
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-tick.C:
			s.reap(now)
		}
	}
}

func (s *SignalingServer) reap(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for code, room := range s.rooms {
		if now.After(room.expiresAt) {
			room.closeRoom()
			delete(s.rooms, code)
		}
	}
}

// ServeHTTP handles /healthz and /v1/rooms/... endpoints (signaling + zero-storage E2E blind relay).
func (s *SignalingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
		return
	}
	if r.URL.Path == "/v1/rooms" && r.Method == http.MethodPost {
		s.handleCreate(w, r)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/rooms/") {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/rooms/")
	parts := strings.Split(rest, "/")
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.handleDelete(w, r, parts[0])
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	code, action := parts[0], parts[1]
	switch action {
	case "offer":
		if r.Method == http.MethodGet {
			s.handleGetOffer(w, r, code)
			return
		}
	case "answer":
		switch r.Method {
		case http.MethodPost:
			s.handleSubmitAnswer(w, r, code)
			return
		case http.MethodGet:
			s.handleWaitAnswer(w, r, code)
			return
		}
	case "relay":
		switch r.Method {
		case http.MethodPost:
			s.handleRelayPush(w, r, code)
			return
		case http.MethodGet:
			s.handleRelayPull(w, r, code)
			return
		}
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

type createRoomReq struct {
	Offer  string `json:"offer"`
	Name   string `json:"name"`
	PubKey string `json:"pub_key,omitempty"`
}

type createRoomResp struct {
	Code      string `json:"code"`
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

type offerResp struct {
	Offer  string `json:"offer"`
	Name   string `json:"name"`
	PubKey string `json:"pub_key,omitempty"`
}

type answerPayload struct {
	Answer    string `json:"answer"`
	PubKey    string `json:"pub_key,omitempty"`
	RecvToken string `json:"recv_token,omitempty"`
}

func (s *SignalingServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRoomReq
	if err := json.NewDecoder(io.LimitReader(r.Body, maxSignalBody)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Offer) == "" {
		http.Error(w, "missing offer", http.StatusBadRequest)
		return
	}
	token, err := generateToken()
	if err != nil {
		http.Error(w, "entropy error", http.StatusInternalServerError)
		return
	}
	now := time.Now()
	s.mu.Lock()
	var code string
	for range 16 {
		cand, genErr := generateCode()
		if genErr != nil {
			s.mu.Unlock()
			http.Error(w, "entropy error", http.StatusInternalServerError)
			return
		}
		if existing, ok := s.rooms[cand]; !ok || now.After(existing.expiresAt) {
			code = cand
			break
		}
	}
	if code == "" {
		s.mu.Unlock()
		http.Error(w, "room space exhausted", http.StatusServiceUnavailable)
		return
	}
	s.rooms[code] = &signalRoom{
		code:         code,
		token:        token,
		name:         req.Name,
		offer:        req.Offer,
		senderPubKey: req.PubKey,
		expiresAt:    now.Add(s.ttl),
		answered:     make(chan struct{}),
		toReceiver:   make(chan []byte, relayChanCap),
		toSender:     make(chan []byte, relayChanCap),
		closed:       make(chan struct{}),
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, createRoomResp{
		Code:      FormatCode(code),
		Token:     token,
		ExpiresIn: int(s.ttl.Seconds()),
	})
}

func (s *SignalingServer) handleGetOffer(w http.ResponseWriter, _ *http.Request, rawCode string) {
	code := NormalizeCode(rawCode)
	now := time.Now()
	s.mu.Lock()
	room, ok := s.rooms[code]
	if !ok || now.After(room.expiresAt) {
		if ok {
			room.closeRoom()
		}
		delete(s.rooms, code)
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if room.claimed {
		s.mu.Unlock()
		http.Error(w, "already claimed", http.StatusConflict)
		return
	}
	resp := offerResp{Offer: room.offer, Name: room.name, PubKey: room.senderPubKey}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

func (s *SignalingServer) handleSubmitAnswer(w http.ResponseWriter, r *http.Request, rawCode string) {
	var req answerPayload
	if err := json.NewDecoder(io.LimitReader(r.Body, maxSignalBody)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Answer) == "" {
		http.Error(w, "missing answer", http.StatusBadRequest)
		return
	}
	recvToken, err := generateToken()
	if err != nil {
		http.Error(w, "entropy error", http.StatusInternalServerError)
		return
	}
	code := NormalizeCode(rawCode)
	now := time.Now()
	s.mu.Lock()
	room, ok := s.rooms[code]
	if !ok || now.After(room.expiresAt) {
		if ok {
			room.closeRoom()
		}
		delete(s.rooms, code)
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if room.claimed {
		s.mu.Unlock()
		http.Error(w, "already claimed", http.StatusConflict)
		return
	}
	room.claimed = true
	room.answer = req.Answer
	room.receiverPubKey = req.PubKey
	room.recvToken = recvToken
	close(room.answered)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, answerPayload{RecvToken: recvToken})
}

func (s *SignalingServer) handleWaitAnswer(w http.ResponseWriter, r *http.Request, rawCode string) {
	code := NormalizeCode(rawCode)
	token := r.URL.Query().Get("token")
	now := time.Now()

	s.mu.Lock()
	room, ok := s.rooms[code]
	if !ok || now.After(room.expiresAt) {
		if ok {
			room.closeRoom()
		}
		delete(s.rooms, code)
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if token == "" || token != room.token {
		s.mu.Unlock()
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if room.answer != "" {
		ans := room.answer
		rpub := room.receiverPubKey
		if room.senderPubKey == "" {
			room.closeRoom()
			delete(s.rooms, code)
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, answerPayload{Answer: ans, PubKey: rpub})
		return
	}
	waitCh := room.answered
	rem := time.Until(room.expiresAt)
	s.mu.Unlock()

	timeout := s.pollTimeout
	if rem < timeout {
		timeout = rem
	}
	if timeout <= 0 {
		http.Error(w, "expired", http.StatusNotFound)
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-waitCh:
		s.mu.Lock()
		cur, exists := s.rooms[code]
		if !exists || cur != room || cur.answer == "" {
			s.mu.Unlock()
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		ans := cur.answer
		rpub := cur.receiverPubKey
		if cur.senderPubKey == "" {
			cur.closeRoom()
			delete(s.rooms, code)
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, answerPayload{Answer: ans, PubKey: rpub})
	case <-timer.C:
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
	}
}

func (s *SignalingServer) authorizeRelay(rawCode, role, token string) (*signalRoom, chan []byte, chan []byte, int) {
	code := NormalizeCode(rawCode)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[code]
	if !ok || now.After(room.expiresAt) {
		if ok {
			room.closeRoom()
		}
		delete(s.rooms, code)
		return nil, nil, nil, http.StatusNotFound
	}
	switch role {
	case "sender":
		if token == "" || token != room.token {
			return nil, nil, nil, http.StatusForbidden
		}
		return room, room.toReceiver, room.toSender, http.StatusOK
	case "receiver":
		if token == "" || token != room.recvToken {
			return nil, nil, nil, http.StatusForbidden
		}
		return room, room.toSender, room.toReceiver, http.StatusOK
	default:
		return nil, nil, nil, http.StatusBadRequest
	}
}

// handleRelayPush accepts one opaque E2E-encrypted ciphertext frame and queues
// it in memory for the peer. Zero bytes ever touch disk.
func (s *SignalingServer) handleRelayPush(w http.ResponseWriter, r *http.Request, rawCode string) {
	role := r.URL.Query().Get("role")
	token := r.URL.Query().Get("token")
	room, sendCh, _, status := s.authorizeRelay(rawCode, role, token)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRelayFrame+1))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if len(body) == 0 || len(body) > maxRelayFrame {
		http.Error(w, "invalid frame size", http.StatusRequestEntityTooLarge)
		return
	}
	if s.onRelay != nil {
		s.onRelay(room.code, role, body)
	}
	timer := time.NewTimer(s.pollTimeout)
	defer timer.Stop()
	select {
	case sendCh <- body:
		w.WriteHeader(http.StatusOK)
	case <-room.closed:
		http.Error(w, "room closed", http.StatusGone)
	case <-timer.C:
		http.Error(w, "peer timeout", http.StatusGatewayTimeout)
	case <-r.Context().Done():
	}
}

// handleRelayPull long-polls one opaque E2E-encrypted ciphertext frame sent by
// the peer, returning 204 No Content on poll timeout so clients retry cleanly.
func (s *SignalingServer) handleRelayPull(w http.ResponseWriter, r *http.Request, rawCode string) {
	role := r.URL.Query().Get("role")
	token := r.URL.Query().Get("token")
	room, _, recvCh, status := s.authorizeRelay(rawCode, role, token)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}
	timer := time.NewTimer(s.pollTimeout)
	defer timer.Stop()
	select {
	case frame := <-recvCh:
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(frame)
	case <-room.closed:
		select {
		case frame := <-recvCh:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(frame)
		default:
			http.Error(w, "room closed", http.StatusGone)
		}
	case <-timer.C:
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
	}
}

func (s *SignalingServer) handleDelete(w http.ResponseWriter, r *http.Request, rawCode string) {
	code := NormalizeCode(rawCode)
	token := r.URL.Query().Get("token")
	s.mu.Lock()
	room, ok := s.rooms[code]
	if !ok {
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if token == "" || token != room.token {
		s.mu.Unlock()
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	delete(s.rooms, code)
	room.closeRoom()
	if !room.claimed {
		room.claimed = true
		close(room.answered)
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// SignalingClient talks to a SignalingServer over HTTP.
type SignalingClient struct {
	baseURL string
	http    *http.Client
}

// NewSignalingClient returns a client for the given base URL (e.g. http://127.0.0.1:8787).
func NewSignalingClient(baseURL string) *SignalingClient {
	return &SignalingClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: 35 * time.Second},
	}
}

// CreateRoom registers an SDP offer and returns the short pairing code and owner token.
func (c *SignalingClient) CreateRoom(ctx context.Context, offerSDP, senderName string) (string, string, error) {
	return c.CreateRoomWithKey(ctx, offerSDP, senderName, "")
}

// CreateRoomWithKey registers an SDP offer plus an ephemeral X25519 public key
// so peers can establish an E2E-encrypted blind relay fallback if direct ICE fails.
func (c *SignalingClient) CreateRoomWithKey(ctx context.Context, offerSDP, senderName, pubKey string) (string, string, error) {
	if c.baseURL == "" {
		return "", "", ErrNoSignaling
	}
	body, err := json.Marshal(createRoomReq{Offer: offerSDP, Name: senderName, PubKey: pubKey})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/rooms", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("raglex: signaling create failed (%d)", resp.StatusCode)
	}
	var out createRoomResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSignalBody)).Decode(&out); err != nil {
		return "", "", err
	}
	if out.Code == "" || out.Token == "" {
		return "", "", errors.New("raglex: invalid signaling response")
	}
	return FormatCode(out.Code), out.Token, nil
}

// GetOffer retrieves the sender's SDP offer for the given pairing code.
func (c *SignalingClient) GetOffer(ctx context.Context, code string) (string, string, error) {
	offer, name, _, err := c.GetOfferWithKey(ctx, code)
	return offer, name, err
}

// GetOfferWithKey retrieves the sender's SDP offer, display name, and ephemeral X25519 public key.
func (c *SignalingClient) GetOfferWithKey(ctx context.Context, code string) (string, string, string, error) {
	if c.baseURL == "" {
		return "", "", "", ErrNoSignaling
	}
	norm := NormalizeCode(code)
	if len(norm) < 4 {
		return "", "", "", ErrBadCode
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/rooms/"+norm+"/offer", nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var out offerResp
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxSignalBody)).Decode(&out); err != nil {
			return "", "", "", err
		}
		return out.Offer, out.Name, out.PubKey, nil
	case http.StatusNotFound:
		return "", "", "", ErrRoomNotFound
	case http.StatusConflict:
		return "", "", "", ErrRoomClaimed
	default:
		return "", "", "", fmt.Errorf("raglex: signaling offer failed (%d)", resp.StatusCode)
	}
}

// SubmitAnswer posts the receiver's SDP answer for the given pairing code.
func (c *SignalingClient) SubmitAnswer(ctx context.Context, code, answerSDP string) error {
	_, err := c.SubmitAnswerWithKey(ctx, code, answerSDP, "")
	return err
}

// SubmitAnswerWithKey posts the receiver's SDP answer and ephemeral X25519 public key,
// returning a receiver relay auth token for the blind E2E relay fallback.
func (c *SignalingClient) SubmitAnswerWithKey(ctx context.Context, code, answerSDP, pubKey string) (string, error) {
	if c.baseURL == "" {
		return "", ErrNoSignaling
	}
	norm := NormalizeCode(code)
	body, err := json.Marshal(answerPayload{Answer: answerSDP, PubKey: pubKey})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/rooms/"+norm+"/answer", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var out answerPayload
		_ = json.NewDecoder(io.LimitReader(resp.Body, maxSignalBody)).Decode(&out)
		return out.RecvToken, nil
	case http.StatusNotFound:
		return "", ErrRoomNotFound
	case http.StatusConflict:
		return "", ErrRoomClaimed
	default:
		return "", fmt.Errorf("raglex: signaling answer failed (%d)", resp.StatusCode)
	}
}

// WaitAnswer long-polls until the receiver submits an answer or ctx is canceled.
func (c *SignalingClient) WaitAnswer(ctx context.Context, code, token string) (string, error) {
	ans, _, err := c.WaitAnswerWithKey(ctx, code, token)
	return ans, err
}

// WaitAnswerWithKey long-polls until the receiver submits an SDP answer and X25519 public key.
func (c *SignalingClient) WaitAnswerWithKey(ctx context.Context, code, token string) (string, string, error) {
	if c.baseURL == "" {
		return "", "", ErrNoSignaling
	}
	norm := NormalizeCode(code)
	u := c.baseURL + "/v1/rooms/" + norm + "/answer?token=" + token
	for {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", "", err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return "", "", err
		}
		status := resp.StatusCode
		var out answerPayload
		var decErr error
		if status == http.StatusOK {
			decErr = json.NewDecoder(io.LimitReader(resp.Body, maxSignalBody)).Decode(&out)
		} else {
			_, _ = io.Copy(io.Discard, resp.Body)
		}
		_ = resp.Body.Close()

		switch status {
		case http.StatusOK:
			if decErr != nil {
				return "", "", decErr
			}
			return out.Answer, out.PubKey, nil
		case http.StatusNoContent:
			continue
		case http.StatusNotFound:
			return "", "", ErrRoomNotFound
		default:
			return "", "", fmt.Errorf("raglex: signaling wait failed (%d)", status)
		}
	}
}

// RelayPush sends one opaque E2E-encrypted frame to the peer through the zero-storage blind relay.
func (c *SignalingClient) RelayPush(ctx context.Context, code, role, token string, ciphertext []byte) error {
	norm := NormalizeCode(code)
	u := fmt.Sprintf("%s/v1/rooms/%s/relay?role=%s&token=%s", c.baseURL, norm, role, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(ciphertext))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		return ErrInterrupted
	}
	return fmt.Errorf("raglex: relay push failed (%d)", resp.StatusCode)
}

// RelayPull long-polls the next opaque E2E-encrypted frame from the peer.
func (c *SignalingClient) RelayPull(ctx context.Context, code, role, token string) ([]byte, error) {
	norm := NormalizeCode(code)
	u := fmt.Sprintf("%s/v1/rooms/%s/relay?role=%s&token=%s", c.baseURL, norm, role, token)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		status := resp.StatusCode
		var data []byte
		var readErr error
		if status == http.StatusOK {
			data, readErr = io.ReadAll(io.LimitReader(resp.Body, maxRelayFrame+1))
		} else {
			_, _ = io.Copy(io.Discard, resp.Body)
		}
		_ = resp.Body.Close()

		switch status {
		case http.StatusOK:
			if readErr != nil {
				return nil, readErr
			}
			return data, nil
		case http.StatusNoContent:
			continue
		case http.StatusGone, http.StatusNotFound:
			return nil, ErrInterrupted
		default:
			return nil, fmt.Errorf("raglex: relay pull failed (%d)", status)
		}
	}
}

// DeleteRoom removes an unclaimed room from the signaling server.
func (c *SignalingClient) DeleteRoom(ctx context.Context, code, token string) error {
	if c.baseURL == "" || code == "" || token == "" {
		return nil
	}
	norm := NormalizeCode(code)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/v1/rooms/"+norm+"?token="+token, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return nil
}
