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
	if apiKeyAuth != nil && (apiKeyAuth.FilePath.Get() != nil && *apiKeyAuth.FilePath.Get() != "" || apiKeyAuth.AllowFromContext.Get() != nil && *apiKeyAuth.AllowFromContext.Get()) {
		if apiKeyAuth.FilePath.Get() != nil && *apiKeyAuth.FilePath.Get() != "" && apiKeyAuth.AllowFromContext.Get() != nil && *apiKeyAuth.AllowFromContext.Get() {
			logger.Warn("Both API key file and context propagation are enabled - context token will take precedence over file-based token")
		}
		if filePath := apiKeyAuth.FilePath.Get(); filePath != nil && *filePath != "" {
			reloadInterval := 10 * time.Second
			if reload := apiKeyAuth.ReloadInterval.Get(); reload != nil {
				reloadInterval = *reload
			}
			tokenFn, err := auth.TokenProvider(*filePath, reloadInterval, logger)
			if err != nil {
				return AuthVars{}, err
			}
			av := AuthVars{
				TokenFn: tokenFn,
				Scheme:  "ApiKey",
			}
			if allow := apiKeyAuth.AllowFromContext.Get(); allow != nil && *allow {
				av.FromCtxFn = apikey.GetAPIKey
			}
			return av, nil
		} else if allow := apiKeyAuth.AllowFromContext.Get(); allow != nil && *allow {
			av := AuthVars{
				TokenFn:   func() string { return "" },
				Scheme:    "ApiKey",
				FromCtxFn: apikey.GetAPIKey,
			}
			return av, nil
		}
	}
	// Only check Bearer Token if API Key is not configured
	if bearerAuth != nil && (bearerAuth.FilePath.Get() != nil && *bearerAuth.FilePath.Get() != "" || bearerAuth.AllowFromContext.Get() != nil && *bearerAuth.AllowFromContext.Get()) {
		if bearerAuth.FilePath.Get() != nil && *bearerAuth.FilePath.Get() != "" && bearerAuth.AllowFromContext.Get() != nil && *bearerAuth.AllowFromContext.Get() {
			logger.Warn("Both Bearer Token file and context propagation are enabled - context token will take precedence over file-based token")
		}
		if filePath := bearerAuth.FilePath.Get(); filePath != nil && *filePath != "" {
			reloadInterval := 10 * time.Second
			if reload := bearerAuth.ReloadInterval.Get(); reload != nil {
				reloadInterval = *reload
			}
			tokenFn, err := auth.TokenProvider(*filePath, reloadInterval, logger)
			if err != nil {
				return AuthVars{}, err
			}
			av := AuthVars{
				TokenFn: tokenFn,
				Scheme:  "Bearer",
			}
			if allow := bearerAuth.AllowFromContext.Get(); allow != nil && *allow {
				av.FromCtxFn = bearertoken.GetBearerToken
			}
			return av, nil
		} else if allow := bearerAuth.AllowFromContext.Get(); allow != nil && *allow {
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
