// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

type server struct {
	config *Config
	server *app.Server
	telset component.TelemetrySettings
}

func newServer(config *Config, otel component.TelemetrySettings) *server {
	return &server{
		config: config,
		telset: otel,
	}
}

func (*server) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

func (s *server) Start(ctx context.Context, host component.Host) error {
	telset := telemetry.FromOtelComponent(s.telset, host)
	telset.Metrics = telset.Metrics.
		Namespace(metrics.NSOptions{Name: "jaeger"}).
		Namespace(metrics.NSOptions{Name: "remote_storage"})
	tf, err := jaegerstorage.GetTraceStoreFactory(s.config.Storage, host)
	if err != nil {
		return fmt.Errorf("cannot find factory for trace storage %s: %w", s.config.Storage, err)
	}

	df, ok := tf.(depstore.Factory)
	if !ok {
		return fmt.Errorf("cannot find factory for dependency storage %s: %w", s.config.Storage, err)
	}

	tm := tenancy.NewManager(&s.config.Tenancy)
	s.server, err = app.NewServer(ctx, s.config.ServerConfig, tf, df, tm, telset)
	if err != nil {
		return fmt.Errorf("could not create remote storage server: %w", err)
	}
	if err := s.server.Start(ctx); err != nil {
		return fmt.Errorf("could not start jaeger remote storage server: %w", err)
	}
	return nil
}

func (s *server) Shutdown(_ context.Context) error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}
