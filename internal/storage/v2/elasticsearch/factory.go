// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"io"
	"strings"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/tracestoremetrics"
	v2depstore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	v2tracestore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

const tagError = "error"

var (
	_ io.Closer          = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
	_ depstore.Factory   = (*Factory)(nil)
)

var materializeSpanKindAndStatus *featuregate.Gate

func init() {
	materializeSpanKindAndStatus = featuregate.GlobalRegistry().MustRegister(
		"jaeger.es.materializeSpanKindAndStatus",
		featuregate.StageBeta, // enabled by default
		featuregate.WithRegisterDescription(
			"When feature is on, searching by span.kind and span.status tags will only search the materialized fields in the ES document. This will become the only behavior in the future. To enable the legacy behavior of querying span kind and status from the nested tags, e.g. to support querying the data written before v2.8, turn off this feature."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/pull/7272"))
}

type Factory struct {
	coreFactory    *elasticsearch.FactoryBase
	config         escfg.Configuration
	metricsFactory metrics.Factory
}

func NewFactory(ctx context.Context, cfg escfg.Configuration, telset telemetry.Settings) (*Factory, error) {
	// Ensure required fields are always included in tagsAsFields
	if materializeSpanKindAndStatus.IsEnabled() {
		cfg = ensureRequiredFields(cfg)
	}

	coreFactory, err := elasticsearch.NewFactoryBase(ctx, cfg, telset.Metrics, telset.Logger)
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
	return tracestoremetrics.NewReaderDecorator(v2tracestore.NewTraceReader(params), f.metricsFactory), nil
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

	requiredTags := map[string]bool{
		model.SpanKindKey: false,
		tagError:          false,
	}

	// Copies existing tags to new slice, trimming whitespace and tracking presence of required tags
	includes := make([]string, 0, len(strings.Split(cfg.Tags.Include, ","))+2)
	for _, tag := range strings.Split(cfg.Tags.Include, ",") {
		tag = strings.TrimSpace(tag)
		includes = append(includes, tag)
		if _, exists := requiredTags[tag]; exists {
			requiredTags[tag] = true
		}
	}

	// Append only missing required tags
	for tag, present := range requiredTags {
		if !present {
			includes = append(includes, tag)
		}
	}

	// Return new configuration with updated includes
	cfg.Tags.Include = strings.Join(includes, ",")
	return cfg
}
