// Command raglex-signal runs a lightweight, self-hostable signaling server
// for Raglex WebRTC pairing-code transfers.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"raglex/internal/share"
)

func main() {
	addr := flag.String("addr", ":8787", "listen address")
	ttl := flag.Duration("ttl", 5*time.Minute, "pairing code lifetime")
	flag.Parse()

	sig := share.NewSignalingServer(*ttl)
	defer sig.Close()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           sig,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("raglex-signal listening on %s (room TTL %s)", *addr, *ttl)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
