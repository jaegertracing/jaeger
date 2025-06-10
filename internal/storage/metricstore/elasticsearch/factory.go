// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

type Factory struct {
	config config.Configuration
	telset telemetry.Settings
}

// NewFactory creates a new Factory with the given configuration and telemetry settings.
func NewFactory(cfg config.Configuration, telset telemetry.Settings) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Factory{
		config: cfg,
		telset: telset,
	}, nil
}

// CreateMetricsReader implements storage.MetricStoreFactory.
func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	mr, _ := NewMetricsReader(f.config, f.telset.Logger, f.telset.TracerProvider)
	// Currently, the NewMetricsReader function does not return an error.
	// if err != nil {
	//	 return nil, err
	// }
	return metricstoremetrics.NewReaderDecorator(mr, f.telset.Metrics), nil
}
