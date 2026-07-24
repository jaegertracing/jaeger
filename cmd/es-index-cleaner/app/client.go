// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"strings"
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
	// govalidator's url validation rejects 0.0.0.0, replace it with 127.0.0.1 for tests
	endpoint = strings.Replace(endpoint, "://0.0.0.0:", "://127.0.0.1:", 1)

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
	if err := esCfg.Validate(); err != nil {
		return nil, err
	}
	return esclient.NewClient(ctx, esCfg, logger, nil)
}
