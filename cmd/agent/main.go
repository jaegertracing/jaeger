package main

import (
	"flag"
	"runtime"

	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"

	"code.uber.internal/infra/jaeger/oss/cmd/agent/app"
)

func main() {
	builder := app.NewBuilder()
	builder.Bind(flag.CommandLine)
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	logger := zap.New(zap.NewJSONEncoder())
	metricsFactory := xkit.Wrap("jaeger-agent", expvar.NewFactory(10))

	// TODO illustrate discovery service wiring
	// TODO illustrate additional reporter

	agent, err := builder.CreateAgent(metricsFactory, logger)
	if err != nil {
		logger.Fatal("Unable to initialize Jaeger Agent", zap.Error(err))
	}

	logger.Info("Starting agent")
	if err := agent.Run(); err != nil {
		logger.Fatal("Failed to run the agent", zap.Error(err))
	}
	select {}
}
