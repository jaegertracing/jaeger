// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensionauth"

	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/internal/metrics"
	otelmetrics "github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	esmetrics "github.com/jaegertracing/jaeger/internal/storage/metricstore/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var _ Extension = (*storageExt)(nil)

type Extension interface {
	extension.Extension
	TraceStorageFactory(name string) (tracestore.Factory, error)
	MetricStorageFactory(name string) (storage.MetricStoreFactory, error)
}

type storageExt struct {
	config           *Config
	telset           telemetry.Settings
	factories        map[string]tracestore.Factory
	metricsFactories map[string]storage.MetricStoreFactory
	factoryMu        sync.Mutex
}

// getStorageFactory locates the extension in Host and retrieves
// a trace storage factory from it with the given name.
func getStorageFactory(name string, host component.Host) (tracestore.Factory, error) {
	ext, err := findExtension(host)
	if err != nil {
		return nil, err
	}
	f, err := ext.TraceStorageFactory(name)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot find definition of storage '%s' in the configuration for extension '%s': %w",
			name, componentType, err,
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
	mf, err := ext.MetricStorageFactory(name)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot find metric storage '%s' declared by '%s' extension: %w",
			name, componentType, err,
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
	// Initialize telemetry.Settings with host=nil, will be set in Start()
	tset := telemetry.Settings{
		Logger:         telset.Logger,
		MeterProvider:  telset.MeterProvider,
		TracerProvider: telset.TracerProvider,
	}
	return &storageExt{
		config:           cfg,
		telset:           tset,
		factories:        make(map[string]tracestore.Factory),
		metricsFactories: make(map[string]storage.MetricStoreFactory),
	}
}

func (s *storageExt) Start(_ context.Context, host component.Host) error {
	// Set host in telset for use in lazy factory initialization
	s.telset.Host = host
	s.telset.Metrics = otelmetrics.NewFactory(s.telset.MeterProvider).Namespace(metrics.NSOptions{Name: "jaeger"})

	// Validate configurations early to catch errors at startup
	for name, cfg := range s.config.TraceBackends {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid configuration for trace storage '%s': %w", name, err)
		}
	}
	for name, cfg := range s.config.MetricBackends {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid configuration for metric storage '%s': %w", name, err)
		}
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

func (s *storageExt) TraceStorageFactory(name string) (tracestore.Factory, error) {
	s.factoryMu.Lock()
	defer s.factoryMu.Unlock()

	// Return cached factory if already created
	if f, ok := s.factories[name]; ok {
		return f, nil
	}

	// Check if configuration exists
	cfg, ok := s.config.TraceBackends[name]
	if !ok {
		return nil, fmt.Errorf(
			"storage '%s' not declared in '%s' extension configuration",
			name, componentType,
		)
	}

	// Create factory on demand
	factory, err := storageconfig.CreateTraceStorageFactory(
		context.Background(),
		name,
		cfg,
		s.telset,
		func(authCfg config.Authentication, backendType, backendName string) (extensionauth.HTTPClient, error) {
			return s.resolveAuthenticator(s.telset.Host, authCfg, backendType, backendName)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage '%s': %w", name, err)
	}

	s.factories[name] = factory
	return factory, nil
}

// createMetricStorageFactory is a helper function to create a metric storage factory
func (s *storageExt) createMetricStorageFactory(name string, cfg storageconfig.MetricBackend, telset telemetry.Settings) (storage.MetricStoreFactory, error) {
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

	s.telset.Logger.Sugar().Infof("Initializing metrics storage '%s'", name)
	var metricStoreFactory storage.MetricStoreFactory
	var err error

	switch {
	case cfg.Prometheus != nil:
		promTelset := telset
		promTelset.Metrics = scopedMetricsFactory(name, "prometheus", "metricstore")
		httpAuth, authErr := s.resolveAuthenticator(s.telset.Host, cfg.Prometheus.Authentication, "prometheus metrics", name)
		if authErr != nil {
			return nil, authErr
		}
		metricStoreFactory, err = prometheus.NewFactoryWithConfig(
			cfg.Prometheus.Configuration,
			promTelset,
			httpAuth,
		)

	case cfg.Elasticsearch != nil:
		esTelset := telset
		esTelset.Metrics = scopedMetricsFactory(name, "elasticsearch", "metricstore")
		httpAuth, authErr := s.resolveAuthenticator(s.telset.Host, cfg.Elasticsearch.Authentication, "elasticsearch metrics", name)
		if authErr != nil {
			return nil, authErr
		}
		metricStoreFactory, err = esmetrics.NewFactory(
			context.Background(),
			*cfg.Elasticsearch,
			esTelset,
			httpAuth,
		)

	case cfg.Opensearch != nil:
		osTelset := telset
		osTelset.Metrics = scopedMetricsFactory(name, "opensearch", "metricstore")
		httpAuth, authErr := s.resolveAuthenticator(s.telset.Host, cfg.Opensearch.Authentication, "opensearch metrics", name)
		if authErr != nil {
			return nil, authErr
		}
		metricStoreFactory, err = esmetrics.NewFactory(
			context.Background(),
			*cfg.Opensearch,
			osTelset,
			httpAuth,
		)

	default:
		err = fmt.Errorf("no metric backend configuration provided for '%s'", name)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize metrics storage '%s': %w", name, err)
	}

	return metricStoreFactory, nil
}

func (s *storageExt) MetricStorageFactory(name string) (storage.MetricStoreFactory, error) {
	s.factoryMu.Lock()
	defer s.factoryMu.Unlock()

	// Return cached factory if already created
	if mf, ok := s.metricsFactories[name]; ok {
		return mf, nil
	}

	// Check if configuration exists
	cfg, ok := s.config.MetricBackends[name]
	if !ok {
		return nil, fmt.Errorf(
			"metric storage '%s' not declared in '%s' extension configuration",
			name, componentType,
		)
	}

	// Create factory on demand using helper
	metricStoreFactory, err := s.createMetricStorageFactory(name, cfg, s.telset)
	if err != nil {
		return nil, err
	}

	s.metricsFactories[name] = metricStoreFactory
	return metricStoreFactory, nil
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
