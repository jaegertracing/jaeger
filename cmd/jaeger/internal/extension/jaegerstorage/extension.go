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
	"github.com/jaegertracing/jaeger/pkg/metrics"
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
	config    *Config
	telset    component.TelemetrySettings
	factories map[string]storage.Factory
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

func (s *storageExt) makeBuilder(mf metrics.Factory, cfg Backend) func() (storage.Factory, error) {
	if cfg.Memory != nil {
		return func() (storage.Factory, error) {
			return memory.NewFactoryWithConfig(*cfg.Memory, mf, s.telset.Logger), nil
		}
	} else if cfg.Badger != nil {
		return func() (storage.Factory, error) {
			return badger.NewFactoryWithConfig(*cfg.Badger, mf, s.telset.Logger)
		}
	} else if cfg.GRPC != nil {
		return func() (storage.Factory, error) {
			return grpc.NewFactoryWithConfig(*cfg.GRPC, mf, s.telset.Logger)
		}
	} else if cfg.Cassandra != nil {
		return func() (storage.Factory, error) {
			return cassandra.NewFactoryWithConfig(*cfg.Cassandra, mf, s.telset.Logger)
		}
	} else if cfg.Elasticsearch != nil {
		return func() (storage.Factory, error) {
			return es.NewFactoryWithConfig(*cfg.Elasticsearch, mf, s.telset.Logger)
		}
	} else if cfg.Opensearch != nil {
		return func() (storage.Factory, error) {
			return es.NewFactoryWithConfig(*cfg.Opensearch, mf, s.telset.Logger)
		}
	}
	return func() (storage.Factory, error) {
		return nil, errors.New("empty configuration")
	}
}

func (s *storageExt) Start(ctx context.Context, host component.Host) error {
	mf := otelmetrics.NewFactory(s.telset.MeterProvider)
	for storageName, cfg := range s.config.Backends {
		s.telset.Logger.Sugar().Infof("Initializing storage '%s'", storageName)
		factory, err := s.makeBuilder(mf, cfg)()
		if err != nil {
			return fmt.Errorf("failed to initialize storage '%s': %w", storageName, err)
		}
		s.factories[storageName] = factory
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
