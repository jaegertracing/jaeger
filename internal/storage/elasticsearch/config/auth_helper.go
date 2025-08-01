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

// initTokenAuthWithTime initializes token authentication injectable time for testing
func initTokenAuthWithTime(tokenAuth *TokenAuthentication, scheme string, logger *zap.Logger, timeFn func() time.Time) (*auth.Method, error) {
	if tokenAuth == nil || (tokenAuth.FilePath == "" && !tokenAuth.AllowFromContext) {
		return nil, nil
	}

	if tokenAuth.FilePath != "" && tokenAuth.AllowFromContext {
		logger.Warn("Both token file and context propagation are enabled - context token will take precedence over file-based token",
			zap.String("auth_scheme", scheme))
	}

	var tokenFn func() string
	var fromCtx func(context.Context) (string, bool)

	// File-based token setup
	if tokenAuth.FilePath != "" {
		tf, err := auth.TokenProviderWithTime(tokenAuth.FilePath, tokenAuth.ReloadInterval, logger, timeFn)
		if err != nil {
			return nil, err
		}
		tokenFn = tf
	}

	// Context-based token setup
	if tokenAuth.AllowFromContext {
		if scheme == "Bearer" {
			fromCtx = bearertoken.GetBearerToken
		} else if scheme == "APIKey" {
			fromCtx = apikey.GetAPIKey
		}
	}

	return &auth.Method{
		Scheme:  scheme,
		TokenFn: tokenFn,
		FromCtx: fromCtx,
	}, nil
}

// Simplified init functions - directly call shared implementation
func initBearerAuth(tokenAuth *TokenAuthentication, logger *zap.Logger) (*auth.Method, error) {
	if tokenAuth == nil {
		return nil, nil
	}	
	return initTokenAuthWithTime(tokenAuth, "Bearer", logger, time.Now)
}

func initAPIKeyAuth(tokenAuth *TokenAuthentication, logger *zap.Logger) (*auth.Method, error) {
	if tokenAuth == nil {
		return nil, nil
	}
	return initTokenAuthWithTime(tokenAuth, "APIKey", logger, time.Now)
}

// Keep initBasicAuth unchanged
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
