// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CachedFileTokenLoader returns a function that loads a token from the given file path,
// caches it for the specified interval, and reloads after the interval expires.
func cachedFileTokenLoader(path string, interval time.Duration) func() (string, error) {
	var (
		mu          sync.Mutex
		cachedToken string
		lastRead    time.Time
	)
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		if !lastRead.IsZero() && now.Sub(lastRead) < interval {
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
	loader := cachedFileTokenLoader(path, interval)

	// current token load
	currentToken, err := loader()
	if err != nil {
		return nil, fmt.Errorf("failed to get token from file: %w", err)
	}

	return func() string {
		newtoken, err := loader()
		if err != nil {
			logger.Warn("Token reload failed", zap.Error(err))
			return currentToken
		}
		// save it in case the load fails later (e.g. if file is removed)
		currentToken = newtoken
		return currentToken
	}, nil
}
