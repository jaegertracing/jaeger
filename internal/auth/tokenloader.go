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
// disable Reloading by setting interval to 0
func cachedFileTokenLoader(path string, interval time.Duration, timeFn func() time.Time) func() (string, error) {
	var (
		mu          sync.Mutex
		cachedToken string
		lastRead    time.Time
	)

	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()

		now := timeFn()

		// Special case: interval = 0 means "never reload after first load"
		// Otherwise reload only if `interval` time has passed since last load.
		if !lastRead.IsZero() && (interval == 0 || now.Sub(lastRead) < interval) {
			return cachedToken, nil
		}

		// Read from file
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
	return TokenProviderWithTime(path, interval, logger, time.Now) // Use real time.Now in production
}

// TokenProviderWithTime creates a token provider with injectable time (for testing)
func TokenProviderWithTime(path string, interval time.Duration, logger *zap.Logger, timeFn func() time.Time) (func() string, error) {
	loader := cachedFileTokenLoader(path, interval, timeFn)

	// current token load
	currentToken, err := loader()
	if err != nil {
		return nil, fmt.Errorf("failed to get token from file: %w", err)
	}

	return func() string {
		newToken, err := loader()
		if err != nil {
			logger.Warn("Token reload failed", zap.Error(err))
			return currentToken
		}

		// save it in case the load fails later (e.g. if file is removed)
		currentToken = newToken
		return currentToken
	}, nil
}
