// Command raglex-mcp runs Raglex's Model Context Protocol (MCP) server over Stdio
// for AI clients such as Claude Desktop, Antigravity, and Cursor.
//
// Usage:
//
//	raglex-mcp -library /path/to/library
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"raglex/internal/config"
	"raglex/internal/library"
	"raglex/internal/mcp"
	"raglex/internal/project"
)

func main() {
	libPath := flag.String("library", "", "Path to a Raglex library folder (defaults to config.json library_path)")
	layaURL := flag.String("laya-url", "", "Local Ollaya/Laya endpoint URL")
	layaMod := flag.String("laya-model", "", "Laya decision model name")
	flag.Parse()

	cfg, _ := config.Load()
	path := *libPath
	if path == "" {
		path = cfg.LibraryPath
	}
	if path == "" {
		log.Fatalf("raglex-mcp: pass -library <path> or open a library in Raglex first")
	}
	url := *layaURL
	if url == "" {
		url = cfg.LayaURL
	}
	mod := *layaMod
	if mod == "" {
		mod = cfg.LayaModel
	}

	lib, err := library.Open(path)
	if err != nil {
		log.Fatalf("raglex-mcp: open library %s: %v", path, err)
	}
	defer lib.Close()

	srv := mcp.New(mcp.Options{
		Library:   lib,
		Projects:  project.New(lib.DB),
		LayaURL:   url,
		LayaModel: mod,
	})
	if err := srv.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatalf("raglex-mcp: %v", err)
	}
}
