package allinone

import (
	"context"

	"github.com/jaegertracing/jaeger/cmd/jaeger-v2/internal/extension/jaegerstorage"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/service"
)

type configProvider struct{}

var _ otelcol.ConfigProvider = (*configProvider)(nil)

// NewConfigProvider creates a new ConfigProvider.
func NewConfigProvider() *configProvider {
	return &configProvider{}
}

func (cp *configProvider) Get(ctx context.Context, factories otelcol.Factories) (*otelcol.Config, error) {
	cfg := &otelcol.Config{
		Service: cp.makeServiceConfig(),
	}
	return cfg, nil
}

// makeServiceConfig creates a default all-in-one service config
// that contains all standard all-in-one extensions and pipelines.
//
// service:
//
//	extensions: [jaeger_storage, jaeger_query]
//	pipelines:
//	  traces:
//	    receivers: [otlp, jaeger, zipkin]
//	    processors: [batch]
//	    exporters: [jaeger_storage_exporter]
func (cp *configProvider) makeServiceConfig() service.Config {
	return service.Config{
		Extensions: []component.ID{
			jaegerstorage.ID,
		},
	}
}

// Watch implements otelcol.ConfigProvider.
func (*configProvider) Watch() <-chan error {
	panic("unimplemented")
}

// Shutdown implements otelcol.ConfigProvider.
func (*configProvider) Shutdown(ctx context.Context) error {
	return nil
}
