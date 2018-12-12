package main

import (
	"flag"
	"time"

	"github.com/jaegertracing/jaeger/internal/tracegen"
	"github.com/opentracing/opentracing-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	jaegerZap "github.com/uber/jaeger-client-go/log/zap"
	"github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"
)

var logger, _ = zap.NewDevelopment()

func main() {
	fs := flag.CommandLine
	cfg := new(tracegen.Config)
	cfg.Flags(fs)
	flag.Parse()

	metricsFactory := prometheus.New()
	tracer, tCloser, err := jaegerConfig.Configuration{
		ServiceName: "tracegen",
		Sampler: &jaegerConfig.SamplerConfig{
			Type:  "const",
			Param: 1,
		},
	}.NewTracer(
		jaegerConfig.Metrics(metricsFactory),
		jaegerConfig.Logger(jaegerZap.NewLogger(logger)),
	)
	if err != nil {
		logger.Fatal("failed to create tracer", zap.Error(err))
	}
	defer tCloser.Close()

	opentracing.InitGlobalTracer(tracer)
	logger.Info("Initialized global tracer")

	cfg.Run(logger)

	logger.Info("Waiting 1.5sec for metrics to flush")
	time.Sleep(3 * time.Second / 2)
}
