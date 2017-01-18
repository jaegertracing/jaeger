package tracing

import (
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/zap"
	jaeger "github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"

	"code.uber.internal/infra/jaeger-demo/pkg/log"
)

// Init creates a new instance of Jaeger tracer.
func Init(serviceName string, logger log.Factory) opentracing.Tracer {
	cfg := config.Configuration{
		Sampler: &config.SamplerConfig{
			Type:  "const",
			Param: 1,
		},
		Reporter: &config.ReporterConfig{
			LogSpans:            false,
			BufferFlushInterval: 1 * time.Second,
		},
		Logger: jaegerLoggerAdapter{logger.Bg()},
	}
	tracer, _, err := cfg.New(serviceName, jaeger.NullStatsReporter)
	if err != nil {
		logger.Bg().Fatal("cannot initialize Jaeger Tracer", zap.Error(err))
	}
	return tracer
}

type jaegerLoggerAdapter struct {
	logger log.Logger
}

func (l jaegerLoggerAdapter) Error(msg string) {
	l.logger.Error(msg)
}

func (l jaegerLoggerAdapter) Infof(msg string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(msg, args...))
}
