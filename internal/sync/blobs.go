package sync

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxBlobUploadBytes = 512 << 20 // 512 MiB per blob

// BlobStore abstracts content-addressed SHA-256 object storage so self-hosted
// instances can store blobs on local disk while Raglex Cloud / NAS deployments
// can use Cloudflare R2 or S3 presigned URLs with zero code changes.
type BlobStore interface {
	BackendName() string
	Has(ctx context.Context, hash string) bool
	Put(ctx context.Context, hash string, r io.Reader) (int64, error)
	Open(ctx context.Context, hash string) (io.ReadCloser, int64, error)
	PresignURL(method, hash string, expires time.Duration) (string, bool)
}

// LocalBlobStore stores content-addressed blobs under <root>/blobs/<aa>/<bb>/<hash>.
type LocalBlobStore struct {
	root string
	r2   *R2Presigner // optional Cloudflare R2 / S3 presigned URL signer
}

// NewLocalBlobStore creates a LocalBlobStore rooted at root/blobs.
func NewLocalBlobStore(root string, r2 *R2Presigner) (*LocalBlobStore, error) {
	dir := filepath.Join(root, "blobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &LocalBlobStore{root: dir, r2: r2}, nil
}

func (l *LocalBlobStore) BackendName() string {
	if l.r2 != nil && l.r2.Enabled() {
		return "s3-r2"
	}
	return "local"
}

func (l *LocalBlobStore) blobPath(hash string) string {
	return filepath.Join(l.root, hash[:2], hash[2:4], hash)
}

func (l *LocalBlobStore) Has(_ context.Context, hash string) bool {
	h, err := ValidateSHA256(hash)
	if err != nil {
		return false
	}
	fi, err := os.Stat(l.blobPath(h))
	return err == nil && !fi.IsDir()
}

// Put streams r into a temp file, verifies the SHA-256 digest matches hash,
// and atomically moves it into place.
func (l *LocalBlobStore) Put(_ context.Context, hash string, r io.Reader) (int64, error) {
	h, err := ValidateSHA256(hash)
	if err != nil {
		return 0, err
	}
	dst := l.blobPath(h)
	if fi, err := os.Stat(dst); err == nil && !fi.IsDir() {
		// Already present — drain and deduplicate immediately.
		_, _ = io.Copy(io.Discard, io.LimitReader(r, maxBlobUploadBytes))
		return fi.Size(), nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".blob-*")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, sum), io.LimitReader(r, maxBlobUploadBytes+1))
	closeErr := tmp.Close()
	if err != nil {
		return 0, err
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if n > maxBlobUploadBytes {
		return 0, fmt.Errorf("blob exceeds %d MB limit", maxBlobUploadBytes>>20)
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if got != h {
		return 0, fmt.Errorf("blob sha256 mismatch: got %s, want %s", got, h)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return 0, err
	}
	cleanup = false
	return n, nil
}

func (l *LocalBlobStore) Open(_ context.Context, hash string) (io.ReadCloser, int64, error) {
	h, err := ValidateSHA256(hash)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(l.blobPath(h))
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

// Delete removes a single content-addressed blob from local disk if present.
func (l *LocalBlobStore) Delete(_ context.Context, hash string) error {
	h, err := ValidateSHA256(hash)
	if err != nil {
		return err
	}
	err = os.Remove(l.blobPath(h))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (l *LocalBlobStore) PresignURL(method, hash string, expires time.Duration) (string, bool) {
	if l.r2 == nil || !l.r2.Enabled() {
		return "", false
	}
	h, err := ValidateSHA256(hash)
	if err != nil {
		return "", false
	}
	u, err := l.r2.Presign(method, "blobs/"+h[:2]+"/"+h[2:4]+"/"+h, expires)
	if err != nil {
		return "", false
	}
	return u, true
}

// R2Presigner generates pure-Go AWS SigV4 presigned URLs for Cloudflare R2,
// Amazon S3, or MinIO with zero third-party SDK dependencies.
type R2Presigner struct {
	Endpoint  string // e.g. https://<account>.r2.cloudflarestorage.com
	Bucket    string
	Region    string // "auto" for Cloudflare R2
	AccessKey string
	SecretKey string
}

// R2FromEnv loads optional Cloudflare R2 / S3 presigning configuration from env vars.
func R2FromEnv() *R2Presigner {
	ep := strings.TrimRight(strings.TrimSpace(os.Getenv("RAGLEX_S3_ENDPOINT")), "/")
	bkt := strings.TrimSpace(os.Getenv("RAGLEX_S3_BUCKET"))
	ak := strings.TrimSpace(os.Getenv("RAGLEX_S3_ACCESS_KEY"))
	sk := strings.TrimSpace(os.Getenv("RAGLEX_S3_SECRET_KEY"))
	reg := strings.TrimSpace(os.Getenv("RAGLEX_S3_REGION"))
	if reg == "" {
		reg = "auto"
	}
	if ep == "" || bkt == "" || ak == "" || sk == "" {
		return nil
	}
	return &R2Presigner{
		Endpoint:  ep,
		Bucket:    bkt,
		Region:    reg,
		AccessKey: ak,
		SecretKey: sk,
	}
}

func (p *R2Presigner) Enabled() bool {
	return p != nil && p.Endpoint != "" && p.Bucket != "" && p.AccessKey != "" && p.SecretKey != ""
}

// Presign signs an HTTP GET or PUT request for objectKey using AWS Signature Version 4.
func (p *R2Presigner) Presign(method, objectKey string, expires time.Duration) (string, error) {
	if !p.Enabled() {
		return "", errors.New("s3/r2 presigner not configured")
	}
	if expires <= 0 {
		expires = 15 * time.Minute
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	region := p.Region
	if region == "" {
		region = "auto"
	}

	parsed, err := url.Parse(p.Endpoint)
	if err != nil {
		return "", err
	}
	canonicalURI := "/" + p.Bucket + "/" + strings.TrimPrefix(objectKey, "/")
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)

	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", p.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")

	canonicalQuery := encodeAWSQuery(q)
	canonicalHeaders := "host:" + parsed.Host + "\n"
	canonicalRequest := strings.Join([]string{
		strings.ToUpper(method),
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")

	crHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(crHash[:]),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+p.SecretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	return fmt.Sprintf("%s%s?%s&X-Amz-Signature=%s", p.Endpoint, canonicalURI, canonicalQuery, sig), nil
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func encodeAWSQuery(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+strings.ReplaceAll(url.QueryEscape(v.Get(k)), "+", "%20"))
	}
	return strings.Join(parts, "&")
}
