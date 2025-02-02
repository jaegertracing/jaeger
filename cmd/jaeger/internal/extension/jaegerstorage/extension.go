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

	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v1/es"
	"github.com/jaegertracing/jaeger/internal/storage/v1/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/plugin/metricstore/prometheus"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

var _ Extension = (*storageExt)(nil)

type Extension interface {
	extension.Extension
	TraceStorageFactory(name string) (storage.Factory, bool)
	MetricStorageFactory(name string) (storage.MetricStoreFactory, bool)
}

type storageExt struct {
	config           *Config
	telset           component.TelemetrySettings
	factories        map[string]storage.Factory
	metricsFactories map[string]storage.MetricStoreFactory
}

// GetStorageFactory locates the extension in Host and retrieves
// a trace storage factory from it with the given name.
func GetStorageFactory(name string, host component.Host) (storage.Factory, error) {
	ext, err := findExtension(host)
	if err != nil {
		return nil, err
	}
	f, ok := ext.TraceStorageFactory(name)
	if !ok {
		return nil, fmt.Errorf(
			"cannot find definition of storage '%s' in the configuration for extension '%s'",
			name, componentType,
		)
	}
	return f, nil
}

// GetMetricStorageFactory locates the extension in Host and retrieves
// a metric storage factory from it with the given name.
func GetMetricStorageFactory(name string, host component.Host) (storage.MetricStoreFactory, error) {
	ext, err := findExtension(host)
	if err != nil {
		return nil, err
	}
	mf, ok := ext.MetricStorageFactory(name)
	if !ok {
		return nil, fmt.Errorf(
			"cannot find metric storage '%s' declared by '%s' extension",
			name, componentType,
		)
	}
	return mf, nil
}

func GetTraceStoreFactory(name string, host component.Host) (tracestore.Factory, error) {
	f, err := GetStorageFactory(name, host)
	if err != nil {
		return nil, err
	}

	return v1adapter.NewFactory(f), nil
}

func findExtension(host component.Host) (Extension, error) {
	var id component.ID
	var comp component.Component
	for i, ext := range host.GetExtensions() {
		if i.Type() == componentType {
			id, comp = i, ext
			break
		}
	}
	if comp == nil {
		return nil, fmt.Errorf(
			"cannot find extension '%s' (make sure it's defined earlier in the config)",
			componentType,
		)
	}
	ext, ok := comp.(Extension)
	if !ok {
		return nil, fmt.Errorf("extension '%s' is not of expected type '%s'", id, componentType)
	}
	return ext, nil
}

func newStorageExt(config *Config, telset component.TelemetrySettings) *storageExt {
	return &storageExt{
		config:           config,
		telset:           telset,
		factories:        make(map[string]storage.Factory),
		metricsFactories: make(map[string]storage.MetricStoreFactory),
	}
}

func (s *storageExt) Start(_ context.Context, host component.Host) error {
	telset := telemetry.FromOtelComponent(s.telset, host)
	telset.Metrics = telset.Metrics.Namespace(metrics.NSOptions{Name: "jaeger"})
	scopedMetricsFactory := func(name, kind, role string) metrics.Factory {
		return telset.Metrics.Namespace(metrics.NSOptions{
			Name: "storage",
			Tags: map[string]string{
				"name": name,
				"kind": kind,
				"role": role,
			},
		})
	}
	for storageName, cfg := range s.config.TraceBackends {
		s.telset.Logger.Sugar().Infof("Initializing storage '%s'", storageName)
		var factory storage.Factory
		var err error = errors.New("empty configuration")
		switch {
		case cfg.Memory != nil:
			factory, err = memory.NewFactoryWithConfig(
				*cfg.Memory,
				scopedMetricsFactory(storageName, "memory", "tracestore"),
				s.telset.Logger,
			), nil
		case cfg.Badger != nil:
			factory, err = badger.NewFactoryWithConfig(
				*cfg.Badger,
				scopedMetricsFactory(storageName, "badger", "tracestore"),
				s.telset.Logger)
		case cfg.GRPC != nil:
			grpcTelset := telset
			grpcTelset.Metrics = scopedMetricsFactory(storageName, "grpc", "tracestore")
			//nolint: contextcheck
			factory, err = grpc.NewFactoryWithConfig(*cfg.GRPC, grpcTelset)
		case cfg.Cassandra != nil:
			factory, err = cassandra.NewFactoryWithConfig(
				*cfg.Cassandra,
				scopedMetricsFactory(storageName, "cassandra", "tracestore"),
				s.telset.Logger,
			)
		case cfg.Elasticsearch != nil:
			factory, err = es.NewFactoryWithConfig(
				*cfg.Elasticsearch,
				scopedMetricsFactory(storageName, "elasticsearch", "tracestore"),
				s.telset.Logger,
			)
		case cfg.Opensearch != nil:
			factory, err = es.NewFactoryWithConfig(
				*cfg.Opensearch,
				scopedMetricsFactory(storageName, "opensearch", "tracestore"),
				s.telset.Logger,
			)
		}
		if err != nil {
			return fmt.Errorf("failed to initialize storage '%s': %w", storageName, err)
		}
		s.factories[storageName] = factory
	}

	for metricStorageName, cfg := range s.config.MetricBackends {
		s.telset.Logger.Sugar().Infof("Initializing metrics storage '%s'", metricStorageName)
		var metricsFactory storage.MetricStoreFactory
		var err error
		if cfg.Prometheus != nil {
			promTelset := telset
			promTelset.Metrics = scopedMetricsFactory(metricStorageName, "prometheus", "metricstore")
			metricsFactory, err = prometheus.NewFactoryWithConfig(
				*cfg.Prometheus,
				promTelset)
		}
		if err != nil {
			return fmt.Errorf("failed to initialize metrics storage '%s': %w", metricStorageName, err)
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

func (s *storageExt) TraceStorageFactory(name string) (storage.Factory, bool) {
	f, ok := s.factories[name]
	return f, ok
}

func (s *storageExt) MetricStorageFactory(name string) (storage.MetricStoreFactory, bool) {
	mf, ok := s.metricsFactories[name]
	return mf, ok
}
