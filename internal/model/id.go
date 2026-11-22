package model

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// entropy is not safe for concurrent use, so NewID serializes it.
var (
	idMu      sync.Mutex
	idEntropy = ulid.Monotonic(rand.Reader, 0)
)

// NewID returns a ULID string. The value has no '/' so it can sit in a
// bbolt prefix key of the form parent/child.
func NewID() string {
	idMu.Lock()
	defer idMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), idEntropy).String()
}
