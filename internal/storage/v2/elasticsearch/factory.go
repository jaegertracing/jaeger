// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"io"
	"strings"

	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/tracestoremetrics"
	v2depstore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	v2tracestore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

const tagError = "error"

// nativeTraceSummariesGate enables computing trace summaries (the metadata shown
// on the search-results page) natively in Elasticsearch/OpenSearch via a single
// aggregation query, instead of loading full traces and aggregating them in the
// query service. When disabled, CreateTraceReader returns a reader that does not
// implement tracestore.SummaryReader, so the query service transparently falls
// back to the full-trace path.
var nativeTraceSummariesGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.es.nativeTraceSummaries",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.20.0"),
	featuregate.WithRegisterDescription(
		"Computes trace summaries natively in Elasticsearch/OpenSearch via aggregations "+
			"instead of loading full traces and aggregating in the query service. Requires "+
			"inline (Painless) scripts to be enabled on the cluster.",
	),
)

var (
	_ io.Closer          = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
	_ depstore.Factory   = (*Factory)(nil)
)

type Factory struct {
	coreFactory    *elasticsearch.FactoryBase
	config         escfg.Configuration
	metricsFactory metrics.Factory
}

func NewFactory(ctx context.Context, cfg escfg.Configuration, telset telemetry.Settings, httpAuth extensionauth.HTTPClient) (*Factory, error) {
	// Ensure required fields are always included in tagsAsFields
	cfg = ensureRequiredFields(cfg)

	coreFactory, err := elasticsearch.NewFactoryBase(ctx, cfg, telset.Metrics, telset.Logger, httpAuth)
	if err != nil {
		return nil, err
	}
	f := &Factory{
		coreFactory:    coreFactory,
		config:         cfg,
		metricsFactory: telset.Metrics,
	}
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	params := f.coreFactory.GetSpanReaderParams()
	base := v2tracestore.NewTraceReader(params)
	var reader tracestore.Reader = base
	if nativeTraceSummariesGate.IsEnabled() {
		reader = v2tracestore.NewReaderWithSummaries(base)
	}
	return tracestoremetrics.NewReaderDecorator(reader, f.metricsFactory), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	params := f.coreFactory.GetSpanWriterParams()
	wr := v2tracestore.NewTraceWriter(params)
	return wr, nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	params := f.coreFactory.GetDependencyStoreParams()
	return v2depstore.NewDependencyStoreV2(params), nil
}

func (f *Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	return f.coreFactory.CreateSamplingStore(maxBuckets)
}

func (f *Factory) Close() error {
	return f.coreFactory.Close()
}

func (f *Factory) Purge(ctx context.Context) error {
	return f.coreFactory.Purge(ctx)
}

// ensureRequiredFields adds span.kind and span.status error to tags-as-fields configuration
// regardless of user settings
func ensureRequiredFields(cfg escfg.Configuration) escfg.Configuration {
	if cfg.Tags.AllAsFields {
		return cfg
	}

	// Return new configuration with updated includes
	if cfg.Tags.Include != "" && !strings.HasSuffix(cfg.Tags.Include, ",") {
		cfg.Tags.Include += ","
	}
	cfg.Tags.Include += model.SpanKindKey + "," + tagError

	return cfg
}
