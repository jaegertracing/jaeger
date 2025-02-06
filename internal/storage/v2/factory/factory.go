// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"errors"
	"fmt"
	"io"

	"github.com/jaegertracing/jaeger/internal/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
)

const (
	clickhouseStorageType = "clickhouse"
)

// AllStorageTypes defines all available storage backends
var AllStorageTypes = []string{
	clickhouseStorageType,
}

type Factory struct {
	Config
	factories map[string]storage.Factory
}

// NewFactory creates the meta-factory.
func NewFactory(config Config) (*Factory, error) {
	f := &Factory{Config: config}
	uniqueTypes := map[string]struct{}{}
	for _, storageType := range f.TraceWriterTypes {
		uniqueTypes[storageType] = struct{}{}
	}

	f.factories = make(map[string]storage.Factory)

	for t := range uniqueTypes {
		ff, err := f.getFactoryOfType(t)
		if err != nil {
			return nil, err
		}
		f.factories[t] = ff
	}

	return f, nil
}

func (*Factory) getFactoryOfType(factoryType string) (storage.Factory, error) {
	switch factoryType {
	case clickhouseStorageType:
		return clickhouse.NewFactory(), nil
	default:
		return nil, fmt.Errorf("unknown storage type %s. Valid types are %v", factoryType, AllStorageTypes)
	}
}

// CreatTraceWriter implements storage.Factory.
func (f *Factory) CreatTraceWriter() (tracestore.Writer, error) {
	var writers []tracestore.Writer
	for _, storageType := range f.TraceWriterTypes {
		factory, ok := f.factories[storageType]
		if !ok {
			return nil, fmt.Errorf("no %s backend registered for trace store", storageType)
		}
		writer, err := factory.CreateTraceWriter()
		if err != nil {
			return nil, err
		}
		writers = append(writers, writer)
	}
	var traceWriter tracestore.Writer
	if len(writers) == 1 {
		traceWriter = writers[0]
		return traceWriter, nil
	}
	return nil, nil
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	var errs []error
	closeFactory := func(factory storage.Factory) {
		if closer, ok := factory.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, storageType := range f.TraceWriterTypes {
		if factory, ok := f.factories[storageType]; ok {
			closeFactory(factory)
		}
	}
	return errors.Join(errs...)
}
