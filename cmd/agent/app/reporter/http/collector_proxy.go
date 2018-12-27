package http

import (
	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	httpManager "github.com/jaegertracing/jaeger/cmd/agent/app/configmanager/http"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
)

// ProxyBuilder create CollectorProxy
type ProxyBuilder struct {
	collectorEndpoint string
	reporter          reporter.Reporter
	manager           configmanager.ClientConfigManager
}

// NewCollectorProxy creates ProxyBuilder
func NewCollectorProxy(builder *Builder, mFactory metrics.Factory, logger *zap.Logger) (*ProxyBuilder, error) {
	r, err := builder.CreateReporter()
	if err != nil {
		return nil, err
	}
	httpMetrics := mFactory.Namespace("", map[string]string{"protocol": "http"})
	return &ProxyBuilder{
		collectorEndpoint: builder.CollectorEndpoint,
		reporter:          r,
		manager: configmanager.WrapWithMetrics(
			httpManager.NewConfigManager(builder.CollectorEndpoint),
			httpMetrics,
		),
	}, nil
}

// GetReporter returns reporter
func (b ProxyBuilder) GetReporter() reporter.Reporter {
	return b.reporter
}

// GetManager returns manager
func (b ProxyBuilder) GetManager() configmanager.ClientConfigManager {
	return b.manager
}
