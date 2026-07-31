// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap"

	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// NewESClient builds an Elasticsearch/OpenSearch admin client from the CLI
// config, threading auth (basic/bearer/API key) and TLS through the shared
// transport stack. Auth methods are assumed mutually exclusive; InitFromViper
// rejects ambiguous combinations.
func NewESClient(ctx context.Context, endpoint string, cfg *Config, logger *zap.Logger) (*esclient.Client, error) {
	esCfg := &escfg.Configuration{
		Servers:      []string{endpoint},
		QueryTimeout: time.Duration(cfg.MasterNodeTimeoutSeconds) * time.Second,
		TLS:          cfg.TLSConfig,
	}
	// Enable basic auth only when both are set, matching the prior behavior
	// of omitting the Authorization header unless username and password are present.
	if cfg.Username != "" && cfg.Password != "" {
		esCfg.Authentication.BasicAuthentication = configoptional.Some(escfg.BasicAuthentication{
			Username: cfg.Username,
			Password: cfg.Password,
		})
	}
	if cfg.TokenFilePath != "" {
		esCfg.Authentication.BearerTokenAuth = configoptional.Some(escfg.TokenAuthentication{
			FilePath: cfg.TokenFilePath,
		})
	}
	if cfg.APIKeyFilePath != "" {
		esCfg.Authentication.APIKeyAuth = configoptional.Some(escfg.TokenAuthentication{
			FilePath: cfg.APIKeyFilePath,
		})
	}
	return esclient.NewClient(ctx, esCfg, logger, nil)
}
