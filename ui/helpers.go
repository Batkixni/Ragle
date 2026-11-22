package ui

import (
	"context"
	"strings"
)

func trim(s string) string { return strings.TrimSpace(s) }

func nilContext() context.Context { return context.Background() }
