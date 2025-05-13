// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"io"

	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	v2depstore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	v2tracestore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	_ io.Closer          = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
	_ depstore.Factory   = (*Factory)(nil)
)

type Factory struct {
	coreFactory *elasticsearch.FactoryBase
}

func NewFactory(cfg escfg.Configuration, telset telemetry.Settings) (*Factory, error) {
	coreFactory, err := elasticsearch.NewFactoryBase(cfg, telset.Metrics, telset.Logger)
	if err != nil {
		return nil, err
	}
	f := &Factory{
		coreFactory: coreFactory,
	}
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	params, err := f.coreFactory.GetSpanReaderParams()
	if err != nil {
		return nil, err
	}
	return v2tracestore.NewTraceReader(params), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	params, err := f.coreFactory.GetSpanWriterParams()
	if err != nil {
		return nil, err
	}
	wr := v2tracestore.NewTraceWriter(params)
	if err := f.createTemplates(wr); err != nil {
		return nil, err
	}
	return wr, nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	params := f.coreFactory.GetDependencyStoreParams()
	return v2depstore.NewDependencyStoreV2(params), nil
}

func (f *Factory) Close() error {
	return f.coreFactory.Close()
}

func (f *Factory) Purge(ctx context.Context) error {
	return f.coreFactory.Purge(ctx)
}

func (f *Factory) createTemplates(writer *v2tracestore.TraceWriter) error {
	cfg := f.coreFactory.GetConfig()
	// Creating a template here would conflict with the one created for ILM resulting to no index rollover
	if cfg.CreateIndexTemplates && !cfg.UseILM {
		spanMapping, serviceMapping, err := f.coreFactory.GetSpanServiceMapping()
		if err != nil {
			return err
		}
		if err := writer.CreateTemplates(spanMapping, serviceMapping, cfg.Indices.IndexPrefix); err != nil {
			return err
		}
	}
	return nil
}
