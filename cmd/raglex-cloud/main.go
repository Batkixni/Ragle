// Command raglex-cloud runs the multi-tenant Raglex Cloud service (Account Auth,
// per-user Sync API v1, SHA-256 deduplicated blob store, and WebRTC signaling).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"raglex/internal/cloud"
)

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}

func main() {
	addr := flag.String("addr", envOr("RAGLEX_CLOUD_ADDR", ":8787"), "HTTP listen address")
	dataDir := flag.String("data", envOr("RAGLEX_CLOUD_DATA", "/data"), "Local NVMe directory for accounts & sync databases")
	blobDir := flag.String("blobs", envOr("RAGLEX_CLOUD_BLOBS", "/blobs"), "Blob storage directory (e.g. Hetzner Storage Box mount)")
	publicHost := flag.String("host", envOr("RAGLEX_CLOUD_HOST", "cloud.raglex.sorai.tw"), "Public hostname")
	quotaGB := flag.Int64("quota-gb", envInt64("RAGLEX_CLOUD_QUOTA_GB", 10), "Per-user storage quota in GiB (0 = unlimited)")
	allowReg := flag.Bool("allow-register", envBool("RAGLEX_CLOUD_ALLOW_REGISTER", true), "Allow account registration")
	inviteOnly := flag.Bool("invite-only", envBool("RAGLEX_CLOUD_INVITE_ONLY", true), "Require a valid invite code to register")
	adminToken := flag.String("admin-token", envOr("RAGLEX_CLOUD_ADMIN_TOKEN", ""), "Bearer token for /api/v1/admin/* invite management")
	openmailKey := flag.String("openmail-key", envOr("OPENMAIL_API_KEY", ""), "OpenMail API key for transactional account emails")
	openmailInbox := flag.String("openmail-inbox", envOr("OPENMAIL_INBOX_ID", ""), "OpenMail inbox ID (auto-resolved from OPENMAIL_FROM if empty)")
	openmailFrom := flag.String("openmail-from", envOr("OPENMAIL_FROM", "raglex@omail.sh"), "OpenMail sender address")
	flag.Parse()

	var quotaBytes int64
	if *quotaGB > 0 {
		quotaBytes = *quotaGB << 30
	}

	var mailer cloud.Mailer
	if strings.TrimSpace(*openmailKey) != "" {
		mailer = cloud.NewOpenMailClient(*openmailKey, *openmailInbox, *openmailFrom)
	}

	srv, err := cloud.Open(cloud.Options{
		DataDir:       *dataDir,
		BlobDir:       *blobDir,
		PublicHost:    *publicHost,
		QuotaBytes:    quotaBytes,
		AllowRegister: *allowReg,
		InviteOnly:    *inviteOnly,
		AdminToken:    *adminToken,
		Mailer:        mailer,
		SignalTTL:     5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("raglex-cloud open failed: %v", err)
	}
	defer srv.Close()

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("raglex-cloud listening on %s (host=%s data=%s blobs=%s quota=%dGiB invite_only=%v mailer=%v)",
			*addr, *publicHost, *dataDir, *blobDir, *quotaGB, *inviteOnly, mailer != nil && mailer.Enabled())
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
