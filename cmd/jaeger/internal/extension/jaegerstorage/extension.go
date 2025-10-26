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
	"go.opentelemetry.io/collector/extension/extensionauth"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	esmetrics "github.com/jaegertracing/jaeger/internal/storage/metricstore/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	es "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var _ Extension = (*storageExt)(nil)

type Extension interface {
	extension.Extension
	TraceStorageFactory(name string) (tracestore.Factory, bool)
	MetricStorageFactory(name string) (storage.MetricStoreFactory, bool)
}

type storageExt struct {
	config           *Config
	telset           component.TelemetrySettings
	factories        map[string]tracestore.Factory
	metricsFactories map[string]storage.MetricStoreFactory
}

// getStorageFactory locates the extension in Host and retrieves
// a trace storage factory from it with the given name.
func getStorageFactory(name string, host component.Host) (tracestore.Factory, error) {
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
	f, err := getStorageFactory(name, host)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func GetSamplingStoreFactory(name string, host component.Host) (storage.SamplingStoreFactory, error) {
	f, err := getStorageFactory(name, host)
	if err != nil {
		return nil, err
	}

	ssf, ok := f.(storage.SamplingStoreFactory)
	if !ok {
		return nil, fmt.Errorf("storage '%s' does not support sampling store", name)
	}

	return ssf, nil
}

func GetPurger(name string, host component.Host) (storage.Purger, error) {
	f, err := getStorageFactory(name, host)
	if err != nil {
		return nil, err
	}

	purger, ok := f.(storage.Purger)
	if !ok {
		return nil, fmt.Errorf("storage '%s' does not support purging", name)
	}

	return purger, nil
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

func newStorageExt(cfg *Config, telset component.TelemetrySettings) *storageExt {
	return &storageExt{
		config:           cfg,
		telset:           telset,
		factories:        make(map[string]tracestore.Factory),
		metricsFactories: make(map[string]storage.MetricStoreFactory),
	}
}

func (s *storageExt) Start(ctx context.Context, host component.Host) error {
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
		var factory tracestore.Factory
		err := errors.New("empty configuration")
		switch {
		case cfg.Memory != nil:
			memTelset := telset
			memTelset.Metrics = scopedMetricsFactory(storageName, "memory", "tracestore")
			factory, err = memory.NewFactory(*cfg.Memory, memTelset)
		case cfg.Badger != nil:
			factory, err = badger.NewFactory(
				*cfg.Badger,
				scopedMetricsFactory(storageName, "badger", "tracestore"),
				s.telset.Logger)
		case cfg.GRPC != nil:
			grpcTelset := telset
			grpcTelset.Metrics = scopedMetricsFactory(storageName, "grpc", "tracestore")
			factory, err = grpc.NewFactory(ctx, *cfg.GRPC, grpcTelset)
		case cfg.Cassandra != nil:
			factory, err = cassandra.NewFactory(
				*cfg.Cassandra,
				scopedMetricsFactory(storageName, "cassandra", "tracestore"),
				s.telset.Logger,
			)
		case cfg.Elasticsearch != nil:
			esTelset := telset
			esTelset.Metrics = scopedMetricsFactory(storageName, "elasticsearch", "tracestore")
			httpAuth, authErr := s.resolveAuthenticator(host, cfg.Elasticsearch.Authentication, "elasticsearch", storageName)
			if authErr != nil {
				return authErr
			}
			factory, err = es.NewFactory(ctx, *cfg.Elasticsearch, esTelset, httpAuth)

		case cfg.Opensearch != nil:
			osTelset := telset
			osTelset.Metrics = scopedMetricsFactory(storageName, "opensearch", "tracestore")
			httpAuth, authErr := s.resolveAuthenticator(host, cfg.Opensearch.Authentication, "opensearch", storageName)
			if authErr != nil {
				return authErr
			}
			factory, err = es.NewFactory(ctx, *cfg.Opensearch, osTelset, httpAuth)

		case cfg.ClickHouse != nil:
			chTelset := telset
			chTelset.Metrics = scopedMetricsFactory(storageName, "clickhouse", "tracestore")
			factory, err = clickhouse.NewFactory(
				ctx,
				*cfg.ClickHouse,
				chTelset,
			)
		default:
			// default case
		}
		if err != nil {
			return fmt.Errorf("failed to initialize storage '%s': %w", storageName, err)
		}

		s.factories[storageName] = factory
	}

	for metricStorageName, cfg := range s.config.MetricBackends {
		s.telset.Logger.Sugar().Infof("Initializing metrics storage '%s'", metricStorageName)
		var metricStoreFactory storage.MetricStoreFactory
		var err error
		switch {
		case cfg.Prometheus != nil:
			promTelset := telset
			promTelset.Metrics = scopedMetricsFactory(metricStorageName, "prometheus", "metricstore")

			var promAuth config.Authentication
			if cfg.Prometheus.Auth != nil && cfg.Prometheus.Auth.Authenticator != "" {
				promAuth.AuthenticatorID = component.MustNewID(cfg.Prometheus.Auth.Authenticator)
			}
			httpAuth, authErr := s.resolveAuthenticator(host, promAuth, "prometheus metrics", metricStorageName)
			if authErr != nil {
				return authErr
			}

			metricStoreFactory, err = prometheus.NewFactoryWithConfig(
				cfg.Prometheus.Configuration,
				promTelset,
				httpAuth,
			)
			if err != nil {
				return fmt.Errorf("failed to initialize metrics storage '%s': %w", metricStorageName, err)
			}

		case cfg.Elasticsearch != nil:
			esTelset := telset
			esTelset.Metrics = scopedMetricsFactory(metricStorageName, "elasticsearch", "metricstore")
			httpAuth, authErr := s.resolveAuthenticator(host, cfg.Elasticsearch.Authentication, "elasticsearch metrics", metricStorageName)
			if authErr != nil {
				return authErr
			}
			metricStoreFactory, err = esmetrics.NewFactory(ctx, *cfg.Elasticsearch, esTelset, httpAuth)

		case cfg.Opensearch != nil:
			osTelset := telset
			osTelset.Metrics = scopedMetricsFactory(metricStorageName, "opensearch", "metricstore")
			httpAuth, authErr := s.resolveAuthenticator(host, cfg.Opensearch.Authentication, "opensearch metrics", metricStorageName)
			if authErr != nil {
				return authErr
			}
			metricStoreFactory, err = esmetrics.NewFactory(ctx, *cfg.Opensearch, osTelset, httpAuth)

		default:
			err = fmt.Errorf("no metric backend configuration provided for '%s'", metricStorageName)
		}
		if err != nil {
			return fmt.Errorf("failed to initialize metrics storage '%s': %w", metricStorageName, err)
		}
		s.metricsFactories[metricStorageName] = metricStoreFactory
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
	for _, metricfactory := range s.metricsFactories {
		if closer, ok := metricfactory.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (s *storageExt) TraceStorageFactory(name string) (tracestore.Factory, bool) {
	f, ok := s.factories[name]
	return f, ok
}

func (s *storageExt) MetricStorageFactory(name string) (storage.MetricStoreFactory, bool) {
	mf, ok := s.metricsFactories[name]
	return mf, ok
}

// getAuthenticator retrieves an HTTP authenticator extension from the host by name.
func (*storageExt) getAuthenticator(host component.Host, authenticatorName string) (extensionauth.HTTPClient, error) {
	if authenticatorName == "" {
		return nil, nil
	}

	for id, ext := range host.GetExtensions() {
		if id.String() == authenticatorName || id.Name() == authenticatorName {
			if httpAuth, ok := ext.(extensionauth.HTTPClient); ok {
				return httpAuth, nil
			}
			return nil, fmt.Errorf("extension '%s' does not implement extensionauth.HTTPClient", authenticatorName)
		}
	}
	return nil, fmt.Errorf("authenticator extension '%s' not found", authenticatorName)
}

// resolveAuthenticator is a helper to resolve and validate HTTP authenticator for a backend
func (s *storageExt) resolveAuthenticator(host component.Host, authCfg config.Authentication, backendType, backendName string) (extensionauth.HTTPClient, error) {
	if authCfg.AuthenticatorID.String() == "" {
		return nil, nil
	}

	httpAuth, err := s.getAuthenticator(host, authCfg.AuthenticatorID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTP authenticator for %s backend '%s': %w", backendType, backendName, err)
	}
	s.telset.Logger.Sugar().Infof("HTTP auth configured for %s backend '%s' with authenticator '%s'",
		backendType, backendName, authCfg.AuthenticatorID.String())
	return httpAuth, nil
}
