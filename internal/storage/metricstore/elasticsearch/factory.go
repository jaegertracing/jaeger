// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

type Factory struct {
	config config.Configuration
	telset telemetry.Settings
	client es.Client
}

// NewFactory creates a new Factory with the given configuration and telemetry settings.
func NewFactory(cfg config.Configuration, telset telemetry.Settings) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client, err := config.NewClient(&cfg, telset.Logger, telset.Metrics)
	if err != nil {
		return nil, err
	}

	return &Factory{
		config: cfg,
		telset: telset,
		client: client,
	}, nil
}

// CreateMetricsReader implements storage.MetricStoreFactory.
func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	mr := NewMetricsReader(f.client, f.telset.Logger, f.telset.TracerProvider)
	return metricstoremetrics.NewReaderDecorator(mr, f.telset.Metrics), nil
}

func (f *Factory) Close() error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}
