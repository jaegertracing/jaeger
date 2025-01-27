// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"errors"
	"io"

	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	storage_v1 "github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

type Factory struct {
	ss storage_v1.Factory
}

func NewFactory(ss storage_v1.Factory) *Factory {
	return &Factory{
		ss: ss,
	}
}

// Initialize implements tracestore.Factory.
func (*Factory) Initialize(_ context.Context) error {
	panic("not implemented")
}

// Close implements tracestore.Factory.
func (f *Factory) Close(_ context.Context) error {
	if closer, ok := f.ss.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// CreateTraceReader implements tracestore.Factory.
func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	spanReader, err := f.ss.CreateSpanReader()
	if err != nil {
		return nil, err
	}
	return NewTraceReader(spanReader), nil
}

// CreateTraceWriter implements tracestore.Factory.
func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	spanWriter, err := f.ss.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	return NewTraceWriter(spanWriter), nil
}

// CreateDependencyReader implements depstore.Factory.
func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	dr, err := f.ss.CreateDependencyReader()
	if err != nil {
		return nil, err
	}
	return NewDependencyReader(dr), nil
}

// CreateLock implements storage_v1.SamplingStoreFactory
func (f *Factory) CreateLock() (distributedlock.Lock, error) {
	ss, ok := f.ss.(storage_v1.SamplingStoreFactory)
	if !ok {
		return nil, errors.New("storage backend does not support sampling store")
	}
	lock, err := ss.CreateLock()
	return lock, err
}

// CreateSamplingStore implements storage_v1.SamplingStoreFactory
func (f *Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	ss, ok := f.ss.(storage_v1.SamplingStoreFactory)
	if !ok {
		return nil, errors.New("storage backend does not support sampling store")
	}
	store, err := ss.CreateSamplingStore(maxBuckets)
	return store, err
}

// Purge implements storage_v1.Purger
func (f *Factory) Purge(ctx context.Context) error {
	p, ok := f.ss.(storage_v1.Purger)
	if !ok {
		return errors.New("storage backend does not support Purger")
	}
	err := p.Purge(ctx)
	return err
}
