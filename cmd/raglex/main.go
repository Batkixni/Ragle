package main

import (
	"log"
	"os"
	"runtime/debug"

	"raglex/ui"
)

func main() {
	// Cap Go heap + runtime memory softly at 1.5 GiB by default (unless overridden
	// via GOMEMLIMIT) so batch RAW/image decoding returns memory to the OS promptly
	// instead of growing unboundedly.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(1536 << 20)
	}
	if err := ui.Run(); err != nil {
		log.Fatal(err)
	}
}
