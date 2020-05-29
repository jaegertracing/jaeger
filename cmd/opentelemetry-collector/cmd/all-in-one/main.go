// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"

	"github.com/opentracing/opentracing-go"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	jaegerClientZapLog "github.com/uber/jaeger-client-go/log/zap"
	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/builder"
	"go.uber.org/zap"

	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	jflags "github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/defaults"
	cassandra2 "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/memory"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
	storagePlugin "github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/storage"
)

func main() {
	handleErr := func(err error) {
		if err != nil {
			log.Fatalf("Failed to run the service: %v", err)
		}
	}

	ver := version.Get()
	info := service.ApplicationStartInfo{
		ExeName:  "jaeger-opentelemetry-all-in-one",
		LongName: "Jaeger OpenTelemetry all-in-one",
		Version:  ver.GitVersion,
		GitHash:  ver.GitCommit,
	}

	v := viper.New()
	storageType := os.Getenv(storagePlugin.SpanStorageTypeEnvVar)
	if storageType == "" {
		storageType = "memory"
	}

	configCreatedWait := sync.WaitGroup{}
	configCreatedWait.Add(1)
	exporters := configmodels.Exporters{}

	cmpts := defaults.Components(v)
	cfgFactory := func(otelViper *viper.Viper, f config.Factories) (*configmodels.Config, error) {
		collectorOpts := &collectorApp.CollectorOptions{}
		collectorOpts.InitFromViper(v)
		cfgOpts := defaults.ComponentSettings{
			ComponentType:  defaults.AllInOne,
			Factories:      cmpts,
			StorageType:    storageType,
			ZipkinHostPort: collectorOpts.CollectorZipkinHTTPHostPort,
		}
		cfg, err := cfgOpts.CreateDefaultConfig()
		if err != nil {
			return nil, err
		}

		if len(builder.GetConfigFile()) > 0 {
			otelCfg, err := service.FileLoaderConfigFactory(otelViper, f)
			if err != nil {
				return nil, err
			}
			err = defaults.MergeConfigs(cfg, otelCfg)
			if err != nil {
				return nil, err
			}
		}

		exporters = cfg.Exporters
		configCreatedWait.Done()
		return cfg, nil
	}

	svc, err := service.New(service.Parameters{
		ApplicationStartInfo: info,
		Factories:            cmpts,
		ConfigFactory:        cfgFactory,
	})
	handleErr(err)

	// Add Jaeger specific flags to service command
	// this passes flag values to viper.
	storageFlags, err := app.AddStorageFlags(storageType, true)
	if err != nil {
		handleErr(err)
	}
	cmd := svc.Command()
	jConfig.AddFlags(v,
		cmd,
		app.AddComponentFlags,
		storageFlags,
		queryApp.AddFlags,
	)

	// parse flags to propagate Jaeger config file flag value to viper
	cmd.ParseFlags(os.Args)
	err = jflags.TryLoadConfigFile(v)
	if err != nil {
		handleErr(fmt.Errorf("could not load Jaeger configuration file %w", err))
	}

	go func() {
		err = svc.Start()
		handleErr(err)
	}()

	configCreatedWait.Wait()
	exp := getStorageExporter(storageType, exporters)
	fac, err := getFactory(exp, v, svc.GetLogger())
	if err != nil {
		svc.ReportFatalError(err)
	}
	startQuery(v, svc.GetLogger(), fac, svc.ReportFatalError)
}

func getStorageExporter(storageType string, exporters configmodels.Exporters) configmodels.Exporter {
	for _, e := range exporters {
		if e.Name() == fmt.Sprintf("jaeger_%s", storageType) {
			return e
		}
	}
	return nil
}

func getFactory(exporter configmodels.Exporter, v *viper.Viper, logger *zap.Logger) (storage.Factory, error) {
	switch exporter.Name() {
	case "jaeger_elasticsearch":
		archiveOpts := es.NewOptions("es-archive")
		archiveOpts.InitFromViper(v)
		primaryConfig := exporter.(*elasticsearch.Config)
		f := es.NewFactory()
		f.InitFromOptions(*es.NewOptionsFromConfig(primaryConfig.Primary.Configuration, archiveOpts.Primary.Configuration))
		if err := f.Initialize(metrics.NullFactory, logger); err != nil {
			return nil, err
		}
		return f, nil
	case "jaeger_cassandra":
		archiveOpts := cassandra.NewOptions("cassandra-archive")
		archiveOpts.InitFromViper(v)
		primaryConfig := exporter.(*cassandra2.Config)
		f := cassandra.NewFactory()
		f.InitFromOptions(cassandra.NewOptionsFromConfig(primaryConfig.Primary.Configuration, archiveOpts.Primary.Configuration))
		if err := f.Initialize(metrics.NullFactory, logger); err != nil {
			return nil, err
		}
		return f, nil
	case "memory":
		return memory.GetFactory(), nil
	default:
		return nil, errors.New("storage type cannot be used with all-in-one")
	}
}

func startQuery(v *viper.Viper, logger *zap.Logger, storageFactory storage.Factory, reportErr func(err error)) {
	spanReader, err := storageFactory.CreateSpanReader()
	if err != nil {
		reportErr(err)
	}
	dependencyReader, err := storageFactory.CreateDependencyReader()
	if err != nil {
		reportErr(err)
	}
	queryOpts := new(queryApp.QueryOptions).InitFromViper(v, logger)
	queryServiceOptions := queryOpts.BuildQueryServiceOptions(storageFactory, logger)
	queryService := querysvc.NewQueryService(
		spanReader,
		dependencyReader,
		*queryServiceOptions)

	tracerCloser := initTracer(logger)
	server := queryApp.NewServer(logger, queryService, queryOpts, opentracing.GlobalTracer())
	if err := server.Start(); err != nil {
		reportErr(err)
	}

	// TODO subscribe to service's lifecycle (shutdown) event and then terminate
	// https://github.com/open-telemetry/opentelemetry-collector/issues/1033
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	tracerCloser.Close()
}

func initTracer(logger *zap.Logger) io.Closer {
	traceCfg := &jaegerClientConfig.Configuration{
		ServiceName: "jaeger-query",
		Sampler: &jaegerClientConfig.SamplerConfig{
			Type:  "const",
			Param: 1.0,
		},
		RPCMetrics: true,
	}
	traceCfg, err := traceCfg.FromEnv()
	if err != nil {
		logger.Fatal("Failed to read tracer configuration", zap.Error(err))
	}
	tracer, closer, err := traceCfg.NewTracer(
		jaegerClientConfig.Logger(jaegerClientZapLog.NewLogger(logger)),
	)
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	opentracing.SetGlobalTracer(tracer)
	return closer
}
