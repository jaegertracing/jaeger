// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CachedFileTokenLoader returns a function that loads a token from the given file path,
// caches it for the specified interval, and reloads after the interval expires.
func CachedFileTokenLoader(path string, interval time.Duration) func() (string, error) {
	var (
		mu          sync.Mutex
		cachedToken string
		lastRead    time.Time
	)
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		if cachedToken != "" && now.Sub(lastRead) < interval {
			return cachedToken, nil
		}
		b, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return "", fmt.Errorf("failed to read token file: %w", err)
		}
		cachedToken = strings.TrimRight(string(b), "\r\n")
		lastRead = now
		return cachedToken, nil
	}
}
