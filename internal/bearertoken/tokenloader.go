// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
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

// TokenProvider creates a token provider that handles file loading and error handling consistently.
func TokenProvider(path string, interval time.Duration, logger *zap.Logger) (func() string, error) {
	loader := CachedFileTokenLoader(path, interval)

	// Initial token load
	t, err := loader()
	if err != nil || t == "" {
		return nil, fmt.Errorf("failed to get token from file: %w", err)
	}

	initialToken := t

	return func() string {
		t, err := loader()
		if err != nil {
			if logger != nil {
				logger.Warn("Token reload failed", zap.Error(err))
			} else {
				log.Printf("Token reload failed: %v", err)
			}
			return initialToken
		}
		initialToken = t
		return t
	}, nil
}
