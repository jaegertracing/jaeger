// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"

	"go.opentelemetry.io/collector/extension/extensionauth"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

type Factory struct {
	config   config.Configuration
	telset   telemetry.Settings
	searcher esclient.Searcher
}

// NewFactory creates a new Factory with the given configuration and telemetry settings.
func NewFactory(
	ctx context.Context,
	cfg config.Configuration,
	telset telemetry.Settings,
	httpAuth extensionauth.HTTPClient,
) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client, err := esclient.NewClient(ctx, &cfg, telset.Logger, httpAuth)
	if err != nil {
		return nil, err
	}

	return &Factory{
		config:   cfg,
		telset:   telset,
		searcher: esclient.SearchClient{Client: client},
	}, nil
}

// CreateMetricsReader implements storage.MetricStoreFactory.
func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	spanRotation := indices.BuildRotation(
		f.config.Indices.IndexPrefix,
		config.SpanIndexName,
		f.config.ResolvedSpanRotation(),
		f.config.RemoteReadClusters,
		f.telset.Logger,
	)
	mr := NewMetricsReader(f.searcher, f.config, f.telset.Logger, f.telset.TracerProvider, spanRotation)
	return metricstoremetrics.NewReaderDecorator(mr, f.telset.Metrics), nil
}

// Close releases the factory's resources. The esclient transport holds no
// resources requiring explicit shutdown (matching the other ES storage factories),
// so this is a no-op kept to satisfy the closer contract.
func (*Factory) Close() error {
	return nil
}
