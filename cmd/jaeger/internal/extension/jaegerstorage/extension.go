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
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage/factoryadapter"
	esCfg "github.com/jaegertracing/jaeger/pkg/es/config"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	ch "github.com/jaegertracing/jaeger/plugin/storage/clickhouse"
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
	logger    *zap.Logger
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

func GetStorageFactoryV2(name string, host component.Host) (bool, spanstore.Factory, error) {
	f, err := GetStorageFactory(name, host)
	if err != nil {
		return false, nil, err
	}

	var clickhouse bool
	switch f.(type) {
	case *ch.Factory:
		clickhouse = true
	}

	return clickhouse, factoryadapter.NewFactory(f), nil
}

func newStorageExt(config *Config, otel component.TelemetrySettings) *storageExt {
	return &storageExt{
		config:    config,
		logger:    otel.Logger,
		factories: make(map[string]storage.Factory),
	}
}

type starter[Config any, Factory storage.Factory] struct {
	ext               *storageExt
	storageKind       string
	cfg               map[string]Config
	builder           func(Config, metrics.Factory, *zap.Logger) (Factory, error)
	clickhouseBuilder func(context.Context, Config, *zap.Logger) Factory
}

func (s *starter[Config, Factory]) build(ctx context.Context, _ component.Host) error {
	for name, cfg := range s.cfg {
		if _, ok := s.ext.factories[name]; ok {
			return fmt.Errorf("duplicate %s storage name %s", s.storageKind, name)
		}
		var factory Factory
		if s.clickhouseBuilder != nil {
			factory = s.clickhouseBuilder(ctx, cfg, s.ext.logger.With(zap.String("storage_name", name)))
		} else {
			var err error
			factory, err = s.builder(
				cfg,
				metrics.NullFactory,
				s.ext.logger.With(zap.String("storage_name", name)),
			)
			if err != nil {
				return fmt.Errorf("failed to initialize %s storage %s: %w", s.storageKind, name, err)
			}
		}
		s.ext.factories[name] = factory
	}
	return nil
}

func (s *storageExt) Start(ctx context.Context, host component.Host) error {
	memStarter := &starter[memoryCfg.Configuration, *memory.Factory]{
		ext:         s,
		storageKind: "memory",
		cfg:         s.config.Memory,
		// memory factory does not return an error, so need to wrap it
		builder: func(
			cfg memoryCfg.Configuration,
			metricsFactory metrics.Factory,
			logger *zap.Logger,
		) (*memory.Factory, error) {
			return memory.NewFactoryWithConfig(cfg, metricsFactory, logger), nil
		},
	}
	badgerStarter := &starter[badger.NamespaceConfig, *badger.Factory]{
		ext:         s,
		storageKind: "badger",
		cfg:         s.config.Badger,
		builder:     badger.NewFactoryWithConfig,
	}
	grpcStarter := &starter[grpc.ConfigV2, *grpc.Factory]{
		ext:         s,
		storageKind: "grpc",
		cfg:         s.config.GRPC,
		builder:     grpc.NewFactoryWithConfig,
	}
	esStarter := &starter[esCfg.Configuration, *es.Factory]{
		ext:         s,
		storageKind: "elasticsearch",
		cfg:         s.config.Elasticsearch,
		builder:     es.NewFactoryWithConfig,
	}
	osStarter := &starter[esCfg.Configuration, *es.Factory]{
		ext:         s,
		storageKind: "opensearch",
		cfg:         s.config.Opensearch,
		builder:     es.NewFactoryWithConfig,
	}
	cassandraStarter := &starter[cassandra.Options, *cassandra.Factory]{
		ext:         s,
		storageKind: "cassandra",
		cfg:         s.config.Cassandra,
		builder:     cassandra.NewFactoryWithConfig,
	}
	clickhouseStarter := &starter[ch.Config, *ch.Factory]{
		ext:               s,
		storageKind:       "clickhouse",
		cfg:               s.config.ClickHouse,
		clickhouseBuilder: ch.NewFactory,
	}

	builders := []func(ctx context.Context, host component.Host) error{
		memStarter.build,
		badgerStarter.build,
		grpcStarter.build,
		esStarter.build,
		osStarter.build,
		cassandraStarter.build,
		clickhouseStarter.build,
		// TODO add support for other backends
	}
	for _, builder := range builders {
		if err := builder(ctx, host); err != nil {
			return err
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
	return errors.Join(errs...)
}

func (s *storageExt) Factory(name string) (storage.Factory, bool) {
	f, ok := s.factories[name]
	return f, ok
}
