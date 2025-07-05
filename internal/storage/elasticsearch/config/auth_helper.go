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

// initAuthVars initializes tokenFn, scheme, fromCtxFn for authentication.
type AuthVars struct {
	TokenFn   func() string
	Scheme    string
	FromCtxFn func(context.Context) (string, bool)
}

// initAuthVars initializes tokenFn, scheme, fromCtxFn for authentication.
func initAuthVars(
	apiKeyAuth *APIKeyAuthentication,
	bearerAuth *BearerTokenAuthentication,
	logger *zap.Logger,
) (AuthVars, error) {
	if apiKeyAuth != nil && bearerAuth != nil {
		logger.Warn("Both API Key and Bearer Token authentication are configured. Priority order: (1) API Key will be used if available—first from context, then from file. If no API Key is found, Bearer Token will be used—first from context, then from file. Bearer Token authentication will be ignored if an API Key is present.")
	}

	// Strict priority: API Key first
	if apiKeyAuth != nil && (apiKeyAuth.FilePath != "" || apiKeyAuth.AllowFromContext) {
		if apiKeyAuth.FilePath != "" && apiKeyAuth.AllowFromContext {
			logger.Warn("Both API key file and context propagation are enabled - context token will take precedence over file-based token")
		}
		if apiKeyAuth.FilePath != "" {
			reloadInterval := 10 * time.Second
			if apiKeyAuth.ReloadInterval.HasValue() {
				reloadInterval = *apiKeyAuth.ReloadInterval.Get()
			}
			tokenFn, err := auth.TokenProvider(apiKeyAuth.FilePath, reloadInterval, logger)
			if err != nil {
				return AuthVars{}, err
			}
			av := AuthVars{
				TokenFn: tokenFn,
				Scheme:  "ApiKey",
			}
			if apiKeyAuth.AllowFromContext {
				av.FromCtxFn = apikey.GetAPIKey
			}
			return av, nil
		} else if apiKeyAuth.AllowFromContext {
			av := AuthVars{
				TokenFn:   func() string { return "" },
				Scheme:    "ApiKey",
				FromCtxFn: apikey.GetAPIKey,
			}
			return av, nil
		}
	}
	// Only check Bearer Token if API Key is not configured
	if bearerAuth != nil && (bearerAuth.FilePath != "" || bearerAuth.AllowFromContext) {
		if bearerAuth.FilePath != "" && bearerAuth.AllowFromContext {
			logger.Warn("Both Bearer Token file and context propagation are enabled - context token will take precedence over file-based token")
		}
		if bearerAuth.FilePath != "" {
			reloadInterval := 10 * time.Second
			if bearerAuth.ReloadInterval.HasValue() {
				reloadInterval = *bearerAuth.ReloadInterval.Get()
			}
			tokenFn, err := auth.TokenProvider(bearerAuth.FilePath, reloadInterval, logger)
			if err != nil {
				return AuthVars{}, err
			}
			av := AuthVars{
				TokenFn: tokenFn,
				Scheme:  "Bearer",
			}
			if bearerAuth.AllowFromContext {
				av.FromCtxFn = bearertoken.GetBearerToken
			}
			return av, nil
		} else if bearerAuth.AllowFromContext {
			av := AuthVars{
				TokenFn:   func() string { return "" },
				Scheme:    "Bearer",
				FromCtxFn: bearertoken.GetBearerToken,
			}
			return av, nil
		}
	}

	// Neither configured
	return AuthVars{}, nil
}
