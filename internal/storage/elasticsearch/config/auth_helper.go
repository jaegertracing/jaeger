// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
)

// initBearerAuth initializes bearer token authentication method
func initBearerAuth(bearerAuth *BearerTokenAuthentication, logger *zap.Logger) (*auth.Method, error) {
	return initBearerAuthWithTime(bearerAuth, logger, time.Now)
}

// initBearerAuthWithTime initializes bearer token authentication method with injectable time for testing
func initBearerAuthWithTime(bearerAuth *BearerTokenAuthentication, logger *zap.Logger, timeFn func() time.Time) (*auth.Method, error) {
	if bearerAuth == nil || (bearerAuth.FilePath == "" && !bearerAuth.AllowFromContext) {
		return nil, nil
	}

	if bearerAuth.FilePath != "" && bearerAuth.AllowFromContext {
		logger.Warn("Both Bearer Token file and context propagation are enabled - context token will take precedence over file-based token")
	}

	var tokenFn func() string
	var fromCtx func(context.Context) (string, bool)

	// file-based token setup
	if bearerAuth.FilePath != "" {
		reloadInterval := bearerAuth.ReloadInterval
		tf, err := auth.TokenProviderWithTime(bearerAuth.FilePath, reloadInterval, logger, timeFn)
		if err != nil {
			return nil, err
		}
		tokenFn = tf
	}

	// context-based token setup
	if bearerAuth.AllowFromContext {
		fromCtx = bearertoken.GetBearerToken
	}

	// Return pointer to the auth method
	return &auth.Method{
		Scheme:  "Bearer",
		TokenFn: tokenFn,
		FromCtx: fromCtx,
	}, nil
}
