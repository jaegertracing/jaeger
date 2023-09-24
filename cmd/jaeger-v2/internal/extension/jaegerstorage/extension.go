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

var _ extension.Extension = (*storageExt)(nil)

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
	f, ok := comp.(*storageExt).factories[name]
	if !ok {
		return nil, fmt.Errorf(
			"cannot find storage '%s' declared with '%s' extension",
			name, componentType,
		)
	}
	return f, nil
}

func newStorageExt(config *Config, otel component.TelemetrySettings) *storageExt {
	return &storageExt{
		config:    config,
		logger:    otel.Logger,
		factories: make(map[string]storage.Factory),
	}
}

func (s *storageExt) Start(ctx context.Context, host component.Host) error {
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
	// TODO add support for other backends
	return nil
}

func (s *storageExt) Shutdown(ctx context.Context) error {
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
