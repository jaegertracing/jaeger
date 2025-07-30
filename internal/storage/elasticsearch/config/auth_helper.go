// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/auth/apikey"
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

func initBasicAuth(basicAuth *BasicAuthentication, logger *zap.Logger) (*auth.Method, error) {
	return initBasicAuthWithTime(basicAuth, logger, time.Now)
}

func initBasicAuthWithTime(basicAuth *BasicAuthentication, logger *zap.Logger, timeFn func() time.Time) (*auth.Method, error) {
	if basicAuth == nil {
		return nil, nil
	}

	if basicAuth.Password != "" && basicAuth.PasswordFilePath != "" {
		return nil, errors.New("both Password and PasswordFilePath are set")
	}

	username := basicAuth.Username
	if username == "" {
		return nil, nil
	}
	var tokenFn func() string

	// Handle password from file or static password
	if basicAuth.PasswordFilePath != "" {
		// Use TokenProvider for password loading
		passwordFn, err := auth.TokenProviderWithTime(basicAuth.PasswordFilePath, basicAuth.ReloadInterval, logger, timeFn)
		if err != nil {
			return nil, fmt.Errorf("failed to load password from file: %w", err)
		}

		// Pre-encode credentials in TokenFn
		tokenFn = func() string {
			password := passwordFn()
			if password == "" {
				return ""
			}
			credentials := username + ":" + password
			return base64.StdEncoding.EncodeToString([]byte(credentials))
		}
	} else {
		// Static password - pre-encode once
		password := basicAuth.Password
		credentials := username + ":" + password
		encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials))

		tokenFn = func() string { return encodedCredentials }
	}

	return &auth.Method{
		Scheme:  "Basic",
		TokenFn: tokenFn, // Returns base64-encoded credentials
	}, nil
}

func initAPIKeyAuth(apiKeyAuth *APIKeyAuthentication, logger *zap.Logger) (*auth.Method, error) {
	return initAPIKeyAuthWithTime(apiKeyAuth, logger, time.Now)
}

func initAPIKeyAuthWithTime(apiKeyAuth *APIKeyAuthentication, logger *zap.Logger, timeFn func() time.Time) (*auth.Method, error) {
	if apiKeyAuth == nil || (apiKeyAuth.FilePath == "" && !apiKeyAuth.AllowFromContext) {
		return nil, nil
	}

	if apiKeyAuth.FilePath != "" && apiKeyAuth.AllowFromContext {
		logger.Warn("Both API Key file and context propagation are enabled - context token will take precedence over file-based token")
	}

	var tokenFn func() string
	var fromCtx func(context.Context) (string, bool)

	// file-based token setup
	if apiKeyAuth.FilePath != "" {
		reloadInterval := apiKeyAuth.ReloadInterval
		tf, err := auth.TokenProviderWithTime(apiKeyAuth.FilePath, reloadInterval, logger, timeFn)
		if err != nil {
			return nil, err
		}
		tokenFn = tf
	}

	// context-based token setup
	if apiKeyAuth.AllowFromContext {
		fromCtx = apikey.GetAPIKey
	}

	// Return nil if no token source is available
	if tokenFn == nil && fromCtx == nil {
		return nil, nil
	}

	// Return pointer to the auth method
	return &auth.Method{
		Scheme:  "APIKey",
		TokenFn: tokenFn,
		FromCtx: fromCtx,
	}, nil
}
