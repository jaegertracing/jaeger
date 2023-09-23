// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
)

var _ extension.Extension = (*server)(nil)

type server struct {
	logger *zap.Logger
}

func newServer(config *Config, otel component.TelemetrySettings) *server {
	return &server{
		logger: otel.Logger,
	}
}

func (s *server) Start(ctx context.Context, host component.Host) error {
	// s.logger.Info("starting jaeger-query")
	return nil
}

func (s *server) Shutdown(ctx context.Context) error {
	return nil
}
