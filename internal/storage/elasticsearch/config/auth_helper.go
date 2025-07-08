// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/auth/apikey"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
)

// AuthVars holds authentication configuration
type AuthVars struct {
	// AuthConfigs contains all configured authentication methods
	AuthConfigs []auth.Config
}

// initAuthVars initializes tokenFn, scheme, fromCtxFn for authentication.
func initAuthVars(
	apiKeyAuth *APIKeyAuthentication,
	bearerAuth *BearerTokenAuthentication,
	logger *zap.Logger,
) (AuthVars, error) {
	if apiKeyAuth != nil && bearerAuth != nil {
		logger.Warn("Both API Key and Bearer Token authentication are configured. The client will attempt to use both methods, prioritizing tokens from the context over files.")
	}

	var authVars AuthVars

	// Configure API Key auth if enabled
	if apiKeyAuth != nil && (apiKeyAuth.FilePath != "" || apiKeyAuth.AllowFromContext) {
		if apiKeyAuth.FilePath != "" && apiKeyAuth.AllowFromContext {
			logger.Warn("Both API key file and context propagation are enabled - context token will take precedence over file-based token")
		}
		var tokenFn func() string
		var fromCtx func(context.Context) (string, bool)

		if apiKeyAuth.FilePath != "" {
			reloadInterval := 10 * time.Second
			if apiKeyAuth.ReloadInterval.HasValue() {
				reloadInterval = *apiKeyAuth.ReloadInterval.Get()
			}
			tf, err := auth.TokenProvider(apiKeyAuth.FilePath, reloadInterval, logger)
			if err != nil {
				return AuthVars{}, err
			}
			tokenFn = tf
		}

		if apiKeyAuth.AllowFromContext {
			fromCtx = apikey.GetAPIKey
		}

		authVars.AuthConfigs = append(authVars.AuthConfigs, auth.Config{
			Scheme:  "ApiKey",
			TokenFn: tokenFn,
			FromCtx: fromCtx,
		})
	}

	// Configure Bearer Token auth if enabled
	if bearerAuth != nil && (bearerAuth.FilePath != "" || bearerAuth.AllowFromContext) {
		if bearerAuth.FilePath != "" && bearerAuth.AllowFromContext {
			logger.Warn("Both Bearer Token file and context propagation are enabled - context token will take precedence over file-based token")
		}
		var tokenFn func() string
		var fromCtx func(context.Context) (string, bool)

		if bearerAuth.FilePath != "" {
			reloadInterval := 10 * time.Second
			if bearerAuth.ReloadInterval.HasValue() {
				reloadInterval = *bearerAuth.ReloadInterval.Get()
			}
			tf, err := auth.TokenProvider(bearerAuth.FilePath, reloadInterval, logger)
			if err != nil {
				return AuthVars{}, err
			}
			tokenFn = tf
		}

		if bearerAuth.AllowFromContext {
			fromCtx = bearertoken.GetBearerToken
		}

		authVars.AuthConfigs = append(authVars.AuthConfigs, auth.Config{
			Scheme:  "Bearer",
			TokenFn: tokenFn,
			FromCtx: fromCtx,
		})
	}

	return authVars, nil
}
