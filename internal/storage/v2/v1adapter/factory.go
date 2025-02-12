// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"io"

	storage_v1 "github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type Factory struct {
	ss storage_v1.Factory
}

func NewFactory(ss storage_v1.Factory) tracestore.Factory {
	factory := &Factory{
		ss: ss,
	}

	var (
		purger, isPurger   = ss.(storage_v1.Purger)
		sampler, isSampler = ss.(storage_v1.SamplingStoreFactory)
		closer, isCloser   = ss.(io.Closer)
	)

	switch {
	case isPurger && isSampler && isCloser:
		return struct {
			*Factory
			storage_v1.Purger
			storage_v1.SamplingStoreFactory
			io.Closer
		}{factory, purger, sampler, closer}
	case isPurger && isCloser:
		return struct {
			*Factory
			storage_v1.Purger
			io.Closer
		}{factory, purger, closer}
	case isSampler && isCloser:
		return struct {
			*Factory
			storage_v1.SamplingStoreFactory
			io.Closer
		}{factory, sampler, closer}
	case isSampler && isPurger:
		return struct {
			*Factory
			storage_v1.Purger
			storage_v1.SamplingStoreFactory
		}{factory, purger, sampler}
	case isCloser:
		return struct {
			*Factory
			io.Closer
		}{factory, closer}
	case isPurger:
		return struct {
			*Factory
			storage_v1.Purger
		}{factory, purger}
	case isSampler:
		return struct {
			*Factory
			storage_v1.SamplingStoreFactory
		}{factory, sampler}
	default:
		return factory
	}
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
