// Command ragle-server runs Ragle's standalone headless server for VPS hosting.
// It serves the Web Gallery & Board Viewer, Companion REST API, and embedded
// WebRTC pairing signaling from a single zero-GUI binary.
//
// Usage:
//
//	ragle-server -addr :8787 -library /path/to/library -token your-secret
//	ragle-server -addr :8787   # signaling-only mode when -library is omitted
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

	"ragle/internal/library"
	"ragle/internal/mcp"
	"ragle/internal/project"
	"ragle/internal/server"
)

func main() {
	addr := flag.String("addr", ":8787", "TCP listen address (e.g. :8787 or 127.0.0.1:41595)")
	libPath := flag.String("library", "", "Path to a Ragle library folder (omit for signaling-only mode)")
	token := flag.String("token", os.Getenv("RAGLE_SERVER_TOKEN"), "API bearer/X-Ragle-Token secret for write endpoints")
	ttl := flag.Duration("signal-ttl", 5*time.Minute, "WebRTC pairing code expiration TTL")
	flag.Parse()

	var lib *library.Library
	var projs *project.Store
	if *libPath != "" {
		var err error
		lib, err = library.Open(*libPath)
		if err != nil {
			log.Fatalf("open library %s: %v", *libPath, err)
		}
		defer lib.Close()
		projs = project.New(lib.DB)
		log.Printf("mounted library: %s", *libPath)
	} else {
		log.Printf("running in signaling-only mode (pass -library to mount a library)")
	}

	tok := *token
	if tok == "" && lib != nil {
		tok = server.GenerateToken()
		log.Printf("generated ephemeral API token: %s", tok)
	}

	var mcpSrv *mcp.Server
	if lib != nil {
		mcpSrv = mcp.New(mcp.Options{Library: lib, Projects: projs})
	}
	srv := server.New(server.Options{
		Library:   lib,
		Projects:  projs,
		Token:     tok,
		SignalTTL: *ttl,
		ExtraRoutes: func(mux *http.ServeMux) {
			if mcpSrv != nil {
				mux.Handle("/mcp", mcpSrv)
			}
		},
	})
	bound, err := srv.Start(*addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	log.Printf("ragle-server listening on http://%s (web gallery + API + MCP + signaling)", bound)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Stop(ctx)
	log.Printf("ragle-server stopped")
}
