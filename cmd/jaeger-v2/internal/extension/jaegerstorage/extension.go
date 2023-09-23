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

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
)

var _ extension.Extension = (*StorageExt)(nil)

type StorageExt struct {
	config    *Config
	logger    *zap.Logger
	factories map[string]storage.Factory
}

func GetStorageFactory(name string, host component.Host) (storage.Factory, error) {
	var comp component.Component
	for id, ext := range host.GetExtensions() {
		if id.Type() == ComponentType {
			comp = ext
			break
		}
	}
	if comp == nil {
		return nil, fmt.Errorf(
			"cannot find extension '%s' (make sure it's defined earlier in the config)",
			ComponentType,
		)
	}
	f, ok := comp.(*StorageExt).factories[name]
	if !ok {
		return nil, fmt.Errorf(
			"cannot find storage '%s' declared with '%s' extension",
			name, ComponentType,
		)
	}
	return f, nil
}

func newStorageExt(config *Config, otel component.TelemetrySettings) *StorageExt {
	return &StorageExt{
		config:    config,
		logger:    otel.Logger,
		factories: make(map[string]storage.Factory),
	}
}

func (s *StorageExt) Start(ctx context.Context, host component.Host) error {
	for name, mem := range s.config.Memory {
		if _, ok := s.factories[name]; ok {
			return fmt.Errorf("duplicate memory storage name %s", name)
		}
		s.factories[name] = memory.NewFactoryWithConfig(
			mem,
			metrics.NullFactory,
			s.logger.With(zap.String("storage_name", name)),
		)
	}
	return nil
}

func (s *StorageExt) Shutdown(ctx context.Context) error {
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
