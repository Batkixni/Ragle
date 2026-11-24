package sync

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	e2eeBlobMagic = "RGLXE2E1" // 8-byte header for AES-256-GCM encrypted blobs
	e2eeOpKind    = "e2ee.v1"
	// E2EEKeyPrefix marks a direct 256-bit randomly generated AES-256 key.
	E2EEKeyPrefix = "rglx-aes256-"
)

type e2eeEnvelope struct {
	E2EE string `json:"e2ee"`
	CT   string `json:"ct"`
}

// GenerateE2EEKey generates a cryptographically random 256-bit AES key encoded
// with the recognizable "rglx-aes256-" prefix for safe backup and transfer.
func GenerateE2EEKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return E2EEKeyPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// FormatE2EEKeyFile builds a human-readable local backup file containing the
// user's zero-knowledge AES-256-GCM recovery key.
func FormatE2EEKeyFile(key, email string) string {
	var b strings.Builder
	b.WriteString("# Raglex Zero-Knowledge E2EE Recovery Key (AES-256-GCM)\n")
	b.WriteString("# Keep this file in a safe place (e.g. password manager or offline backup).\n")
	b.WriteString("# Never share this key with anyone — Raglex Cloud cannot recover lost keys.\n")
	if strings.TrimSpace(email) != "" {
		fmt.Fprintf(&b, "# Account: %s\n", strings.TrimSpace(email))
	}
	fmt.Fprintf(&b, "# Exported: %s\n", time.Now().Format(time.RFC3339))
	b.WriteString("#\n")
	b.WriteString("# To decrypt synced data on another device:\n")
	b.WriteString("# Preferences -> Sync & Collab -> Load Key File (or paste the key below).\n\n")
	b.WriteString(strings.TrimSpace(key))
	b.WriteString("\n")
	return b.String()
}

// ParseE2EEKeyFile extracts a Raglex E2EE key (or custom passphrase) from an
// exported recovery key file, ignoring comment lines starting with '#'.
func ParseE2EEKeyFile(raw string) string {
	var fallback string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasPrefix(s, E2EEKeyPrefix) {
			return s
		}
		if fallback == "" {
			fallback = s
		}
	}
	return fallback
}

// deriveE2EEKey resolves a user's local E2EE secret into a 32-byte AES-256 key.
// Auto-generated "rglx-aes256-..." keys decode directly to 32 random bytes;
// custom user passphrases are stretched via PBKDF2-SHA256 (210,000 iterations).
// The key NEVER leaves the user's device.
func deriveE2EEKey(passphrase string) ([]byte, error) {
	pass := strings.TrimSpace(passphrase)
	if pass == "" {
		return nil, nil
	}
	if strings.HasPrefix(pass, E2EEKeyPrefix) {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(pass, E2EEKeyPrefix))
		if err == nil && len(raw) == 32 {
			return raw, nil
		}
	}
	return pbkdf2.Key(sha256.New, pass, []byte("raglex-sync-e2ee-v1"), 210_000, 32)
}

func e2eeNonce(key []byte, domain, id string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(id))
	return mac.Sum(nil)[:12]
}

// encryptBlob encrypts raw file bytes with AES-256-GCM using a user-scoped
// deterministic nonce so identical files from the same user still deduplicate
// cleanly while remaining opaque ciphertext to the server and other users.
func encryptBlob(key []byte, plainHash string, plain []byte) ([]byte, string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", err
	}
	nonce := e2eeNonce(key, "blob", plainHash)
	ct := gcm.Seal(nil, nonce, plain, []byte("blob:"+plainHash))
	out := make([]byte, 0, len(e2eeBlobMagic)+len(nonce)+len(ct))
	out = append(out, e2eeBlobMagic...)
	out = append(out, nonce...)
	out = append(out, ct...)
	sum := sha256.Sum256(out)
	return out, hex.EncodeToString(sum[:]), nil
}

// decryptBlob verifies and decrypts an AES-256-GCM blob produced by encryptBlob.
func decryptBlob(key []byte, plainHash string, data []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("missing e2ee key")
	}
	minLen := len(e2eeBlobMagic) + 12 + 16
	if len(data) < minLen || !bytes.Equal(data[:len(e2eeBlobMagic)], []byte(e2eeBlobMagic)) {
		return nil, errors.New("invalid e2ee blob header")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := data[len(e2eeBlobMagic) : len(e2eeBlobMagic)+12]
	ct := data[len(e2eeBlobMagic)+12:]
	return gcm.Open(nil, nonce, ct, []byte("blob:"+plainHash))
}

// encryptOpPayload encrypts a SyncOp JSON payload into an opaque e2ee.v1 envelope.
func encryptOpPayload(key []byte, opID string, rawJSON []byte) (json.RawMessage, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := e2eeNonce(key, "op", opID)
	ct := gcm.Seal(nil, nonce, rawJSON, []byte("op:"+opID))
	raw := make([]byte, 0, len(nonce)+len(ct))
	raw = append(raw, nonce...)
	raw = append(raw, ct...)
	env := e2eeEnvelope{
		E2EE: "v1",
		CT:   base64.StdEncoding.EncodeToString(raw),
	}
	b, err := json.Marshal(env)
	return json.RawMessage(b), err
}

// decryptOpPayload decrypts an opaque e2ee.v1 envelope back into raw SyncOp JSON.
func decryptOpPayload(key []byte, opID string, wrapped json.RawMessage) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("missing e2ee key")
	}
	var env e2eeEnvelope
	if err := json.Unmarshal(wrapped, &env); err != nil || env.E2EE != "v1" || env.CT == "" {
		return nil, errors.New("invalid e2ee envelope")
	}
	raw, err := base64.StdEncoding.DecodeString(env.CT)
	if err != nil || len(raw) < 12+16 {
		return nil, errors.New("corrupt e2ee ciphertext")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, raw[:12], raw[12:], []byte("op:"+opID))
}
