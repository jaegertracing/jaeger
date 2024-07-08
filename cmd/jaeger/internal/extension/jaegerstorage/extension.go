// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage/factoryadapter"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

var _ Extension = (*storageExt)(nil)

type Extension interface {
	extension.Extension
	Factory(name string) (storage.Factory, bool)
}

type storageExt struct {
	config           *Config
	telset           component.TelemetrySettings
	factories        map[string]storage.Factory
	metricsFactories map[string]storage.MetricsFactory
}

// GetStorageFactory locates the extension in Host and retrieves a storage factory from it with the given name.
func GetStorageFactory(name string, host component.Host) (storage.Factory, error) {
	var comp component.Component
	for id, ext := range host.GetExtensions() {
		if id.Type() == componentType {
			comp = ext
			break
		}
	}
	if comp == nil {
		return nil, fmt.Errorf(
			"cannot find extension '%s' (make sure it's defined earlier in the config)",
			componentType,
		)
	}
	f, ok := comp.(Extension).Factory(name)
	if !ok {
		return nil, fmt.Errorf(
			"cannot find storage '%s' declared by '%s' extension",
			name, componentType,
		)
	}
	return f, nil
}

func GetStorageFactoryV2(name string, host component.Host) (spanstore.Factory, error) {
	f, err := GetStorageFactory(name, host)
	if err != nil {
		return nil, err
	}

	return factoryadapter.NewFactory(f), nil
}

func newStorageExt(config *Config, telset component.TelemetrySettings) *storageExt {
	return &storageExt{
		config:    config,
		telset:    telset,
		factories: make(map[string]storage.Factory),
	}
}

func (s *storageExt) Start(_ context.Context, _ component.Host) error {
	mf := otelmetrics.NewFactory(s.telset.MeterProvider)
	for storageName, cfg := range s.config.Backends {
		s.telset.Logger.Sugar().Infof("Initializing storage '%s'", storageName)
		var factory storage.Factory
		var err error = errors.New("empty configuration")
		switch {
		case cfg.Memory != nil:
			factory, err = memory.NewFactoryWithConfig(*cfg.Memory, mf, s.telset.Logger), nil
		case cfg.Badger != nil:
			factory, err = badger.NewFactoryWithConfig(*cfg.Badger, mf, s.telset.Logger)
		case cfg.GRPC != nil:
			//nolint: contextcheck
			factory, err = grpc.NewFactoryWithConfig(*cfg.GRPC, mf, s.telset.Logger)
		case cfg.Cassandra != nil:
			factory, err = cassandra.NewFactoryWithConfig(*cfg.Cassandra, mf, s.telset.Logger)
		case cfg.Elasticsearch != nil:
			factory, err = es.NewFactoryWithConfig(*cfg.Elasticsearch, mf, s.telset.Logger)
		case cfg.Opensearch != nil:
			factory, err = es.NewFactoryWithConfig(*cfg.Opensearch, mf, s.telset.Logger)
		}
		if err != nil {
			return fmt.Errorf("failed to initialize storage '%s': %w", storageName, err)
		}
		s.factories[storageName] = factory
	}

	for metricStorageName, cfg := range s.config.MetricBackends {
		s.telset.Logger.Sugar().Infof("Initializing metrics storage '%s'", metricStorageName)
		var metricsFactory storage.MetricsFactory
		var err error = errors.New("empty configuration")
		if cfg.Prometheus != nil {
			// metricsFactory, err = prometheus.NewFactoryWithConfig(*cfg.Prometheus, mf, s.telset.Logger) // TODO: Need to implement NewFactoryWithConfig)
		}
		if err != nil {
			return fmt.Errorf("failed to initialize storage '%s': %w", metricStorageName, err)
		}
		s.metricsFactories[metricStorageName] = metricsFactory
	}

	return nil
}

func (s *storageExt) Shutdown(context.Context) error {
	var errs []error
	for _, factory := range s.factories {
		if closer, ok := factory.(io.Closer); ok {
			err := closer.Close()
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (s *storageExt) Factory(name string) (storage.Factory, bool) {
	f, ok := s.factories[name]
	return f, ok
}
